package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gracianFelipe/caixa/internal/aplicacao"
	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/categorizacao"
)

var _ aplicacao.RepositorioDeRegras = (*Regras)(nil)

type Regras struct {
	pool *pgxpool.Pool
}

func NovoRepositorioDeRegras(pool *pgxpool.Pool) *Regras {
	return &Regras{pool: pool}
}

const sqlRegrasAtivas = `
SELECT id, categoria_id, tipo, padrao, prioridade
FROM regras_categorizacao
WHERE ativa
ORDER BY id`

// Ativas devolve as regras validas. Linha que nao passa no construtor do
// dominio (ex.: regex aprendida corrompida) e pulada, nao derruba tudo:
// o efeito de uma regra ruim deve ser "um lancamento fica pendente",
// nunca "o sistema para de registrar".
func (r *Regras) Ativas(ctx context.Context) ([]categorizacao.Regra, error) {
	rows, err := r.pool.Query(ctx, sqlRegrasAtivas)
	if err != nil {
		return nil, fmt.Errorf("consultando regras: %w", err)
	}
	defer rows.Close()

	linhas, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (linhaDeRegra, error) {
		var l linhaDeRegra
		err := row.Scan(&l.id, &l.categoriaID, &l.tipo, &l.padrao, &l.prioridade)
		return l, err
	})
	if err != nil {
		return nil, fmt.Errorf("lendo regras: %w", err)
	}

	regras := make([]categorizacao.Regra, 0, len(linhas))
	for _, l := range linhas {
		regra, err := categorizacao.NovaRegra(l.id, categoria.ID(l.categoriaID), categorizacao.Tipo(l.tipo), l.padrao, l.prioridade)
		if err != nil {
			continue
		}
		regras = append(regras, regra)
	}
	return regras, nil
}

type linhaDeRegra struct {
	id          int64
	categoriaID int16
	tipo        string
	padrao      string
	prioridade  int16
}
