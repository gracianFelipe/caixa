package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

var (
	idDeTeste  = identidade.ID{0x01, 0x92, 0x6a, 0x5c, 0x12, 0x34, 0x70, 0x00, 0x80, 0x00, 0, 0, 0, 0, 0, 7}
	perguntaID = identidade.ID{0x01, 0x92, 0x6a, 0x5c, 0x12, 0x34, 0x70, 0x00, 0x80, 0x00, 0, 0, 0, 0, 0, 8}
)

func lancamentoDeTeste(t *testing.T, contraparte string) lancamento.Lancamento {
	t.Helper()
	l, err := lancamento.Novo(idDeTeste, lancamento.Dados{
		OcorridoEm:  time.Date(2026, 9, 17, 15, 0, 0, 0, time.UTC),
		Valor:       -4790,
		Meio:        lancamento.MeioPix,
		Contraparte: contraparte,
	}, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestPerguntarCategoria(t *testing.T) {
	servidor := novoServidorFalso(t, `{"ok":true,"result":{"message_id":55}}`)
	m := NovoMensageiro(NovoCliente(tokenDeTeste, servidor.URL))

	mercado, _ := categoria.Nova(1, "mercado")
	msgID, err := m.PerguntarCategoria(context.Background(), 777, perguntaID, lancamentoDeTeste(t, "LOJA MISTERIOSA"), []categoria.Categoria{mercado})
	if err != nil {
		t.Fatalf("PerguntarCategoria: %v", err)
	}
	if msgID != 55 {
		t.Errorf("mensagem id = %d", msgID)
	}

	texto := servidor.corpo["text"].(string)
	if !strings.Contains(texto, "-R$ 47,90") || !strings.Contains(texto, "LOJA MISTERIOSA") {
		t.Errorf("texto da pergunta = %q", texto)
	}
}

func TestPerguntarConciliacao(t *testing.T) {
	servidor := novoServidorFalso(t, `{"ok":true,"result":{"message_id":56}}`)
	m := NovoMensageiro(NovoCliente(tokenDeTeste, servidor.URL))

	_, err := m.PerguntarConciliacao(context.Background(), 777, perguntaID,
		lancamentoDeTeste(t, "LOJA DUVIDOSA"), lancamentoDeTeste(t, "OUTRA LOJA"))
	if err != nil {
		t.Fatalf("PerguntarConciliacao: %v", err)
	}

	markup := servidor.corpo["reply_markup"].(map[string]any)
	linhas := markup["inline_keyboard"].([]any)
	if len(linhas) != 1 || len(linhas[0].([]any)) != 2 {
		t.Errorf("teclado = %v, queria dois botoes numa linha", linhas)
	}
}

func TestCallbacksIdaEVolta(t *testing.T) {
	casos := []struct {
		nome  string
		dados string
		quer  Callback
	}{
		{"categoria", DadosDeCategoria(perguntaID, 12), Callback{PerguntaID: perguntaID, Categoria: 12}},
		{"mesmo gasto", DadosDeConciliacao(perguntaID, true), Callback{PerguntaID: perguntaID, EhConciliacao: true, Conciliar: true}},
		{"gasto novo", DadosDeConciliacao(perguntaID, false), Callback{PerguntaID: perguntaID, EhConciliacao: true, Conciliar: false}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if len(c.dados) > 64 {
				t.Fatalf("callback com %d bytes estoura o limite do Telegram", len(c.dados))
			}
			obtido, err := AnalisarCallback(c.dados)
			if err != nil || obtido != c.quer {
				t.Errorf("AnalisarCallback(%q) = (%+v, %v), queria %+v", c.dados, obtido, err, c.quer)
			}
		})
	}
}

func TestAnalisarCallbackErro(t *testing.T) {
	casos := []string{
		"",
		"cat:",
		"cat:abc:1",
		"cat:" + perguntaID.String(),
		"cat:" + perguntaID.String() + ":zero",
		"cat:" + perguntaID.String() + ":0",
		"cat:" + perguntaID.String() + ":-2",
		"cat:" + perguntaID.String() + ":99999999",
		"con:" + perguntaID.String() + ":talvez",
		"con:" + perguntaID.String(),
		"rel:" + perguntaID.String() + ":s",
	}
	for _, dados := range casos {
		t.Run(dados, func(t *testing.T) {
			if _, err := AnalisarCallback(dados); !errors.Is(err, ErrCallbackInvalido) {
				t.Errorf("AnalisarCallback(%q) = %v, queria ErrCallbackInvalido", dados, err)
			}
		})
	}
}
