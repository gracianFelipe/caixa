package competencia

import (
	"errors"
	"testing"
	"time"
)

// Fuso fixo de -03:00, sem depender de tzdata: o teste do dominio precisa
// rodar em qualquer maquina, inclusive Windows sem base IANA.
var saoPaulo = time.FixedZone("America/Sao_Paulo", -3*60*60)

func TestDo(t *testing.T) {
	casos := []struct {
		nome     string
		instante time.Time
		querida  Competencia
	}{
		{
			"meio do mes",
			time.Date(2026, time.September, 17, 15, 0, 0, 0, time.UTC),
			Competencia{2026, time.September},
		},
		{
			// 22:30 de 30/09 em Sao Paulo = 01:30 UTC de 01/10. Pertence a setembro.
			"virada do mes em UTC, ainda setembro em Sao Paulo",
			time.Date(2026, time.October, 1, 1, 30, 0, 0, time.UTC),
			Competencia{2026, time.September},
		},
		{
			"virada do ano",
			time.Date(2027, time.January, 1, 2, 59, 59, 0, time.UTC),
			Competencia{2026, time.December},
		},
		{
			"instante ja em Sao Paulo",
			time.Date(2026, time.October, 1, 0, 0, 0, 0, saoPaulo),
			Competencia{2026, time.October},
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if obtida := Do(c.instante, saoPaulo); obtida != c.querida {
				t.Errorf("Do(%v) = %v, queria %v", c.instante, obtida, c.querida)
			}
		})
	}
}

func TestAnalisar(t *testing.T) {
	casos := []struct {
		nome    string
		entrada string
		querida Competencia
		erro    error
	}{
		{"valida", "2026-09", Competencia{2026, time.September}, nil},
		{"dezembro", "2026-12", Competencia{2026, time.December}, nil},
		{"mes zero", "2026-00", Competencia{}, ErrFormato},
		{"mes treze", "2026-13", Competencia{}, ErrFormato},
		{"sem zero a esquerda", "2026-9", Competencia{}, ErrFormato},
		{"com dia", "2026-09-01", Competencia{}, ErrFormato},
		{"vazio", "", Competencia{}, ErrFormato},
		{"letras", "abcd-ef", Competencia{}, ErrFormato},
		{"separador errado", "2026/09", Competencia{}, ErrFormato},
		{"ano negativo", "-026-09", Competencia{}, ErrFormato},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			obtida, err := Analisar(c.entrada)
			if !errors.Is(err, c.erro) {
				t.Fatalf("Analisar(%q) erro = %v, queria %v", c.entrada, err, c.erro)
			}
			if obtida != c.querida {
				t.Errorf("Analisar(%q) = %v, queria %v", c.entrada, obtida, c.querida)
			}
		})
	}
}

func TestStringEPrimeiroDia(t *testing.T) {
	c := Competencia{2026, time.September}

	if s := c.String(); s != "2026-09" {
		t.Errorf("String() = %q, queria %q", s, "2026-09")
	}

	querido := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	if d := c.PrimeiroDia(); !d.Equal(querido) {
		t.Errorf("PrimeiroDia() = %v, queria %v", d, querido)
	}

	if !(Competencia{}).EhZero() || c.EhZero() {
		t.Error("EhZero nao distingue competencia vazia de definida")
	}
}

func TestIdaEVolta(t *testing.T) {
	original := Competencia{2026, time.September}
	obtida, err := Analisar(original.String())
	if err != nil || obtida != original {
		t.Errorf("ida e volta de %v virou %v (erro %v)", original, obtida, err)
	}
}
