package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gracianFelipe/caixa/internal/aplicacao"
	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

var _ aplicacao.MensageiroDeCategoria = (*Mensageiro)(nil)

var ErrCallbackInvalido = errors.New("telegram: callback fora do formato cat:<lancamento>:<categoria>")

// Mensageiro traduz a porta da aplicacao para chamadas do Cliente.
type Mensageiro struct {
	cliente *Cliente
}

func NovoMensageiro(c *Cliente) *Mensageiro {
	return &Mensageiro{cliente: c}
}

// PerguntarCategoria monta a mensagem com valor e contraparte — e o chat
// privado do dono; a regra de nao vazar PII vale para logs, nao para ele.
func (m *Mensageiro) PerguntarCategoria(ctx context.Context, chatID int64, l lancamento.Lancamento, opcoes []categoria.Categoria) (int64, error) {
	texto := fmt.Sprintf("%s — %s (%s, %s)\nQual categoria?",
		l.Valor, l.Contraparte, l.Meio, l.OcorridoEm.Format("02/01"))

	botoes := make([]Botao, 0, len(opcoes))
	for _, c := range opcoes {
		botoes = append(botoes, Botao{Texto: c.Nome, Dados: DadosDeCallback(l.ID, c.ID)})
	}

	msg, err := m.cliente.EnviarPergunta(ctx, chatID, texto, botoes)
	if err != nil {
		return 0, err
	}
	return msg.ID, nil
}

// DadosDeCallback cabe com folga nos 64 bytes do Telegram: 4 + 36 + 1 + ate 5.
func DadosDeCallback(lancamentoID identidade.ID, cat categoria.ID) string {
	return "cat:" + lancamentoID.String() + ":" + strconv.Itoa(int(cat))
}

// AnalisarCallback e o inverso estrito: qualquer desvio e erro, porque o
// dado vem de fora (IV) — mesmo vindo do proprio teclado que enviamos.
func AnalisarCallback(dados string) (identidade.ID, categoria.ID, error) {
	resto, ok := strings.CutPrefix(dados, "cat:")
	if !ok {
		return identidade.ID{}, 0, ErrCallbackInvalido
	}
	idTexto, catTexto, ok := strings.Cut(resto, ":")
	if !ok {
		return identidade.ID{}, 0, ErrCallbackInvalido
	}

	lancamentoID, err := identidade.Analisar(idTexto)
	if err != nil {
		return identidade.ID{}, 0, ErrCallbackInvalido
	}
	n, err := strconv.ParseInt(catTexto, 10, 16)
	if err != nil || n <= 0 {
		return identidade.ID{}, 0, ErrCallbackInvalido
	}
	return lancamentoID, categoria.ID(n), nil
}
