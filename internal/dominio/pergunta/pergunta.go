// Package pergunta modela uma pergunta aberta ao dono no Telegram.
// Existe para nao perguntar duas vezes e para editar a mensagem certa
// quando a resposta chegar.
package pergunta

import (
	"errors"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
)

type Estado string

const (
	Aberta     Estado = "aberta"
	Respondida Estado = "respondida"
	Expirada   Estado = "expirada"
)

var (
	ErrIDVazio       = errors.New("pergunta: id vazio")
	ErrSemLancamento = errors.New("pergunta: lancamento vazio")
	ErrChatInvalido  = errors.New("pergunta: chat invalido")
)

type Pergunta struct {
	ID           identidade.ID
	LancamentoID identidade.ID
	ChatID       int64
	MensagemID   int64 // preenchido depois que o Telegram confirma o envio
	Estado       Estado
	CriadaEm     time.Time
}

func Nova(id, lancamentoID identidade.ID, chatID int64, criadaEm time.Time) (Pergunta, error) {
	if id.EhZero() {
		return Pergunta{}, ErrIDVazio
	}
	if lancamentoID.EhZero() {
		return Pergunta{}, ErrSemLancamento
	}
	if chatID == 0 {
		return Pergunta{}, ErrChatInvalido
	}
	return Pergunta{
		ID:           id,
		LancamentoID: lancamentoID,
		ChatID:       chatID,
		Estado:       Aberta,
		CriadaEm:     criadaEm,
	}, nil
}
