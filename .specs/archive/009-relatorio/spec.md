# 009 — Relatório mensal com cinco detectores

Status: concluída em 2026-09-17 — marco "pronto para entrevista"
Fase do plano: 5 — **marco "pronto para entrevista"**

## Objetivo

Responder a pergunta do produto: para onde o dinheiro foi, e o que está
errado. Cinco detectores determinísticos sobre uma janela de 7 competências
(alvo + 6 anteriores), relatório por HTTP e por Telegram, e o agendador que
manda o do mês fechado no dia 1 às 08:00 de São Paulo.

## Os cinco detectores (do plano, agora com bordas)

| Sinal | Regra executável |
|---|---|
| Vazamento por repetição | no mês alvo, por categoria: ≥ 8 saídas de até R$ 30, somando ≥ R$ 150 |
| Assinatura esquecida | mesma `contraparte_norm` em ≥ 3 competências consecutivas terminando no alvo, intervalo 28-32 dias entre ocorrências, variação de valor ≤ 5% |
| Categoria em escalada | soma mensal estritamente crescente por ≥ 3 meses até o alvo, alta ≥ 25% do primeiro ao último, alvo ≥ R$ 100 |
| Estouro de orçamento | gasto do alvo > limite vigente; severidade = gasto·100/limite |
| Gasto atípico | \|valor\| > mediana + 4·MAD das saídas dos 6 meses anteriores (≥ 10 amostras), e ≥ R$ 100 |

Mediana + MAD, não média + desvio: com 6 meses o próprio outlier contamina
média e desvio e o sinal deixa de disparar exatamente no caso que interessa.
Tudo em inteiros (`Centavos`); percentuais por multiplicação cruzada.

## Decisões

* `dominio/relatorio`: `Janela` (competência alvo, lançamentos confirmados
  da janela, limites vigentes), `type Detector func(Janela) []Sinal`,
  `Gerar(Janela) Relatorio` (totais, por categoria, variação vs. mês
  anterior, sinais). `Formatar(Relatorio, nomes) string` para o Telegram —
  puro, testável, sem `fmt` de float.
* **Golden files** (`testdata/*.golden.json`, `-update` para regravar):
  regressão visível em diff, não em asserção opaca. Um cenário sintético
  dispara os cinco detectores.
* **Agendador sem cron**: tique de 1 min no worker; quando "agora" em São
  Paulo já passou de dia 1 08:00, tenta `RegistrarSeNovo("relatorio",
  competência anterior)` — a tabela de alertas é a "última execução
  persistida": restart não duplica nem pula (se o worker estava fora no dia
  1, manda assim que voltar). "Dia 30" do requisito original não existe em
  fevereiro; dia 1 sobre o mês fechado é a correção do plano.
* `GET /relatorio/{competencia}` com `r.PathValue` (ServeMux 1.22) e
  `/relatorio [AAAA-MM]` no Telegram (mensagem de texto do dono).
* Portas novas: `DaJanela(ctx, de, ate)` em lançamentos (só confirmados),
  `Vigentes(ctx, comp)` em orçamentos (específico vence padrão por
  categoria, `DISTINCT ON`).

## Verificação

Tabela por detector com casos de borda (7 vs 8 repetições, 27/28/32/33
dias, 24%/25%, 9 vs 10 amostras); golden do cenário completo; httptest do
endpoint; integração de `DaJanela`/`Vigentes`; `curl.exe /relatorio/2026-09`
no dev.
