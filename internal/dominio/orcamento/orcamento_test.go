package orcamento

import (
	"errors"
	"testing"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
)

func TestNovo(t *testing.T) {
	setembro, _ := competencia.Nova(2026, time.September)

	if _, err := Novo(1, setembro, 80000); err != nil {
		t.Fatalf("Novo valido devolveu %v", err)
	}
	if _, err := Novo(1, competencia.Competencia{}, 80000); err != nil {
		t.Fatalf("competencia zero e o limite padrao, devolveu %v", err)
	}
	if _, err := Novo(0, setembro, 80000); !errors.Is(err, ErrCategoriaVazia) {
		t.Errorf("categoria zero: %v", err)
	}
	if _, err := Novo(1, setembro, 0); !errors.Is(err, ErrLimiteInvalido) {
		t.Errorf("limite zero: %v", err)
	}
	if _, err := Novo(1, setembro, -100); !errors.Is(err, ErrLimiteInvalido) {
		t.Errorf("limite negativo: %v", err)
	}
}

func TestNivel(t *testing.T) {
	casos := []struct {
		nome   string
		gasto  int64
		limite int64
		nivel  int
	}{
		{"zero", 0, 80000, 0},
		{"abaixo de 80", 63999, 80000, 0},
		{"exatamente 79.99%", 63992, 80000, 0},
		{"exatamente 80%", 64000, 80000, 80},
		{"entre 80 e 100", 79999, 80000, 80},
		{"exatamente 100%", 80000, 80000, 100},
		{"estourado", 123456, 80000, 100},
		{"limite ausente", 50000, 0, 0},
		{"gasto negativo por engano", -100, 80000, 0},
		{"um centavo de limite", 1, 1, 100},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if n := Nivel(dinheiro.Centavos(c.gasto), dinheiro.Centavos(c.limite)); n != c.nivel {
				t.Errorf("Nivel(%d, %d) = %d, queria %d", c.gasto, c.limite, n, c.nivel)
			}
		})
	}
}
