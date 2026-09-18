package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gracianFelipe/caixa/internal/aplicacao"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/pergunta"
)

var _ aplicacao.RepositorioDePerguntas = (*Perguntas)(nil)

type Perguntas struct {
	pool *pgxpool.Pool
}

func NovoRepositorioDePerguntas(pool *pgxpool.Pool) *Perguntas {
	return &Perguntas{pool: pool}
}

func (r *Perguntas) Criar(ctx context.Context, p pergunta.Pergunta) error {
	var mensagemID *int64
	if p.MensagemID != 0 {
		mensagemID = &p.MensagemID
	}
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO perguntas (id, lancamento_id, chat_id, mensagem_id, estado, criada_em)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		p.ID, p.LancamentoID, p.ChatID, mensagemID, string(p.Estado), p.CriadaEm,
	); err != nil {
		return fmt.Errorf("inserindo pergunta: %w", err)
	}
	return nil
}

func (r *Perguntas) AbertaDoLancamento(ctx context.Context, lancamentoID identidade.ID) (pergunta.Pergunta, bool, error) {
	var (
		p          pergunta.Pergunta
		mensagemID *int64
		estado     string
	)
	err := r.pool.QueryRow(ctx,
		`SELECT id, lancamento_id, chat_id, mensagem_id, estado, criada_em
		 FROM perguntas WHERE lancamento_id = $1 AND estado = 'aberta'`,
		lancamentoID,
	).Scan(&p.ID, &p.LancamentoID, &p.ChatID, &mensagemID, &estado, &p.CriadaEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return pergunta.Pergunta{}, false, nil
	}
	if err != nil {
		return pergunta.Pergunta{}, false, fmt.Errorf("consultando pergunta: %w", err)
	}

	p.Estado = pergunta.Estado(estado)
	if mensagemID != nil {
		p.MensagemID = *mensagemID
	}
	return p, true, nil
}

func (r *Perguntas) MarcarRespondida(ctx context.Context, lancamentoID identidade.ID) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE perguntas SET estado = 'respondida', respondida_em = $1
		 WHERE lancamento_id = $2 AND estado = 'aberta'`,
		time.Now().UTC(), lancamentoID,
	); err != nil {
		return fmt.Errorf("fechando pergunta: %w", err)
	}
	return nil
}
