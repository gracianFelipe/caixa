# Caixa — controle financeiro pessoal em Go

## Contexto

Primeiro projeto em Go do autor, escrito para servir de evidência técnica de
backend (Go, PostgreSQL, Docker, DDD, REST, WebSockets, testes e revisão
crítica de código gerado por IA) e para ser defendido linha a linha numa
entrevista.

O projeto resolve um problema real em paralelo: controlar para onde o dinheiro
está indo. Projeto de portfólio que o autor **usa de verdade** sobrevive à
pergunta "por que você fez isso?".

**Resultado esperado:** um repositório em Go que o autor consiga ler e defender
linha a linha, com deploy no ar, ao fim da Fase 5 (~2 semanas). O Dia 1 entrega
a base funcionando.

---

## Decisões estruturantes

| # | Decisão | Razão |
|---|---|---|
| 1 | **Dois binários (`api` + `worker`)**, não cinco microsserviços | Um usuário não justifica malha de serviços. O que justifica é a fronteira assíncrona por eventos. Cinco serviços para um usuário lê como cargo cult. |
| 2 | **Domínio em português, técnico em inglês** | Ubiquitous language de DDD: "lançamento", "competência", "conciliação". Regra dura: **sem acento em identificador** (`Orcamento`). |
| 3 | **Dinheiro é `int64` em centavos**, coluna `BIGINT` | `float` para dinheiro é o erro nº 1 em sistema financeiro. Inteiro soma nativo e compara exato. |
| 4 | **Sem ORM, sem framework web, sem testify** | `net/http.ServeMux` (Go 1.22+), `pgx` com SQL na mão, `log/slog`, `testing` + `go-cmp`. O diferencial dele é banco de dados; ORM esconde exatamente isso. |
| 5 | **PWA embutido no binário Go** (`embed.FS`) | Um domínio, zero CORS, zero cookie cross-site, deploy único. |
| 6 | **Categorização e relatório 100% determinísticos** | Sem IA no caminho crítico: testável, auditável, explicável. A decisão de *não* usar IA impressiona mais que usá-la. |

---

## Arquitetura

### Regra de dependência

```
cmd → adaptadores → aplicacao → dominio
                                   ↑
                    ninguém importa para dentro do domínio
```

`internal/dominio/...` importa **apenas a stdlib**. Sem `pgx`, sem `net/http`, sem `time.Now()` implícito (o tempo entra como parâmetro).

Diferente do Truco, aqui isso é **verificável por máquina** — passo obrigatório na CI:

```bash
go list -deps ./internal/dominio/... | grep -vE '^[a-z0-9_/]+$' && exit 1
```

Um princípio de arquitetura virando asserção executável. É o melhor argumento do repositório.

### Estrutura

```
caixa/
├── AGENTS.md                  # contrato DOX: tabela de camadas + regra de dependência
├── cmd/
│   ├── api/                   # HTTP + webhook Telegram + WebSocket (síncrono)
│   ├── worker/                # IMAP, outbox, agendador mensal
│   └── caixactl/              # CLI: migrar, importar OFX/CSV, gerar relatório
├── internal/
│   ├── dominio/               # ► PURO — só stdlib
│   │   ├── dinheiro/          # Centavos, parsing "R$ 1.234,56", aritmética inteira
│   │   ├── competencia/       # Ano-mês, fuso America/Sao_Paulo
│   │   ├── lancamento/        # Agregado raiz + invariantes
│   │   ├── categoria/         # Categoria e Orcamento
│   │   ├── categorizacao/     # Classificador por regras (precedência determinística)
│   │   ├── conciliacao/       # Deduplicação por pontuação (função pura)
│   │   ├── relatorio/         # Os 5 detectores
│   │   └── evento/
│   ├── aplicacao/             # casos de uso + portas.go (interfaces no consumidor)
│   ├── adaptadores/
│   │   ├── entrada/           # web/ ws/ telegram/ email/bradesco/ extrato/
│   │   └── saida/             # postgres/ telegram/ fila/ arquivos/ relogio/
│   └── plataforma/            # config, registro (slog), bd, migracoes
├── migracoes/                 # 001..008, embutidas via //go:embed
├── web/                       # PWA (build estático entra no embed)
├── docs/decisoes/             # ADRs
├── docs/revisao-ia.md         # ★ onde a IA errou e como foi corrigido
└── deploy/                    # Dockerfile, docker-compose.yml, Caddyfile
```

### Tabela de camadas

| Camada | Pacote | Pode importar |
|---|---|---|
| Domínio | `dominio/*` | stdlib + outros pacotes de domínio |
| Aplicação | `aplicacao` | domínio (**declara as interfaces**) |
| Entrada | `adaptadores/entrada/*` | aplicação + domínio |
| Saída | `adaptadores/saida/*` | aplicação + domínio |
| Plataforma | `plataforma/*` | stdlib + libs |
| Montagem | `cmd/*` | tudo |

**Ponto de entrevista:** em Java você criaria um pacote `ports`. Em Go a interface mora em **quem consome**, não em quem implementa.

---

## Modelo de dados (essencial)

Migrações com **goose** embutidas. Numeração sequencial; nunca editar migração aplicada.

**`lancamentos`** — o fato
```sql
id UUID PK (UUIDv7, gerado em Go)
ocorrido_em TIMESTAMPTZ          -- instante do gasto, UTC
competencia DATE                 -- 1º dia do mês, calculado em America/Sao_Paulo
competencia_fatura DATE          -- só crédito: mês da fatura
valor_centavos BIGINT CHECK (<> 0)   -- negativo = saída
meio TEXT CHECK (pix|credito|debito|boleto|dinheiro|transferencia)
contraparte TEXT, contraparte_norm TEXT   -- maiúscula, sem acento, sem dígito
categoria_id SMALLINT FK
categoria_origem TEXT CHECK (pendente|regra|manual|importacao)
situacao TEXT CHECK (provisorio|confirmado|descartado)
```
Índice parcial `WHERE categoria_id IS NULL` = a fila de "perguntar no Telegram" vira varredura mínima.

**`ocorrencias`** — a evidência (o coração da deduplicação)

O modelo certo: **`Lancamento` é o fato, `Ocorrencia` é a evidência.** Conciliar não apaga linha — anexa uma segunda evidência ao mesmo fato.
```sql
id UUID PK, origem_id FK, id_externo TEXT   -- FITID do OFX, Message-Id do e-mail
impressao TEXT                               -- sha256 da carga normalizada
payload JSONB                                -- bruto (texto, nunca número interpretado)
lancamento_id UUID FK
resultado TEXT CHECK (pendente|criou|conciliou|duplicada|erro|ignorada)

UNIQUE (origem_id, impressao)                -- idempotência exata
UNIQUE (origem_id, id_externo) WHERE id_externo IS NOT NULL
```

**Dois níveis de deduplicação:**
1. **Idempotência** (mesma mensagem duas vezes) → constraint no banco, `ON CONFLICT DO NOTHING`. Zero lógica.
2. **Reconciliação** (mesmo gasto por fontes diferentes) → pontuação determinística no domínio:

```
valor idêntico ao centavo            → +50  (sem isso, descarta)
datas: 0d +25 | 1d +18 | 2d +12 | 3d +6 | >3d descarta
similaridade(contraparte) >= 0.90    → +25   (Jaccard sobre trigramas, ~30 linhas)
mesmo meio                           → +10
origens diferentes                   → +5
mesma origem                         → -20

>= 85 concilia sozinho · 60-84 pergunta no Telegram · < 60 lançamento novo
```

**`regras_categorizacao`** — a memória aprendida (sem IA)

Precedência fixa: `prioridade DESC` → `exata > prefixo > contem > regex` → `length(padrao) DESC` → `id ASC`. **Nunca empate não resolvido**, com teste provando. Ao responder o botão no Telegram, cria regra `exata/aprendida` sobre `contraparte_norm`. Regra com `erros > acertos` é desativada sozinha.

**`orcamentos`** (limite por categoria, `competencia NULL` = padrão) · **`alertas`** (UNIQUE `(tipo, chave)` garante que o aviso de 80% não vai 14 vezes) · **`eventos`** (outbox transacional) · **`perguntas`** · **`arquivos`**.

**Outbox:** o evento é gravado **na mesma transação** do lançamento. O worker consome com `FOR UPDATE SKIP LOCKED` — a peça que permite mais de um consumidor sem duplicar trabalho.

**Sem `usuario_id` em todas as tabelas.** Não existe segundo usuário. Na entrevista: *"multi-tenant aqui é uma coluna e uma policy de RLS; não coloquei porque seria coluna morta."*

---

## Os 5 detectores do relatório

`type Detector func(Janela) []Sinal` — puros, table-driven, testados com golden files.

| Sinal | Regra |
|---|---|
| Vazamento por repetição | ≥ 8 lançamentos de até R$ 30 na mesma categoria, somando ≥ R$ 150 |
| Assinatura esquecida | mesmo `contraparte_norm` em ≥ 3 competências consecutivas, variação ≤ 5%, intervalo 28-32 dias |
| Categoria em escalada | ≥ 3 meses de crescimento monotônico, alta ≥ 25%, mês atual ≥ R$ 100 |
| Estouro de orçamento | gasto > limite, severidade proporcional |
| Gasto atípico | \|valor\| > mediana(6 meses) + 4 × MAD, e ≥ R$ 100 |

**Mediana + MAD, não média + desvio padrão:** com 6 meses de dados o próprio outlier contamina a média e o desvio, e o sinal deixa de disparar exatamente no caso que interessa. Boa resposta quando perguntarem "por que não a média?".

---

## Dia 1 — meta: Fase 1 + importador de extrato

| Bloco | Entrega |
|---|---|
| 0:00-0:45 | `winget install GoLang.Go`, VS Code + extensão. Disparar download do Docker **em paralelo**. `go version` respondendo. |
| 0:45-2:00 | Pular "hello world". `internal/dominio/dinheiro` **com teste**: `Analisar`, `String`, `Somar`. Aprende struct, método, error, múltiplo retorno e `go test` de uma vez. Primeiro verde. |
| 2:00-2:45 | `docker compose up -d postgres`. Migrações 001-002 via `docker-entrypoint-initdb.d`. |
| 2:45-4:00 | `internal/dominio/lancamento`: construtor `Novo` com invariantes, erros sentinela, testes de borda. |
| 4:00-5:30 | `saida/postgres`: pool pgx, `Salvar`, `DaCompetencia`. Teste com build tag `//go:build integracao`. |
| 5:30-7:00 | `entrada/web`: `POST /lancamentos`, `GET /lancamentos?competencia=`, `GET /saude`. `cmd/api` com slog, config por env, shutdown gracioso. Teste com `httptest`. |
| 7:00-8:00 | README, AGENTS.md, .gitignore, commit, push, CI (`go vet` + `go test ./...`). |
| **Stretch** | `entrada/extrato` (OFX) + `caixactl importar` carregando os 6 meses de histórico. |

**Aviso sobre o stretch:** o OFX do Bradesco é **SGML, não XML** — tags sem fechamento, `encoding/xml` não lê, ISO-8859-1, datas `20260915120000[-3:BRT]`. Não existe biblioteca Go boa. É um tokenizador tolerante próprio de ~200 linhas e pode consumir o dia inteiro sozinho. **Se às 7:00 a Fase 1 não estiver verde, adie o importador** — ele é o primeiro item da Fase 2, não vale sacrificar a base.

**Fora do Dia 1, sem exceção:** Telegram, IMAP, categorias, orçamento, conciliação, relatório, WebSocket, PWA, deploy.

**Se o Docker não subir até 2:45** (risco do WSL2): `winget install PostgreSQL.PostgreSQL.17` e siga. Não gaste o Dia 1 brigando com Docker.

---

## Fases seguintes

| Fase | Duração | Entrega |
|---|---|---|
| 2 | 3-4d | goose embutido, migrações 003-005, categorização com regras-semente, leitor OFX/CSV, `caixactl importar`, Dockerfile multi-stage, CI completa. **Marco: 6 meses de dados reais no banco, produto útil sem interface.** |
| 3 | 3d | Cliente Telegram próprio (~150 linhas, sem biblioteca), pergunta de categoria com teclado inline, endpoint para o Atalho do iOS, outbox, **nasce o `cmd/worker`**. Marco: dois deploys, comunicação assíncrona. |
| 4 | 3d | Conciliação completa, pergunta de conciliação na faixa 60-84, orçamentos, alertas de 80%/100%. |
| 5 | 3d | Motor de relatório, golden files, agendador, `GET /relatorio/{competencia}`, comando `/relatorio`. **Marco: pronto para entrevista.** |
| 6 | 4d | PWA Next.js `output: 'export'` embutido via `embed.FS`, login argon2id + cookie HttpOnly, hub de WebSocket. |
| 7 | 2-3d | IMAP (`go-imap` v2), polling 2min com última UID persistida, analisador `bradesco/` com fixtures `.eml`. **Por último de propósito** — é a única premissa não verificada. |
| 8 | 2-3d | docker-compose, Caddy com TLS, deploy Oracle Cloud ARM (`GOOS=linux GOARCH=arm64` cross-compila do Windows sem toolchain C), backup pg_dump, govulncheck. |
| 9 | opcional | `saida/fila/sqs` e `saida/arquivos/s3` como segundas implementações das portas existentes. **O valor é provar que a porta existia antes do adaptador.** |

**Correção no requisito "dia 30":** não existe em fevereiro. Agendar **dia 1 às 08:00 America/Sao_Paulo sobre o mês fechado anterior**, com última execução persistida para restart não pular nem duplicar.

---

## Setup Windows

```powershell
winget install GoLang.Go            # abrir terminal NOVO depois (PATH)
wsl --install                       # reiniciar se pedir
winget install Docker.DockerDesktop
winget install Microsoft.VisualStudioCode
code --install-extension golang.go

go install github.com/pressly/goose/v3/cmd/goose@latest
go install honnef.co/go/tools/cmd/staticcheck@latest
go install golang.org/x/vuln/cmd/govulncheck@latest

git config --global core.autocrlf input
```

**Quatro armadilhas do Windows:**
1. `curl` no PowerShell é alias de `Invoke-WebRequest` e não aceita as flags — use `curl.exe`.
2. **Sem dotenv.** Use `local.ps1` gitignored com `$env:VAR = "..."`.
3. **Fuso:** Windows não tem tzdata do IANA — `time.LoadLocation("America/Sao_Paulo")` **falha aqui e funciona no Linux**. Solução: `import _ "time/tzdata"` no `main.go`. Achar agora, não no deploy.
4. `gofmt` ao salvar. Em Go, formatação não é preferência.

---

## Testes

**`testing` da stdlib + `google/go-cmp`. Sem testify** — combina com o instinto dele no Truco (`node --test`, sem dependência) e é o que o time do Go faz. Tudo **table-driven**.

| Alvo | Tipo | Por quê |
|---|---|---|
| `dominio/dinheiro` | unitário + **fuzz** | Bug aqui corrompe tudo em silêncio |
| `dominio/conciliacao` | table-driven ~25 casos | Mais difícil de acertar, mais fácil de testar (é puro). Caso que quebra implementações ingênuas: **dois gastos legítimos iguais no mesmo dia** |
| `dominio/relatorio` | **golden files** | Regressão visível em diff, atualizável com `-update` |
| `entrada/extrato` | unitário + **fuzz** | Entrada externa malformada |
| `entrada/email/bradesco` | fixtures `.eml` | Congela o formato; avisa quando o banco mudar |
| `saida/postgres` | integração, build tag | Prova a constraint de idempotência e o `SKIP LOCKED` |

```
go test ./...                    # rápido, sem banco
go test -race ./...              # obrigatório na CI (o hub de WS só vale com isso)
go test -tags=integracao ./...
go test -fuzz=FuzzAnalisarOFX -fuzztime=60s ./internal/adaptadores/entrada/extrato
```

**Fakes em memória, não mocks gerados.** Testa comportamento, não sequência de chamadas. Meta de cobertura é lista ("todo detector tem caso de borda"), não número.

---

## Riscos

1. **O e-mail do Bradesco pode não existir.** Risco nº 1 e estrutural: é a única fonte de gatilho automático, já que o iOS não deixa reagir a notificação de outro app. **Verificar hoje:** buscar na caixa por remetentes do Bradesco dos últimos 90 dias e conferir se o alerta traz **valor e destinatário no corpo** — muitos bancos mandam só "você tem uma movimentação". Mitigação já embutida: o e-mail é uma origem entre quatro, e a espinha dorsal é o OFX. Por isso é Fase 7.
2. **OFX brasileiro é hostil** (SGML, latin-1, `<MEMO>` com lixo). Tokenizador próprio + fuzz. Pelo lado bom, vira um dos artefatos mais interessantes do repo.
3. **Fuso:** `TIMESTAMPTZ` em UTC, mas `competencia` calculada em São Paulo — um PIX às 22:30 de 30/set é 01:30 UTC de 01/out e cairia no mês errado. Nunca `AddDate(0,-1,0)` a partir do dia 31.
4. **Dinheiro:** nunca `float`, em lugar nenhum. Cuidado: `encoding/json` decodifica número para `float64` quando o destino é `interface{}` — por isso o `payload JSONB` guarda texto bruto.
5. **Webhook do Telegram exposto:** segredo de 32 bytes no caminho + header `X-Telegram-Bot-Api-Secret-Token` com `subtle.ConstantTimeCompare`, rejeitar `chat_id` alheio, `MaxBytesReader`, `ReadHeaderTimeout`, rate limit. **Em dev, long polling** — não expor nada.
6. **Oracle Cloud ARM frequentemente sem capacidade.** Plano B: VPS ~R$ 25/mês. Não deixar bloquear a Fase 8.
7. **Escopo é o maior risco.** "Pronto para entrevista" é o fim da Fase 5. Se precisar cortar, **corte PWA antes de cortar testes**.
8. **Código de IA que compila mas não é idiomático.** Manter `docs/revisao-ia.md` com os casos rejeitados (`float64` para valor, goroutine solta no handler, ORM, `panic` em vez de `error`). **Regra dura: nenhuma linha entra que o autor não saiba explicar em voz alta.**

---

## Verificação

**Dia 1 está pronto quando todos passarem:**
```powershell
go vet ./...
go test ./...                                  # verde, incluindo dinheiro e lancamento
docker compose -f deploy/docker-compose.yml up -d
go test -tags=integracao ./...                 # verde com Postgres no ar
go run ./cmd/api                               # sobe sem erro
curl.exe http://localhost:8080/saude            # 200
curl.exe -X POST http://localhost:8080/lancamentos -H "Content-Type: application/json" -d "{\"valor_centavos\":-4790,\"meio\":\"pix\",\"contraparte\":\"SUPERMERCADO XYZ\",\"ocorrido_em\":\"2026-09-17T15:00:00Z\"}"
curl.exe "http://localhost:8080/lancamentos?competencia=2026-09"   # devolve o lançamento
git log --oneline                              # commit no GitHub
```

**Verificação do stretch (importador):**
```powershell
go run ./cmd/caixactl importar --arquivo extrato.ofx
# conferir no banco: contagem de linhas, nenhuma duplicata, valores conferindo com o extrato
```

**Regra de dependência (todas as fases):**
```bash
go list -deps ./internal/dominio/... | grep -vE '^[a-z0-9_/]+$' && exit 1
```

**Ao fim da Fase 5 (pronto para entrevista):** `git clone && docker compose up && go run ./cmd/api` funciona num diretório limpo; relatório de um mês real gerado e conferido à mão contra o extrato; o autor consegue abrir um arquivo aleatório do repo e explicar cada decisão em voz alta.

---

## Próximo passo depois da aprovação

Criar o repositório `caixa` no GitHub e escrever o **prompt de abertura** para o chat daquele repo, contendo: contexto do projeto, as 6 decisões estruturantes, a regra de dependência, o roteiro do Dia 1 e o `AGENTS.md` inicial a ser criado.
