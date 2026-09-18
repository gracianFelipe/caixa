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

**Fase 2 em andamento.** Concluído até aqui (specs em [`.specs/archive/`](.specs/archive/)):

* **001** — espinha vertical: lançamento entra por HTTP, atravessa domínio e
  aplicação, grava no Postgres, volta por competência.
* **002** — migrações embutidas (`go:embed`) e migrador próprio idempotente
  (`caixactl migrar`), sem goose — decisão registrada na spec.
* **003** — tokenizador OFX próprio (SGML, Windows-1252, dinheiro sem float,
  fuzz) e `caixactl importar`: linha de extrato vira ocorrência (evidência) +
  lançamento (fato); reimportar é seguro por constraint, não por lógica.
  *Pendente: validar contra OFX real do Bradesco.*

* **004** — categorias fixas + categorização determinística por regras
  (precedência total: prioridade → exata>prefixo>contem>regex → tamanho → id;
  teste embaralha as regras e prova determinismo). Todo lançamento novo é
  classificado na entrada; sem regra, fica `pendente` para a fila do Telegram.

Próximo: Dockerfile, Telegram, conciliação, relatório com cinco detectores,
PWA embutido, deploy.

## Arquitetura em uma frase

```
cmd → adaptadores → aplicacao → dominio
```

`internal/dominio/**` importa **só a stdlib**. Não é convenção, é asserção na
CI, usando o critério do próprio toolchain (`.Standard`) em vez de regex:

```bash
go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./internal/dominio/... | grep -v '^github.com/gracianFelipe/caixa/'
# saida vazia = conforme
```

As interfaces moram em quem consome (`internal/aplicacao/portas.go`,
`internal/adaptadores/entrada/web`), nunca em quem implementa. Dinheiro é
`int64` em centavos do domínio ao JSON. Sem ORM, sem framework web, sem
testify: `net/http.ServeMux`, `pgx` com SQL na mão, `log/slog`, `testing` +
`go-cmp`.

## Rodar localmente (Windows)

Pré-requisitos: Go 1.27+, PostgreSQL 17 (nativo ou `deploy/docker-compose.yml`).

```powershell
# 1. banco (uma vez)
psql -U postgres -h localhost -c "CREATE DATABASE caixa"

# 2. variaveis de ambiente: crie local.ps1 (gitignored) com
#    $env:CAIXA_BD_URL = "postgres://postgres:SENHA@localhost:5432/caixa"
. .\local.ps1

# 3. migracoes: embutidas no binario via go:embed, aplicadas pelo migrador
#    proprio (tabela migracoes_aplicadas, uma transacao por arquivo,
#    advisory lock). Idempotente: rode em todo deploy.
go run ./cmd/caixactl migrar

# 4. subir
go run ./cmd/api
```

Endpoints:

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/saude` | liveness |
| `POST` | `/lancamentos` | registra um lançamento |
| `GET` | `/lancamentos?competencia=AAAA-MM` | lista o mês |
| `GET` | `/categorias` | vocabulário fixo de categorias |

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
