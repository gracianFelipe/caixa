# 001 — Fundação: dinheiro, lançamento, persistência, HTTP

Status: concluída em 2026-09-17
Fase do plano: 1 (Dia 1)

## Desvios em relação ao planejado

* **Postgres nativo, não Docker.** Docker Desktop caiu duas vezes na criação
  de sockets Unix em `AppData\Local` (erro do Windows, não do projeto). Seguiu
  a regra do plano: plano B às 2:45, sem brigar. `deploy/docker-compose.yml`
  foi escrito e é validado pela CI, que usa `postgres:17-alpine` como serviço.
* **OFX adiado.** Fase 1 ficou verde depois da marca de 7:00; pela regra, o
  importador abre a Fase 2.
* **Pacote `identidade` acrescentado.** UUIDv7 em stdlib pura, porque
  `aplicacao` gera os ids e não pode importar lib externa.
* **Interface do serviço no adaptador web**, não em `portas.go`: `portas.go`
  guarda portas de saída; o consumidor da aplicação é o handler, e a
  interface mora nele. Contrato 2 do `AGENTS.md` ajustado para dizer isso.
* **Verificação da regra de dependência corrigida** (excluir o módulo antes de
  filtrar). Registrado em `docs/revisao-ia.md` #001.
* **Bug em `lancamento.Novo`** (meio validado mas gravado sem normalizar),
  pego ao desenhar o handler. Registrado em `docs/revisao-ia.md` #002.

## Objetivo

Levantar a espinha vertical do Caixa: um lançamento entra por HTTP, atravessa
domínio e aplicação, é gravado no Postgres e volta numa consulta por
competência. Nenhuma funcionalidade além disso.

O critério real não é "funciona" e sim **"o autor explica cada linha em voz
alta"**. Esta spec existe para que o escopo do Dia 1 não cresça durante o Dia 1.

## Fora de escopo (explícito)

Telegram, IMAP, categorias, orçamento, conciliação, relatório, WebSocket, PWA,
deploy, autenticação, goose. Nenhum deles é antecipado, nem "só a estrutura".

O leitor de OFX + `caixactl importar` é **stretch**: só começa se a verificação
abaixo estiver inteira verde. Não é critério de conclusão desta spec.

## Entregas

| # | Pacote / arquivo | Conteúdo |
|---|---|---|
| 1 | `internal/dominio/dinheiro` | `Centavos int64`; `Analisar(string) (Centavos, error)` para "R$ 1.234,56"; `String() string`; `Somar(...)`. Teste table-driven junto. |
| 2 | `deploy/docker-compose.yml` + `migracoes/001_*.sql`, `002_*.sql` | Postgres 17 local; DDL de `lancamentos` e `ocorrencias` conforme o plano. Aplicadas via `docker-entrypoint-initdb.d`. |
| 3 | `internal/dominio/lancamento` | Construtor `Novo` com invariantes, erros sentinela, `Competencia` calculada em `America/Sao_Paulo`. Testes de borda. |
| 4 | `internal/aplicacao` | Caso de uso de registrar e de listar; `portas.go` com as interfaces **declaradas aqui**, no consumidor. |
| 5 | `internal/adaptadores/saida/postgres` | Pool `pgx`, `Salvar`, `DaCompetencia`. SQL na mão, sempre parametrizado. Teste `//go:build integracao`. |
| 6 | `internal/adaptadores/entrada/web` | `POST /lancamentos`, `GET /lancamentos?competencia=`, `GET /saude`. Teste com `httptest`. |
| 7 | `cmd/api/main.go` | `log/slog`, config por env, `ReadHeaderTimeout`, shutdown gracioso, `import _ "time/tzdata"`. |
| 8 | `README.md`, `.gitignore`, CI | `go vet` + `go test ./...` + regra de dependência + Gitleaks + govulncheck. |

## Contratos aplicáveis

Valem os 6 contratos do `AGENTS.md` sem exceção. Os que esta spec exercita
diretamente:

* `dominio/**` importa só stdlib — inclusive `lancamento`, que recebe o instante
  como parâmetro em vez de chamar `time.Now()`.
* Interfaces em `internal/aplicacao/portas.go`. O pacote `postgres` **implementa
  sem importar** uma interface declarada por ele mesmo.
* Dinheiro é `int64` em centavos do domínio ao JSON. O JSON carrega
  `valor_centavos`, nunca um número decimal.
* Sem ORM, sem framework web, sem testify.

## SEC-CHECK

Foco declarado para esta spec:

* **PQ** — todo SQL com `$1, $2`. Nenhuma concatenação de string com input, em
  nenhuma consulta, nem nas de teste.
* **Secrets** — senha do Postgres só por env var. `local.ps1` e `.env` no
  `.gitignore` desde o primeiro commit; o repo é público.
* **Logging** — `valor_centavos` e `contraparte` são PII. Não entram em log.
  Log de erro carrega `id` e tipo do erro, não o conteúdo do lançamento.
* **IV** — corpo do `POST` com `MaxBytesReader` e `DisallowUnknownFields`;
  `competencia` validada como `AAAA-MM` antes de chegar ao banco.
* **Fail secure** — erro HTTP devolve mensagem genérica; o detalhe fica no slog
  do servidor.

## Verificação (definição de pronto)

```powershell
go vet ./...
go test ./...
docker compose -f deploy/docker-compose.yml up -d
go test -tags=integracao ./...
go run ./cmd/api
curl.exe http://localhost:8080/saude
curl.exe -X POST http://localhost:8080/lancamentos -H "Content-Type: application/json" -d "{\"valor_centavos\":-4790,\"meio\":\"pix\",\"contraparte\":\"SUPERMERCADO XYZ\",\"ocorrido_em\":\"2026-09-17T15:00:00Z\"}"
curl.exe "http://localhost:8080/lancamentos?competencia=2026-09"
```

Mais a regra de dependência, que é a asserção que vale o repositório:

```bash
go list -deps ./internal/dominio/... | grep -vE '^[a-z0-9_/]+$' && exit 1
```

## Riscos assumidos hoje

* **Docker/WSL2 pode pedir reinício.** Decisão tomada: download em paralelo; se
  o daemon não estiver de pé no bloco do banco, cai para Postgres nativo e o
  `docker-compose.yml` fica escrito e validado depois. Não se gasta o Dia 1
  brigando com Docker.
* **Fuso.** `time.LoadLocation("America/Sao_Paulo")` falha no Windows sem
  `_ "time/tzdata"`. Resolvido no `main.go` e coberto por teste de competência
  na virada do mês (22:30 de 30/09 em São Paulo é 01:30 UTC de 01/10 e pertence
  a setembro).

## Ao concluir

Mover para `.specs/archive/001-fundacao-lancamento/`. Só então escrever as
skills de `.claude/skills/` destiladas do que foi feito à mão aqui
(repositório pgx, handler, construtor de domínio) — nunca antes.
