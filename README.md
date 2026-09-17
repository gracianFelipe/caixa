# Caixa

Controle financeiro pessoal em Go. Responde uma pergunta: **para onde o meu
dinheiro vai?** Um usuário, uma conta, ingestão por extrato OFX, e-mail do
banco e Telegram; categorização e relatório determinísticos, sem IA no
caminho crítico.

É também o meu primeiro projeto em Go, escrito para ser defendido linha a linha
numa entrevista técnica. Os contratos que o código obedece estão em
[`AGENTS.md`](AGENTS.md); o roteiro completo em
[`pode-planejar-breezy-harp.md`](pode-planejar-breezy-harp.md).

## Estado

**Fase 1 concluída** — a espinha vertical: um lançamento entra por HTTP,
atravessa domínio e aplicação, é gravado no Postgres e volta numa consulta por
competência. Spec em [`.specs/archive/001-fundacao-lancamento/`](.specs/archive/001-fundacao-lancamento/spec.md).

Próximas fases: importador OFX, categorização por regras, Telegram, conciliação,
relatório com cinco detectores, PWA embutido, deploy.

## Arquitetura em uma frase

```
cmd → adaptadores → aplicacao → dominio
```

`internal/dominio/**` importa **só a stdlib**. Não é convenção, é asserção na
CI:

```bash
go list -deps ./internal/dominio/... | grep -v '^github.com/gracianFelipe/caixa/' | grep -vE '^[a-z0-9_/]+$' && exit 1
```

As interfaces moram em quem consome (`internal/aplicacao/portas.go`,
`internal/adaptadores/entrada/web`), nunca em quem implementa. Dinheiro é
`int64` em centavos do domínio ao JSON. Sem ORM, sem framework web, sem
testify: `net/http.ServeMux`, `pgx` com SQL na mão, `log/slog`, `testing` +
`go-cmp`.

## Rodar localmente (Windows)

Pré-requisitos: Go 1.27+, PostgreSQL 17 (nativo ou `deploy/docker-compose.yml`).

```powershell
# 1. banco e migracoes (uma vez)
psql -U postgres -h localhost -c "CREATE DATABASE caixa"
psql -U postgres -h localhost -d caixa -f migracoes/001_lancamentos.sql
psql -U postgres -h localhost -d caixa -f migracoes/002_ocorrencias.sql

# 2. variaveis de ambiente: crie local.ps1 (gitignored) com
#    $env:CAIXA_BD_URL = "postgres://postgres:SENHA@localhost:5432/caixa"
. .\local.ps1

# 3. subir
go run ./cmd/api
```

Endpoints:

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/saude` | liveness |
| `POST` | `/lancamentos` | registra um lançamento |
| `GET` | `/lancamentos?competencia=AAAA-MM` | lista o mês |

```powershell
# no PowerShell, mande JSON por arquivo: aspas escapadas na linha de comando quebram
'{"valor_centavos":-4790,"meio":"pix","contraparte":"SUPERMERCADO XYZ","ocorrido_em":"2026-09-17T15:00:00Z"}' | Set-Content -Encoding ascii corpo.json
curl.exe -X POST http://localhost:8080/lancamentos -H "Content-Type: application/json" -d "@corpo.json"
curl.exe "http://localhost:8080/lancamentos?competencia=2026-09"
```

`valor_centavos` é inteiro; `47.90` é recusado com `400` pelo próprio decoder.

## Verificar

```powershell
go vet ./...
go test ./...                              # unitarios, sem banco
go test -tags=integracao ./...             # exige CAIXA_BD_URL
go test -fuzz=FuzzAnalisar -fuzztime=30s ./internal/dominio/dinheiro
```

A CI roda `gofmt`, `go vet`, a regra de dependência, `go test -race`, os testes
de integração contra um Postgres 17 de serviço, `govulncheck` e Gitleaks.

## Armadilhas do Windows que este repo já contorna

* `time.LoadLocation("America/Sao_Paulo")` falha sem `import _ "time/tzdata"`
  — está em `cmd/api/main.go`. Testes de domínio usam `time.FixedZone`.
* `curl` no PowerShell é alias de `Invoke-WebRequest`: use `curl.exe`.
* Sem dotenv: `local.ps1` gitignored, carregado com `. .\local.ps1`.

## Revisão crítica de código gerado por IA

Parte do código nasce com assistência de IA. Tudo que foi rejeitado ou
corrigido, e por quê, está em [`docs/revisao-ia.md`](docs/revisao-ia.md).
