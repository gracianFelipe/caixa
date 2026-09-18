# 008 — Orçamentos, alertas e a resposta de conciliação

Status: concluída em 2026-09-17
Fase do plano: 4 (segunda de duas: 007 ✔ · **008**)

## Objetivo

Fechar a Fase 4: limite por categoria com alerta de 80%/100% que chega UMA
vez (UNIQUE `(tipo, chave)`), e o teclado "mesmo gasto / gasto novo" que
resolve o lançamento provisório da faixa 60-84.

## Decisões

* **Callback carrega o id da PERGUNTA**, não os dois lançamentos: dois UUIDs
  estouram os 64 bytes do Telegram (`con:<uuid>:<uuid>:s` = 79). A pergunta
  ganha `tipo` (`categoria|conciliacao`) e `referencia` (o candidato), e o
  callback vira `con:<perguntaID>:s|n` (42 bytes). O id da pergunta é gerado
  ANTES do envio para viajar no botão.
* **O worker recalcula os candidatos do provisório** ao perguntar, em vez de
  a importação guardar o vencedor: a conciliação é determinística, então
  recalcular dá o mesmo resultado — sem estado novo, sem coluna nova no
  lançamento. Se o candidato tiver sumido (descartado), o provisório é
  confirmado sem incomodar.
* **"Mesmo gasto"** move as evidências do provisório para o vencedor e marca
  o provisório `descartado` (a linha fica — apagar sem apagar), numa
  transação. **"Gasto novo"** confirma; se ainda sem categoria, a pergunta de
  categoria sai na hora.
* **Alerta é idempotente por constraint**: `RegistrarSeNovo` com
  `ON CONFLICT DO NOTHING`; o aviso de 80% não vai 14 vezes. Chave =
  `categoria:competencia:limiar`. Gasto conta só `confirmado` e só saídas.
* **Orçamento com `competencia NULL` = padrão do mês** (`UNIQUE NULLS NOT
  DISTINCT`, PG15+); o vigente é o específico se existir, senão o padrão.
* Definição de limite via `caixactl orcamento -categoria N -limite "R$ X"
  [-competencia AAAA-MM]` — endpoint HTTP fica para o PWA (Fase 6).
* Porta `MensageiroDeCategoria` vira `Mensageiro` (pergunta categoria,
  pergunta conciliação, aviso) — continua no consumidor.

## Verificação

Unit: `orcamento.Nivel` (bordas 79/80/99/100), fila com fakes (alerta uma
vez, fusão, confirmação com pergunta de categoria imediata); integração:
UNIQUE de alerta, limite vigente específico>padrão, `FundirProvisorio`
atômico. `caixactl orcamento` ponta a ponta no dev.
