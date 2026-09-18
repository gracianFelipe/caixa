// Package postgres implementa as portas de persistencia com pgx e SQL
// escrito a mao. Todo parametro entra por $n; nenhuma query concatena texto.
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
	"github.com/gracianFelipe/caixa/internal/dominio/conciliacao"
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/evento"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
	"github.com/gracianFelipe/caixa/internal/dominio/ocorrencia"
)

// Prova em tempo de compilacao que *Repositorio satisfaz a porta declarada
// pela aplicacao. Se um metodo mudar de assinatura, o build quebra aqui,
// nao em producao.
var _ aplicacao.RepositorioDeLancamentos = (*Repositorio)(nil)

// Conectar abre o pool e confirma que o banco responde antes de devolver.
// A URL carrega a senha: nunca passa por log.
func Conectar(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("configurando pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("conectando ao postgres: %w", err)
	}
	return pool, nil
}

// Repositorio guarda o pool; o pool ja e seguro para uso concorrente.
type Repositorio struct {
	pool *pgxpool.Pool
}

func NovoRepositorio(pool *pgxpool.Pool) *Repositorio {
	return &Repositorio{pool: pool}
}

const sqlInserir = `
INSERT INTO lancamentos
    (id, ocorrido_em, competencia, valor_centavos, meio, contraparte, contraparte_norm, categoria_id, categoria_origem, situacao)
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

// Salvar grava lancamento e evento do outbox na MESMA transacao: um commit,
// dois fatos. Conversoes explicitas para os tipos base (int64, string): o
// dominio nao precisa saber como o pgx codifica.
func (r *Repositorio) Salvar(ctx context.Context, l lancamento.Lancamento, e evento.Evento) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abrindo transacao: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, sqlInserir, argumentosDeInsercao(l)...); err != nil {
		return fmt.Errorf("inserindo lancamento: %w", err)
	}
	if err := inserirEvento(ctx, tx, e); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const sqlPorID = `
SELECT id, ocorrido_em, competencia, valor_centavos, meio, contraparte, contraparte_norm, categoria_id, categoria_origem, situacao
FROM lancamentos
WHERE id = $1`

func (r *Repositorio) PorID(ctx context.Context, id identidade.ID) (lancamento.Lancamento, bool, error) {
	rows, err := r.pool.Query(ctx, sqlPorID, id)
	if err != nil {
		return lancamento.Lancamento{}, false, fmt.Errorf("consultando lancamento: %w", err)
	}
	l, err := pgx.CollectOneRow(rows, lerLancamento)
	if errors.Is(err, pgx.ErrNoRows) {
		return lancamento.Lancamento{}, false, nil
	}
	if err != nil {
		return lancamento.Lancamento{}, false, err
	}
	return l, true, nil
}

// AtribuirCategoria muda so a categoria; o resto do lancamento e imutavel.
func (r *Repositorio) AtribuirCategoria(ctx context.Context, id identidade.ID, cat categoria.ID, origem lancamento.OrigemDaCategoria) error {
	tag, err := r.pool.Exec(ctx,
		"UPDATE lancamentos SET categoria_id = $1, categoria_origem = $2 WHERE id = $3",
		int16(cat), string(origem), id,
	)
	if err != nil {
		return fmt.Errorf("atribuindo categoria: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("atribuindo categoria: lancamento %s nao existe", id)
	}
	return nil
}

// GastoConfirmado soma so saidas confirmadas: provisorio e descartado ficam
// fora, e entrada (valor positivo) nao e gasto. Devolve magnitude positiva.
func (r *Repositorio) GastoConfirmado(ctx context.Context, cat categoria.ID, comp competencia.Competencia) (dinheiro.Centavos, error) {
	var soma int64
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(-SUM(valor_centavos), 0)
		 FROM lancamentos
		 WHERE categoria_id = $1 AND competencia = $2
		   AND situacao = 'confirmado' AND valor_centavos < 0`,
		int16(cat), comp.PrimeiroDia(),
	).Scan(&soma)
	if err != nil {
		return 0, fmt.Errorf("somando gasto: %w", err)
	}
	return dinheiro.Centavos(soma), nil
}

// FundirProvisorio e o "mesmo gasto": as evidencias migram para o destino
// com resultado conciliou e o provisorio vira descartado — tudo ou nada.
func (r *Repositorio) FundirProvisorio(ctx context.Context, provisorioID, destinoID identidade.ID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abrindo transacao: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx,
		`UPDATE ocorrencias SET lancamento_id = $1, resultado = 'conciliou' WHERE lancamento_id = $2`,
		destinoID, provisorioID,
	); err != nil {
		return fmt.Errorf("migrando evidencias: %w", err)
	}
	tag, err := tx.Exec(ctx,
		`UPDATE lancamentos SET situacao = 'descartado' WHERE id = $1 AND situacao = 'provisorio'`,
		provisorioID,
	)
	if err != nil {
		return fmt.Errorf("descartando provisorio: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("fundindo: lancamento %s nao esta provisorio", provisorioID)
	}
	return tx.Commit(ctx)
}

func (r *Repositorio) ConfirmarProvisorio(ctx context.Context, id identidade.ID) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE lancamentos SET situacao = 'confirmado' WHERE id = $1 AND situacao = 'provisorio'`, id,
	); err != nil {
		return fmt.Errorf("confirmando provisorio: %w", err)
	}
	return nil
}

// argumentosDeInsercao existe porque duas queries inserem lancamento (Salvar
// e a importacao com ocorrencia); a lista de colunas muda junto nos dois.
func argumentosDeInsercao(l lancamento.Lancamento) []any {
	var categoriaID *int16
	if l.CategoriaID > 0 {
		v := int16(l.CategoriaID)
		categoriaID = &v // NULL no banco quando pendente; zero violaria a FK
	}
	return []any{
		l.ID,
		l.OcorridoEm,
		l.Competencia.PrimeiroDia(),
		int64(l.Valor),
		string(l.Meio),
		l.Contraparte,
		l.ContraparteNorm,
		categoriaID,
		string(l.CategoriaOrigem),
		string(l.Situacao),
	}
}

const sqlDaCompetencia = `
SELECT id, ocorrido_em, competencia, valor_centavos, meio, contraparte, contraparte_norm, categoria_id, categoria_origem, situacao
FROM lancamentos
WHERE competencia = $1
ORDER BY ocorrido_em, id`

// DaCompetencia lista os lancamentos de um mes em ordem cronologica.
func (r *Repositorio) DaCompetencia(ctx context.Context, c competencia.Competencia) ([]lancamento.Lancamento, error) {
	rows, err := r.pool.Query(ctx, sqlDaCompetencia, c.PrimeiroDia())
	if err != nil {
		return nil, fmt.Errorf("consultando competencia: %w", err)
	}
	defer rows.Close()

	// CollectRows fecha o cursor e devolve o primeiro erro de rede ou de scan.
	return pgx.CollectRows(rows, lerLancamento)
}

// lerLancamento monta o agregado a partir de uma linha. Le nos tipos base e
// converte: o banco ja garantiu as invariantes por CHECK, entao nao passa
// por lancamento.Novo de novo.
func lerLancamento(row pgx.CollectableRow) (lancamento.Lancamento, error) {
	var (
		id              identidade.ID
		ocorridoEm      time.Time
		primeiroDia     time.Time
		valor           int64
		meio            string
		contraparte     string
		contraparteNorm string
		categoriaID     *int16
		categoriaOrigem string
		situacao        string
	)
	if err := row.Scan(&id, &ocorridoEm, &primeiroDia, &valor, &meio, &contraparte, &contraparteNorm, &categoriaID, &categoriaOrigem, &situacao); err != nil {
		return lancamento.Lancamento{}, fmt.Errorf("lendo lancamento: %w", err)
	}

	// DATE volta como meia-noite UTC; o mes em UTC e o mes da competencia.
	comp, err := competencia.Nova(primeiroDia.Year(), primeiroDia.Month())
	if err != nil {
		return lancamento.Lancamento{}, fmt.Errorf("lendo competencia do lancamento %s: %w", id, err)
	}

	l := lancamento.Lancamento{
		ID:              id,
		OcorridoEm:      ocorridoEm.UTC(),
		Competencia:     comp,
		Valor:           dinheiro.Centavos(valor),
		Meio:            lancamento.Meio(meio),
		Contraparte:     contraparte,
		ContraparteNorm: contraparteNorm,
		CategoriaOrigem: lancamento.OrigemDaCategoria(categoriaOrigem),
		Situacao:        lancamento.Situacao(situacao),
	}
	if categoriaID != nil {
		l.CategoriaID = categoria.ID(*categoriaID)
	}
	return l, nil
}

const sqlCandidatos = `
SELECT l.id, l.ocorrido_em, l.competencia, l.valor_centavos, l.meio, l.contraparte, l.contraparte_norm, l.categoria_id, l.categoria_origem, l.situacao,
       COALESCE(array_agg(oc.origem_id) FILTER (WHERE oc.origem_id IS NOT NULL), '{}') AS origens
FROM lancamentos l
LEFT JOIN ocorrencias oc ON oc.lancamento_id = l.id
WHERE l.valor_centavos = $1
  AND l.ocorrido_em BETWEEN $2::timestamptz - interval '3 days' AND $2::timestamptz + interval '3 days'
  AND l.situacao <> 'descartado'
  AND NOT EXISTS (
      SELECT 1 FROM ocorrencias o2
      WHERE o2.lancamento_id = l.id AND o2.origem_id = $3
  )
GROUP BY l.id
ORDER BY l.ocorrido_em, l.id`

// CandidatosParaConciliacao implementa o filtro do nivel 2: valor identico,
// ate 3 dias, nao descartado e SEM evidencia da origem que esta chegando.
// A janela do SQL corta o grosso; a distancia fina em dias de calendario e
// pontuada no dominio.
func (r *Repositorio) CandidatosParaConciliacao(ctx context.Context, valor dinheiro.Centavos, instante time.Time, origem ocorrencia.Origem) ([]conciliacao.Candidato, error) {
	rows, err := r.pool.Query(ctx, sqlCandidatos, int64(valor), instante, int16(origem))
	if err != nil {
		return nil, fmt.Errorf("consultando candidatos: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, lerCandidato)
}

func lerCandidato(row pgx.CollectableRow) (conciliacao.Candidato, error) {
	var (
		id              identidade.ID
		ocorridoEm      time.Time
		primeiroDia     time.Time
		valor           int64
		meio            string
		contraparte     string
		contraparteNorm string
		categoriaID     *int16
		categoriaOrigem string
		situacao        string
		origens         []int16
	)
	if err := row.Scan(&id, &ocorridoEm, &primeiroDia, &valor, &meio, &contraparte, &contraparteNorm, &categoriaID, &categoriaOrigem, &situacao, &origens); err != nil {
		return conciliacao.Candidato{}, fmt.Errorf("lendo candidato: %w", err)
	}
	comp, err := competencia.Nova(primeiroDia.Year(), primeiroDia.Month())
	if err != nil {
		return conciliacao.Candidato{}, err
	}

	c := conciliacao.Candidato{
		Lancamento: lancamento.Lancamento{
			ID:              id,
			OcorridoEm:      ocorridoEm.UTC(),
			Competencia:     comp,
			Valor:           dinheiro.Centavos(valor),
			Meio:            lancamento.Meio(meio),
			Contraparte:     contraparte,
			ContraparteNorm: contraparteNorm,
			CategoriaOrigem: lancamento.OrigemDaCategoria(categoriaOrigem),
			Situacao:        lancamento.Situacao(situacao),
		},
	}
	if categoriaID != nil {
		c.Lancamento.CategoriaID = categoria.ID(*categoriaID)
	}
	for _, o := range origens {
		c.Origens = append(c.Origens, ocorrencia.Origem(o))
	}
	return c, nil
}
