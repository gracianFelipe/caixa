// Comando api sobe o servidor HTTP do Caixa. E o unico lugar que conhece
// todas as camadas: le a configuracao, abre o banco e monta as dependencias.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // Windows nao tem a base IANA; sem isto LoadLocation falha aqui e passa no Linux.

	"github.com/gracianFelipe/caixa/internal/adaptadores/entrada/web"
	"github.com/gracianFelipe/caixa/internal/adaptadores/saida/postgres"
	"github.com/gracianFelipe/caixa/internal/adaptadores/saida/relogio"
	"github.com/gracianFelipe/caixa/internal/adaptadores/saida/senha"
	"github.com/gracianFelipe/caixa/internal/aplicacao"
	webapp "github.com/gracianFelipe/caixa/web"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// main so decide o codigo de saida; a logica fica em executar, que devolve
	// error e por isso deixa os defers rodarem antes do os.Exit.
	if err := executar(context.Background(), log); err != nil {
		log.Error("encerrando com erro", "erro", err)
		os.Exit(1)
	}
}

type config struct {
	bdURL          string
	httpEndereco   string
	atalhoToken    string
	usuario        string
	senhaHash      string
	cookieInseguro bool
}

// lerConfig le tudo do ambiente. URL do banco, token e hash carregam segredo
// e nunca sao logados.
func lerConfig() (config, error) {
	c := config{
		bdURL:          os.Getenv("CAIXA_BD_URL"),
		httpEndereco:   os.Getenv("CAIXA_HTTP_ENDERECO"),
		atalhoToken:    os.Getenv("CAIXA_ATALHO_TOKEN"),
		usuario:        os.Getenv("CAIXA_USUARIO"),
		senhaHash:      os.Getenv("CAIXA_SENHA_HASH"),
		cookieInseguro: os.Getenv("CAIXA_HTTP_INSEGURO") == "1",
	}
	if c.bdURL == "" {
		return config{}, errors.New("CAIXA_BD_URL nao definida")
	}
	if c.httpEndereco == "" {
		// Loopback por padrao: expor em todas as interfaces e decisao
		// explicita de deploy (CAIXA_HTTP_ENDERECO=:8080 atras do Caddy).
		c.httpEndereco = "127.0.0.1:8080"
	}
	if c.atalhoToken != "" && len(c.atalhoToken) < 32 {
		return config{}, errors.New("CAIXA_ATALHO_TOKEN muito curto: use 32+ caracteres aleatorios")
	}
	if (c.usuario == "") != (c.senhaHash == "") {
		return config{}, errors.New("CAIXA_USUARIO e CAIXA_SENHA_HASH andam juntos")
	}
	return c, nil
}

func executar(ctx context.Context, log *slog.Logger) error {
	cfg, err := lerConfig()
	if err != nil {
		return err
	}

	fuso, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return fmt.Errorf("carregando fuso: %w", err)
	}

	ctxConexao, cancelar := context.WithTimeout(ctx, 10*time.Second)
	defer cancelar()
	pool, err := postgres.Conectar(ctxConexao, cfg.bdURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	lancamentos := aplicacao.NovoServicoDeLancamentos(
		postgres.NovoRepositorio(pool),
		postgres.NovoRepositorioDeRegras(pool),
		relogio.Sistema{},
		fuso,
	)
	catalogo := aplicacao.NovoCatalogo(postgres.NovoRepositorioDeCategorias(pool))
	relatorios := aplicacao.NovoServicoDeRelatorios(
		postgres.NovoRepositorio(pool),
		postgres.NovoRepositorioDeOrcamentos(pool),
		postgres.NovoRepositorioDeCategorias(pool),
	)
	orcamentos := aplicacao.NovoServicoDeOrcamentos(
		postgres.NovoRepositorioDeOrcamentos(pool),
		postgres.NovoRepositorio(pool),
		postgres.NovoRepositorioDeCategorias(pool),
	)
	acesso := aplicacao.NovoAcesso(
		postgres.NovoRepositorioDeSessoes(pool),
		senha.Argon2id{},
		relogio.Sistema{},
		cfg.usuario, cfg.senhaHash,
	)

	// Hub + LISTEN: o gatilho da migracao 006 avisa a cada evento novo; o hub
	// repassa aos WebSockets. A escuta morre com o contexto do servidor.
	hub := web.NovoHub(log)
	ctxEscuta, pararEscuta := context.WithCancel(ctx)
	defer pararEscuta()
	go func() {
		err := postgres.NovoOuvinteDeNotificacoes(pool).Escutar(ctxEscuta, "caixa_eventos", func(tipo string) {
			hub.Transmitir([]byte(`{"tipo":"` + tipo + `"}`))
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Error("escuta de notificacoes encerrada", "erro", err)
		}
	}()

	servidor := &http.Server{
		Addr: cfg.httpEndereco,
		Handler: web.NovoHandler(web.Servicos{
			Lancamentos:    lancamentos,
			Catalogo:       catalogo,
			Relatorios:     relatorios,
			Orcamentos:     orcamentos,
			Acesso:         acesso,
			Hub:            hub,
			App:            webapp.App(),
			AtalhoToken:    cfg.atalhoToken,
			CookieInseguro: cfg.cookieInseguro,
		}, log),
		ReadHeaderTimeout: 5 * time.Second, // fecha conexao que abre e nao manda cabecalho (slowloris)
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Ctrl+C ou SIGTERM do orquestrador cancelam o contexto; o servidor para de
	// aceitar conexoes e espera as em andamento terminarem, ate o limite.
	ctxSinal, pararSinal := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer pararSinal()

	erroServidor := make(chan error, 1)
	go func() {
		log.Info("api ouvindo", "endereco", cfg.httpEndereco)
		erroServidor <- servidor.ListenAndServe()
	}()

	select {
	case err := <-erroServidor:
		return fmt.Errorf("servidor http: %w", err)
	case <-ctxSinal.Done():
	}

	log.Info("sinal recebido, encerrando")
	ctxDesligar, cancelarDesligar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelarDesligar()
	if err := servidor.Shutdown(ctxDesligar); err != nil {
		return fmt.Errorf("desligando servidor: %w", err)
	}
	return nil
}
