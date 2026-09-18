package web

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Hub retransmite avisos curtos a todos os clientes WebSocket. O cliente
// lento e desconectado em vez de travar os outros: o canal de envio tem
// buffer e o envio nunca bloqueia.
type Hub struct {
	mu       sync.Mutex
	clientes map[chan []byte]struct{}
	log      *slog.Logger
}

func NovoHub(log *slog.Logger) *Hub {
	return &Hub{clientes: make(map[chan []byte]struct{}), log: log}
}

const bufferDoCliente = 16

// Transmitir entrega a mensagem a todos; quem nao tiver espaco no buffer e
// removido (o proprio cliente reconecta e recarrega).
func (h *Hub) Transmitir(mensagem []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clientes {
		select {
		case c <- mensagem:
		default:
			delete(h.clientes, c)
			close(c)
		}
	}
}

// Clientes devolve quantos estao conectados (para teste e diagnostico).
func (h *Hub) Clientes() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clientes)
}

func (h *Hub) registrar() chan []byte {
	c := make(chan []byte, bufferDoCliente)
	h.mu.Lock()
	h.clientes[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *Hub) remover(c chan []byte) {
	h.mu.Lock()
	if _, ainda := h.clientes[c]; ainda {
		delete(h.clientes, c)
		close(c)
	}
	h.mu.Unlock()
}

// ServeHTTP faz o upgrade e mantem a conexao: uma goroutine escreve o que o
// hub mandar; a leitura existe so para detectar fechamento (o cliente nao
// envia dados). OriginPatterns vazio = so mesma origem, que e o caso.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return // o Accept ja respondeu com o status adequado
	}
	defer conn.CloseNow()

	ctx, cancelar := context.WithCancel(r.Context())
	defer cancelar()

	envio := h.registrar()
	defer h.remover(envio)

	go func() {
		// Le e descarta ate a conexao fechar; entao cancela o ctx.
		defer cancelar()
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case mensagem, aberto := <-envio:
			if !aberto {
				_ = conn.Close(websocket.StatusPolicyViolation, "cliente lento")
				return
			}
			ctxEscrita, cancelarEscrita := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Write(ctxEscrita, websocket.MessageText, mensagem)
			cancelarEscrita()
			if err != nil {
				return
			}
		}
	}
}
