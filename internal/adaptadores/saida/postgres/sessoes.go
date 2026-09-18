package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gracianFelipe/caixa/internal/aplicacao"
)

var _ aplicacao.RepositorioDeSessoes = (*Sessoes)(nil)

type Sessoes struct {
	pool *pgxpool.Pool
}

func NovoRepositorioDeSessoes(pool *pgxpool.Pool) *Sessoes {
	return &Sessoes{pool: pool}
}

func (r *Sessoes) Criar(ctx context.Context, idHash string, expiraEm time.Time) error {
	if _, err := r.pool.Exec(ctx,
		"INSERT INTO sessoes (id, expira_em) VALUES ($1, $2)", idHash, expiraEm,
	); err != nil {
		return fmt.Errorf("criando sessao: %w", err)
	}
	return nil
}

// Renovar valida e desliza numa unica instrucao: o UPDATE so afeta linha
// viva, e RowsAffected diz se havia uma. Sem SELECT + UPDATE, sem corrida.
func (r *Sessoes) Renovar(ctx context.Context, idHash string, agora, novaExpiracao time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE sessoes SET ultimo_uso = $2, expira_em = $3
		 WHERE id = $1 AND expira_em > $2`,
		idHash, agora, novaExpiracao,
	)
	if err != nil {
		return false, fmt.Errorf("renovando sessao: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *Sessoes) Apagar(ctx context.Context, idHash string) error {
	if _, err := r.pool.Exec(ctx, "DELETE FROM sessoes WHERE id = $1", idHash); err != nil {
		return fmt.Errorf("apagando sessao: %w", err)
	}
	return nil
}
