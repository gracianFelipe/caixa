// Comando caixactl reune as operacoes de linha de comando do Caixa:
// hoje so `migrar`; `importar` chega na spec 003.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/gracianFelipe/caixa/internal/adaptadores/saida/postgres"
	"github.com/gracianFelipe/caixa/migracoes"
)

func main() {
	// CLI: texto legivel em stderr, nao JSON. stdout fica livre para dados.
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if err := executar(context.Background(), os.Args[1:], log); err != nil {
		log.Error("caixactl", "erro", err)
		os.Exit(1)
	}
}

func executar(ctx context.Context, args []string, log *slog.Logger) error {
	if len(args) == 0 {
		return errors.New("uso: caixactl <migrar>")
	}

	// Um FlagSet por subcomando: cada um tem as proprias flags, e os.Args[1]
	// decide qual. E o que `go` e `git` fazem, sem biblioteca.
	switch args[0] {
	case "migrar":
		flags := flag.NewFlagSet("migrar", flag.ContinueOnError)
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		return migrar(ctx, log)
	default:
		return fmt.Errorf("subcomando desconhecido: %q (uso: caixactl <migrar>)", args[0])
	}
}

func migrar(ctx context.Context, log *slog.Logger) error {
	url := os.Getenv("CAIXA_BD_URL")
	if url == "" {
		return errors.New("CAIXA_BD_URL nao definida")
	}

	ctx, cancelar := context.WithTimeout(ctx, 60*time.Second)
	defer cancelar()

	pool, err := postgres.Conectar(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	aplicadas, err := postgres.Aplicar(ctx, pool, migracoes.Arquivos)
	// Loga antes de olhar o erro: o que foi commitado antes da falha e fato.
	for _, a := range aplicadas {
		log.Info("migracao aplicada", "versao", a.Versao, "nome", a.Nome)
	}
	if err != nil {
		return err
	}
	if len(aplicadas) == 0 {
		log.Info("nada a aplicar: banco ja esta na ultima versao")
	}
	return nil
}
