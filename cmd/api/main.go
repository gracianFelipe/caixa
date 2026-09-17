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
	"github.com/gracianFelipe/caixa/internal/aplicacao"
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
	bdURL        string
	httpEndereco string
}

// lerConfig le tudo do ambiente. A URL do banco carrega a senha e nunca e logada.
func lerConfig() (config, error) {
	c := config{
		bdURL:        os.Getenv("CAIXA_BD_URL"),
		httpEndereco: os.Getenv("CAIXA_HTTP_ENDERECO"),
	}
	if c.bdURL == "" {
		return config{}, errors.New("CAIXA_BD_URL nao definida")
	}
	if c.httpEndereco == "" {
		c.httpEndereco = ":8080"
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
		relogio.Sistema{},
		fuso,
	)

	servidor := &http.Server{
		Addr:              cfg.httpEndereco,
		Handler:           web.NovoHandler(lancamentos, log),
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
