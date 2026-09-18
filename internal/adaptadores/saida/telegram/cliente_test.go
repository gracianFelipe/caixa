package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const tokenDeTeste = "999999:SEGREDO-DE-TESTE"

// servidorFalso grava a ultima chamada e devolve o JSON programado.
type servidorFalso struct {
	*httptest.Server
	metodo string
	corpo  map[string]any
}

func novoServidorFalso(t *testing.T, resposta string) *servidorFalso {
	t.Helper()
	f := &servidorFalso{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.metodo = strings.TrimPrefix(r.URL.Path, "/bot"+tokenDeTeste+"/")
		_ = json.NewDecoder(r.Body).Decode(&f.corpo)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resposta))
	}))
	t.Cleanup(f.Close)
	return f
}

func TestEnviarPergunta(t *testing.T) {
	servidor := novoServidorFalso(t, `{"ok":true,"result":{"message_id":42}}`)
	c := NovoCliente(tokenDeTeste, servidor.URL)

	botoes := []Botao{{"mercado", "cat:a:1"}, {"restaurante", "cat:a:2"}, {"transporte", "cat:a:3"}, {"saude", "cat:a:6"}}
	msg, err := c.EnviarPergunta(context.Background(), 777, "Qual categoria?", botoes)
	if err != nil {
		t.Fatalf("EnviarPergunta: %v", err)
	}
	if msg.ID != 42 {
		t.Errorf("message_id = %d, queria 42", msg.ID)
	}
	if servidor.metodo != "sendMessage" {
		t.Errorf("metodo chamado = %q", servidor.metodo)
	}

	// 4 botoes viram 2 linhas (3 + 1): largura de celular.
	markup := servidor.corpo["reply_markup"].(map[string]any)
	linhas := markup["inline_keyboard"].([]any)
	if len(linhas) != 2 || len(linhas[0].([]any)) != 3 || len(linhas[1].([]any)) != 1 {
		t.Errorf("teclado mal distribuido: %v", linhas)
	}
}

func TestBuscarAtualizacoes(t *testing.T) {
	servidor := novoServidorFalso(t, `{"ok":true,"result":[
		{"update_id":10,"callback_query":{"id":"cb1","data":"cat:x:2","message":{"message_id":42,"chat":{"id":777}}}},
		{"update_id":11,"message":{"chat":{"id":666}}}
	]}`)
	c := NovoCliente(tokenDeTeste, servidor.URL)

	atualizacoes, err := c.BuscarAtualizacoes(context.Background(), 9, 0)
	if err != nil {
		t.Fatalf("BuscarAtualizacoes: %v", err)
	}
	if len(atualizacoes) != 2 {
		t.Fatalf("veio %d atualizacoes", len(atualizacoes))
	}

	clique := atualizacoes[0]
	if clique.ChatID != 777 || clique.Callback != "cat:x:2" || clique.CallbackID != "cb1" || clique.MensagemID != 42 {
		t.Errorf("callback achatado errado: %+v", clique)
	}
	if texto := atualizacoes[1]; texto.ChatID != 666 || texto.Callback != "" {
		t.Errorf("mensagem comum achatada errado: %+v", texto)
	}
	if servidor.corpo["offset"].(float64) != 9 {
		t.Errorf("offset enviado = %v", servidor.corpo["offset"])
	}
}

func TestErroDaAPINaoVazaToken(t *testing.T) {
	casos := []struct {
		nome     string
		resposta string
	}{
		{"ok falso", `{"ok":false,"error_code":400,"description":"Bad Request"}`},
		{"json quebrado", `nao-e-json`},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			servidor := novoServidorFalso(t, caso.resposta)
			c := NovoCliente(tokenDeTeste, servidor.URL)

			err := c.EditarMensagem(context.Background(), 777, 42, "x")
			if !errors.Is(err, ErrAPI) {
				t.Fatalf("erro = %v, queria ErrAPI", err)
			}
			if strings.Contains(err.Error(), "SEGREDO-DE-TESTE") {
				t.Error("token vazou na mensagem de erro")
			}
		})
	}
}

func TestFalhaDeRedeNaoVazaToken(t *testing.T) {
	// Porta 1: conexao recusada na hora.
	c := NovoCliente(tokenDeTeste, "http://127.0.0.1:1")
	err := c.ConfirmarCallback(context.Background(), "cb", "ok")
	if err == nil {
		t.Fatal("deveria falhar")
	}
	if strings.Contains(err.Error(), "SEGREDO-DE-TESTE") {
		t.Errorf("token vazou no erro de rede: %v", err)
	}
}
