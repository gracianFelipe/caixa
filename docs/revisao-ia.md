# Revisão crítica de conteúdo gerado por IA

Registro de cada caso em que uma sugestão ou um trecho gerado por IA foi
rejeitado ou corrigido neste repositório, e por quê. O objetivo não é
constranger a ferramenta: é provar que nenhuma linha entra sem ser lida.

Formato de cada entrada: **onde**, **o que a IA produziu**, **por que estava
errado**, **como foi pego**, **o que entrou no lugar**.

---

## 001 — Verificação da regra de dependência marcava o próprio domínio

* **Onde:** `AGENTS.md`, contrato 1; `pode-planejar-breezy-harp.md`, seção Verificação.
* **O que a IA produziu:** o plano (gerado por IA) propôs
  `go list -deps ./internal/dominio/... | grep -vE '^[a-z0-9_/]+$' && exit 1`
  como asserção de que o domínio importa só a stdlib.
* **Por que estava errado:** o filtro aceita só caminhos minúsculos sem ponto,
  o que descreve a stdlib — mas os próprios pacotes do domínio
  (`github.com/gracianFelipe/caixa/internal/dominio/...`) têm ponto e maiúscula
  e aparecem na saída de `go list -deps`. A CI falharia sempre, e um comando
  que falha sempre é desligado, não corrigido.
* **Como foi pego:** ao rodar o comando pela primeira vez, no Dia 1, sobre
  quatro pacotes que comprovadamente só importam stdlib.
* **O que entrou no lugar:** excluir o módulo antes de filtrar:
  `| grep -v '^github.com/gracianFelipe/caixa/' | grep -vE '^[a-z0-9_/]+$'`.
  Está no `AGENTS.md`, no `README.md` e em `.github/workflows/ci.yml`.

## 002 — Construtor validava o meio de pagamento mas gravava o valor bruto

* **Onde:** `internal/dominio/lancamento/lancamento.go`, função `Novo`.
* **O que a IA produziu:**
  ```go
  if _, err := AnalisarMeio(string(d.Meio)); err != nil { return ..., err }
  // ...
  Meio: d.Meio,
  ```
  `AnalisarMeio` trima e minusculiza; o resultado era descartado e `d.Meio`
  original ia para o struct.
* **Por que estava errado:** `" PIX"` passava no construtor e estouraria o
  `CHECK (meio IN ('pix', ...))` no banco — a invariante que o domínio dizia
  garantir não era garantida. Compilava, os testes passavam (nenhum caso tinha
  espaço ou maiúscula), e o erro só apareceria em produção.
* **Como foi pego:** ao desenhar o handler HTTP e decidir se ele deveria
  normalizar o meio antes de chamar o domínio. A resposta certa era "não,
  isso é do domínio" — e ao olhar o domínio, ele não fazia.
* **O que entrou no lugar:** `meio, err := AnalisarMeio(...)` e `Meio: meio`,
  mais `TestNovoNormalizaMeio` para o caso nunca voltar.

## Decisões discutidas e mantidas (não são rejeições)

Registradas para mostrar que houve decisão, não omissão.

* **`Somar` sem verificação de estouro.** `int64` em centavos cobre 92
  quatrilhões de reais; toda entrada externa passa por `Analisar`, que verifica
  estouro na fronteira. Verificar em toda soma contaminaria o relatório com
  `error` que nunca dispara.
* **`"R$ 1,5"` recusado** em vez de interpretado. Um dígito decimal é ambíguo
  (5 ou 50 centavos); em dinheiro, recusar é mais seguro que adivinhar.
* **`stretchr/testify` aparece no `go.sum`** sem ser usado: é dependência de
  teste do `pgx`, e o `go.sum` registra hashes de todo o grafo. Nenhum
  `import` do repositório o usa; o contrato "sem testify" fala do nosso código.
