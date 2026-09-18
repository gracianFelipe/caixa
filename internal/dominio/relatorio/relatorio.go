// Package relatorio responde "para onde o dinheiro foi e o que esta errado"
// com cinco detectores deterministicos. Funcoes puras sobre uma Janela:
// mesma entrada, mesmo relatorio — por isso da para congelar em golden file.
package relatorio

import (
	"cmp"
	"slices"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

// MesesDeHistorico e quantas competencias ANTES do alvo a janela carrega.
const MesesDeHistorico = 6

// Janela e tudo que os detectores precisam. Quem monta e a aplicacao; o
// dominio nao consulta nada.
type Janela struct {
	Alvo        competencia.Competencia
	Lancamentos []lancamento.Lancamento            // confirmados, alvo + 6 anteriores
	Limites     map[categoria.ID]dinheiro.Centavos // vigentes no alvo
}

type TipoDeSinal string

const (
	SinalRepeticao  TipoDeSinal = "vazamento_por_repeticao"
	SinalAssinatura TipoDeSinal = "assinatura_esquecida"
	SinalEscalada   TipoDeSinal = "categoria_em_escalada"
	SinalEstouro    TipoDeSinal = "estouro_de_orcamento"
	SinalAtipico    TipoDeSinal = "gasto_atipico"
)

// Sinal e um achado. Severidade e um inteiro comparavel entre sinais do
// mesmo tipo (percentual ou contagem), nunca float.
type Sinal struct {
	Tipo        TipoDeSinal
	Categoria   categoria.ID
	Contraparte string
	Valor       dinheiro.Centavos
	Severidade  int
	Detalhe     string
}

// Detector e a assinatura comum: recebe a janela, devolve zero ou mais sinais.
type Detector func(Janela) []Sinal

// Detectores e a lista fixa, na ordem em que aparecem no relatorio.
var Detectores = []Detector{
	DetectarRepeticao,
	DetectarAssinatura,
	DetectarEscalada,
	DetectarEstouro,
	DetectarAtipico,
}

type ResumoDeCategoria struct {
	Categoria  categoria.ID
	Total      dinheiro.Centavos // magnitude positiva das saidas
	Quantidade int
}

type Relatorio struct {
	Competencia       competencia.Competencia
	TotalSaidas       dinheiro.Centavos // magnitude positiva
	TotalEntradas     dinheiro.Centavos
	Saldo             dinheiro.Centavos
	SaidasMesAnterior dinheiro.Centavos
	PorCategoria      []ResumoDeCategoria
	Sinais            []Sinal
}

// Gerar monta totais e roda todos os detectores.
func Gerar(j Janela) Relatorio {
	r := Relatorio{Competencia: j.Alvo}
	porCategoria := map[categoria.ID]*ResumoDeCategoria{}
	anterior := mesAnterior(j.Alvo)

	for _, l := range j.Lancamentos {
		switch {
		case l.Competencia == j.Alvo && l.EhSaida():
			r.TotalSaidas += -l.Valor
			res := porCategoria[l.CategoriaID]
			if res == nil {
				res = &ResumoDeCategoria{Categoria: l.CategoriaID}
				porCategoria[l.CategoriaID] = res
			}
			res.Total += -l.Valor
			res.Quantidade++
		case l.Competencia == j.Alvo:
			r.TotalEntradas += l.Valor
		case l.Competencia == anterior && l.EhSaida():
			r.SaidasMesAnterior += -l.Valor
		}
	}
	r.Saldo = r.TotalEntradas - r.TotalSaidas

	for _, res := range porCategoria {
		r.PorCategoria = append(r.PorCategoria, *res)
	}
	slices.SortFunc(r.PorCategoria, func(a, b ResumoDeCategoria) int {
		if c := cmp.Compare(b.Total, a.Total); c != 0 {
			return c
		}
		return cmp.Compare(a.Categoria, b.Categoria)
	})

	for _, d := range Detectores {
		r.Sinais = append(r.Sinais, d(j)...)
	}
	return r
}

// --- detector 1: vazamento por repeticao ---

const (
	repeticaoValorMaximo = 3000 // R$ 30,00
	repeticaoMinima      = 8
	repeticaoSomaMinima  = 15000 // R$ 150,00
)

func DetectarRepeticao(j Janela) []Sinal {
	type acumulado struct {
		quantidade int
		soma       dinheiro.Centavos
	}
	por := map[categoria.ID]*acumulado{}

	for _, l := range j.Lancamentos {
		if l.Competencia != j.Alvo || !l.EhSaida() || -l.Valor > repeticaoValorMaximo {
			continue
		}
		a := por[l.CategoriaID]
		if a == nil {
			a = &acumulado{}
			por[l.CategoriaID] = a
		}
		a.quantidade++
		a.soma += -l.Valor
	}

	var sinais []Sinal
	for cat, a := range por {
		if a.quantidade >= repeticaoMinima && a.soma >= repeticaoSomaMinima {
			sinais = append(sinais, Sinal{
				Tipo: SinalRepeticao, Categoria: cat, Valor: a.soma,
				Severidade: a.quantidade,
				Detalhe:    "muitos gastos pequenos somam alto",
			})
		}
	}
	ordenar(sinais)
	return sinais
}

// --- detector 2: assinatura esquecida ---

const (
	assinaturaMesesMinimos   = 3
	assinaturaIntervaloMin   = 28 // dias
	assinaturaIntervaloMax   = 32
	assinaturaVariacaoMaxPct = 5
)

func DetectarAssinatura(j Janela) []Sinal {
	porContraparte := map[string][]lancamento.Lancamento{}
	for _, l := range j.Lancamentos {
		if l.EhSaida() && l.ContraparteNorm != "" {
			porContraparte[l.ContraparteNorm] = append(porContraparte[l.ContraparteNorm], l)
		}
	}

	var sinais []Sinal
	for norm, ls := range porContraparte {
		slices.SortFunc(ls, func(a, b lancamento.Lancamento) int {
			return a.OcorridoEm.Compare(b.OcorridoEm)
		})
		// Precisa terminar no alvo: assinatura que parou nao e "esquecida".
		if ls[len(ls)-1].Competencia != j.Alvo {
			continue
		}

		// Conta, do fim para o inicio, quantas ocorrencias consecutivas
		// respeitam intervalo e variacao.
		cadeia := 1
		for i := len(ls) - 1; i > 0; i-- {
			atual, anterior := ls[i], ls[i-1]
			// Arredonda para o dia mais proximo: 27,9 dias e uma cobranca
			// mensal (28), nao uma quebra — truncar falharia em fevereiro.
			dias := int((atual.OcorridoEm.Sub(anterior.OcorridoEm).Hours() + 12) / 24)
			if dias < assinaturaIntervaloMin || dias > assinaturaIntervaloMax {
				break
			}
			if !variacaoDentro(-atual.Valor, -anterior.Valor, assinaturaVariacaoMaxPct) {
				break
			}
			cadeia++
		}

		if cadeia >= assinaturaMesesMinimos {
			ultimo := ls[len(ls)-1]
			sinais = append(sinais, Sinal{
				Tipo: SinalAssinatura, Categoria: ultimo.CategoriaID, Contraparte: norm,
				Valor: -ultimo.Valor, Severidade: cadeia,
				Detalhe: "cobranca recorrente ha meses; ainda usa?",
			})
		}
	}
	ordenar(sinais)
	return sinais
}

// variacaoDentro diz se |a-b| <= pct% do maior, em inteiros.
func variacaoDentro(a, b dinheiro.Centavos, pct int64) bool {
	maior, diferenca := a, a-b
	if b > a {
		maior, diferenca = b, b-a
	}
	return diferenca*100 <= maior*dinheiro.Centavos(pct)
}

// --- detector 3: categoria em escalada ---

const (
	escaladaMesesMinimos = 3
	escaladaAltaMinPct   = 25
	escaladaValorMinimo  = 10000 // R$ 100,00 no alvo
)

func DetectarEscalada(j Janela) []Sinal {
	meses := competenciasDaJanela(j.Alvo)
	somas := map[categoria.ID][]dinheiro.Centavos{}
	for _, l := range j.Lancamentos {
		if !l.EhSaida() {
			continue
		}
		idx := indiceDaCompetencia(meses, l.Competencia)
		if idx < 0 {
			continue
		}
		if somas[l.CategoriaID] == nil {
			somas[l.CategoriaID] = make([]dinheiro.Centavos, len(meses))
		}
		somas[l.CategoriaID][idx] += -l.Valor
	}

	var sinais []Sinal
	for cat, serie := range somas {
		ultimo := len(serie) - 1
		if serie[ultimo] < escaladaValorMinimo {
			continue
		}
		// Do alvo para tras: enquanto cada mes for estritamente maior que o anterior.
		crescentes := 1
		for i := ultimo; i > 0 && serie[i] > serie[i-1] && serie[i-1] > 0; i-- {
			crescentes++
		}
		if crescentes < escaladaMesesMinimos {
			continue
		}
		primeiro := serie[ultimo-crescentes+1]
		alta := (serie[ultimo] - primeiro) * 100 / primeiro
		if alta < escaladaAltaMinPct {
			continue
		}
		sinais = append(sinais, Sinal{
			Tipo: SinalEscalada, Categoria: cat, Valor: serie[ultimo],
			Severidade: int(alta),
			Detalhe:    "gasto cresce ha meses seguidos",
		})
	}
	ordenar(sinais)
	return sinais
}

// --- detector 4: estouro de orcamento ---

func DetectarEstouro(j Janela) []Sinal {
	gasto := map[categoria.ID]dinheiro.Centavos{}
	for _, l := range j.Lancamentos {
		if l.Competencia == j.Alvo && l.EhSaida() {
			gasto[l.CategoriaID] += -l.Valor
		}
	}

	var sinais []Sinal
	for cat, limite := range j.Limites {
		if limite <= 0 || gasto[cat] <= limite {
			continue
		}
		sinais = append(sinais, Sinal{
			Tipo: SinalEstouro, Categoria: cat, Valor: gasto[cat],
			Severidade: int(gasto[cat] * 100 / limite),
			Detalhe:    "passou do limite definido",
		})
	}
	ordenar(sinais)
	return sinais
}

// --- detector 5: gasto atipico ---

const (
	atipicoAmostrasMinimas = 10
	atipicoMultiplicador   = 4
	atipicoValorMinimo     = 10000 // R$ 100,00
)

func DetectarAtipico(j Janela) []Sinal {
	var historico []dinheiro.Centavos
	for _, l := range j.Lancamentos {
		if l.EhSaida() && l.Competencia != j.Alvo {
			historico = append(historico, -l.Valor)
		}
	}
	if len(historico) < atipicoAmostrasMinimas {
		return nil
	}

	med := mediana(historico)
	desvios := make([]dinheiro.Centavos, len(historico))
	for i, v := range historico {
		if v >= med {
			desvios[i] = v - med
		} else {
			desvios[i] = med - v
		}
	}
	corte := med + atipicoMultiplicador*mediana(desvios)
	// MAD zero (historico de valores identicos) faria o corte cair na propria
	// mediana e qualquer centavo acima viraria "atipico". Piso: o dobro.
	if corte <= med {
		corte = 2 * med
	}

	var sinais []Sinal
	for _, l := range j.Lancamentos {
		if l.Competencia != j.Alvo || !l.EhSaida() {
			continue
		}
		valor := -l.Valor
		if valor > corte && valor >= atipicoValorMinimo {
			severidade := 0
			if corte > 0 {
				severidade = int(valor * 100 / corte)
			}
			sinais = append(sinais, Sinal{
				Tipo: SinalAtipico, Categoria: l.CategoriaID, Contraparte: l.ContraparteNorm,
				Valor: valor, Severidade: severidade,
				Detalhe: "muito acima do seu padrao dos ultimos meses",
			})
		}
	}
	ordenar(sinais)
	return sinais
}

// mediana ordena uma copia e pega o meio (media inteira dos dois centrais).
func mediana(valores []dinheiro.Centavos) dinheiro.Centavos {
	copia := slices.Clone(valores)
	slices.Sort(copia)
	n := len(copia)
	if n%2 == 1 {
		return copia[n/2]
	}
	return (copia[n/2-1] + copia[n/2]) / 2
}

// --- utilitarios ---

// competenciasDaJanela devolve as 7 competencias em ordem cronologica
// (alvo por ultimo).
func competenciasDaJanela(alvo competencia.Competencia) []competencia.Competencia {
	meses := make([]competencia.Competencia, MesesDeHistorico+1)
	c := alvo
	for i := MesesDeHistorico; i >= 0; i-- {
		meses[i] = c
		c = mesAnterior(c)
	}
	return meses
}

func indiceDaCompetencia(meses []competencia.Competencia, c competencia.Competencia) int {
	for i, m := range meses {
		if m == c {
			return i
		}
	}
	return -1
}

// mesAnterior anda um mes para tras a partir do dia 1: nunca AddDate a
// partir do dia 31 (que pularia fevereiro).
func mesAnterior(c competencia.Competencia) competencia.Competencia {
	d := c.PrimeiroDia().AddDate(0, -1, 0)
	anterior, _ := competencia.Nova(d.Year(), d.Month())
	return anterior
}

// Inicio da janela: a primeira competencia carregada.
func InicioDaJanela(alvo competencia.Competencia) competencia.Competencia {
	return competenciasDaJanela(alvo)[0]
}

// ordenar fixa a ordem dos sinais: severidade desc, depois categoria e
// contraparte — determinismo para o golden file, mesmo vindo de map.
func ordenar(sinais []Sinal) {
	slices.SortFunc(sinais, func(a, b Sinal) int {
		if c := cmp.Compare(b.Severidade, a.Severidade); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Categoria, b.Categoria); c != 0 {
			return c
		}
		return cmp.Compare(a.Contraparte, b.Contraparte)
	})
}
