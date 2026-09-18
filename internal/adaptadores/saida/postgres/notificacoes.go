package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gracianFelipe/caixa/internal/aplicacao"
)

var _ aplicacao.Notificacoes = (*Notificacoes)(nil)

// Notificacoes e o lado LISTEN do LISTEN/NOTIFY. Uma conexao dedicada fica
// presa esperando avisos; se cair, a espera reabre com recuo exponencial.
type Notificacoes struct {
	pool *pgxpool.Pool
}

func NovoOuvinteDeNotificacoes(pool *pgxpool.Pool) *Notificacoes {
	return &Notificacoes{pool: pool}
}

// Escutar bloqueia ate o contexto ser cancelado. Aviso perdido durante uma
// reconexao nao e recuperado — e sinal de "recarregue", nao dado: o cliente
// que perdeu um aviso so demora um pouco mais para ver a novidade.
func (n *Notificacoes) Escutar(ctx context.Context, canal string, receber func(carga string)) error {
	recuo := time.Second
	for {
		err := n.escutarUmaVez(ctx, canal, receber)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_ = err // a falha de uma sessao de escuta nao e fatal; tenta de novo

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(recuo):
		}
		if recuo < 30*time.Second {
			recuo *= 2
		}
	}
}

func (n *Notificacoes) escutarUmaVez(ctx context.Context, canal string, receber func(string)) error {
	conn, err := n.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("obtendo conexao para LISTEN: %w", err)
	}
	// Uma conexao que esteve em LISTEN nao volta limpa ao pool: descarta-se
	// ao sair, e o pool abre outra quando precisar. O Hijack fica DENTRO da
	// funcao adiada: `defer conn.Hijack().Close(...)` avaliaria Hijack() ja
	// no defer e sequestraria a conexao antes do LISTEN (panic garantido).
	defer func() {
		_ = conn.Hijack().Close(context.WithoutCancel(ctx))
	}()

	// Nome de canal nao aceita $1; e constante do codigo, entre aspas duplas.
	if _, err := conn.Exec(ctx, `LISTEN "`+canal+`"`); err != nil {
		return fmt.Errorf("LISTEN: %w", err)
	}

	for {
		aviso, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			return fmt.Errorf("esperando aviso: %w", err)
		}
		receber(aviso.Payload)
	}
}
