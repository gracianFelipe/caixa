---
name: repositorio-pgx
description: Como este repo implementa uma porta de persistência em internal/adaptadores/saida/postgres com pgx e SQL à mão — asserção de interface em compilação, SQL em const parametrizado, leitura por CollectRows, teste de integração com build tag. Use ao criar ou estender um repositório.
---

# Repositório pgx (padrão do Caixa)

Destilado de `internal/adaptadores/saida/postgres/lancamentos.go` e do seu
teste de integração, escritos à mão na spec 001.

## Forma

1. **A interface já existe em `internal/aplicacao/portas.go`.** O repositório
   a satisfaz; não a declara. Primeira linha útil do arquivo:
   `var _ aplicacao.PortaX = (*Repositorio)(nil)` — quebra o build se a
   assinatura divergir.
2. **SQL em `const`**, uma por operação, com placeholders `$1..$n`. Nunca
   `fmt.Sprintf` ou concatenação com valor. Colunas listadas explicitamente,
   nunca `SELECT *`.
3. **Escrita:** `pool.Exec(ctx, sql, args...)`. Converter para tipos base ao
   passar (`int64(l.Valor)`, `string(l.Meio)`); o pgx não conhece o domínio.
4. **Leitura:** `pool.Query` + `defer rows.Close()` + `pgx.CollectRows(rows, lerX)`.
   `lerX(row pgx.CollectableRow) (T, error)` lê em variáveis de tipo base e
   monta o struct **direto**, sem passar por `Novo` — o banco já garantiu as
   invariantes por `CHECK`, e revalidar na leitura faria dado antigo válido
   virar erro quando uma regra mudasse.
5. **Erros embrulhados** com `fmt.Errorf("verbo objeto: %w", err)`. O texto
   pode carregar `id`; nunca valor ou contraparte.
6. `DATE` volta como meia-noite UTC: `competencia.Nova(t.Year(), t.Month())`.
7. `Conectar(ctx, url)` faz `pgxpool.New` + `Ping`; fecha o pool se o ping
   falhar. A URL carrega a senha e não passa por log.

## Teste de integração (`*_integracao_test.go`)

* Primeira linha: `//go:build integracao`. Sem a tag o arquivo não compila e
  `go test ./...` fica rápido e sem banco.
* Lê `CAIXA_BD_URL`; `t.Skip` se vazia.
* Cria dados em **ano remoto** (1999) para não colidir com dados reais;
  registra `t.Cleanup` que apaga por `id`.
* Prova: ida e volta com `cmp.Diff`, ordenação, filtro, constraint de
  duplicata, consulta vazia devolve slice vazia.

## Checagem antes de fechar

```powershell
go vet -tags=integracao ./...
. .\local.ps1; go test -tags=integracao -count=1 ./internal/adaptadores/saida/postgres/
```
