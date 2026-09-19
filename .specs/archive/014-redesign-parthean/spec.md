# 014 — Redesign do PWA no padrão Parthean

Status: concluída em 2026-09-19, verificada no navegador (claro/escuro, 375px e desktop, dados reais)
Pedido do autor: "o front não ficou legal", referência explícita
`parthean.com` (achada via minimal.gallery), skill ui-ux-pro-max.

## Leitura da referência (parthean.com, 2026-09-19)

* **Claro por padrão.** Fundo quase branco, painéis lavanda/periwinkle de
  raio grande (24px+), cartões brancos com borda fininha.
* **Serifada editorial nos títulos** + grotesca limpa na UI; números
  grandes com contexto pequeno embaixo ("$88,000" / "17.4%").
* **Títulos em pergunta:** "Where is my money invested?", "What is my tax
  allocation?" — o dado responde uma pergunta, não decora.
* **Donut de categorias** com legenda percentual + valor; **heatmap de
  calendário** com o gasto de cada dia; chips/pílulas para filtros; CTA
  única em pílula preta.

## Decisões (e o que foi rejeitado)

1. **Gerador da skill rejeitado em dois pontos** (registrar em
   revisao-ia): sugeriu navy escuro + fonte Calistoga via Google Fonts. A
   referência do autor é clara/lavanda, e a decisão da spec 010 (revisao-ia
   005) proíbe requisição a terceiro no PWA offline. Fica: **claro por
   padrão** (escuro continua via `prefers-color-scheme`) e **fontes de
   sistema** — display serifada `Georgia, 'Times New Roman', serif`,
   UI `system-ui`. Georgia é ubíqua, tem dígitos elegantes e custo zero.
2. **Sem biblioteca de gráfico** (contrato da spec 010 mantido): donut é
   SVG desenhado à mão (`stroke-dasharray` em círculo), heatmap é grid
   CSS. Ambos com alternativa textual para leitor de tela.
3. **Heatmap usa os lançamentos do mês** (`GET /api/lancamentos`), que a
   tela Lançamentos já consome — nenhum endpoint novo; o dashboard passa
   a buscar relatório + lançamentos em `Promise.all`.
4. **Donut limitado a 5 fatias** + "outros" (regra no-pie-overuse do
   próprio guia); cores categóricas pastel com 3:1 contra o fundo.
5. **Títulos em pergunta, em português:** "Para onde foi o dinheiro?"
   (categorias), "Quando saiu?" (heatmap), "O que precisa de você?"
   (pendentes/sinais).
6. A tela Mês ganha o **chip de pendentes** (131 hoje): um toque leva a
   Lançamentos já filtrado em "sem categoria" — o caminho de categorizar
   é o coração do produto.

## Entregas

| # | Arquivo | Conteúdo |
|---|---|---|
| 1 | `web/app/app.css` | Tokens novos (paleta Parthean clara + variante escura, raios 12/20/28, display serif, pílula preta como CTA), componentes reescritos. |
| 2 | `web/app/index.html` | `theme-color` claro + variante escura por media. |
| 3 | `web/app/telas/mes.{js,css}` | Dashboard: hero lavanda com saídas do mês + comparação, chip de pendentes, donut por categoria, heatmap diário, sinais, captura rápida mantida. |
| 4 | `web/app/telas/{login,lancamentos,relatorio,orcamentos}.css` | Ajustes para a linguagem nova (herdam quase tudo dos tokens). |
| 5 | `web/design-system/caixa/MASTER.md` | Overrides do topo reescritos para o padrão Parthean. |
| 6 | `docs/revisao-ia.md` | Entrada 007: gerador da skill contradito pela referência do usuário e pela decisão 005. |

## SEC-CHECK

Front puro: sem endpoint novo, sem dado novo. CSP inalterada (`'self'`,
sem inline script); SVGs continuam gerados por função própria, nunca de
dado da API. Valores monetários seguem indo só para o DOM, nunca para log.

## Verificação

`gofmt`/`vet`/`test` (embed recompila), navegador em 375px e desktop,
login → Mês com dados reais (240 lançamentos), modo claro e escuro,
`prefers-reduced-motion`.

## Adendo (2026-09-19): revisão no navegador do autor

Revisão visual no Chrome do autor (1555px, modo escuro) achou seis defeitos
que a verificação no painel embutido não pegou:

1. **Service worker servia CSS obsoleto.** `CACHE = 'caixa-v1'` fixo +
   cache-first puro: a tela Relatório continuou com a paleta anterior ao
   redesign. Corrigido com nome de cache versionado (`caixa-v2`) e
   **stale-while-revalidate** — responde do cache (offline continua
   funcionando) e atualiza em segundo plano. Custo assumido: quem já tinha
   o v1 precisa de um ciclo a mais para ver a versão nova.
2. **Cabeçalho fora do eixo do conteúdo.** Título colado na borda da janela
   enquanto os cartões ficavam centralizados. Agora `.cabecalho__interno`
   tem a mesma `max-width` e o mesmo padding do `#conteudo`.
3. **Cabeçalho translúcido** deixava o texto passar borrado por trás ao
   rolar. Fundo sólido: legibilidade acima do efeito.
4. **Barra inferior esticada por 1500px** no desktop. A partir de 1024px
   ela vira **barra lateral** (regra `adaptive-navigation` do guia). O
   recuo vai no `body`, não no cabeçalho/conteúdo — com padding neles, o
   `margin: 0 auto` continuava centralizando na janela inteira.
5. **`text-transform: lowercase` cascateando.** A regra era do título do
   dia mas estava na `<section>` do grupo: "R$" virava "r$" em todo valor
   monetário da tela Lançamentos. Movida para o próprio título.
6. **Hora falsa `00:00`.** O CSV não traz hora e o lançamento fica à
   meia-noite; exibir isso inventava precisão inexistente. A hora agora só
   aparece quando existe de verdade.

Mais: valor não quebra mais linha, `clamp()` no número do hero (a 3rem fixos
"R$ 12.345,67" não cabia em 375px) e os resumos de Relatório e Orçamentos
usam o mesmo painel lavanda do Mês.
