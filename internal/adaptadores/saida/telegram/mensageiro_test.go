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

var idDeTeste = identidade.ID{0x01, 0x92, 0x6a, 0x5c, 0x12, 0x34, 0x70, 0x00, 0x80, 0x00, 0, 0, 0, 0, 0, 7}

func TestPerguntarCategoria(t *testing.T) {
	servidor := novoServidorFalso(t, `{"ok":true,"result":{"message_id":55}}`)
	m := NovoMensageiro(NovoCliente(tokenDeTeste, servidor.URL))

	l, err := lancamento.Novo(idDeTeste, lancamento.Dados{
		OcorridoEm:  time.Date(2026, 9, 17, 15, 0, 0, 0, time.UTC),
		Valor:       -4790,
		Meio:        lancamento.MeioPix,
		Contraparte: "LOJA MISTERIOSA",
	}, time.UTC)
	if err != nil {
		t.Fatal(err)
	}

	mercado, _ := categoria.Nova(1, "mercado")
	msgID, err := m.PerguntarCategoria(context.Background(), 777, l, []categoria.Categoria{mercado})
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

func TestCallbackIdaEVolta(t *testing.T) {
	dados := DadosDeCallback(idDeTeste, 12)
	if len(dados) > 64 {
		t.Fatalf("callback com %d bytes estoura o limite do Telegram", len(dados))
	}

	l, cat, err := AnalisarCallback(dados)
	if err != nil || l != idDeTeste || cat != 12 {
		t.Errorf("ida e volta = (%s, %d, %v)", l, cat, err)
	}
}

func TestAnalisarCallbackErro(t *testing.T) {
	casos := []string{
		"",
		"cat:",
		"cat:abc:1",
		"cat:" + idDeTeste.String(),
		"cat:" + idDeTeste.String() + ":zero",
		"cat:" + idDeTeste.String() + ":0",
		"cat:" + idDeTeste.String() + ":-2",
		"rel:" + idDeTeste.String() + ":1",
		"cat:" + idDeTeste.String() + ":99999999",
	}
	for _, dados := range casos {
		t.Run(dados, func(t *testing.T) {
			if _, _, err := AnalisarCallback(dados); !errors.Is(err, ErrCallbackInvalido) {
				t.Errorf("AnalisarCallback(%q) = %v, queria ErrCallbackInvalido", dados, err)
			}
		})
	}
}
