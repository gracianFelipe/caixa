# Caixa

Serviço de controle financeiro pessoal em Go. Primeiro projeto Go do autor:
cada linha precisa ser explicável em voz alta numa entrevista. Resolve um
problema real (para onde o dinheiro vai) e serve de evidência técnica de Go,
PostgreSQL, Docker, DDD, REST e testes.

O plano completo vive em `pode-planejar-breezy-harp.md`. Este doc carrega os
contratos invioláveis; o plano carrega o roteiro.

## Contratos de código (inegociáveis)

1. **Regra de dependência.** `cmd → adaptadores → aplicacao → dominio`. Ninguém
   importa para dentro do domínio. `internal/dominio/...` importa **apenas a
   stdlib** — sem `pgx`, sem `net/http`, sem `time.Now()` implícito (o tempo
   entra como parâmetro). Verificado na CI pelo critério do próprio toolchain
   (`.Standard`), não por regex de caminho — a stdlib moderna tem caminhos
   com ponto e versão (`crypto/internal/entropy/v1.0.0`):
   `go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./internal/dominio/... | grep -v '^github.com/gracianFelipe/caixa/'`
   (saída vazia = conforme).
2. **Interfaces moram no consumidor**, nunca em quem implementa. Portas de
   saída em `internal/aplicacao/portas.go`; a interface do serviço que um
   adaptador de entrada consome fica no próprio adaptador
   (`internal/adaptadores/entrada/web`). Sem pacote `ports`. O implementador
   prova a satisfação com `var _ Porta = (*Impl)(nil)`.
3. **Domínio em português, técnico em inglês.** `Lancamento`, `Orcamento`,
   `Competencia`; `Repository`, `Handler`, `context.Context`. **Sem acento em
   identificador** (`Orcamento`, não `Orçamento`).
4. **Dinheiro é `int64` em centavos**, coluna `BIGINT`. Nunca `float` — nem no
   domínio, nem no banco, nem no JSON.
5. **Sem ORM, sem framework web, sem testify.** `net/http.ServeMux` (Go 1.22+),
   `pgx` com SQL na mão, `log/slog`, `testing` + `go-cmp`.
6. **Teste junto com o código**, table-driven, não depois.

### Tabela de camadas

| Camada | Pacote | Importa | Não importa |
|---|---|---|---|
| Domínio | `internal/dominio/**` | só stdlib | tudo mais |
| Aplicação | `internal/aplicacao/**` | domínio, stdlib | adaptadores, `cmd` |
| Adaptadores | `internal/adaptadores/**` | aplicação, domínio, libs | outros adaptadores |
| Entrada | `cmd/api`, `cmd/worker` | tudo | — |

## Fluxo de trabalho (spec-driven)

Padrão adotado de projeto de referência, com uma adaptação deliberada.

* **Toda mudança nasce como spec** em `.specs/changes/NNN-nome/`, numerada
  (a ordem importa), escrita antes do código. Spec executada vai para
  `.specs/archive/`. Uma spec cabe num dia de trabalho: fase do plano com
  várias entregas independentes é fatiada (Fase 2 = specs 002-005), e a
  primeira spec da fase lista o fatiamento.
* **Skills do repo** ficam em `.claude/skills/`: o jeito *deste* projeto de
  escrever cada padrão (repositório pgx, handler, construtor de domínio). Regra
  local: **a skill é escrita depois de fazer o primeiro caso à mão**, destilando
  o que se aprendeu — nunca antes. Escrever a skill é o que fixa o aprendizado.
* **Uma spec por vez, cada diff lido pelo autor.** Sem sub-agentes paralelos
  gerando o repositório inteiro: o objetivo é entender o código, não só tê-lo.
  Paralelizar só se cogita a partir do PWA (Fase 6), terreno já dominado.
* **`docs/revisao-ia.md`** registra onde uma sugestão da IA foi rejeitada e por
  quê. Requisito literal da vaga ("validar criticamente conteúdo gerado por IA").

# DOX framework

* DOX is highly performant AGENTS.md hierarchy installed here
* Agent must follow DOX instructions across any edits

## Core Contract

* AGENTS.md files are binding work contracts for their subtrees
* Work products, source materials, instructions, records, assets, and durable docs must stay understandable from the nearest applicable AGENTS.md plus every parent AGENTS.md above it

## Read Before Editing

1. Read the root AGENTS.md
2. Identify every file or folder you expect to touch
3. Walk from the repository root to each target path
4. Read every AGENTS.md found along each route
5. If a parent AGENTS.md lists a child AGENTS.md whose scope contains the path, read that child and continue from there
6. Use the nearest AGENTS.md as the local contract and parent docs for repo-wide rules
7. If docs conflict, the closer doc controls local work details, but no child doc may weaken DOX

Do not rely on memory. Re-read the applicable DOX chain in the current session before editing.

## Update After Editing

Every meaningful change requires a DOX pass before the task is done.

Update the closest owning AGENTS.md when a change affects:

* purpose, scope, ownership, or responsibilities
* durable structure, contracts, workflows, or operating rules
* required inputs, outputs, permissions, constraints, side effects, or artifacts
* user preferences about behavior, communication, process, organization, or quality
* AGENTS.md creation, deletion, move, rename, or index contents

Update parent docs when parent-level structure, ownership, workflow, or child index changes. Update child docs when parent changes alter local rules. Remove stale or contradictory text immediately. Small edits that do not change behavior or contracts may leave docs unchanged, but the DOX pass still must happen.

## Hierarchy

* Root AGENTS.md is the DOX rail: project-wide instructions, global preferences, durable workflow rules, and the top-level Child DOX Index
* Child AGENTS.md files own domain-specific instructions and their own Child DOX Index
* Each parent explains what its direct children cover and what stays owned by the parent
* The closer a doc is to the work, the more specific and practical it must be

## Child Doc Shape

* Create a child AGENTS.md when a folder becomes a durable boundary with its own purpose, rules, responsibilities, workflow, materials, or quality standards
* Work Guidance must reflect the current standards of the project or user instructions; if there are no specific standards or instructions yet, leave it empty
* Verification must reflect an existing check; if no verification framework exists yet, leave it empty and update it when one exists

Default section order:

* Purpose
* Ownership
* Local Contracts
* Work Guidance
* Verification
* Child DOX Index

## Style

* Keep docs concise, current, and operational
* Document stable contracts, not diary entries
* Put broad rules in parent docs and concrete details in child docs
* Prefer direct bullets with explicit names
* Do not duplicate rules across many files unless each scope needs a local version
* Delete stale notes instead of explaining history
* Trim obvious statements, repeated rules, misplaced detail, and warnings for risks that no longer exist

## Closeout

1. Re-check changed paths against the DOX chain
2. Update nearest owning docs and any affected parents or children
3. Refresh every affected Child DOX Index
4. Remove stale or contradictory text
5. Run existing verification when relevant
6. Report any docs intentionally left unchanged and why

## User Preferences

* Explicar cada decisão idiomática de Go enquanto o código é escrito (ponteiro
  vs valor, tamanho de interface, `errors.Is`, `(T, error)`), e apontar
  trade-offs reais para o autor decidir em vez de entregar pronto.
* Senhas e tokens: o autor digita no próprio terminal. O agente nunca pede,
  recebe, exibe ou grava credencial — nem em arquivo gitignored.
* Uma spec por vez; o autor lê cada diff antes do commit. Commit e push só
  com pedido explícito. **Exceção vigente:** em 2026-09-17 o autor autorizou
  execução contínua das specs 003 em diante ("siga até finalizar todo o
  projeto"), com commit+push por spec concluída e leitura a posteriori; cada
  spec arquivada deve carregar as decisões e o porquê, como material de
  estudo. O front (Fase 6) usa a skill ui-ux-pro-max, por pedido do autor.
* Decisões operacionais seguem o plano sem renegociar (ex.: Docker não subiu
  no prazo do Dia 1 → Postgres nativo, sem gastar o dia).

## Verificação

```powershell
gofmt -l .                       # vazio
go vet ./... ; go vet -tags=integracao ./...
go test ./...
. .\local.ps1 ; go test -tags=integracao ./...
go build ./cmd/api
```

Mais a regra de dependência do contrato 1. A CI (`.github/workflows/ci.yml`)
roda tudo isso com `-race`, um Postgres 17 de serviço, `govulncheck` e Gitleaks.

## Child DOX Index

* `internal/dominio/AGENTS.md` — contratos locais do domínio puro (só stdlib,
  tempo/fuso/aleatoriedade por parâmetro, construtores, sentinelas) e o
  padrão de teste dos pacotes `dinheiro`, `competencia`, `identidade`,
  `lancamento`, `ocorrencia`.

Fica na raiz: `internal/aplicacao` (casos de uso + `portas.go`),
`internal/adaptadores/{entrada/web, entrada/extrato, saida/postgres, saida/relogio, saida/telegram}`
(`extrato` é o tokenizador OFX próprio — SGML, Windows-1252, dinheiro sem
float; heurística de meio ajustável com dados reais),
`cmd/api`, `cmd/worker` (outbox, long polling do Telegram, agendador do
relatório mensal), `cmd/caixactl`
(subcomandos por `flag.NewFlagSet`: `migrar`, `importar`, `orcamento`),
`migracoes/` (SQLs + pacote `migracoes` com `go:embed`; migrador próprio em
`saida/postgres/migrador.go` — decisão na spec 002, sem goose), `deploy/`,
`.specs/` (specs numeradas; executadas vão para `archive/`),
`.claude/skills/` (`construtor-de-dominio`, `repositorio-pgx`, `handler-http`
— destiladas da spec 001), `docs/revisao-ia.md`.
Criar AGENTS.md filho em `internal/adaptadores/` quando nascer o segundo
adaptador de entrada (Telegram, Fase 3).

## Segurança (SEC-CHECK)

Todo código escrito ou editado neste repo deve seguir as 12 regras de
checklist-seguranca.md (secrets, injection, IV/OE, authn/authz,
fail secure, deps, headers, rate limit, IDOR/SSRF, logging).
Se violar alguma, avise antes de entregar.

Para auditoria completa de um arquivo específico, use o prompt da seção 0
desse mesmo arquivo.

