// Comando worker e o segundo binario do Caixa: consome o outbox e conversa
// com o dono pelo Telegram (long polling — nada e exposto na internet em dev).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/gracianFelipe/caixa/internal/adaptadores/saida/postgres"
	"github.com/gracianFelipe/caixa/internal/adaptadores/saida/relogio"
	"github.com/gracianFelipe/caixa/internal/adaptadores/saida/telegram"
	"github.com/gracianFelipe/caixa/internal/aplicacao"
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
}

func lerConfig() (config, error) {
	c := config{
		bdURL: os.Getenv("CAIXA_BD_URL"),
		token: os.Getenv("CAIXA_TELEGRAM_TOKEN"),
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

	ctxSinal, pararSinal := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer pararSinal()

	log.Info("worker de pe", "chat", cfg.chatID)

	var espera sync.WaitGroup
	espera.Add(2)
	go func() {
		defer espera.Done()
		lacoDoOutbox(ctxSinal, fila, log)
	}()
	go func() {
		defer espera.Done()
		lacoDeAtualizacoes(ctxSinal, cliente, fila, cfg.chatID, log)
	}()
	espera.Wait()

	log.Info("worker encerrado")
	return nil
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
func lacoDeAtualizacoes(ctx context.Context, cliente *telegram.Cliente, fila *aplicacao.Fila, chatID int64, log *slog.Logger) {
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
				continue // texto livre nao tem uso ainda; /relatorio chega na Fase 5
			}
			if err := responderCallback(ctx, cliente, fila, a); err != nil {
				log.Error("respondendo callback", "erro", err)
			}
		}
	}
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
