// Package lancamento e o agregado raiz do Caixa: um movimento de dinheiro que
// aconteceu. Toda instancia nasce por Novo, que garante as invariantes; nao
// existe Lancamento invalido em memoria.
package lancamento

import (
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
)

// Meio e a forma de pagamento. Tipo proprio sobre string: o compilador impede
// passar qualquer texto onde se espera um meio, e o banco tem o mesmo CHECK.
type Meio string

const (
	MeioPix           Meio = "pix"
	MeioCredito       Meio = "credito"
	MeioDebito        Meio = "debito"
	MeioBoleto        Meio = "boleto"
	MeioDinheiro      Meio = "dinheiro"
	MeioTransferencia Meio = "transferencia"
)

// AnalisarMeio valida texto vindo de fora (JSON, extrato) contra a lista fechada.
func AnalisarMeio(texto string) (Meio, error) {
	switch m := Meio(strings.ToLower(strings.TrimSpace(texto))); m {
	case MeioPix, MeioCredito, MeioDebito, MeioBoleto, MeioDinheiro, MeioTransferencia:
		return m, nil
	default:
		return "", ErrMeioInvalido
	}
}

// Limite de tamanho da contraparte: extrato do banco raramente passa de 60.
const ContraparteMaxima = 200

var (
	ErrIDVazio           = errors.New("lancamento: id vazio")
	ErrInstanteZero      = errors.New("lancamento: instante nao informado")
	ErrValorZero         = errors.New("lancamento: valor nao pode ser zero")
	ErrMeioInvalido      = errors.New("lancamento: meio de pagamento invalido")
	ErrContraparteVazia  = errors.New("lancamento: contraparte vazia")
	ErrContraparteLonga  = errors.New("lancamento: contraparte acima do limite")
	ErrCategoriaInvalida = errors.New("lancamento: categoria invalida")
	ErrOrigemDeCategoria = errors.New("lancamento: origem de categoria invalida")
)

// Situacao e o ciclo de vida do fato. Provisorio existe para a faixa de
// conciliacao 60-84: o gasto aparece nas listas (nada some), mas esta
// marcado ate o dono confirmar se e novo ou o mesmo de outra origem.
// Descartado e o "apagar sem apagar": a linha fica, fora das somas.
type Situacao string

const (
	SituacaoProvisoria Situacao = "provisorio"
	SituacaoConfirmada Situacao = "confirmado"
	SituacaoDescartada Situacao = "descartado"
)

// OrigemDaCategoria registra quem decidiu a categoria. Espelha o CHECK do
// banco; "pendente" e o estado de quem ainda vai para a fila de pergunta.
type OrigemDaCategoria string

const (
	CategoriaPendente  OrigemDaCategoria = "pendente"
	CategoriaPorRegra  OrigemDaCategoria = "regra"
	CategoriaManual    OrigemDaCategoria = "manual"
	CategoriaImportada OrigemDaCategoria = "importacao"
)

// Lancamento e imutavel apos Novo: campos exportados para leitura e para o
// repositorio gravar, mas nenhum metodo os altera.
type Lancamento struct {
	ID              identidade.ID
	OcorridoEm      time.Time // sempre em UTC
	Competencia     competencia.Competencia
	Valor           dinheiro.Centavos // negativo = saida
	Meio            Meio
	Contraparte     string
	ContraparteNorm string            // forma canonica para regras e conciliacao
	CategoriaID     categoria.ID      // zero = sem categoria
	CategoriaOrigem OrigemDaCategoria // pendente enquanto CategoriaID for zero
	Situacao        Situacao
}

// Dados e o que vem de fora para criar um lancamento. Struct em vez de seis
// parametros posicionais: no chamador fica claro qual campo e qual.
type Dados struct {
	OcorridoEm  time.Time
	Valor       dinheiro.Centavos
	Meio        Meio
	Contraparte string
}

// Novo valida as invariantes e monta o lancamento. O fuso entra como parametro
// porque a competencia depende dele e o dominio nao decide fuso nem le tzdata.
func Novo(id identidade.ID, d Dados, fuso *time.Location) (Lancamento, error) {
	if id.EhZero() {
		return Lancamento{}, ErrIDVazio
	}
	if d.OcorridoEm.IsZero() {
		return Lancamento{}, ErrInstanteZero
	}
	if d.Valor == 0 {
		return Lancamento{}, ErrValorZero
	}
	meio, err := AnalisarMeio(string(d.Meio))
	if err != nil {
		return Lancamento{}, err
	}

	contraparte := strings.TrimSpace(d.Contraparte)
	if contraparte == "" {
		return Lancamento{}, ErrContraparteVazia
	}
	if len([]rune(contraparte)) > ContraparteMaxima {
		return Lancamento{}, ErrContraparteLonga
	}

	return Lancamento{
		ID:              id,
		OcorridoEm:      d.OcorridoEm.UTC(),
		Competencia:     competencia.Do(d.OcorridoEm, fuso),
		Valor:           d.Valor,
		Meio:            meio,
		Contraparte:     contraparte,
		ContraparteNorm: Normalizar(contraparte),
		CategoriaOrigem: CategoriaPendente,
		Situacao:        SituacaoConfirmada,
	}, nil
}

// Provisorio devolve uma copia marcada para revisao humana (faixa 60-84 da
// conciliacao). Mesmo padrao de ComCategoria: copia, nunca mutacao.
func (l Lancamento) Provisorio() Lancamento {
	l.Situacao = SituacaoProvisoria
	return l
}

// ComCategoria devolve uma copia do lancamento com a categoria atribuida.
// Copia, nao mutacao: o agregado continua imutavel apos criado, e o chamador
// decide o que fazer com as duas versoes.
func (l Lancamento) ComCategoria(id categoria.ID, origem OrigemDaCategoria) (Lancamento, error) {
	if id <= 0 {
		return Lancamento{}, ErrCategoriaInvalida
	}
	switch origem {
	case CategoriaPorRegra, CategoriaManual, CategoriaImportada:
	default:
		// "pendente" com categoria preenchida seria estado contraditorio.
		return Lancamento{}, ErrOrigemDeCategoria
	}
	l.CategoriaID = id
	l.CategoriaOrigem = origem
	return l, nil
}

// EhSaida informa se o dinheiro saiu (valor negativo).
func (l Lancamento) EhSaida() bool {
	return l.Valor < 0
}

// Normalizar produz a forma canonica da contraparte: maiuscula, sem acento,
// sem digito, com espacos colapsados. "Pão de Açúcar 1234" -> "PAO DE ACUCAR".
// E sobre essa forma que regras de categoria e conciliacao comparam.
func Normalizar(texto string) string {
	var b strings.Builder
	espacoPendente := false

	for _, r := range strings.ToUpper(texto) {
		r = semAcento(r)
		switch {
		case unicode.IsLetter(r):
			if espacoPendente && b.Len() > 0 {
				b.WriteByte(' ')
			}
			espacoPendente = false
			b.WriteRune(r)
		case unicode.IsDigit(r):
			// Numero de terminal, parcela ou pedido nao identifica o lugar.
		default:
			espacoPendente = true
		}
	}
	return b.String()
}

// semAcento cobre o alfabeto do portugues. Tabela propria em vez de
// golang.org/x/text/unicode/norm: o dominio importa so a stdlib.
func semAcento(r rune) rune {
	switch r {
	case 'Á', 'À', 'Â', 'Ã', 'Ä':
		return 'A'
	case 'É', 'È', 'Ê', 'Ë':
		return 'E'
	case 'Í', 'Ì', 'Î', 'Ï':
		return 'I'
	case 'Ó', 'Ò', 'Ô', 'Õ', 'Ö':
		return 'O'
	case 'Ú', 'Ù', 'Û', 'Ü':
		return 'U'
	case 'Ç':
		return 'C'
	case 'Ñ':
		return 'N'
	}
	return r
}
