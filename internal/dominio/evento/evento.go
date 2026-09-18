// Package evento e o vocabulario do outbox: fatos que ja aconteceram e
// precisam ser comunicados a outro processo. O evento e gravado na mesma
// transacao da mudanca que o gerou; publicar e problema do worker.
package evento

import (
	"errors"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
)

// Tipo e a lista fechada de eventos que existem no sistema.
type Tipo string

const (
	LancamentoCriado Tipo = "lancamento_criado"
)

var (
	ErrIDVazio      = errors.New("evento: id vazio")
	ErrTipoInvalido = errors.New("evento: tipo desconhecido")
	ErrSemAssunto   = errors.New("evento: lancamento de origem vazio")
)

// Evento carrega o fato minimo: tipo e o id do agregado envolvido. O payload
// nao repete os dados do lancamento — quem consome busca a versao atual no
// banco, nunca uma copia possivelmente velha.
type Evento struct {
	ID           identidade.ID
	Tipo         Tipo
	LancamentoID identidade.ID
	CriadoEm     time.Time
}

func Novo(id identidade.ID, tipo Tipo, lancamentoID identidade.ID, criadoEm time.Time) (Evento, error) {
	if id.EhZero() {
		return Evento{}, ErrIDVazio
	}
	if tipo != LancamentoCriado {
		return Evento{}, ErrTipoInvalido
	}
	if lancamentoID.EhZero() {
		return Evento{}, ErrSemAssunto
	}
	return Evento{ID: id, Tipo: tipo, LancamentoID: lancamentoID, CriadoEm: criadoEm}, nil
}
