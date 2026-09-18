package conciliacao

import (
	"testing"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
	"github.com/gracianFelipe/caixa/internal/dominio/ocorrencia"
)

var base = time.Date(2026, time.September, 10, 15, 0, 0, 0, time.UTC)

func evidencia(ajustes ...func(*Evidencia)) Evidencia {
	e := Evidencia{
		Valor:           -4790,
		OcorridoEm:      base,
		ContraparteNorm: "SUPERMERCADO XYZ",
		Meio:            lancamento.MeioPix,
		Origem:          ocorrencia.OrigemExtratoOFX,
	}
	for _, f := range ajustes {
		f(&e)
	}
	return e
}

func candidato(ajustes ...func(*Candidato)) Candidato {
	c := Candidato{
		Lancamento: lancamento.Lancamento{
			ID:              identidade.ID{1},
			OcorridoEm:      base,
			Valor:           -4790,
			Meio:            lancamento.MeioPix,
			ContraparteNorm: "SUPERMERCADO XYZ",
		},
		Origens: []ocorrencia.Origem{ocorrencia.OrigemEmailBanco},
	}
	for _, f := range ajustes {
		f(&c)
	}
	return c
}

func TestPontuar(t *testing.T) {
	casos := []struct {
		nome      string
		evidencia Evidencia
		candidato Candidato
		pontos    int
	}{
		{
			// 50 + 25 (0d) + 25 (identica) + 10 (meio) + 5 (origem nova) = 115
			"caso perfeito entre origens diferentes",
			evidencia(), candidato(), 115,
		},
		{
			"valor diferente descarta",
			evidencia(func(e *Evidencia) { e.Valor = -4791 }), candidato(), 0,
		},
		{
			"quatro dias descarta",
			evidencia(func(e *Evidencia) { e.OcorridoEm = base.AddDate(0, 0, 4) }), candidato(), 0,
		},
		{
			// 50 + 18 + 25 + 10 + 5 = 108
			"um dia de distancia",
			evidencia(func(e *Evidencia) { e.OcorridoEm = base.AddDate(0, 0, 1) }), candidato(), 108,
		},
		{
			// 23:50 UTC vs 00:10 UTC do dia seguinte: 1 dia de calendario.
			"virada de dia em minutos conta um dia",
			evidencia(func(e *Evidencia) {
				e.OcorridoEm = time.Date(2026, 9, 11, 0, 10, 0, 0, time.UTC)
			}),
			candidato(func(c *Candidato) {
				c.Lancamento.OcorridoEm = time.Date(2026, 9, 10, 23, 50, 0, 0, time.UTC)
			}),
			108,
		},
		{
			// 50 + 12 + 25 + 10 + 5 = 102
			"dois dias",
			evidencia(func(e *Evidencia) { e.OcorridoEm = base.AddDate(0, 0, 2) }), candidato(), 102,
		},
		{
			// 50 + 6 + 25 + 10 + 5 = 96
			"tres dias",
			evidencia(func(e *Evidencia) { e.OcorridoEm = base.AddDate(0, 0, 3) }), candidato(), 96,
		},
		{
			// 50 + 25 + 25 + 10 - 20 = 90: mesma origem penaliza mas nao mata.
			"mesma origem",
			evidencia(), candidato(func(c *Candidato) {
				c.Origens = []ocorrencia.Origem{ocorrencia.OrigemExtratoOFX}
			}), 90,
		},
		{
			// 50 + 25 + 10 + 5 = 90: contraparte de outro formato, sem os +25.
			"contrapartes diferentes",
			evidencia(func(e *Evidencia) { e.ContraparteNorm = "PAGAMENTO CARTAO FINAL" }),
			candidato(), 90,
		},
		{
			// 50 + 25 + 25 + 5 = 105: meio diferente perde so os 10.
			"meio diferente",
			evidencia(func(e *Evidencia) { e.Meio = lancamento.MeioDebito }), candidato(), 105,
		},
		{
			// 50 + 6 - 20 = 36: o pior caso que ainda pontua.
			"tres dias, contraparte outra, meio outro, mesma origem",
			evidencia(func(e *Evidencia) {
				e.OcorridoEm = base.AddDate(0, 0, 3)
				e.ContraparteNorm = "OUTRA COISA QUALQUER"
				e.Meio = lancamento.MeioBoleto
			}),
			candidato(func(c *Candidato) {
				c.Origens = []ocorrencia.Origem{ocorrencia.OrigemExtratoOFX}
			}),
			36,
		},
		{
			// Sufixo de gateway derruba o Jaccard para ~0.74 (< 0.90): perde
			// os +25 — e mesmo assim 50+25+10+5 = 90 concilia. A tabela foi
			// desenhada para nao depender do bonus de similaridade quando o
			// resto casa. (Overlap coefficient daria 1.0 aqui; Jaccard foi a
			// escolha do plano e fica, com este teste documentando o efeito.)
			"contraparte com lixo de gateway concilia sem o bonus",
			evidencia(func(e *Evidencia) { e.ContraparteNorm = "SUPERMERCADO XYZ LTDA" }),
			candidato(), 90,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if p := Pontuar(c.evidencia, c.candidato); p != c.pontos {
				t.Errorf("Pontuar = %d, queria %d", p, c.pontos)
			}
		})
	}
}

func TestDecidir(t *testing.T) {
	casos := []struct {
		pontos  int
		decisao Decisao
	}{
		{115, Conciliar},
		{85, Conciliar},
		{84, Perguntar},
		{60, Perguntar},
		{59, LancamentoNovo},
		{0, LancamentoNovo},
	}
	for _, c := range casos {
		if d := Decidir(c.pontos); d != c.decisao {
			t.Errorf("Decidir(%d) = %v, queria %v", c.pontos, d, c.decisao)
		}
	}
}

// TestDoisGastosLegitimosIguaisNoMesmoDia e O caso do plano: o segundo cafe
// identico nao pode ser engolido. A defesa e o filtro de candidatos — o
// repositorio nao devolve lancamento que ja tem evidencia da mesma origem —
// representado aqui pela lista vazia de candidatos.
func TestDoisGastosLegitimosIguaisNoMesmoDia(t *testing.T) {
	segundoCafe := evidencia()

	// O primeiro cafe ja foi criado por ESTE extrato, entao ja tem evidencia
	// extrato_ofx e o repositorio o exclui: nenhum candidato chega aqui.
	_, pontos := Melhor(segundoCafe, nil)
	if Decidir(pontos) != LancamentoNovo {
		t.Fatal("segundo gasto identico deveria virar lancamento novo")
	}

	// Contraste: se o filtro NAO existisse, o score conciliaria errado (90).
	candidatoIndevido := candidato(func(c *Candidato) {
		c.Origens = []ocorrencia.Origem{ocorrencia.OrigemExtratoOFX}
	})
	if p := Pontuar(segundoCafe, candidatoIndevido); Decidir(p) != Conciliar {
		t.Fatalf("sem o filtro o score daria %d e conciliaria: o teste documenta por que o filtro existe", p)
	}
}

func TestMelhor(t *testing.T) {
	longe := candidato(func(c *Candidato) {
		c.Lancamento.ID = identidade.ID{9}
		c.Lancamento.OcorridoEm = base.AddDate(0, 0, 3)
	})
	perto := candidato(func(c *Candidato) { c.Lancamento.ID = identidade.ID{7} })

	vencedor, pontos := Melhor(evidencia(), []Candidato{longe, perto})
	if vencedor.Lancamento.ID != perto.Lancamento.ID {
		t.Errorf("vencedor = %v", vencedor.Lancamento.ID)
	}
	if pontos != 115 {
		t.Errorf("pontos = %d", pontos)
	}
}

func TestSimilaridade(t *testing.T) {
	casos := []struct {
		a, b   string
		minimo float64
		maximo float64
	}{
		{"SUPERMERCADO XYZ", "SUPERMERCADO XYZ", 1, 1},
		{"", "", 0, 0},
		{"UBER", "", 0, 0},
		{"AB", "AB", 1, 1}, // curtas: conjunto e a propria string
		{"AB", "AC", 0, 0},
		{"SUPERMERCADO XYZ", "SUPERMERCADO XYZ LTDA", 0.7, 0.99},
		{"UBER TRIP", "IFOOD RESTAURANTE", 0, 0.1},
		{"PAO DE ACUCAR", "ACUCAR DE PAO", 0.4, 0.9}, // mesmos trigramas em outra ordem
	}

	for _, c := range casos {
		t.Run(c.a+"/"+c.b, func(t *testing.T) {
			s := Similaridade(c.a, c.b)
			if s < c.minimo || s > c.maximo {
				t.Errorf("Similaridade(%q, %q) = %.3f, esperava [%.2f, %.2f]", c.a, c.b, s, c.minimo, c.maximo)
			}
			if s != Similaridade(c.b, c.a) {
				t.Error("similaridade nao e simetrica")
			}
		})
	}
}
