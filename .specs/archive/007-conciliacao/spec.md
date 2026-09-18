# 007 — Conciliação por pontuação

Status: concluída em 2026-09-17 (pergunta de conciliação no Telegram fica na 008)
Fase do plano: 4 (primeira de duas: **007 conciliação** · 008 orçamentos)

## Objetivo

Nível 2 da deduplicação: o mesmo gasto chegando por fontes diferentes vira
UMA linha com duas evidências. `Lancamento` é o fato, `Ocorrencia` é a
evidência; conciliar anexa, nunca apaga.

## A pontuação (do plano, agora executável)

```
valor identico ao centavo            → +50  (sem isso, nem candidato)
datas: 0d +25 | 1d +18 | 2d +12 | 3d +6 | >3d descarta
similaridade(contraparte) >= 0.90    → +25  (Jaccard sobre trigramas)
mesmo meio                           → +10
origens diferentes                   → +5
mesma origem                         → -20

>= 85 concilia sozinho · 60-84 pergunta no Telegram · < 60 lancamento novo
```

## O caso que quebra implementações ingênuas

Dois cafés idênticos no mesmo dia, no mesmo extrato: pontuariam 90 e a
segunda compra sumiria. A defesa não é o score — é o **filtro de
candidatos**: só concorre o lançamento que **ainda não tem evidência da
origem que está chegando**. O primeiro café ganha a evidência do extrato;
o segundo café não encontra candidato elegível e vira lançamento novo.
Teste com nome explícito prova exatamente essa cena.

## Decisões

* `dominio/conciliacao`: `Pontuar(nova Evidencia, candidato Candidato) int`
  e `Decidir(pontos) Decisao` (concilia/pergunta/novo), puros. Jaccard sobre
  trigramas da contraparte normalizada (~30 linhas, com testes de borda:
  strings curtas, iguais, vazias).
* Faixa 60-84 **nesta spec cria o lançamento como `provisorio`** e registra
  pergunta de conciliação; o teclado no Telegram ("mesmo gasto"/"gasto novo")
  entra na 008 junto do fluxo de alertas para não inchar esta. Até lá, a
  competência lista o provisório normalmente (produto continua íntegro:
  nada some, nada duplica silenciosamente).
* `situacao` sobe para o domínio (`provisorio|confirmado|descartado`);
  conciliar marca a ocorrência como `conciliou` e aponta para o fato antigo.
* Porta nova: `CandidatosParaConciliacao(ctx, valor, instante, origem)` —
  SQL devolve lançamentos de valor idêntico a ≤3 dias, não descartados, SEM
  ocorrência da origem que chega, com as origens das evidências existentes.
* `AnexarEvidencia(ctx, o, lancamentoID)`: grava ocorrência `conciliou`
  apontando para o fato existente (mesma transação; sem lançamento novo).

## Verificação

Tabela ~25 casos no domínio (inclui empate de fronteira 84/85 e 59/60);
integração: importar extrato com gasto que já existe por outra origem →
concilia; dois iguais no mesmo arquivo → duas linhas.
