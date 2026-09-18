// Package categorizacao decide a categoria de um lancamento a partir de
// regras sobre a contraparte normalizada. Sem IA e sem sorte: a precedencia
// e uma ordem total, entao duas execucoes com as mesmas regras dao sempre a
// mesma resposta — inclusive com as regras embaralhadas.
package categorizacao

import (
	"cmp"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
)

// Tipo e a forma de casamento da regra. O peso da precedencia e do mais
// especifico para o mais generico: exata > prefixo > contem > regex.
type Tipo string

const (
	TipoExata   Tipo = "exata"
	TipoPrefixo Tipo = "prefixo"
	TipoContem  Tipo = "contem"
	TipoRegex   Tipo = "regex"
)

func (t Tipo) peso() int {
	switch t {
	case TipoExata:
		return 0
	case TipoPrefixo:
		return 1
	case TipoContem:
		return 2
	case TipoRegex:
		return 3
	}
	return 4
}

var (
	ErrTipoInvalido   = errors.New("categorizacao: tipo de regra desconhecido")
	ErrPadraoVazio    = errors.New("categorizacao: padrao vazio")
	ErrRegexInvalida  = errors.New("categorizacao: regex invalida")
	ErrCategoriaVazia = errors.New("categorizacao: regra sem categoria")
)

// Regra compara um padrao com a contraparte normalizada. A regex e compilada
// no construtor: regra invalida nao existe em memoria, e o RE2 do Go nao
// retrocede, entao nao ha ReDoS por construcao.
type Regra struct {
	ID         int64
	Categoria  categoria.ID
	Tipo       Tipo
	Padrao     string
	Prioridade int16

	compilada *regexp.Regexp
}

func NovaRegra(id int64, cat categoria.ID, tipo Tipo, padrao string, prioridade int16) (Regra, error) {
	if cat <= 0 {
		return Regra{}, ErrCategoriaVazia
	}
	padrao = strings.TrimSpace(padrao)
	if padrao == "" {
		return Regra{}, ErrPadraoVazio
	}

	r := Regra{ID: id, Categoria: cat, Tipo: tipo, Padrao: padrao, Prioridade: prioridade}
	switch tipo {
	case TipoExata, TipoPrefixo, TipoContem:
		// Padrao textual compara em maiusculas, como contraparte_norm.
		r.Padrao = strings.ToUpper(padrao)
	case TipoRegex:
		// (?i): a contraparte normalizada e maiuscula; regex escrita em
		// minusculas casaria nunca e falharia em silencio.
		compilada, err := regexp.Compile("(?i)" + padrao)
		if err != nil {
			return Regra{}, ErrRegexInvalida
		}
		r.compilada = compilada
	default:
		return Regra{}, ErrTipoInvalido
	}
	return r, nil
}

// Corresponde diz se a regra casa com a contraparte normalizada.
func (r Regra) Corresponde(contraparteNorm string) bool {
	switch r.Tipo {
	case TipoExata:
		return contraparteNorm == r.Padrao
	case TipoPrefixo:
		return strings.HasPrefix(contraparteNorm, r.Padrao)
	case TipoContem:
		return strings.Contains(contraparteNorm, r.Padrao)
	case TipoRegex:
		return r.compilada != nil && r.compilada.MatchString(contraparteNorm)
	}
	return false
}

// Resultado carrega a decisao e a regra que a tomou — a Fase 3 usa o ID da
// regra para contabilizar acertos e erros do feedback humano.
type Resultado struct {
	Categoria categoria.ID
	RegraID   int64
}

// Classificar devolve a categoria da primeira regra na ordem de precedencia
// que corresponder. A ordem e total: prioridade DESC, tipo mais especifico,
// padrao mais longo, menor id — nunca ha empate nao resolvido.
func Classificar(contraparteNorm string, regras []Regra) (Resultado, bool) {
	candidatas := make([]Regra, 0, len(regras))
	for _, r := range regras {
		if r.Corresponde(contraparteNorm) {
			candidatas = append(candidatas, r)
		}
	}
	if len(candidatas) == 0 {
		return Resultado{}, false
	}

	slices.SortFunc(candidatas, func(a, b Regra) int {
		if c := cmp.Compare(b.Prioridade, a.Prioridade); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Tipo.peso(), b.Tipo.peso()); c != 0 {
			return c
		}
		if c := cmp.Compare(len(b.Padrao), len(a.Padrao)); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})

	vencedora := candidatas[0]
	return Resultado{Categoria: vencedora.Categoria, RegraID: vencedora.ID}, true
}
