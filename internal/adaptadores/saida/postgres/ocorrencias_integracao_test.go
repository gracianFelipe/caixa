//go:build integracao

package postgres

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
	"github.com/gracianFelipe/caixa/internal/dominio/ocorrencia"
)

func ocorrenciasDeTeste(t *testing.T) *Ocorrencias {
	t.Helper()
	repo := repositorioDeTeste(t) // reusa conexao e skip da suite de lancamentos
	return NovoRepositorioDeOcorrencias(repo.pool)
}

func evidencia(t *testing.T, repo *Ocorrencias, payload, idExterno string) (ocorrencia.Ocorrencia, lancamento.Lancamento) {
	t.Helper()
	idO, err := identidade.NovaV7(time.Now(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	o, err := ocorrencia.Nova(idO, ocorrencia.OrigemExtratoOFX, idExterno, payload)
	if err != nil {
		t.Fatal(err)
	}

	idL, err := identidade.NovaV7(time.Now(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	l, err := lancamento.Novo(idL, lancamento.Dados{
		OcorridoEm:  time.Date(1998, time.July, 10, 15, 0, 0, 0, time.UTC),
		Valor:       -4790,
		Meio:        lancamento.MeioPix,
		Contraparte: "Integração Ocorrência",
	}, saoPaulo)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = repo.pool.Exec(ctx, "DELETE FROM ocorrencias WHERE id = $1", o.ID)
		_, _ = repo.pool.Exec(ctx, "DELETE FROM lancamentos WHERE id = $1", l.ID)
	})
	return o, l
}

func TestCriarComLancamento(t *testing.T) {
	repo := ocorrenciasDeTeste(t)
	ctx := context.Background()

	o, l := evidencia(t, repo, "payload unico "+time.Now().String(), "FIT-INT-1-"+time.Now().String())

	criada, err := repo.CriarComLancamento(ctx, o, l)
	if err != nil {
		t.Fatalf("CriarComLancamento: %v", err)
	}
	if !criada {
		t.Fatal("primeira insercao deveria criar")
	}

	var resultado string
	var lancamentoID identidade.ID
	if err := repo.pool.QueryRow(ctx,
		"SELECT resultado, lancamento_id FROM ocorrencias WHERE id = $1", o.ID,
	).Scan(&resultado, &lancamentoID); err != nil {
		t.Fatal(err)
	}
	if resultado != "criou" || lancamentoID != l.ID {
		t.Errorf("ocorrencia ficou (%s, %s), queria (criou, %s)", resultado, lancamentoID, l.ID)
	}
}

func TestCriarComLancamentoDuplicada(t *testing.T) {
	repo := ocorrenciasDeTeste(t)
	ctx := context.Background()
	agora := time.Now().String()

	t.Run("mesma impressao", func(t *testing.T) {
		o1, l1 := evidencia(t, repo, "payload repetido "+agora, "FIT-DUP-A-"+agora)
		if _, err := repo.CriarComLancamento(ctx, o1, l1); err != nil {
			t.Fatal(err)
		}

		// Mesmo payload => mesma impressao, id externo diferente.
		o2, l2 := evidencia(t, repo, "payload repetido "+agora, "FIT-DUP-B-"+agora)
		criada, err := repo.CriarComLancamento(ctx, o2, l2)
		if err != nil {
			t.Fatalf("duplicata de impressao nao pode ser erro: %v", err)
		}
		if criada {
			t.Fatal("mesma impressao deveria ser duplicata")
		}

		// O lancamento da duplicata nao pode ter entrado (transacao atomica).
		var existe bool
		if err := repo.pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM lancamentos WHERE id = $1)", l2.ID).Scan(&existe); err != nil {
			t.Fatal(err)
		}
		if existe {
			t.Error("lancamento da ocorrencia duplicada foi gravado")
		}
	})

	t.Run("mesmo id externo com payload diferente", func(t *testing.T) {
		o1, l1 := evidencia(t, repo, "payload C "+agora, "FIT-DUP-C-"+agora)
		if _, err := repo.CriarComLancamento(ctx, o1, l1); err != nil {
			t.Fatal(err)
		}

		o2, l2 := evidencia(t, repo, "payload D "+agora, "FIT-DUP-C-"+agora)
		criada, err := repo.CriarComLancamento(ctx, o2, l2)
		if err != nil {
			t.Fatalf("duplicata de id externo nao pode ser erro: %v", err)
		}
		if criada {
			t.Error("mesmo FITID deveria ser duplicata mesmo com payload diferente")
		}
	})
}
