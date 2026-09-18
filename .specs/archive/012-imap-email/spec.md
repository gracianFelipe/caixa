# 012 — E-mail do banco por IMAP

Status: concluída em 2026-09-18 (fixtures sintéticas; validação com .eml reais e credenciais IMAP pendente do autor)
Fase do plano: 7 — a última premissa não verificada

## Objetivo

O gatilho automático: o alerta de movimentação que o Bradesco manda por
e-mail vira lançamento minutos depois do gasto, sem o dono fazer nada. O
worker consulta a caixa por IMAP a cada 2 minutos, com a última UID
persistida — restart não relê nem pula.

## O risco número 1 do plano, por escrito

Nunca foi verificado que o e-mail do Bradesco (a) existe para toda
movimentação e (b) traz valor e contraparte no corpo. O parser nasce sobre
**fixtures sintéticas** com os formatos plausíveis (texto plano,
multipart com HTML, quoted-printable, ISO-8859-1); a validação com
mensagens reais é pendência do autor: exportar 2-3 `.eml` reais para
`extratos/` (já ignorado pelo git) e ajustar os rótulos do parser. Se o
e-mail real não trouxer valor+contraparte, esta origem morre e a espinha
continua sendo o OFX — o plano já precificou isso.

## Decisões

* **`go-imap/v2` só para o transporte.** O parsing do `.eml` é 100% stdlib
  (`net/mail`, `mime`, `mime/multipart`, `mime/quotedprintable`,
  `encoding/base64`) — testável sem rede e sem a lib.
* **UID + UIDVALIDITY persistidas** (`imap_estado`, migração 008, linha
  única com `CHECK (id = 1)`). UIDVALIDITY mudou = o servidor renumerou:
  recomeça do zero SEM reimportar duplicado, porque a idempotência por
  `Message-Id` (id externo) e impressão já segura a reentrada.
* **Filtro por remetente** (`CAIXA_IMAP_REMETENTE`, ex.:
  `@infobradesco.com.br`): o resto da caixa não é lido — o parser nunca vê
  e-mail pessoal.
* **Item que não parseia não derruba o lote**: conta como ignorado e o log
  leva UID e motivo, nunca o conteúdo (PII).
* **`OcorridoEm` = header `Date`** do e-mail (o alerta chega em segundos);
  corpo com data explícita pode refinar depois dos exemplos reais.
* **Conciliação de graça**: o e-mail cria o fato na hora; quando o OFX do
  mesmo gasto chegar dias depois, o nível 2 (spec 007) anexa a evidência em
  vez de duplicar — este é o fluxo que justificou o modelo fato/evidência.
* **Envs**: `CAIXA_IMAP_SERVIDOR` (host:993, TLS implícito),
  `CAIXA_IMAP_USUARIO`, `CAIXA_IMAP_SENHA` (senha de app, digitada pelo
  autor no `local.ps1`), `CAIXA_IMAP_REMETENTE`. Ausentes = laço desligado
  (worker segue só com Telegram/outbox).
* **Cliente IMAP sem teste unitário** (documentado): é cola fina sobre a
  lib; o que tem lógica — parser, estado, laço de decisão — é testado.

## Verificação

Parser com tabela de fixtures sintéticas (texto puro, HTML+texto,
quoted-printable, latin-1, sem valor → erro); integração do estado IMAP;
`go vet`/CI. Smoke real pendente das credenciais do autor.
