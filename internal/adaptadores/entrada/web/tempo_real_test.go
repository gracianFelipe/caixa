package web

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestHubTransmiteATodos(t *testing.T) {
	hub := NovoHub(logSilencioso())
	servidor := httptest.NewServer(hub)
	t.Cleanup(servidor.Close)
	url := "ws" + strings.TrimPrefix(servidor.URL, "http")
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()

	var clientes []*websocket.Conn
	for i := 0; i < 3; i++ {
		c, _, err := websocket.Dial(ctx, url, nil)
		if err != nil {
			t.Fatalf("cliente %d: %v", i, err)
		}
		t.Cleanup(func() { c.CloseNow() })
		clientes = append(clientes, c)
	}

	// Espera o hub registrar os tres (o Accept roda na goroutine do servidor).
	prazo := time.Now().Add(2 * time.Second)
	for hub.Clientes() < 3 && time.Now().Before(prazo) {
		time.Sleep(10 * time.Millisecond)
	}
	if hub.Clientes() != 3 {
		t.Fatalf("hub tem %d clientes, queria 3", hub.Clientes())
	}

	hub.Transmitir([]byte(`{"tipo":"lancamento_criado"}`))

	for i, c := range clientes {
		tipo, dados, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("cliente %d lendo: %v", i, err)
		}
		if tipo != websocket.MessageText || string(dados) != `{"tipo":"lancamento_criado"}` {
			t.Errorf("cliente %d recebeu (%v, %s)", i, tipo, dados)
		}
	}
}

func TestHubDerrubaClienteLento(t *testing.T) {
	hub := NovoHub(logSilencioso())

	lento := hub.registrar()
	rapido := hub.registrar()

	// O lento nunca le; o rapido drena a cada rodada. Depois de encher o
	// buffer do lento, a transmissao seguinte o remove — e SO ele.
	for i := 0; i <= bufferDoCliente; i++ {
		hub.Transmitir([]byte("x"))
		for len(rapido) > 0 { // drena o rapido para ele nunca estourar
			<-rapido
		}
	}

	if hub.Clientes() != 1 {
		t.Fatalf("hub tem %d clientes, queria so o rapido", hub.Clientes())
	}
	// O canal do lento foi fechado pelo hub.
	for range lento {
	}
	// O rapido continua funcional: recebe a proxima transmissao.
	hub.Transmitir([]byte("y"))
	select {
	case m := <-rapido:
		if string(m) != "y" {
			t.Errorf("rapido recebeu %q", m)
		}
	default:
		t.Error("cliente rapido deveria ter recebido a transmissao")
	}
	hub.remover(rapido)
	if hub.Clientes() != 0 {
		t.Error("remover nao limpou")
	}
}

func TestHubFechamentoDoClienteLiberaRegistro(t *testing.T) {
	hub := NovoHub(logSilencioso())
	servidor := httptest.NewServer(hub)
	t.Cleanup(servidor.Close)
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()

	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(servidor.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	prazo := time.Now().Add(2 * time.Second)
	for hub.Clientes() < 1 && time.Now().Before(prazo) {
		time.Sleep(10 * time.Millisecond)
	}

	_ = c.Close(websocket.StatusNormalClosure, "tchau")

	prazo = time.Now().Add(2 * time.Second)
	for hub.Clientes() > 0 && time.Now().Before(prazo) {
		time.Sleep(10 * time.Millisecond)
	}
	if hub.Clientes() != 0 {
		t.Errorf("cliente fechado continua registrado: %d", hub.Clientes())
	}
}
