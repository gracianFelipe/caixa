# 006 — Outbox, worker e Telegram

Status: concluída em 2026-09-17 (worker aguardando token real do autor para o smoke manual)
Fase do plano: 3

## Objetivo

Nasce o segundo binário (`cmd/worker`) e a comunicação assíncrona: todo
lançamento criado grava um evento **na mesma transação** (outbox); o worker
consome com `FOR UPDATE SKIP LOCKED` e, para lançamento sem categoria,
pergunta no Telegram com teclado inline. A resposta aplica a categoria
(origem `manual`), cria uma **regra aprendida** (`exata` sobre
`contraparte_norm`) e edita a mensagem. Entra também o endpoint do Atalho do
iOS — captura rápida autenticada por token.

## Decisões

* **Outbox de verdade**: `eventos` gravado por `INSERT` dentro da transação
  de `Salvar`/`CriarComLancamento`. Publicar depois de commitar perderia
  evento em queda; publicar antes, publicaria mentira. O worker consome em
  lote com `SELECT ... FOR UPDATE SKIP LOCKED` — dois workers não pegam o
  mesmo evento, e evento de transação não commitada é invisível.
* **Cliente Telegram próprio** (~150 linhas, `net/http` puro) em
  `saida/telegram`: `sendMessage` (teclado inline), `getUpdates` (long
  polling — em dev nada é exposto na internet), `answerCallbackQuery`,
  `editMessageText`. Sem webhook nesta fase; o risco 5 do plano (webhook
  exposto) fica adiado junto.
* **Segurança Telegram**: só o `chat_id` do dono (env) é aceito; update de
  qualquer outro chat é ignorado e logado sem conteúdo. Token nunca em log —
  ele faz parte da URL, então a URL de request nunca é logada.
* **Callback carrega dados, não estado**: `cat:<lancamentoID>:<categoriaID>`
  (< 64 bytes do limite do Telegram). A tabela `perguntas` existe para não
  perguntar duas vezes e para editar a mensagem certa depois.
* **Regra aprendida**: resposta humana vira `exata`/`aprendida` com
  prioridade 100 — acima das sementes (`contem`, prioridade 0), abaixo de
  nada. `ON CONFLICT (tipo, padrao) DO UPDATE` atualiza a categoria: a
  última palavra humana vence.
* **Atalho iOS**: `POST /atalho/lancamentos` com `Authorization: Bearer`
  comparado com `subtle.ConstantTimeCompare`. Corpo mínimo: `valor` em texto
  brasileiro ("R$ 47,90" — reusa `dinheiro.Analisar`), `contraparte`, `meio`
  opcional (padrão pix), instante = agora. 401 sem corpo detalhado.
* **Worker sem framework**: dois loops (`outbox` a cada 2s; `getUpdates` com
  timeout 50s) em goroutines com `errgroup`? Não — `sync.WaitGroup` +
  contexto cancelável, stdlib pura, mesmo shutdown gracioso da API.

## Entregas

1. Migração `004_eventos_perguntas.sql`: `eventos` (outbox, indice parcial
   em pendentes) e `perguntas` (UNIQUE lancamento_id).
2. `dominio/evento` (tipo fechado + construtor) e `dominio/pergunta`.
3. Portas novas: `RepositorioDeEventos.ConsumirPendentes` (callback dentro
   da transação), `RepositorioDePerguntas`, `RegistrarAprendida` em regras,
   `AtribuirCategoria` em lançamentos, `MensageiroDeCategoria` (o que o
   worker precisa do Telegram — interface no consumidor, implementada por
   `saida/telegram`).
4. `aplicacao/fila.go`: `ProcessarEventos` (pergunta categoria de pendente)
   e `ResponderCategoria` (aplica manual + aprende regra + fecha pergunta).
5. `saida/postgres`: `eventos.go`, `perguntas.go`, `SalvarComEvento`,
   `AtribuirCategoria`, `RegistrarAprendida`.
6. `saida/telegram/cliente.go` + testes com `httptest` (fixtures JSON).
7. `entrada/web`: `POST /atalho/lancamentos` + middleware Bearer.
8. `cmd/worker/main.go`.
9. Integração: outbox com dois consumidores concorrentes não duplica
   (SKIP LOCKED provado); evento invisível antes do commit.

## SEC-CHECK

Secrets (token/chat via env, nunca em log — teste garante); AuthN no atalho
com comparação constante; IV no callback (parse estrito, ids validados);
rate implícito: `getUpdates` sequencial, `MaxBytesReader` nas respostas do
Telegram (1 MiB). PII: pergunta no Telegram contém contraparte e valor — é o
canal privado do dono, decisão consciente; log continua limpo.

## Verificação

```powershell
go vet ./... ; go test ./...
. .\local.ps1 ; go run ./cmd/caixactl migrar
go test -tags=integracao -count=1 ./...
# com CAIXA_TELEGRAM_TOKEN/CHAT_ID no local.ps1: go run ./cmd/worker
# POST /atalho/lancamentos com e sem Bearer correto (201 / 401)
```
