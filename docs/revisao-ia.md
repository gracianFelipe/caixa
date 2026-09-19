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

## 003 — A correção da verificação de dependência também estava errada

* **Onde:** a mesma verificação do caso 001 (`AGENTS.md`, CI, README, skills).
* **O que a IA produziu:** a correção do caso 001 manteve a abordagem por
  regex (`grep -vE '^[a-z0-9_/]+$'`), só excluindo o módulo antes.
* **Por que estava errado:** a stdlib do Go moderno tem pacotes internos com
  ponto e versão no caminho — `crypto/sha256` (usado pela `ocorrencia` para a
  impressão) puxa `crypto/internal/entropy/v1.0.0`, que o regex marca como
  "fora da stdlib". Classificar stdlib por formato de caminho é premissa
  frágil: o formato mudou e a asserção quebrou.
* **Como foi pego:** pela própria CI, no push da spec 003 — a asserção
  executável fez exatamente o papel dela.
* **O que entrou no lugar:** perguntar ao toolchain, que é quem define o que
  é stdlib: `go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}'`,
  restando ao grep só excluir o próprio módulo. Lição registrada: quando a
  ferramenta expõe o critério (`.Standard`), usar o critério, não uma
  aproximação textual dele.

## 004 — Auditoria multiagente: 36 suspeitas, triagem manual, 12 correções

* **Onde:** repo inteiro (spec 010).
* **O que aconteceu:** uma revisão com 7 agentes-lente levantou 36 suspeitas;
  a fase de verificação adversarial caiu por limite de sessão, então a
  triagem foi manual. Aproximadamente um terço era real e virou correção:
  `defer conn.Hijack().Close(...)` avaliava o Hijack na hora do defer e
  sequestrava a conexão antes do LISTEN (pego também no smoke: panic);
  UNIQUE total de perguntas impedia a segunda pergunta legítima do mesmo
  lançamento (migração 007: índice parcial em `estado='aberta'`); alerta e
  relatório registravam a idempotência ANTES do envio (falha transitória
  perderia o aviso para sempre — agora há compensação com `Remover`);
  callback reentregue agia sobre pergunta já respondida; evento envenenado
  travava a fila (tentativas + corte em 5); `Sscanf` aceitava sinal em
  "AAAA-MM"; regex de regra era sensível a caixa contra texto maiúsculo;
  MAD zero marcava gasto normal como atípico; truncamento de 24h quebrava a
  assinatura em fevereiro (arredonda agora); "TED" casava dentro de
  LIMITED (palavra inteira agora); erro de `http.NewRequest` ecoava a URL
  com o token; API escutava em todas as interfaces por padrão (loopback
  agora). O restante era falso positivo ou preferência sem consequência.
* **Lição:** gerador e revisor erram nos mesmos lugares que humanos —
  fronteiras, defer, idempotência. A triagem humana continua sendo o filtro.

## 005 — Front vanilla no lugar do Next.js do plano

* **Onde:** `web/app` (spec 010).
* **O que o plano (gerado por IA) previa:** PWA Next.js `output: 'export'`.
* **Por que foi rejeitado:** a vaga é backend; React/Next já está no
  currículo do autor; o export do Next arrastaria Node para o repo, o
  Dockerfile e a CI. HTML+CSS+JS a mão mantém `go build` como única
  toolchain e cada linha do front tão explicável quanto as do back. O
  design veio da skill ui-ux-pro-max com overrides registrados no
  MASTER.md (fonte de sistema, sem CDN — o gerador sugeria Google Fonts e
  tipografia de landing page, rejeitados: app financeiro offline não faz
  requisição a terceiros).

## 006 — Parser planejado para um formato que o banco não exporta

**Quando:** 2026-09-19, spec 013.

**O que o plano (escrito com IA) assumia:** importação de extrato por OFX,
formato clássico de conciliação bancária. A spec 003 construiu o tokenizador
SGML completo, com fuzz, sobre fixtures sintéticas — a regra "não escrever o
parser sem ver o arquivo real" foi conscientemente flexibilizada quando o
autor autorizou execução contínua.

**O que a realidade mostrou:** o Internet Banking do Bradesco do autor não
oferece exportação OFX. Só CSV. O risco não era "o parser está errado"; era
"o formato de entrada não existe para este usuário". Nenhuma quantidade de
fuzz pega isso: é risco de premissa, não de código.

**E o CSV real ainda ensinou o que fixture nenhuma continha:** o arquivo
termina com uma seção "Últimos Lancamentos" que REPETE os movimentos finais
da tabela principal. A primeira versão do leitor (desta sessão, também
gerada com IA) teria importado a repetição como lançamentos novos — o
ordinal anti-colapso viraria gerador de duplicata. Pego ao rodar contra o
arquivo real antes do commit; virou teste (`TestAnalisarCSVIgnoraRecapitulacao`)
e regra: a leitura para no segundo cabeçalho.

**Lição:** validar cedo contra o dado real não é etapa de polimento, é a
única defesa contra premissa errada. O tokenizador OFX fica no repo — código
testado, custo zero, e outra conta bancária pode exportar OFX amanhã.

**Adendo (2026-09-19, um dia depois): a correção da recapitulação também
estava errada** — eco direto do caso 003. A regra "descarte a segunda
tabela" nasceu de UM arquivo, em que ela repetia a principal. O export
seguinte provou o contrário: a tabela principal do Bradesco atrasa dias, e
a recapitulação era a única portadora dos movimentos de 18-21/09 (inclusive
um PIX agendado futuro). Descartá-la perdia os lançamentos mais recentes;
o banco somava R$ 694,50 contra R$ 210,23 do extrato. Regra final:
recapitulação é lida com abate — tupla já vista na principal é repetição,
tupla inédita é movimento. Auditoria que pegou: somar os lançamentos e
conferir contra o saldo real informado pelo autor. Dois exports diferentes
ainda ensinaram que o Bradesco RENOMEIA históricos entre exports
("PIX QR CODE DINAMICO" → "TRANSFERENCIA PIX"), o que inviabiliza dedup
textual entre arquivos — a operação registrada no README passa a ser:
exportar sempre o período completo e conferir a soma contra o saldo.

## 007 — Gerador de design contradito pela referência do usuário

**Quando:** 2026-09-19, spec 014 (redesign do PWA).

**O que a skill ui-ux-pro-max sugeriu:** para "personal finance dashboard",
paleta navy escuro (#0F172A) com azul/verde e fonte display Calistoga via
Google Fonts.

**Por que foi rejeitado:** (1) o autor apontou uma referência concreta —
parthean.com — que é o oposto: clara, quase branca, painéis lavanda,
serifada editorial. Recomendação genérica não vence referência escolhida
pelo usuário. (2) Calistoga viria de CDN, e a decisão da spec 010
(revisao-ia 005) proíbe requisição a terceiros no PWA offline — a regra
antiga venceu a sugestão nova, que nem sabia da restrição.

**O que ficou:** paleta Parthean com tokens próprios nos dois modos, e
Georgia (serifada de sistema, presente em todo SO) no lugar de fonte de
CDN. O custo: Georgia é menos distintiva que uma serifada display; o
benefício: zero requisição externa e zero FOIT. Trade-off consciente.

**Adendo — o redesign passou no meu teste e falhou no do autor.** Verifiquei
o redesign no navegador embutido e dei por bom. Aberto no Chrome do autor,
em tela larga, seis defeitos apareceram de uma vez: cabeçalho fora do eixo
do conteúdo, barra inferior esticada por 1500px, `text-transform` do título
do dia cascateando e transformando "R$" em "r$" em toda a tela de
Lançamentos, hora `00:00` inventada para dado que não tem hora, e — o pior
— o service worker com cache-first e nome de cache fixo servindo o CSS
anterior ao redesign. Nenhum deles aparece em 375px, que foi onde testei.
Lição registrada: verificação de front precisa rodar no ambiente real do
usuário (largura, tema do SO, service worker já instalado), não só no
viewport que eu escolhi.

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
