// Package postgres implementa as portas de persistencia com pgx e SQL
// escrito a mao. Todo parametro entra por $n; nenhuma query concatena texto.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gracianFelipe/caixa/internal/aplicacao"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

// Prova em tempo de compilacao que *Repositorio satisfaz a porta declarada
// pela aplicacao. Se um metodo mudar de assinatura, o build quebra aqui,
// nao em producao.
var _ aplicacao.RepositorioDeLancamentos = (*Repositorio)(nil)

// Conectar abre o pool e confirma que o banco responde antes de devolver.
// A URL carrega a senha: nunca passa por log.
func Conectar(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("configurando pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("conectando ao postgres: %w", err)
	}
	return pool, nil
}

// Repositorio guarda o pool; o pool ja e seguro para uso concorrente.
type Repositorio struct {
	pool *pgxpool.Pool
}

func NovoRepositorio(pool *pgxpool.Pool) *Repositorio {
	return &Repositorio{pool: pool}
}

const sqlInserir = `
INSERT INTO lancamentos
    (id, ocorrido_em, competencia, valor_centavos, meio, contraparte, contraparte_norm)
VALUES
    ($1, $2, $3, $4, $5, $6, $7)`

// Salvar grava um lancamento novo. Conversoes explicitas para os tipos base
// (int64, string): o dominio nao precisa saber como o pgx codifica.
func (r *Repositorio) Salvar(ctx context.Context, l lancamento.Lancamento) error {
	_, err := r.pool.Exec(ctx, sqlInserir,
		l.ID,
		l.OcorridoEm,
		l.Competencia.PrimeiroDia(),
		int64(l.Valor),
		string(l.Meio),
		l.Contraparte,
		l.ContraparteNorm,
	)
	if err != nil {
		return fmt.Errorf("inserindo lancamento: %w", err)
	}
	return nil
}

const sqlDaCompetencia = `
SELECT id, ocorrido_em, competencia, valor_centavos, meio, contraparte, contraparte_norm
FROM lancamentos
WHERE competencia = $1
ORDER BY ocorrido_em, id`

// DaCompetencia lista os lancamentos de um mes em ordem cronologica.
func (r *Repositorio) DaCompetencia(ctx context.Context, c competencia.Competencia) ([]lancamento.Lancamento, error) {
	rows, err := r.pool.Query(ctx, sqlDaCompetencia, c.PrimeiroDia())
	if err != nil {
		return nil, fmt.Errorf("consultando competencia: %w", err)
	}
	defer rows.Close()

	// CollectRows fecha o cursor e devolve o primeiro erro de rede ou de scan.
	return pgx.CollectRows(rows, lerLancamento)
}

// lerLancamento monta o agregado a partir de uma linha. Le nos tipos base e
// converte: o banco ja garantiu as invariantes por CHECK, entao nao passa
// por lancamento.Novo de novo.
func lerLancamento(row pgx.CollectableRow) (lancamento.Lancamento, error) {
	var (
		id              identidade.ID
		ocorridoEm      time.Time
		primeiroDia     time.Time
		valor           int64
		meio            string
		contraparte     string
		contraparteNorm string
	)
	if err := row.Scan(&id, &ocorridoEm, &primeiroDia, &valor, &meio, &contraparte, &contraparteNorm); err != nil {
		return lancamento.Lancamento{}, fmt.Errorf("lendo lancamento: %w", err)
	}

	// DATE volta como meia-noite UTC; o mes em UTC e o mes da competencia.
	comp, err := competencia.Nova(primeiroDia.Year(), primeiroDia.Month())
	if err != nil {
		return lancamento.Lancamento{}, fmt.Errorf("lendo competencia do lancamento %s: %w", id, err)
	}

	return lancamento.Lancamento{
		ID:              id,
		OcorridoEm:      ocorridoEm.UTC(),
		Competencia:     comp,
		Valor:           dinheiro.Centavos(valor),
		Meio:            lancamento.Meio(meio),
		Contraparte:     contraparte,
		ContraparteNorm: contraparteNorm,
	}, nil
}
