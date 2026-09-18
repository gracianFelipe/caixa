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

const colunasDePergunta = "id, lancamento_id, chat_id, mensagem_id, tipo, referencia, estado, criada_em"

func (r *Perguntas) Criar(ctx context.Context, p pergunta.Pergunta) error {
	var mensagemID *int64
	if p.MensagemID != 0 {
		mensagemID = &p.MensagemID
	}
	var referencia *identidade.ID
	if !p.Referencia.EhZero() {
		referencia = &p.Referencia
	}
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO perguntas (`+colunasDePergunta+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		p.ID, p.LancamentoID, p.ChatID, mensagemID, string(p.Tipo), referencia, string(p.Estado), p.CriadaEm,
	); err != nil {
		return fmt.Errorf("inserindo pergunta: %w", err)
	}
	return nil
}

func (r *Perguntas) PorID(ctx context.Context, id identidade.ID) (pergunta.Pergunta, bool, error) {
	return r.uma(ctx, `SELECT `+colunasDePergunta+` FROM perguntas WHERE id = $1`, id)
}

func (r *Perguntas) AbertaDoLancamento(ctx context.Context, lancamentoID identidade.ID) (pergunta.Pergunta, bool, error) {
	return r.uma(ctx, `SELECT `+colunasDePergunta+` FROM perguntas WHERE lancamento_id = $1 AND estado = 'aberta'`, lancamentoID)
}

func (r *Perguntas) uma(ctx context.Context, sql string, arg any) (pergunta.Pergunta, bool, error) {
	var (
		p          pergunta.Pergunta
		mensagemID *int64
		tipo       string
		referencia *identidade.ID
		estado     string
	)
	err := r.pool.QueryRow(ctx, sql, arg).Scan(
		&p.ID, &p.LancamentoID, &p.ChatID, &mensagemID, &tipo, &referencia, &estado, &p.CriadaEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return pergunta.Pergunta{}, false, nil
	}
	if err != nil {
		return pergunta.Pergunta{}, false, fmt.Errorf("consultando pergunta: %w", err)
	}

	p.Tipo = pergunta.Tipo(tipo)
	p.Estado = pergunta.Estado(estado)
	if mensagemID != nil {
		p.MensagemID = *mensagemID
	}
	if referencia != nil {
		p.Referencia = *referencia
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
