# 015 — Higiene de dados reais em specs, docs e testes

Status: em execução
Origem: auditoria externa de vazamentos (sessão separada, `C:\scripts\vagas`),
relatório entregue pelo autor em 2026-10-01.

## Motivo

O repositório é **público** e vai ser linkado no LinkedIn do autor. A
auditoria não encontrou credencial vazada, mas encontrou **dados
financeiros pessoais do autor**, vindos do extrato real do Bradesco, em
specs e documentação — e o nome de usuário de login de produção num exemplo
de `.env`.

Tudo está na ponta da `main` e em commits de 17 a 19/09/2026. Repositório
público: o conteúdo pode já ter sido lido, clonado ou indexado.

## Regra que passa a valer (também no AGENTS.md)

Spec, doc, teste, mensagem de commit e fixture usam **apenas dados
sintéticos**. Nunca linhas, totais, contagens ou saldos do extrato real.
Revisão feita por IA cita divergência em termos **relativos** ("a soma do
banco divergia da do extrato"), nunca com os valores.

Este documento segue a própria regra: cada achado é referido por
arquivo:linha e por tipo de dado, sem reproduzir valor.

## Achados e tratamento

| # | Gravidade | Onde | O que sai |
|---|---|---|---|
| F1 | média | `.specs/archive/013-extrato-csv-bradesco/spec.md` linhas 20 e 25-29 | Linhas de movimentação reais (data, nº de documento do banco, valor, saldo corrente) e linha de Total (soma de créditos, soma de débitos, saldo final) trocadas por linhas sintéticas no mesmo formato, no padrão da fixture `sintetico_bradesco.csv`. Contagem real de linhas do arquivo vira descrição genérica. |
| F1 | média | mesma spec, linhas 42-47 | A lista deixa de ser "históricos **reais observados**" e passa a ser vocabulário do banco: não revela que a conta do autor tem empréstimo pessoal, parcela de crédito pessoal e conta-salário. |
| F2 | baixa | `docs/revisao-ia.md` (adendo do caso 006) | As duas somas reais (banco × extrato) viram divergência relativa; sai a menção ao intervalo de datas e ao PIX agendado futuro. |
| F2 | baixa | `README.md` linha 86 | Contagem real de lançamentos importados vira descrição genérica. |
| F5 | baixa | `docs/deploy.md` linha 24 | Usuário de login de produção vira placeholder. |
| F5 | baixa | `internal/aplicacao/acesso_test.go` (6 ocorrências) | Mesmo valor vira `usuario_teste`. |
| F4 | melhoria | `.gitignore` | `*.csv`, `*.ofx`, `*.qfx` e `*.eml` ignorados, com exceção explícita só para as fixtures sintéticas. Hoje `*.csv` não é ignorado: um extrato real exportado entra num `git add` por descuido. |
| F3 | sem ação | CI, testes | Senha do Postgres efêmero do CI, tokens falsos e DSN fictícia. Nenhum tem formato de token real e o compose exige senha por variável, sem default. |

Os testes que classificam `EMPRESTIMO PESSOAL` e `PARCELA CREDITO PESSOAL`
(`csv_bradesco_test.go`) **ficam**: são vocabulário público do Bradesco e,
sem a afirmação "observado na conta real" da spec 013, não dizem nada sobre
a conta do autor.

## Fase 2 — histórico do git

Reescrita com `git filter-repo --replace-text` numa cópia-espelho fora do
repositório de trabalho, substituindo cada dado real pelo sintético
correspondente (não por `***REMOVIDO***`: assim os commits antigos seguem
legíveis). O arquivo de substituições carrega valores reais, então vive em
pasta temporária fora do repo e é apagado no fim.

Critério para incluir um valor na lista: ele tem de aparecer **somente** nos
arquivos esperados, verificado em todas as revisões. Valor curto (um saldo,
um total) pode coincidir com número legítimo em golden file ou teste — por
isso a substituição é feita por **linha inteira** do CSV, que é única, e não
por número solto.

O force-push só acontece com confirmação explícita do autor no chat.

## Fase 3 — fora do alcance do repositório

Commits antigos continuam acessíveis por SHA até o GitHub recolher os
órfãos: exige chamado no GitHub Support. Forks, se existirem, mantêm o
histórico. Detalhes e texto do chamado entregues ao autor no chat.

## SEC-CHECK

* **Secrets** — nenhuma credencial entra ou sai do repo nesta spec; o
  usuário de produção deixa de ser público (rotação é decisão do autor).
* **Logging** — o SEC-CHECK já proibia valor e contraparte em log. Esta
  spec estende a mesma regra a **spec, doc e mensagem de commit**, que
  estavam fora do alcance do checklist e foi por onde o dado escapou.
* **Fail secure** — pendência levantada pela auditoria: conferir se o login
  tem limite de tentativas. Se não tiver, abre spec própria.

## Verificação

```powershell
go test ./... ; go vet ./...
gitleaks detect --no-git -v        # se instalado localmente
```
Mais: busca pelos valores reais em todas as revisões deve voltar vazia
depois da Fase 2 (script que imprime só contagens).
