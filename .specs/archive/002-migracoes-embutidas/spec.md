# 002 — Migrações embutidas e `caixactl migrar`

Status: concluída em 2026-09-17
Fase do plano: 2 (primeira de quatro specs: 002 migrações · 003 OFX +
importar · 004 categorias · 005 Dockerfile)

## Objetivo

Trocar o `psql -f` manual por um migrador dentro do binário: `caixactl migrar`
descobre o que já foi aplicado, aplica o que falta, na ordem, cada arquivo
numa transação. Sem isso, toda spec que mexer no banco (003, 004...) depende
de alguém lembrar de rodar SQL à mão em cada ambiente.

## Decisão: migrador próprio, não goose

O plano previa goose. Escolha registrada aqui: **~80 linhas próprias** sobre
`embed.FS` + tabela de controle + transação por arquivo. Razões: zero
dependência nova; cada linha explicável; combina com "sem ORM, o diferencial
é banco"; "down" não seria usado porque o projeto proíbe editar migração
aplicada (correção é migração nova). Custo aceito: menos testado em batalha
que o goose. Se um dia precisar de "down" ou de migração em Go, revisita-se.

## Fora de escopo

OFX, importação, categorias, Dockerfile, `caixactl` além de `migrar`.
Migrações 003+ não nascem aqui — nascem na spec que precisar delas.

## Entregas

| # | Pacote / arquivo | Conteúdo |
|---|---|---|
| 1 | `migracoes/migracoes.go` | Pacote `migracoes` com `//go:embed *.sql` expondo `var Arquivos embed.FS`. Só stdlib. Os `.sql` existentes ficam como estão. |
| 2 | `internal/adaptadores/saida/postgres/migrador.go` | `Aplicar(ctx, pool, fs.FS) ([]Aplicada, error)`. Recebe `fs.FS`, não `embed.FS`: testável com `fstest.MapFS` sem banco. |
| 3 | `internal/adaptadores/saida/postgres/migrador_test.go` | Unitário: nomes `NNN_nome.sql` válidos, ordem numérica (010 depois de 002), versão duplicada e nome fora do padrão são erro, arquivo não-`.sql` ignorado. |
| 4 | `internal/adaptadores/saida/postgres/migrador_integracao_test.go` | `//go:build integracao`: banco já migrado → `Aplicar` devolve vazio e a tabela de controle bate com os arquivos embutidos; migração que falha no meio não deixa rastro (rollback). |
| 5 | `cmd/caixactl/main.go` | Subcomando `migrar`. Lê `CAIXA_BD_URL`, conecta, aplica, loga cada versão aplicada com `slog`. Exit 1 em erro. |
| 6 | `.github/workflows/ci.yml` | Passo "migracoes" passa a ser `go run ./cmd/caixactl migrar` — o migrador é exercitado em toda CI. |
| 7 | `README.md`, `AGENTS.md` | Rodar localmente via `caixactl migrar`; índice DOX com `cmd/caixactl` e `migracoes`. |

## Desenho

**Tabela de controle**, criada pelo próprio migrador se não existir:

```sql
CREATE TABLE IF NOT EXISTS migracoes_aplicadas (
    versao      INTEGER     PRIMARY KEY,
    nome        TEXT        NOT NULL,
    aplicada_em TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**Algoritmo:**

1. Pegar **uma** conexão do pool (`pool.Acquire`) e segurar
   `pg_advisory_lock(<chave fixa>)` até o fim: dois `caixactl migrar` ao mesmo
   tempo (dois deploys, CI + humano) não aplicam a mesma migração duas vezes.
2. Ler `fs.FS`: só `*.sql`; nome obrigatoriamente `NNN_nome.sql`; versão =
   inteiro antes do `_`; ordenar por versão; versão repetida é erro.
3. `SELECT versao FROM migracoes_aplicadas` → conjunto do que já rodou.
4. Para cada pendente, em ordem: `BEGIN` → `Exec(sql)` **sem parâmetros**
   (protocolo simples do Postgres aceita vários comandos numa string; com
   parâmetros aceitaria só um) → `INSERT INTO migracoes_aplicadas` → `COMMIT`.
   Erro em qualquer passo → `ROLLBACK` e para; o que já foi commitado fica.
5. Devolver a lista do que foi aplicado nesta execução.

**`caixactl`:** `os.Args[1]` escolhe o subcomando; `flag.NewFlagSet` por
subcomando para crescer depois (`importar` na 003). Nenhuma flag em `migrar`
hoje.

## Contratos aplicáveis

* `migracoes` é pacote de raiz, só stdlib (`embed`). Não é domínio: não entra
  na regra do contrato 1, mas segue o mesmo espírito.
* O migrador mora no adaptador `postgres` porque fala pgx. Recebe `fs.FS`
  (interface da stdlib) — interface no consumidor, de novo.
* Todo SQL do migrador que leva valor (`INSERT` na tabela de controle) usa
  `$1, $2`. O conteúdo das migrações é código do repositório, não input.
* Nomes: `Aplicar`, `Aplicada`, `migracoes_aplicadas` em português; `fs.FS`,
  `embed`, `Tx` técnicos.

## SEC-CHECK

* **Secrets** — `CAIXA_BD_URL` só por ambiente; nunca em log, nem em erro
  (erro de conexão do pgx não inclui a senha, mas o caminho é verificado no
  teste: a mensagem de erro de `Conectar` não contém a URL).
* **PQ** — o único SQL com valor externo é o `INSERT` de controle, parametrizado.
* **Fail secure** — migração que falha aborta com `ROLLBACK`; nunca marca
  como aplicada o que não terminou.
* **Logging** — log carrega versão e nome do arquivo, nada mais.

## Efeito no banco de dev

O banco atual foi migrado à mão e não tem `migracoes_aplicadas`; o migrador
tentaria reaplicar a 001. Como só há dado de teste, recria-se:
`DROP DATABASE caixa; CREATE DATABASE caixa;` e `caixactl migrar`. A partir
daqui, `psql -f` não é mais usado para migrar em lugar nenhum.

## Verificação (definição de pronto)

```powershell
go vet ./... ; go vet -tags=integracao ./...
go test ./...                                       # unitario do migrador com fstest.MapFS
. .\local.ps1
go run ./cmd/caixactl migrar                        # aplica 001 e 002 no banco recriado
go run ./cmd/caixactl migrar                        # segunda vez: "nada a aplicar"
go test -tags=integracao ./...
go run ./cmd/api                                    # continua subindo
```

CI verde com o passo `migracoes` trocado.

## Ao concluir

Mover para `.specs/archive/002-migracoes-embutidas/`. Skill candidata:
`subcomando-caixactl` — só depois da 003 criar o segundo subcomando, quando
houver dois casos para destilar.
