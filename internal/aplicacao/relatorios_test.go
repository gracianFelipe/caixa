package aplicacao

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

func TestRelatoriosGerar(t *testing.T) {
	repo := &lancamentosDaFila{}
	setembro, _ := competencia.Nova(2026, time.September)

	// Um gasto de mercado em setembro, um em agosto, uma entrada em setembro.
	for i, d := range []struct {
		mes   time.Month
		valor int64
		cat   categoria.ID
	}{{time.September, -90000, 1}, {time.August, -70000, 1}, {time.September, 500000, 12}} {
		id, _ := identidadeFixa(byte(50 + i))
		l, _ := lancamento.Novo(id, lancamento.Dados{
			OcorridoEm: time.Date(2026, d.mes, 10, 12, 0, 0, 0, time.UTC),
			Valor:      dinheiro.Centavos(d.valor), Meio: lancamento.MeioPix, Contraparte: "X",
		}, saoPaulo)
		l, _ = l.ComCategoria(d.cat, lancamento.CategoriaPorRegra)
		repo.salvos = append(repo.salvos, l)
	}

	mercado, _ := categoria.Nova(1, "mercado")
	renda, _ := categoria.Nova(12, "renda")
	s := NovoServicoDeRelatorios(repo, orcamentosFixos{1: 80000}, categoriasFixas{mercado, renda})

	pronto, err := s.Gerar(context.Background(), setembro)
	if err != nil {
		t.Fatalf("Gerar: %v", err)
	}

	r := pronto.Relatorio
	if r.TotalSaidas != 90000 || r.TotalEntradas != 500000 || r.SaidasMesAnterior != 70000 {
		t.Errorf("totais = saidas %d, entradas %d, anterior %d", r.TotalSaidas, r.TotalEntradas, r.SaidasMesAnterior)
	}
	if len(r.Sinais) != 1 || r.Sinais[0].Categoria != 1 {
		t.Errorf("sinais = %+v, queria so o estouro de mercado (900 > 800)", r.Sinais)
	}
	if !strings.Contains(pronto.Texto, "mercado em 112% do limite") {
		t.Errorf("texto = %q", pronto.Texto)
	}
	if pronto.Nomes[12] != "renda" {
		t.Error("nomes das categorias nao chegaram")
	}
}

func TestCompetenciaAgendada(t *testing.T) {
	sp := time.FixedZone("America/Sao_Paulo", -3*60*60)
	agosto, _ := competencia.Nova(2026, time.August)
	setembro, _ := competencia.Nova(2026, time.September)
	janeiro, _ := competencia.Nova(2026, time.January)

	casos := []struct {
		nome    string
		agora   time.Time
		querida competencia.Competencia
		manda   bool
	}{
		{"dia 1 as 07:59 nao manda", time.Date(2026, 9, 1, 7, 59, 0, 0, sp), competencia.Competencia{}, false},
		{"dia 1 as 08:00 manda agosto", time.Date(2026, 9, 1, 8, 0, 0, 0, sp), agosto, true},
		{"dia 1 as 23:00 manda agosto", time.Date(2026, 9, 1, 23, 0, 0, 0, sp), agosto, true},
		{"dia 15 manda agosto (atrasado e seguro pela chave)", time.Date(2026, 9, 15, 12, 0, 0, 0, sp), agosto, true},
		{"dia 1 de outubro manda setembro", time.Date(2026, 10, 1, 9, 0, 0, 0, sp), setembro, true},
		{"1 de fevereiro manda janeiro (sem dia 30)", time.Date(2026, 2, 1, 9, 0, 0, 0, sp), janeiro, true},
		{"31 de marco manda fevereiro, nao pula", time.Date(2026, 3, 31, 9, 0, 0, 0, sp), mustComp(2026, time.February), true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			obtida, manda := CompetenciaAgendada(c.agora)
			if manda != c.manda || obtida != c.querida {
				t.Errorf("CompetenciaAgendada(%v) = (%v, %v), queria (%v, %v)", c.agora, obtida, manda, c.querida, c.manda)
			}
		})
	}
}

func mustComp(ano int, mes time.Month) competencia.Competencia {
	c, _ := competencia.Nova(ano, mes)
	return c
}
