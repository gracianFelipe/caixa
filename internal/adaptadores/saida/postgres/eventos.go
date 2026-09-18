package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gracianFelipe/caixa/internal/aplicacao"
	"github.com/gracianFelipe/caixa/internal/dominio/evento"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
)

var _ aplicacao.RepositorioDeEventos = (*Eventos)(nil)

type Eventos struct {
	pool *pgxpool.Pool
}

func NovoRepositorioDeEventos(pool *pgxpool.Pool) *Eventos {
	return &Eventos{pool: pool}
}

// inserirEvento roda dentro da transacao de quem criou o fato — e a metade
// gravadora do outbox, usada por Salvar e CriarComLancamento.
func inserirEvento(ctx context.Context, tx pgx.Tx, e evento.Evento) error {
	payload, err := json.Marshal(map[string]string{"lancamento_id": e.LancamentoID.String()})
	if err != nil {
		return fmt.Errorf("serializando evento: %w", err)
	}
	if _, err := tx.Exec(ctx,
		"INSERT INTO eventos (id, tipo, payload, criado_em) VALUES ($1, $2, $3, $4)",
		e.ID, string(e.Tipo), payload, e.CriadoEm,
	); err != nil {
		return fmt.Errorf("inserindo evento: %w", err)
	}
	return nil
}

const sqlEventosPendentes = `
SELECT id, tipo, payload, criado_em
FROM eventos
WHERE processado_em IS NULL
ORDER BY criado_em, id
FOR UPDATE SKIP LOCKED
LIMIT $1`

// ConsumirPendentes tranca ate `limite` eventos com SKIP LOCKED: dois workers
// simultaneos pegam lotes disjuntos em vez de disputar linha a linha. Tudo em
// uma transacao — se `processar` falhar, nada e marcado e os eventos voltam
// a fila no rollback.
func (r *Eventos) ConsumirPendentes(ctx context.Context, limite int, processar func(evento.Evento) error) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("abrindo transacao do outbox: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := tx.Query(ctx, sqlEventosPendentes, limite)
	if err != nil {
		return 0, fmt.Errorf("trancando eventos pendentes: %w", err)
	}
	eventos, err := pgx.CollectRows(rows, lerEvento)
	if err != nil {
		return 0, fmt.Errorf("lendo eventos: %w", err)
	}

	for i, e := range eventos {
		if err := processar(e); err != nil {
			return i, err // rollback via defer: nada deste lote fica marcado
		}
	}

	for _, e := range eventos {
		if _, err := tx.Exec(ctx,
			"UPDATE eventos SET processado_em = now() WHERE id = $1", e.ID,
		); err != nil {
			return 0, fmt.Errorf("marcando evento processado: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("confirmando lote do outbox: %w", err)
	}
	return len(eventos), nil
}

func lerEvento(row pgx.CollectableRow) (evento.Evento, error) {
	var (
		id       identidade.ID
		tipo     string
		payload  []byte
		criadoEm time.Time
	)
	if err := row.Scan(&id, &tipo, &payload, &criadoEm); err != nil {
		return evento.Evento{}, fmt.Errorf("lendo evento: %w", err)
	}

	var corpo struct {
		LancamentoID string `json:"lancamento_id"`
	}
	if err := json.Unmarshal(payload, &corpo); err != nil {
		return evento.Evento{}, fmt.Errorf("payload do evento %s: %w", id, err)
	}
	lancamentoID, err := identidade.Analisar(corpo.LancamentoID)
	if err != nil {
		return evento.Evento{}, fmt.Errorf("payload do evento %s: %w", id, err)
	}

	return evento.Novo(id, evento.Tipo(tipo), lancamentoID, criadoEm)
}
