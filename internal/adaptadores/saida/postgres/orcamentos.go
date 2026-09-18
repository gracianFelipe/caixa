package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gracianFelipe/caixa/internal/aplicacao"
	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/orcamento"
)

var (
	_ aplicacao.RepositorioDeOrcamentos = (*Orcamentos)(nil)
	_ aplicacao.RepositorioDeAlertas    = (*Alertas)(nil)
)

type Orcamentos struct {
	pool *pgxpool.Pool
}

func NovoRepositorioDeOrcamentos(pool *pgxpool.Pool) *Orcamentos {
	return &Orcamentos{pool: pool}
}

// Definir cria ou substitui o limite. Competencia zero grava NULL (padrao);
// a UNIQUE NULLS NOT DISTINCT faz o ON CONFLICT funcionar tambem para ele.
func (r *Orcamentos) Definir(ctx context.Context, o orcamento.Orcamento) error {
	var comp *time.Time
	if !o.Competencia.EhZero() {
		d := o.Competencia.PrimeiroDia()
		comp = &d
	}
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO orcamentos (categoria_id, competencia, limite_centavos)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (categoria_id, competencia) DO UPDATE SET limite_centavos = EXCLUDED.limite_centavos`,
		int16(o.Categoria), comp, int64(o.Limite),
	); err != nil {
		return fmt.Errorf("definindo orcamento: %w", err)
	}
	return nil
}

// LimiteVigente prefere o limite do mes; sem ele, o padrao (competencia NULL).
// ORDER BY competencia NULLS LAST + LIMIT 1 resolve a precedencia no SQL.
func (r *Orcamentos) LimiteVigente(ctx context.Context, cat categoria.ID, comp competencia.Competencia) (dinheiro.Centavos, bool, error) {
	var limite int64
	err := r.pool.QueryRow(ctx,
		`SELECT limite_centavos FROM orcamentos
		 WHERE categoria_id = $1 AND (competencia = $2 OR competencia IS NULL)
		 ORDER BY competencia NULLS LAST
		 LIMIT 1`,
		int16(cat), comp.PrimeiroDia(),
	).Scan(&limite)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("consultando orcamento: %w", err)
	}
	return dinheiro.Centavos(limite), true, nil
}

// Vigentes resolve todas as categorias de uma vez. DISTINCT ON com o mesmo
// ORDER BY de LimiteVigente: uma linha por categoria, a mais especifica.
func (r *Orcamentos) Vigentes(ctx context.Context, comp competencia.Competencia) (map[categoria.ID]dinheiro.Centavos, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT ON (categoria_id) categoria_id, limite_centavos
		 FROM orcamentos
		 WHERE competencia = $1 OR competencia IS NULL
		 ORDER BY categoria_id, competencia NULLS LAST`,
		comp.PrimeiroDia(),
	)
	if err != nil {
		return nil, fmt.Errorf("consultando orcamentos vigentes: %w", err)
	}
	defer rows.Close()

	limites := map[categoria.ID]dinheiro.Centavos{}
	for rows.Next() {
		var (
			cat    int16
			limite int64
		)
		if err := rows.Scan(&cat, &limite); err != nil {
			return nil, fmt.Errorf("lendo orcamento vigente: %w", err)
		}
		limites[categoria.ID(cat)] = dinheiro.Centavos(limite)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lendo orcamentos vigentes: %w", err)
	}
	return limites, nil
}

// VigentesDetalhados e Vigentes com a informacao "e do mes ou e o padrao":
// a tela de orcamentos mostra a diferenca. competencia IS NOT NULL na linha
// vencedora do DISTINCT ON = limite especifico.
func (r *Orcamentos) VigentesDetalhados(ctx context.Context, comp competencia.Competencia) ([]aplicacao.LimiteVigenteDetalhado, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT ON (categoria_id) categoria_id, limite_centavos, competencia IS NOT NULL
		 FROM orcamentos
		 WHERE competencia = $1 OR competencia IS NULL
		 ORDER BY categoria_id, competencia NULLS LAST`,
		comp.PrimeiroDia(),
	)
	if err != nil {
		return nil, fmt.Errorf("consultando orcamentos vigentes: %w", err)
	}
	defer rows.Close()

	var detalhados []aplicacao.LimiteVigenteDetalhado
	for rows.Next() {
		var (
			cat        int16
			limite     int64
			especifico bool
		)
		if err := rows.Scan(&cat, &limite, &especifico); err != nil {
			return nil, fmt.Errorf("lendo orcamento vigente: %w", err)
		}
		detalhados = append(detalhados, aplicacao.LimiteVigenteDetalhado{
			Categoria: categoria.ID(cat), Limite: dinheiro.Centavos(limite), Especifico: especifico,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lendo orcamentos vigentes: %w", err)
	}
	return detalhados, nil
}

var _ aplicacao.ConsultaDeOrcamentos = (*Orcamentos)(nil)

type Alertas struct {
	pool *pgxpool.Pool
}

func NovoRepositorioDeAlertas(pool *pgxpool.Pool) *Alertas {
	return &Alertas{pool: pool}
}

// RegistrarSeNovo e a idempotencia do aviso: a UNIQUE (tipo, chave) decide,
// e RowsAffected == 0 significa "ja avisado".
func (r *Alertas) RegistrarSeNovo(ctx context.Context, tipo, chave string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		"INSERT INTO alertas (tipo, chave) VALUES ($1, $2) ON CONFLICT DO NOTHING",
		tipo, chave,
	)
	if err != nil {
		return false, fmt.Errorf("registrando alerta: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// Remover desfaz um registro cujo envio falhou (compensacao).
func (r *Alertas) Remover(ctx context.Context, tipo, chave string) error {
	if _, err := r.pool.Exec(ctx,
		"DELETE FROM alertas WHERE tipo = $1 AND chave = $2", tipo, chave,
	); err != nil {
		return fmt.Errorf("removendo alerta: %w", err)
	}
	return nil
}
