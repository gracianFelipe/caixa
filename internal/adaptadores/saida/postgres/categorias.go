package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gracianFelipe/caixa/internal/aplicacao"
	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
)

var _ aplicacao.RepositorioDeCategorias = (*Categorias)(nil)

type Categorias struct {
	pool *pgxpool.Pool
}

func NovoRepositorioDeCategorias(pool *pgxpool.Pool) *Categorias {
	return &Categorias{pool: pool}
}

const sqlCategorias = `SELECT id, nome FROM categorias ORDER BY id`

func (r *Categorias) Listar(ctx context.Context) ([]categoria.Categoria, error) {
	rows, err := r.pool.Query(ctx, sqlCategorias)
	if err != nil {
		return nil, fmt.Errorf("consultando categorias: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (categoria.Categoria, error) {
		var (
			id   int16
			nome string
		)
		if err := row.Scan(&id, &nome); err != nil {
			return categoria.Categoria{}, fmt.Errorf("lendo categoria: %w", err)
		}
		return categoria.Nova(categoria.ID(id), nome)
	})
}
