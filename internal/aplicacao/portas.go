package aplicacao

import (
	"context"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/categorizacao"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
	"github.com/gracianFelipe/caixa/internal/dominio/ocorrencia"
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

// RepositorioDeOcorrencias grava a evidencia e o fato na mesma transacao.
// Devolve false quando a ocorrencia ja existia (impressao ou id externo
// repetidos) — a idempotencia mora na constraint, nao em logica de consulta.
type RepositorioDeOcorrencias interface {
	CriarComLancamento(ctx context.Context, o ocorrencia.Ocorrencia, l lancamento.Lancamento) (criada bool, err error)
}

// RepositorioDeRegras entrega as regras ativas de categorizacao. A ordenacao
// e problema do dominio (Classificar), nao da consulta.
type RepositorioDeRegras interface {
	Ativas(ctx context.Context) ([]categorizacao.Regra, error)
}

// RepositorioDeCategorias lista o vocabulario fixo de categorias.
type RepositorioDeCategorias interface {
	Listar(ctx context.Context) ([]categoria.Categoria, error)
}

// Relogio abstrai time.Now para que o caso de uso seja testavel com tempo fixo.
type Relogio interface {
	Agora() time.Time
}
