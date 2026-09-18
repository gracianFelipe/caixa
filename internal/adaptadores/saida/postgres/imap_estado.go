package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EstadoIMAP persiste a posicao de leitura da caixa: restart do worker nao
// rele nem pula. Linha unica (CHECK id = 1 na migracao 008).
type EstadoIMAP struct {
	pool *pgxpool.Pool
}

func NovoEstadoIMAP(pool *pgxpool.Pool) *EstadoIMAP {
	return &EstadoIMAP{pool: pool}
}

// Carregar devolve (uidvalidity, ultimaUID, existe).
func (r *EstadoIMAP) Carregar(ctx context.Context) (uint32, uint32, bool, error) {
	var uidvalidity, ultimaUID int64
	err := r.pool.QueryRow(ctx,
		"SELECT uidvalidity, ultima_uid FROM imap_estado WHERE id = 1",
	).Scan(&uidvalidity, &ultimaUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, fmt.Errorf("carregando estado imap: %w", err)
	}
	return uint32(uidvalidity), uint32(ultimaUID), true, nil
}

func (r *EstadoIMAP) Salvar(ctx context.Context, uidvalidity, ultimaUID uint32) error {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO imap_estado (id, uidvalidity, ultima_uid, atualizado_em)
		 VALUES (1, $1, $2, now())
		 ON CONFLICT (id) DO UPDATE
		 SET uidvalidity = EXCLUDED.uidvalidity, ultima_uid = EXCLUDED.ultima_uid, atualizado_em = now()`,
		int64(uidvalidity), int64(ultimaUID),
	); err != nil {
		return fmt.Errorf("salvando estado imap: %w", err)
	}
	return nil
}
