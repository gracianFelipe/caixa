// Comando worker e o segundo binario do Caixa: consome o outbox e conversa
// com o dono pelo Telegram (long polling — nada e exposto na internet em dev).
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/gracianFelipe/caixa/internal/adaptadores/entrada/email"
	"github.com/gracianFelipe/caixa/internal/adaptadores/entrada/email/bradesco"
	"github.com/gracianFelipe/caixa/internal/adaptadores/saida/postgres"
	"github.com/gracianFelipe/caixa/internal/adaptadores/saida/relogio"
	"github.com/gracianFelipe/caixa/internal/adaptadores/saida/telegram"
	"github.com/gracianFelipe/caixa/internal/aplicacao"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/ocorrencia"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := executar(context.Background(), log); err != nil {
		log.Error("encerrando com erro", "erro", err)
		os.Exit(1)
	}
}

type config struct {
	bdURL  string
	token  string
	chatID int64

	// IMAP e opcional: sem as quatro envs o laco de e-mail fica desligado.
	imapServidor  string
	imapUsuario   string
	imapSenha     string
	imapRemetente string
}

func lerConfig() (config, error) {
	c := config{
		bdURL:         os.Getenv("CAIXA_BD_URL"),
		token:         os.Getenv("CAIXA_TELEGRAM_TOKEN"),
		imapServidor:  os.Getenv("CAIXA_IMAP_SERVIDOR"),
		imapUsuario:   os.Getenv("CAIXA_IMAP_USUARIO"),
		imapSenha:     os.Getenv("CAIXA_IMAP_SENHA"),
		imapRemetente: os.Getenv("CAIXA_IMAP_REMETENTE"),
	}
	if c.bdURL == "" {
		return config{}, errors.New("CAIXA_BD_URL nao definida")
	}
	if c.token == "" {
		return config{}, errors.New("CAIXA_TELEGRAM_TOKEN nao definida")
	}
	chat, err := strconv.ParseInt(os.Getenv("CAIXA_TELEGRAM_CHAT_ID"), 10, 64)
	if err != nil || chat == 0 {
		return config{}, errors.New("CAIXA_TELEGRAM_CHAT_ID invalida ou ausente")
	}
	c.chatID = chat

	imapConfigurado := c.imapServidor != "" || c.imapUsuario != "" || c.imapSenha != "" || c.imapRemetente != ""
	imapCompleto := c.imapServidor != "" && c.imapUsuario != "" && c.imapSenha != "" && c.imapRemetente != ""
	if imapConfigurado && !imapCompleto {
		return config{}, errors.New("CAIXA_IMAP_SERVIDOR, _USUARIO, _SENHA e _REMETENTE andam juntas")
	}
	return c, nil
}

func executar(ctx context.Context, log *slog.Logger) error {
	cfg, err := lerConfig()
	if err != nil {
		return err
	}

	ctxConexao, cancelar := context.WithTimeout(ctx, 10*time.Second)
	defer cancelar()
	pool, err := postgres.Conectar(ctxConexao, cfg.bdURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	cliente := telegram.NovoCliente(cfg.token, "")
	fila := aplicacao.NovaFila(aplicacao.DependenciasDaFila{
		Eventos:     postgres.NovoRepositorioDeEventos(pool),
		Lancamentos: postgres.NovoRepositorio(pool),
		Perguntas:   postgres.NovoRepositorioDePerguntas(pool),
		Categorias:  postgres.NovoRepositorioDeCategorias(pool),
		Regras:      postgres.NovoRepositorioDeRegras(pool),
		Orcamentos:  postgres.NovoRepositorioDeOrcamentos(pool),
		Alertas:     postgres.NovoRepositorioDeAlertas(pool),
		Mensageiro:  telegram.NovoMensageiro(cliente),
		Relogio:     relogio.Sistema{},
		ChatID:      cfg.chatID,
	})

	fuso, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return fmt.Errorf("carregando fuso: %w", err)
	}
	relatorios := aplicacao.NovoServicoDeRelatorios(
		postgres.NovoRepositorio(pool),
		postgres.NovoRepositorioDeOrcamentos(pool),
		postgres.NovoRepositorioDeCategorias(pool),
	)
	agendador := &agendador{
		relatorios: relatorios,
		alertas:    postgres.NovoRepositorioDeAlertas(pool),
		cliente:    cliente,
		fuso:       fuso,
		chatID:     cfg.chatID,
	}

	ctxSinal, pararSinal := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer pararSinal()

	log.Info("worker de pe", "chat", cfg.chatID)

	var espera sync.WaitGroup
	espera.Add(3)
	go func() {
		defer espera.Done()
		lacoDoOutbox(ctxSinal, fila, log)
	}()
	go func() {
		defer espera.Done()
		lacoDeAtualizacoes(ctxSinal, cliente, fila, relatorios, cfg.chatID, log)
	}()
	go func() {
		defer espera.Done()
		agendador.laco(ctxSinal, log)
	}()

	if cfg.imapServidor != "" {
		importacao := aplicacao.NovoServicoDeImportacao(
			postgres.NovoRepositorioDeOcorrencias(pool),
			postgres.NovoRepositorio(pool),
			postgres.NovoRepositorioDeRegras(pool),
			relogio.Sistema{},
			fuso,
		)
		leitor := &leitorDeEmail{
			caixa:      email.Caixa{Servidor: cfg.imapServidor, Usuario: cfg.imapUsuario, Senha: cfg.imapSenha},
			remetente:  strings.ToLower(cfg.imapRemetente),
			estado:     postgres.NovoEstadoIMAP(pool),
			importacao: importacao,
		}
		espera.Add(1)
		go func() {
			defer espera.Done()
			leitor.laco(ctxSinal, log)
		}()
		log.Info("laco de e-mail ligado", "remetente", cfg.imapRemetente)
	} else {
		log.Info("laco de e-mail desligado: CAIXA_IMAP_* ausentes")
	}

	espera.Wait()

	log.Info("worker encerrado")
	return nil
}

// agendador manda o relatorio do mes fechado no dia 1 as 08:00 (Sao Paulo).
// Sem cron: um tique por minuto pergunta "ja passou da hora?" e a tabela de
// alertas e a "ultima execucao persistida" — restart nao duplica nem pula.
type agendador struct {
	relatorios *aplicacao.Relatorios
	alertas    *postgres.Alertas
	cliente    *telegram.Cliente
	fuso       *time.Location
	chatID     int64
}

func (a *agendador) laco(ctx context.Context, log *slog.Logger) {
	tique := time.NewTicker(time.Minute)
	defer tique.Stop()

	for {
		if err := a.tentar(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("relatorio agendado", "erro", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tique.C:
		}
	}
}

func (a *agendador) tentar(ctx context.Context) error {
	alvo, deve := aplicacao.CompetenciaAgendada(time.Now().In(a.fuso))
	if !deve {
		return nil
	}
	novo, err := a.alertas.RegistrarSeNovo(ctx, "relatorio", alvo.String())
	if err != nil || !novo {
		return err
	}
	pronto, err := a.relatorios.Gerar(ctx, alvo)
	if err != nil {
		// Compensa: sem isto o relatorio do mes se perderia para sempre por
		// uma falha momentanea de banco ou Telegram.
		_ = a.alertas.Remover(ctx, "relatorio", alvo.String())
		return err
	}
	if err := a.cliente.EnviarTexto(ctx, a.chatID, pronto.Texto); err != nil {
		_ = a.alertas.Remover(ctx, "relatorio", alvo.String())
		return err
	}
	return nil
}

// leitorDeEmail transforma alerta do banco em lancamento: a cada 2 minutos
// busca as UIDs novas, filtra o remetente, parseia e importa pela MESMA
// Importacao do OFX — idempotencia por Message-Id e conciliacao de graca.
type leitorDeEmail struct {
	caixa      email.Caixa
	remetente  string
	estado     *postgres.EstadoIMAP
	importacao *aplicacao.Importacao
}

func (l *leitorDeEmail) laco(ctx context.Context, log *slog.Logger) {
	tique := time.NewTicker(2 * time.Minute)
	defer tique.Stop()

	for {
		if err := l.rodada(ctx, log); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("lendo e-mail", "erro", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tique.C:
		}
	}
}

func (l *leitorDeEmail) rodada(ctx context.Context, log *slog.Logger) error {
	uidvalidity, ultimaUID, _, err := l.estado.Carregar(ctx)
	if err != nil {
		return err
	}

	mensagens, novaValidity, err := l.caixa.Buscar(ctx, uidvalidity, ultimaUID)
	if err != nil {
		return err
	}
	if novaValidity != uidvalidity {
		ultimaUID = 0 // servidor renumerou; Message-Id segura duplicatas
	}

	maiorUID := ultimaUID
	var itens []aplicacao.ItemDeExtrato
	for _, m := range mensagens {
		if m.UID > maiorUID {
			maiorUID = m.UID
		}
		if !remetenteBate(m.Bruto, l.remetente) {
			continue
		}
		t, err := bradesco.Analisar(m.Bruto)
		if err != nil {
			// UID e motivo, nunca conteudo: e-mail e PII.
			log.Warn("alerta nao parseado", "uid", m.UID, "erro", err)
			continue
		}
		itens = append(itens, aplicacao.ItemDeExtrato{
			OcorridoEm:  t.OcorridoEm,
			Valor:       t.Valor,
			Meio:        t.Meio,
			Contraparte: t.Contraparte,
			IDExterno:   t.IDExterno,
			Payload:     t.Payload,
		})
	}

	if len(itens) > 0 {
		resumo, err := l.importacao.Importar(ctx, ocorrencia.OrigemEmailBanco, itens)
		if err != nil {
			return err // UID nao avanca: proxima rodada tenta de novo
		}
		log.Info("e-mails importados", "criados", resumo.Criados,
			"conciliados", resumo.Conciliados, "duplicados", resumo.Duplicados)
	}

	if maiorUID != ultimaUID || novaValidity != uidvalidity {
		return l.estado.Salvar(ctx, novaValidity, maiorUID)
	}
	return nil
}

// remetenteBate olha so o cabecalho From, sem parsear o corpo.
func remetenteBate(bruto []byte, sufixo string) bool {
	msg, err := mail.ReadMessage(bytes.NewReader(bruto))
	if err != nil {
		return false
	}
	endereco, err := msg.Header.AddressList("From")
	if err != nil || len(endereco) == 0 {
		return false
	}
	return strings.HasSuffix(strings.ToLower(endereco[0].Address), sufixo)
}

// lacoDoOutbox drena a fila a cada 2s. Erro nao derruba o worker: loga e
// tenta de novo no proximo tique — os eventos ficaram pendentes no rollback.
func lacoDoOutbox(ctx context.Context, fila *aplicacao.Fila, log *slog.Logger) {
	tique := time.NewTicker(2 * time.Second)
	defer tique.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tique.C:
			n, err := fila.ProcessarLote(ctx, 20)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Error("processando outbox", "erro", err)
				continue
			}
			if n > 0 {
				log.Info("eventos processados", "quantidade", n)
			}
		}
	}
}

// lacoDeAtualizacoes faz long polling. So o chat do dono e atendido; o resto
// e contado e descartado sem logar conteudo.
func lacoDeAtualizacoes(ctx context.Context, cliente *telegram.Cliente, fila *aplicacao.Fila, relatorios *aplicacao.Relatorios, chatID int64, log *slog.Logger) {
	var offset int64
	for {
		if ctx.Err() != nil {
			return
		}

		atualizacoes, err := cliente.BuscarAtualizacoes(ctx, offset, 50)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Error("buscando atualizacoes", "erro", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		for _, a := range atualizacoes {
			offset = a.ID + 1

			if a.ChatID != chatID {
				log.Warn("atualizacao de chat desconhecido descartada")
				continue
			}
			if a.Callback == "" {
				if strings.HasPrefix(a.Texto, "/relatorio") {
					if err := responderRelatorio(ctx, cliente, relatorios, a); err != nil {
						log.Error("respondendo /relatorio", "erro", err)
					}
				}
				continue
			}
			if err := responderCallback(ctx, cliente, fila, a); err != nil {
				log.Error("respondendo callback", "erro", err)
			}
		}
	}
}

// responderRelatorio atende "/relatorio" (mes atual) ou "/relatorio AAAA-MM".
func responderRelatorio(ctx context.Context, cliente *telegram.Cliente, relatorios *aplicacao.Relatorios, a telegram.Atualizacao) error {
	fuso, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return err
	}
	agora := time.Now().In(fuso)
	alvo, _ := competencia.Nova(agora.Year(), agora.Month())

	if partes := strings.Fields(a.Texto); len(partes) > 1 {
		if alvo, err = competencia.Analisar(partes[1]); err != nil {
			return cliente.EnviarTexto(ctx, a.ChatID, "uso: /relatorio [AAAA-MM]")
		}
	}

	pronto, err := relatorios.Gerar(ctx, alvo)
	if err != nil {
		_ = cliente.EnviarTexto(ctx, a.ChatID, "nao consegui gerar o relatorio agora")
		return err
	}
	return cliente.EnviarTexto(ctx, a.ChatID, pronto.Texto)
}

func responderCallback(ctx context.Context, cliente *telegram.Cliente, fila *aplicacao.Fila, a telegram.Atualizacao) error {
	cb, err := telegram.AnalisarCallback(a.Callback)
	if err != nil {
		return cliente.ConfirmarCallback(ctx, a.CallbackID, "botao invalido")
	}

	if cb.EhConciliacao {
		if _, err := fila.ResponderConciliacao(ctx, cb.PerguntaID, cb.Conciliar); err != nil {
			_ = cliente.ConfirmarCallback(ctx, a.CallbackID, "nao consegui aplicar")
			return err
		}
		resposta := "gasto novo confirmado"
		if cb.Conciliar {
			resposta = "fundido: uma linha, duas evidencias"
		}
		if err := cliente.ConfirmarCallback(ctx, a.CallbackID, resposta); err != nil {
			return err
		}
		return cliente.EditarMensagem(ctx, a.ChatID, a.MensagemID, "conciliacao: "+resposta+" ✔")
	}

	l, escolhida, err := fila.ResponderCategoria(ctx, cb.PerguntaID, cb.Categoria)
	if err != nil {
		_ = cliente.ConfirmarCallback(ctx, a.CallbackID, "nao consegui aplicar")
		return err
	}

	if err := cliente.ConfirmarCallback(ctx, a.CallbackID, "anotado: "+escolhida.Nome); err != nil {
		return err
	}
	texto := fmt.Sprintf("%s — %s\ncategoria: %s ✔", l.Valor, l.Contraparte, escolhida.Nome)
	return cliente.EditarMensagem(ctx, a.ChatID, a.MensagemID, texto)
}
