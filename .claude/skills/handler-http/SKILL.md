---
name: handler-http
description: Como este repo expõe um caso de uso por HTTP em internal/adaptadores/entrada/web com net/http puro — interface no consumidor, DTOs com valor_centavos int64, lerJSON com limite/DisallowUnknownFields, lista fechada de erros 400, log sem PII, teste com httptest. Use ao criar ou alterar rotas.
---

# Handler HTTP (padrão do Caixa)

Destilado de `internal/adaptadores/entrada/web/handler.go` e
`handler_test.go`, escritos à mão na spec 001.

## Forma

1. **Interface do serviço declarada neste pacote** (`type Lancamentos interface`)
   com só os métodos que as rotas usam. `*aplicacao.X` satisfaz sem saber.
   O teste usa um `servicoFalso` sem banco.
2. **Rotas no `ServeMux` com método:** `mux.HandleFunc("POST /lancamentos", ...)`.
   `NovoHandler` devolve `http.Handler`, embrulhado no middleware de acesso.
3. **DTOs separados do domínio**, com tags `json:"snake_case"`. Dinheiro é
   `valor_centavos int64` — o decoder recusa `47.90` por tipo. Resposta pode
   trazer também `valor` formatado (`"-R$ 47,90"`), nunca número decimal.
4. **`lerJSON`**: `http.MaxBytesReader` (64 KiB), `DisallowUnknownFields`,
   segundo `Decode` deve dar `io.EOF` (recusa dois documentos). Detalhe do
   decoder **não volta ao cliente** — descreve tipos internos.
5. **O handler não valida regra de negócio.** Converte o DTO em
   `lancamento.Dados` e chama o serviço; quem valida é `Novo`.
6. **Erros:** lista fechada `errosDoCliente` → 400 com a mensagem do sentinela;
   `errCorpoGrande` → 413; **qualquer outro → 500 genérico** e
   `log.ErrorContext` com método, rota e erro. Fail secure por padrão.
7. **Lista vazia é `[]`, não `null`:** `make([]T, 0, len(xs))`.
8. `responderJSON` sempre põe `Content-Type: application/json; charset=utf-8`
   e `X-Content-Type-Options: nosniff`.
9. **Log de acesso:** método, rota (`r.URL.Path`, sem query), status, duração.
   Query e corpo nunca — podem carregar dado financeiro.

## Teste (`handler_test.go`, `httptest`)

* `httptest.NewRequest` + `httptest.NewRecorder` + `h.ServeHTTP`; sem servidor.
* Tabela por rota: válido, JSON quebrado, campo desconhecido, decimal, texto
  onde é número, dois documentos, vazio, cada sentinela do domínio, corpo
  acima do limite, erro de infraestrutura.
* Asserções de contrato: status, `Content-Type`, corpo do 500 não contém o
  erro interno.
* `TestRotas`: método errado → 405, rota inexistente → 404.
* **`TestLogNaoVazaPII`**: captura o `slog` num buffer, força um 500 com valor
  e contraparte marcantes, garante que não aparecem — e que o erro interno
  aparece.

## Checagem antes de fechar

```powershell
go test ./internal/adaptadores/entrada/web/
go build ./cmd/api
```
