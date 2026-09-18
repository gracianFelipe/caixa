//go:build integracao

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/ocorrencia"
)

// TestCandidatosParaConciliacao prova os tres filtros do SQL: valor, janela
// de dias e "sem evidencia da origem que chega".
func TestCandidatosParaConciliacao(t *testing.T) {
	repo := repositorioDeTeste(t)
	ocorrencias := NovoRepositorioDeOcorrencias(repo.pool)
	ctx := context.Background()
	marcador := time.Now().Format("150405.000000")

	// Fato existente com evidencia MANUAL (origem 1), em 1996 para isolamento.
	quando := time.Date(1996, time.June, 10, 12, 0, 0, 0, time.UTC)
	o, l := evidencia(t, ocorrencias, "manual "+marcador, "CAND-"+marcador)
	l.OcorridoEm = quando
	// evidencia() fixa 1998; reconstroi com a data desejada:
	l2 := l
	l2.OcorridoEm = quando
	if _, err := ocorrencias.CriarComLancamento(ctx, o, l2, eventoDeImportacao(t, ocorrencias, l2)); err != nil {
		t.Fatal(err)
	}
	// Corrige a origem da evidencia para manual (o helper usa extrato).
	if _, err := repo.pool.Exec(ctx, "UPDATE ocorrencias SET origem_id = 1 WHERE id = $1", o.ID); err != nil {
		t.Fatal(err)
	}

	t.Run("acha por outra origem", func(t *testing.T) {
		candidatos, err := repo.CandidatosParaConciliacao(ctx, l2.Valor, quando.AddDate(0, 0, 1), ocorrencia.OrigemExtratoOFX)
		if err != nil {
			t.Fatal(err)
		}
		achou := false
		for _, c := range candidatos {
			if c.Lancamento.ID == l2.ID {
				achou = true
				if len(c.Origens) != 1 || c.Origens[0] != ocorrencia.OrigemManual {
					t.Errorf("origens = %v, queria [manual]", c.Origens)
				}
			}
		}
		if !achou {
			t.Error("candidato legitimo nao foi devolvido")
		}
	})

	t.Run("filtra mesma origem", func(t *testing.T) {
		candidatos, err := repo.CandidatosParaConciliacao(ctx, l2.Valor, quando, ocorrencia.OrigemManual)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range candidatos {
			if c.Lancamento.ID == l2.ID {
				t.Error("lancamento com evidencia manual nao pode ser candidato para origem manual")
			}
		}
	})

	t.Run("filtra valor diferente", func(t *testing.T) {
		candidatos, err := repo.CandidatosParaConciliacao(ctx, l2.Valor-1, quando, ocorrencia.OrigemExtratoOFX)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range candidatos {
			if c.Lancamento.ID == l2.ID {
				t.Error("valor diferente nao pode ser candidato")
			}
		}
	})

	t.Run("filtra janela de dias", func(t *testing.T) {
		candidatos, err := repo.CandidatosParaConciliacao(ctx, l2.Valor, quando.AddDate(0, 0, 10), ocorrencia.OrigemExtratoOFX)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range candidatos {
			if c.Lancamento.ID == l2.ID {
				t.Error("dez dias de distancia nao pode ser candidato")
			}
		}
	})

	t.Run("anexar evidencia concilia e e idempotente", func(t *testing.T) {
		nova, _ := evidencia(t, ocorrencias, "extrato "+marcador, "CAND-OFX-"+marcador)
		anexada, err := ocorrencias.AnexarEvidencia(ctx, nova, l2.ID)
		if err != nil || !anexada {
			t.Fatalf("AnexarEvidencia = (%v, %v)", anexada, err)
		}

		var resultado string
		if err := repo.pool.QueryRow(ctx, "SELECT resultado FROM ocorrencias WHERE id = $1", nova.ID).Scan(&resultado); err != nil {
			t.Fatal(err)
		}
		if resultado != "conciliou" {
			t.Errorf("resultado = %s", resultado)
		}

		denovo, _ := evidencia(t, ocorrencias, "extrato "+marcador, "CAND-OFX2-"+marcador)
		anexada, err = ocorrencias.AnexarEvidencia(ctx, denovo, l2.ID)
		if err != nil || anexada {
			t.Errorf("mesma impressao deveria ser duplicata: (%v, %v)", anexada, err)
		}
	})
}
