// Package conciliacao decide se uma evidencia recem-chegada e o mesmo gasto
// de um lancamento que ja existe. Funcao pura sobre uma pontuacao fixa:
// mesma entrada, mesma resposta, sempre — da para imprimir a conta na
// entrevista e conferir no papel.
package conciliacao

import (
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
	"github.com/gracianFelipe/caixa/internal/dominio/ocorrencia"
	"time"
)

// Evidencia e o que a origem nova afirma sobre o gasto.
type Evidencia struct {
	Valor           dinheiro.Centavos
	OcorridoEm      time.Time
	ContraparteNorm string
	Meio            lancamento.Meio
	Origem          ocorrencia.Origem
}

// Candidato e um lancamento existente elegivel — o repositorio ja garantiu
// que ele NAO tem evidencia da origem que esta chegando (e o filtro que
// salva dois gastos legitimos iguais no mesmo dia).
type Candidato struct {
	Lancamento lancamento.Lancamento
	Origens    []ocorrencia.Origem // origens das evidencias ja anexadas
}

type Decisao int

const (
	LancamentoNovo Decisao = iota
	Perguntar
	Conciliar
)

const (
	limiarConciliar = 85
	limiarPerguntar = 60
	// LimiarSimilaridade e o corte do Jaccard para valer +25.
	LimiarSimilaridade = 0.90
)

// Pontuar aplica a tabela. Valor diferente ou distancia > 3 dias devolve
// zero direto: nem candidato e.
func Pontuar(nova Evidencia, candidato Candidato) int {
	l := candidato.Lancamento
	if nova.Valor != l.Valor {
		return 0
	}

	dias := diasDeDistancia(nova.OcorridoEm, l.OcorridoEm)
	var pontosData int
	switch dias {
	case 0:
		pontosData = 25
	case 1:
		pontosData = 18
	case 2:
		pontosData = 12
	case 3:
		pontosData = 6
	default:
		return 0
	}

	pontos := 50 + pontosData

	if Similaridade(nova.ContraparteNorm, l.ContraparteNorm) >= LimiarSimilaridade {
		pontos += 25
	}
	if nova.Meio == l.Meio {
		pontos += 10
	}
	if origemJaPresente(nova.Origem, candidato.Origens) {
		pontos -= 20
	} else {
		pontos += 5
	}
	return pontos
}

// Decidir traduz pontos em acao. As fronteiras sao inclusivas por cima:
// 85 concilia, 84 pergunta, 60 pergunta, 59 e lancamento novo.
func Decidir(pontos int) Decisao {
	switch {
	case pontos >= limiarConciliar:
		return Conciliar
	case pontos >= limiarPerguntar:
		return Perguntar
	default:
		return LancamentoNovo
	}
}

// Melhor pontua todos os candidatos e devolve o vencedor. Empate resolve
// pelo mais antigo na lista (o repositorio ordena por ocorrido_em, id) —
// deterministico como tudo aqui.
func Melhor(nova Evidencia, candidatos []Candidato) (Candidato, int) {
	var (
		vencedor Candidato
		maximo   int
	)
	for _, c := range candidatos {
		if p := Pontuar(nova, c); p > maximo {
			vencedor, maximo = c, p
		}
	}
	return vencedor, maximo
}

// diasDeDistancia conta em dias de calendario UTC, nao em blocos de 24h:
// 23:50 e 00:10 do dia seguinte distam 1 dia, como um humano contaria.
func diasDeDistancia(a, b time.Time) int {
	au := a.UTC()
	bu := b.UTC()
	diaA := au.Truncate(24 * time.Hour)
	diaB := bu.Truncate(24 * time.Hour)
	dias := int(diaA.Sub(diaB) / (24 * time.Hour))
	if dias < 0 {
		return -dias
	}
	return dias
}

func origemJaPresente(origem ocorrencia.Origem, existentes []ocorrencia.Origem) bool {
	for _, o := range existentes {
		if o == origem {
			return true
		}
	}
	return false
}

// Similaridade e o indice de Jaccard sobre trigramas: |A∩B| / |A∪B|.
// Robusto a prefixo/sufixo trocado ("UBER TRIP" vs "UBER *TRIP HELP US"),
// barato e sem dependencia — as ~30 linhas prometidas no plano.
func Similaridade(a, b string) float64 {
	if a == b {
		if a == "" {
			return 0
		}
		return 1
	}

	ta := trigramas(a)
	tb := trigramas(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}

	intersecao := 0
	for t := range ta {
		if tb[t] {
			intersecao++
		}
	}
	uniao := len(ta) + len(tb) - intersecao
	return float64(intersecao) / float64(uniao)
}

func trigramas(s string) map[string]bool {
	runas := []rune(s)
	if len(runas) < 3 {
		if len(runas) == 0 {
			return nil
		}
		return map[string]bool{string(runas): true}
	}
	conjunto := make(map[string]bool, len(runas)-2)
	for i := 0; i+3 <= len(runas); i++ {
		conjunto[string(runas[i:i+3])] = true
	}
	return conjunto
}
