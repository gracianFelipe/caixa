package aplicacao

import (
	"context"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

// As interfaces moram aqui, no consumidor, e nao em quem implementa.
// O adaptador postgres satisfaz RepositorioDeLancamentos sem importar este
// pacote para "declarar" isso: em Go a satisfacao e implicita e estrutural.
// Cada interface tem so o que este pacote usa; quando um caso de uso novo
// precisar de mais, o metodo entra aqui, e nao no adaptador.

// RepositorioDeLancamentos persiste e consulta o agregado.
type RepositorioDeLancamentos interface {
	Salvar(ctx context.Context, l lancamento.Lancamento) error
	DaCompetencia(ctx context.Context, c competencia.Competencia) ([]lancamento.Lancamento, error)
}

// Relogio abstrai time.Now para que o caso de uso seja testavel com tempo fixo.
type Relogio interface {
	Agora() time.Time
}
