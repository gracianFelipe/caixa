package postgres

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// chaveDoLockDeMigracao identifica "o migrador do Caixa" para o Postgres.
// Advisory lock e um mutex do servidor sobre um numero arbitrario: dois
// processos que pedem a mesma chave se enfileiram.
const chaveDoLockDeMigracao int64 = 20260917

var (
	ErrNomeDeMigracao = errors.New("migracoes: nome fora do padrao NNN_nome.sql")
	ErrVersaoRepetida = errors.New("migracoes: versao repetida")
)

const sqlTabelaDeControle = `
CREATE TABLE IF NOT EXISTS migracoes_aplicadas (
    versao      INTEGER     PRIMARY KEY,
    nome        TEXT        NOT NULL,
    aplicada_em TIMESTAMPTZ NOT NULL DEFAULT now()
)`

// Aplicada identifica uma migracao que este processo acabou de aplicar.
type Aplicada struct {
	Versao int
	Nome   string
}

type migracao struct {
	versao int
	nome   string
	sql    string
}

// Aplicar roda, em ordem, toda migracao de arquivos que ainda nao consta em
// migracoes_aplicadas. Cada arquivo roda na propria transacao: falha no meio
// de um nao deixa rastro dele, e o que ja foi commitado antes fica. Nesse caso
// o retorno traz o que foi aplicado junto com o erro, para o chamador logar.
//
// Recebe fs.FS, nao embed.FS: em producao vem o embed, no teste vem um
// fstest.MapFS montado na hora.
func Aplicar(ctx context.Context, pool *pgxpool.Pool, arquivos fs.FS) ([]Aplicada, error) {
	migracoes, err := lerMigracoes(arquivos)
	if err != nil {
		return nil, err
	}

	// Uma conexao dedicada: o advisory lock e por sessao, e o pool poderia
	// executar cada comando numa conexao diferente.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("obtendo conexao: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", chaveDoLockDeMigracao); err != nil {
		return nil, fmt.Errorf("obtendo lock de migracao: %w", err)
	}
	// WithoutCancel: o unlock precisa rodar mesmo que o ctx tenha sido cancelado,
	// senao a conexao volta ao pool ainda segurando o lock.
	defer conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", chaveDoLockDeMigracao) //nolint:errcheck

	if _, err := conn.Exec(ctx, sqlTabelaDeControle); err != nil {
		return nil, fmt.Errorf("criando tabela de controle: %w", err)
	}

	rows, err := conn.Query(ctx, "SELECT versao FROM migracoes_aplicadas")
	if err != nil {
		return nil, fmt.Errorf("lendo migracoes aplicadas: %w", err)
	}
	versoes, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return nil, fmt.Errorf("lendo migracoes aplicadas: %w", err)
	}
	jaAplicadas := make(map[int]bool, len(versoes))
	for _, v := range versoes {
		jaAplicadas[v] = true
	}

	var novas []Aplicada
	for _, m := range migracoes {
		if jaAplicadas[m.versao] {
			continue
		}
		if err := aplicarUma(ctx, conn, m); err != nil {
			return novas, fmt.Errorf("migracao %s: %w", m.nome, err)
		}
		novas = append(novas, Aplicada{Versao: m.versao, Nome: m.nome})
	}
	return novas, nil
}

func aplicarUma(ctx context.Context, conn *pgxpool.Conn, m migracao) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abrindo transacao: %w", err)
	}
	// Rollback depois de Commit e no-op (devolve ErrTxClosed, ignorado):
	// o defer cobre todos os caminhos de erro sem repetir Rollback em cada um.
	defer tx.Rollback(ctx) //nolint:errcheck

	// Sem parametros de proposito: assim o pgx usa o protocolo simples, que
	// aceita varios comandos numa string. Com $1 o Postgres aceitaria um so.
	// O SQL e codigo do repositorio, nao input — nao ha o que parametrizar.
	if _, err := tx.Exec(ctx, m.sql); err != nil {
		return fmt.Errorf("executando: %w", err)
	}
	if _, err := tx.Exec(ctx,
		"INSERT INTO migracoes_aplicadas (versao, nome) VALUES ($1, $2)",
		m.versao, m.nome,
	); err != nil {
		return fmt.Errorf("registrando: %w", err)
	}
	return tx.Commit(ctx)
}

// lerMigracoes lista os *.sql da raiz do fs, valida os nomes e devolve em
// ordem de versao. Arquivos que nao sao .sql (README, etc.) sao ignorados.
func lerMigracoes(arquivos fs.FS) ([]migracao, error) {
	entradas, err := fs.ReadDir(arquivos, ".")
	if err != nil {
		return nil, fmt.Errorf("listando migracoes: %w", err)
	}

	vistas := make(map[int]string)
	var migracoes []migracao
	for _, e := range entradas {
		nome := e.Name()
		if e.IsDir() || !strings.HasSuffix(nome, ".sql") {
			continue
		}

		versao, err := versaoDoNome(nome)
		if err != nil {
			return nil, err
		}
		if outro, repetida := vistas[versao]; repetida {
			return nil, fmt.Errorf("%w: %s e %s", ErrVersaoRepetida, outro, nome)
		}
		vistas[versao] = nome

		conteudo, err := fs.ReadFile(arquivos, nome)
		if err != nil {
			return nil, fmt.Errorf("lendo %s: %w", nome, err)
		}
		migracoes = append(migracoes, migracao{versao: versao, nome: nome, sql: string(conteudo)})
	}

	// Ordem numerica, nao lexica: "010" depois de "002" mesmo que o fs liste
	// em outra ordem.
	slices.SortFunc(migracoes, func(a, b migracao) int {
		return cmp.Compare(a.versao, b.versao)
	})
	return migracoes, nil
}

// versaoDoNome exige exatamente "NNN_alguma-coisa.sql" com NNN em 001..999.
// Rigido de proposito: nome fora do padrao e quase sempre erro de digitacao,
// e uma migracao fora de ordem e pior que uma que nao roda.
func versaoDoNome(nome string) (int, error) {
	prefixo, resto, temSeparador := strings.Cut(strings.TrimSuffix(nome, ".sql"), "_")
	if !temSeparador || resto == "" || len(prefixo) != 3 {
		return 0, fmt.Errorf("%w: %s", ErrNomeDeMigracao, nome)
	}
	for i := 0; i < len(prefixo); i++ {
		if prefixo[i] < '0' || prefixo[i] > '9' {
			return 0, fmt.Errorf("%w: %s", ErrNomeDeMigracao, nome)
		}
	}
	versao, _ := strconv.Atoi(prefixo) // so digitos: Atoi nao falha
	if versao == 0 {
		return 0, fmt.Errorf("%w: %s", ErrNomeDeMigracao, nome)
	}
	return versao, nil
}
