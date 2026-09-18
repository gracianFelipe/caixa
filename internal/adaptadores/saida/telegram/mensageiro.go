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

var _ aplicacao.Mensageiro = (*Mensageiro)(nil)

var ErrCallbackInvalido = errors.New("telegram: callback fora do formato esperado")

// Mensageiro traduz a porta da aplicacao para chamadas do Cliente.
type Mensageiro struct {
	cliente *Cliente
}

func NovoMensageiro(c *Cliente) *Mensageiro {
	return &Mensageiro{cliente: c}
}

// PerguntarCategoria monta a mensagem com valor e contraparte — e o chat
// privado do dono; a regra de nao vazar PII vale para logs, nao para ele.
func (m *Mensageiro) PerguntarCategoria(ctx context.Context, chatID int64, perguntaID identidade.ID, l lancamento.Lancamento, opcoes []categoria.Categoria) (int64, error) {
	texto := fmt.Sprintf("%s — %s (%s, %s)\nQual categoria?",
		l.Valor, l.Contraparte, l.Meio, l.OcorridoEm.Format("02/01"))

	botoes := make([]Botao, 0, len(opcoes))
	for _, c := range opcoes {
		botoes = append(botoes, Botao{Texto: c.Nome, Dados: DadosDeCategoria(perguntaID, c.ID)})
	}

	msg, err := m.cliente.EnviarPergunta(ctx, chatID, texto, botoes)
	if err != nil {
		return 0, err
	}
	return msg.ID, nil
}

// PerguntarConciliacao mostra os dois lados e dois botoes.
func (m *Mensageiro) PerguntarConciliacao(ctx context.Context, chatID int64, perguntaID identidade.ID, provisorio, candidato lancamento.Lancamento) (int64, error) {
	texto := fmt.Sprintf("Chegou %s — %s (%s)\nParece o mesmo gasto de %s — %s (%s).\nE o mesmo?",
		provisorio.Valor, provisorio.Contraparte, provisorio.OcorridoEm.Format("02/01"),
		candidato.Valor, candidato.Contraparte, candidato.OcorridoEm.Format("02/01"))

	botoes := []Botao{
		{Texto: "mesmo gasto", Dados: DadosDeConciliacao(perguntaID, true)},
		{Texto: "gasto novo", Dados: DadosDeConciliacao(perguntaID, false)},
	}
	msg, err := m.cliente.EnviarPergunta(ctx, chatID, texto, botoes)
	if err != nil {
		return 0, err
	}
	return msg.ID, nil
}

func (m *Mensageiro) EnviarAviso(ctx context.Context, chatID int64, texto string) error {
	return m.cliente.EnviarTexto(ctx, chatID, texto)
}

// Formato dos callbacks (limite de 64 bytes do Telegram):
//   cat:<perguntaID>:<categoriaID>   4 + 36 + 1 + ate 5 = 46
//   con:<perguntaID>:s|n             4 + 36 + 2        = 42

func DadosDeCategoria(perguntaID identidade.ID, cat categoria.ID) string {
	return "cat:" + perguntaID.String() + ":" + strconv.Itoa(int(cat))
}

func DadosDeConciliacao(perguntaID identidade.ID, mesmoGasto bool) string {
	sufixo := "n"
	if mesmoGasto {
		sufixo = "s"
	}
	return "con:" + perguntaID.String() + ":" + sufixo
}

// Callback e o resultado de AnalisarCallback: um dos dois tipos preenchido.
type Callback struct {
	PerguntaID    identidade.ID
	Categoria     categoria.ID // > 0 quando e resposta de categoria
	Conciliar     bool         // valido quando EhConciliacao
	EhConciliacao bool
}

// AnalisarCallback e estrito: o dado vem de fora (IV) mesmo sendo do
// proprio teclado que enviamos — qualquer desvio e erro.
func AnalisarCallback(dados string) (Callback, error) {
	prefixo, resto, ok := strings.Cut(dados, ":")
	if !ok {
		return Callback{}, ErrCallbackInvalido
	}
	idTexto, cauda, ok := strings.Cut(resto, ":")
	if !ok {
		return Callback{}, ErrCallbackInvalido
	}
	perguntaID, err := identidade.Analisar(idTexto)
	if err != nil {
		return Callback{}, ErrCallbackInvalido
	}

	switch prefixo {
	case "cat":
		n, err := strconv.ParseInt(cauda, 10, 16)
		if err != nil || n <= 0 {
			return Callback{}, ErrCallbackInvalido
		}
		return Callback{PerguntaID: perguntaID, Categoria: categoria.ID(n)}, nil
	case "con":
		switch cauda {
		case "s":
			return Callback{PerguntaID: perguntaID, EhConciliacao: true, Conciliar: true}, nil
		case "n":
			return Callback{PerguntaID: perguntaID, EhConciliacao: true, Conciliar: false}, nil
		}
	}
	return Callback{}, ErrCallbackInvalido
}
