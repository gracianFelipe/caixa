package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gracianFelipe/caixa/internal/aplicacao"
	"github.com/gracianFelipe/caixa/internal/dominio/evento"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
	"github.com/gracianFelipe/caixa/internal/dominio/ocorrencia"
)

var _ aplicacao.RepositorioDeOcorrencias = (*Ocorrencias)(nil)

// Ocorrencias persiste a evidencia e o fato que ela criou.
type Ocorrencias struct {
	pool *pgxpool.Pool
}

func NovoRepositorioDeOcorrencias(pool *pgxpool.Pool) *Ocorrencias {
	return &Ocorrencias{pool: pool}
}

// ON CONFLICT DO NOTHING sem alvo: qualquer UNIQUE violada (impressao ou
// id externo) vira "zero linhas inseridas" em vez de erro — a definicao de
// duplicata mora nas constraints da migracao 002, nao aqui.
const sqlInserirOcorrencia = `
INSERT INTO ocorrencias (id, origem_id, id_externo, impressao, payload, resultado)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT DO NOTHING`

const sqlVincularLancamento = `
UPDATE ocorrencias SET lancamento_id = $1, resultado = $2 WHERE id = $3`

// CriarComLancamento grava evidencia, fato e evento na MESMA transacao: ou
// os tres entram, ou nenhum. Devolve false (sem erro) quando a ocorrencia ja
// existia — e nesse caso o evento tambem nao e gravado (nada aconteceu).
func (r *Ocorrencias) CriarComLancamento(ctx context.Context, o ocorrencia.Ocorrencia, l lancamento.Lancamento, e evento.Evento) (bool, error) {
	// payload JSONB guarda o texto cru embrulhado em JSON aqui, na borda:
	// numero nunca e interpretado, entao float nunca acontece.
	payload, err := json.Marshal(map[string]string{"bruto": o.Payload})
	if err != nil {
		return false, fmt.Errorf("serializando payload: %w", err)
	}

	var idExterno *string
	if o.IDExterno != "" {
		idExterno = &o.IDExterno // NULL no banco quando vazio, senao a UNIQUE parcial nao vale
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("abrindo transacao: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tag, err := tx.Exec(ctx, sqlInserirOcorrencia,
		o.ID, int16(o.Origem), idExterno, o.Impressao, payload, string(o.Resultado))
	if err != nil {
		return false, fmt.Errorf("inserindo ocorrencia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil // duplicata: nada a fazer, transacao morre no defer
	}

	if _, err := tx.Exec(ctx, sqlInserir, argumentosDeInsercao(l)...); err != nil {
		return false, fmt.Errorf("inserindo lancamento da ocorrencia: %w", err)
	}

	if _, err := tx.Exec(ctx, sqlVincularLancamento,
		l.ID, string(ocorrencia.ResultadoCriou), o.ID); err != nil {
		return false, fmt.Errorf("vinculando lancamento: %w", err)
	}

	if err := inserirEvento(ctx, tx, e); err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("confirmando importacao: %w", err)
	}
	return true, nil
}
