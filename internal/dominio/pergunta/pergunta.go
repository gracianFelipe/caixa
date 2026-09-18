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

// Tipo distingue o que se pergunta: categoria de um lancamento ou se um
// provisorio e o mesmo gasto de outro (conciliacao).
type Tipo string

const (
	TipoCategoria   Tipo = "categoria"
	TipoConciliacao Tipo = "conciliacao"
)

var (
	ErrIDVazio       = errors.New("pergunta: id vazio")
	ErrSemLancamento = errors.New("pergunta: lancamento vazio")
	ErrChatInvalido  = errors.New("pergunta: chat invalido")
)

var ErrSemReferencia = errors.New("pergunta: conciliacao exige o lancamento de referencia")

type Pergunta struct {
	ID           identidade.ID
	LancamentoID identidade.ID
	ChatID       int64
	MensagemID   int64 // preenchido depois que o Telegram confirma o envio
	Tipo         Tipo
	Referencia   identidade.ID // conciliacao: o candidato do outro lado
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
		Tipo:         TipoCategoria,
		Estado:       Aberta,
		CriadaEm:     criadaEm,
	}, nil
}

// NovaDeConciliacao pergunta se lancamentoID (provisorio) e o mesmo gasto de
// referencia (o candidato que pontuou na faixa 60-84).
func NovaDeConciliacao(id, lancamentoID, referencia identidade.ID, chatID int64, criadaEm time.Time) (Pergunta, error) {
	p, err := Nova(id, lancamentoID, chatID, criadaEm)
	if err != nil {
		return Pergunta{}, err
	}
	if referencia.EhZero() {
		return Pergunta{}, ErrSemReferencia
	}
	p.Tipo = TipoConciliacao
	p.Referencia = referencia
	return p, nil
}
