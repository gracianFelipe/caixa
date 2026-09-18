# 010 — PWA embutida, login e atualização ao vivo

Status: concluída em 2026-09-18 (verificada no navegador: login, 4 telas, categoria em 1 toque, captura rapida, ao vivo, 375px)
Fase do plano: 6

## Objetivo

Uma interface para o dono ver o mês no celular: totais, por categoria, sinais,
lançamentos, relatório e orçamentos — servida pelo próprio binário `api`
(`embed.FS`), atrás de login, com atualização ao vivo quando um lançamento
novo entra por qualquer origem.

## Decisões (e os desvios do plano)

* **Sem Next.js, sem build.** O plano previa Next `output: 'export'`. Aqui
  entra HTML + CSS + JS (ES modules) escritos à mão em `web/app/`, embutidos
  direto: `go test ./...` e o Dockerfile continuam sem Node, e cada linha do
  front é tão explicável quanto a do back. A vaga é backend; React já está no
  currículo do autor. Desvio registrado em `docs/revisao-ia.md` (decisão
  discutida). Toolchain Node existe na máquina, mas não entra no repo.
* **Design system gerado com a skill `ui-ux-pro-max`** e persistido em
  `web/design-system/caixa/MASTER.md`, com overrides do projeto (fonte de
  sistema, sem CDN, dashboard e não landing). Escuro por padrão.
* **PWA de verdade:** `manifest.webmanifest`, ícone SVG, service worker com
  cache-first para os assets estáticos e network-only para `/api`.
* **Rotas HTTP ganham prefixo `/api/`** (`/api/lancamentos`, `/api/relatorio/…`);
  as antigas sem prefixo continuam por uma versão como alias, para o Atalho do
  iOS não quebrar. A raiz serve o app com fallback SPA (`/`, `/mes`, `/lancamentos`
  → `index.html`; asset inexistente → 404 de verdade).
* **Login argon2id + sessão em cookie.** Usuário único: `CAIXA_USUARIO` e
  `CAIXA_SENHA_HASH` (PHC gerado por `caixactl senha`) no ambiente. Sessão
  opaca de 32 bytes aleatórios, gravada em `sessoes` com expiração (30 dias,
  renovada a cada uso), cookie `HttpOnly; Secure; SameSite=Strict; Path=/`.
  `Secure` cai quando `CAIXA_HTTP_INSEGURO=1` (dev em `localhost`). Logout
  apaga a sessão no banco. Comparação de usuário em tempo constante; hash
  sempre verificado mesmo com usuário errado (sem oráculo de existência).
* **Rate limit no login**: balde por IP em memória (5 tentativas / minuto),
  stdlib. Único endpoint público além de `/saude` e do atalho por token.
* **Origin check** em todo método mutável autenticado: `Origin`/`Referer`
  precisa bater com o host — defesa em profundidade sobre o `SameSite=Strict`.
* **Cabeçalhos**: CSP estrita (`default-src 'self'`; sem inline script —
  o JS é externo), `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`,
  `Permissions-Policy` desligando câmera/geo/microfone, HSTS só atrás de TLS
  (Fase 8 no Caddy).
* **Ao vivo com WebSocket + `LISTEN/NOTIFY`.** A vaga pede WebSocket; SSE
  bastaria mas não é o que se avalia. Lib mínima `github.com/coder/websocket`
  (adaptador pode importar lib). Ponte: gatilho `AFTER INSERT ON eventos`
  faz `pg_notify('caixa_eventos', tipo)`; a API mantém UMA conexão dedicada em
  `LISTEN` e o hub retransmite `{"tipo":"lancamento_criado"}` a todos os
  clientes autenticados; o front reage refazendo a consulta da tela.
  Cross-processo de graça: `caixactl importar` e o worker também acordam a
  tela. Hub testado com `-race` na CI; reconexão exponencial no cliente.
* **Mutações que o front precisa** e que ainda não existiam: `PUT
  /api/lancamentos/{id}/categoria` (categoria manual — reaproveita
  `AtribuirCategoria` + regra aprendida, o mesmo caminho do Telegram) e `PUT
  /api/orcamentos` (define limite). Só isso; nada de CRUD genérico.

## Entregas

1. Migração `006_sessoes_notify.sql` (tabela `sessoes`, função + gatilho de
   `pg_notify`).
2. `internal/adaptadores/entrada/web`: `autenticacao.go` (login/logout/sessão,
   rate limit, origin check), `cabecalhos.go` (CSP e cia.), `estatico.go`
   (embed + SPA fallback), `tempo_real.go` (hub WS), rotas `/api/*` +
   aliases, `PUT` de categoria e orçamento.
3. `internal/adaptadores/saida/postgres`: `sessoes.go`, `notificacoes.go`
   (LISTEN com reconexão).
4. `internal/aplicacao`: `Acesso` (login/validar sessão), `Categorizar` manual
   exposto, `Orcamentos.Definir`. Portas: `RepositorioDeSessoes`,
   `Notificacoes`, `VerificadorDeSenha` (argon2 fica no adaptador
   `saida/senha`).
5. `cmd/caixactl senha` — gera o PHC argon2id a partir de senha digitada no
   terminal (sem eco; `golang.org/x/term`), nunca por argumento de linha de
   comando (histórico do shell).
6. `web/app/`: `index.html`, `app.css` (tokens do MASTER.md), `app.js`
   (roteador por `history`, cliente `/api`, WS), telas `mes`, `lancamentos`,
   `relatorio`, `orcamentos`, `login`; `manifest.webmanifest`, `sw.js`,
   ícones SVG. Vanilla, sem dependência.
7. Testes: unitários de hub (broadcast, cliente lento não trava os outros,
   `-race`), sessão (expiração, renovação, cookie flags), rate limit, CSP
   presente, SPA fallback vs 404 de asset, login com senha errada e com
   usuário inexistente levam o mesmo tempo (ordem de grandeza); integração do
   LISTEN/NOTIFY (INSERT em eventos acorda o listener).
8. Verificação no navegador embutido do Claude: fluxo login → mês → alterar
   categoria → orçamento → relatório em 375px e desktop, claro e escuro.

## Fora de escopo

Multiusuário, recuperação de senha, edição de valor/data de lançamento
(imutável por decisão de domínio), gráficos de biblioteca, notificações push.

## Verificação

```powershell
go vet ./... ; go test -race ./... (CI) ; go test ./... (local)
. .\local.ps1 ; go run ./cmd/caixactl migrar ; go run ./cmd/caixactl senha
go run ./cmd/api   # abre http://localhost:8080 → login → telas
```
