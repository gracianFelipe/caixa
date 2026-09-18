// Comando caixactl reune as operacoes de linha de comando do Caixa:
// `migrar` aplica as migracoes embutidas; `importar` carrega extratos OFX.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"
	_ "time/tzdata" // caixactl calcula competencia: precisa do fuso no Windows

	"github.com/gracianFelipe/caixa/internal/adaptadores/entrada/extrato"
	"github.com/gracianFelipe/caixa/internal/adaptadores/saida/postgres"
	"github.com/gracianFelipe/caixa/internal/adaptadores/saida/relogio"
	"github.com/gracianFelipe/caixa/internal/aplicacao"
	"github.com/gracianFelipe/caixa/internal/dominio/ocorrencia"
	"github.com/gracianFelipe/caixa/migracoes"
)

const uso = "uso: caixactl <migrar | importar arquivo.ofx [outro.ofx ...]>"

func main() {
	// CLI: texto legivel em stderr; stdout fica livre para dados.
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if err := executar(context.Background(), os.Args[1:], log); err != nil {
		log.Error("caixactl", "erro", err)
		os.Exit(1)
	}
}

func executar(ctx context.Context, args []string, log *slog.Logger) error {
	if len(args) == 0 {
		return errors.New(uso)
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
	case "importar":
		flags := flag.NewFlagSet("importar", flag.ContinueOnError)
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() == 0 {
			return errors.New(uso)
		}
		return importar(ctx, flags.Args(), log)
	default:
		return fmt.Errorf("subcomando desconhecido: %q (%s)", args[0], uso)
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

func importar(ctx context.Context, caminhos []string, log *slog.Logger) error {
	url := os.Getenv("CAIXA_BD_URL")
	if url == "" {
		return errors.New("CAIXA_BD_URL nao definida")
	}
	fuso, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return fmt.Errorf("carregando fuso: %w", err)
	}

	ctx, cancelar := context.WithTimeout(ctx, 5*time.Minute)
	defer cancelar()

	pool, err := postgres.Conectar(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	servico := aplicacao.NovoServicoDeImportacao(
		postgres.NovoRepositorioDeOcorrencias(pool),
		relogio.Sistema{},
		fuso,
	)

	var total aplicacao.ResumoDaImportacao
	for _, caminho := range caminhos {
		dados, err := os.ReadFile(caminho)
		if err != nil {
			return fmt.Errorf("lendo %s: %w", caminho, err)
		}

		transacoes, err := extrato.Analisar(dados, fuso)
		if err != nil {
			return fmt.Errorf("%s: %w", caminho, err)
		}

		itens := make([]aplicacao.ItemDeExtrato, 0, len(transacoes))
		for _, t := range transacoes {
			itens = append(itens, aplicacao.ItemDeExtrato{
				OcorridoEm:  t.OcorridoEm,
				Valor:       t.Valor,
				Meio:        t.Meio,
				Contraparte: t.Contraparte,
				IDExterno:   t.IDExterno,
				Payload:     t.Payload,
			})
		}

		resumo, err := servico.Importar(ctx, ocorrencia.OrigemExtratoOFX, itens)
		// Contagens em stdout; linha de extrato (valor, contraparte) jamais.
		fmt.Printf("%s: %d criados, %d duplicados, %d ignorados\n",
			caminho, resumo.Criados, resumo.Duplicados, resumo.Ignorados)
		if err != nil {
			return fmt.Errorf("%s: %w", caminho, err)
		}
		total.Criados += resumo.Criados
		total.Duplicados += resumo.Duplicados
		total.Ignorados += resumo.Ignorados
	}

	if len(caminhos) > 1 {
		fmt.Printf("total: %d criados, %d duplicados, %d ignorados\n",
			total.Criados, total.Duplicados, total.Ignorados)
	}
	return nil
}
