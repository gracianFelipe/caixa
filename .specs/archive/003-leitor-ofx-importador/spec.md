# 003 — Leitor OFX e `caixactl importar`

Status: concluída em 2026-09-17 (pendente: validação com OFX real do autor)
Fase do plano: 2 (segunda de quatro: 002 migrações ✔ · **003 OFX** · 004
categorias · 005 Dockerfile)

## Objetivo

Carregar extratos OFX do Bradesco no banco: cada linha do extrato vira uma
`ocorrencia` (evidência) e um `lancamento` (fato), com idempotência garantida
pelas constraints — importar o mesmo arquivo duas vezes não duplica nada.
Marco da Fase 2: dados reais no banco, produto útil sem interface.

## Risco assumido: parser antes do arquivo real

A regra era "não escrever o parser sem ver o OFX real". O autor autorizou
execução contínua e o arquivo ainda não foi exportado; o tokenizador nasce
sobre **fixtures sintéticas** do formato documentado do OFX 1.02 brasileiro
(SGML sem fechamento de tag, Windows-1252, `DTPOSTED` com `[-3:BRT]`,
`TRNAMT` com ponto decimal, `FITID` presente, `MEMO` com lixo). Quando o
arquivo real chegar em `extratos/`, a validação é obrigatória e qualquer
divergência vira caso de teste + entrada em `docs/revisao-ia.md`.

## Modelo (relembrando o plano)

`Lancamento` é o fato; `Ocorrencia` é a evidência. Importação usa só o
nível 1 de deduplicação — idempotência exata por constraint
(`UNIQUE (origem_id, impressao)` e `UNIQUE (origem_id, id_externo)`), com
`ON CONFLICT DO NOTHING`. O nível 2 (reconciliação por pontuação entre
origens diferentes) é a Fase 4, spec futura.

## Entregas

| # | Pacote / arquivo | Conteúdo |
|---|---|---|
| 1 | `internal/dominio/ocorrencia` | `Origem` (manual/extrato_ofx/email_bradesco/telegram, espelhando a tabela `origens`), `Resultado`, `Ocorrencia`, construtor `Nova` que calcula `Impressao` = sha256 hex do payload normalizado (`crypto/sha256`, stdlib). |
| 2 | `internal/adaptadores/entrada/extrato` | Tokenizador OFX próprio (~200 linhas): cabeçalho `CHAVE:VALOR`, decodificação Windows-1252 → UTF-8 (via `x/text/charmap`, permitido em adaptador), tag soup SGML, captura do bloco `STMTTRN` bruto como payload. Parsers estritos de valor (`-47.90`, `-1.234,56`; nunca float) e de data (`AAAAMMDD[HHMMSS][[-3:BRT]]`). Heurística de `Meio` a partir do MEMO (PIX/TED/BOLETO/SAQUE/CARTÃO), documentada como ajustável com dados reais. Fuzz. |
| 3 | `internal/aplicacao/importacao.go` + porta nova | Caso de uso `Importacao.Importar(ctx, origem, itens)` recebendo `ItemDeExtrato` (DTO da aplicação — quem converte de `extrato.Transacao` é o `cmd`, mantendo a regra de dependência). Valor zero → ignorado (linha informativa de extrato). Resumo {criados, duplicados, ignorados}. |
| 4 | `internal/adaptadores/saida/postgres/ocorrencias.go` | `CriarComLancamento`: uma transação por item — `INSERT ocorrencias ... ON CONFLICT DO NOTHING` (sem alvo: qualquer unique conta como duplicata), se inseriu → `INSERT lancamentos` + `UPDATE ocorrencias SET lancamento_id, resultado='criou'`. Payload gravado como JSONB `{"bruto": texto}` — texto cru, número nunca interpretado no JSON. |
| 5 | `cmd/caixactl` | Subcomando `importar arquivo.ofx [outro.ofx...]`. Resumo por arquivo em stdout. `_ "time/tzdata"` também aqui (caixactl carrega fuso). |
| 6 | Testes | Domínio e extrato unitários (tabela + fuzz com corpus sintético em `testdata/`); aplicação com fake; integração provando: importar duas vezes → segunda tudo duplicado; mesmo `id_externo` com payload diferente → duplicado; transação atômica. |

## SEC-CHECK

* **IV** — arquivo OFX é entrada externa hostil: tamanho máximo (8 MiB),
  tokenizador nunca entra em pânico (fuzz), campos obrigatórios validados,
  transação com valor fora de int64 é erro, não truncamento.
* **PQ** — SQL novo todo parametrizado.
* **Logging** — resumo em stdout traz contagens; linha individual nunca é
  logada (valor+contraparte = PII). Erro de item identifica posição no
  arquivo, não o conteúdo.
* **Secrets** — nada novo; extratos reais ficam em `extratos/`, já ignorado.

## Verificação

```powershell
go vet ./... ; go test ./...
go test -run=NADA -fuzz=FuzzAnalisar -fuzztime=30s ./internal/adaptadores/entrada/extrato
. .\local.ps1 ; go test -tags=integracao -count=1 ./...
go run ./cmd/caixactl importar .\internal\adaptadores\entrada\extrato\testdata\sintetico_basico.ofx   # cria
go run ./cmd/caixactl importar .\internal\adaptadores\entrada\extrato\testdata\sintetico_basico.ofx   # tudo duplicado
```

Com arquivo real (pendente do autor): `caixactl importar extratos\*.ofx` e
conferência manual de contagem e soma do mês contra o extrato.

## Ao concluir

Arquivar. Pendência aberta: validação com OFX real — registrada aqui e no
README de estado.
