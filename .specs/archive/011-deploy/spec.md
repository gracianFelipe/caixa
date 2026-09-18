# 011 — Deploy: compose completo, Caddy e runbook

Status: concluída em 2026-09-18 (runtime real pendente do primeiro deploy; CI valida build e compose)
Fase do plano: 8 (adiantada sobre a 7: IMAP é a última premissa não
verificada e fica por último, como o plano manda)

## Objetivo

`git clone && docker compose up` num servidor limpo = sistema inteiro de pé:
Postgres, migração, API+PWA e worker, atrás de Caddy com TLS automático.
Fecha os achados de deploy da auditoria (worker ausente da imagem/compose,
sem política de restart, senha crua interpolada em URL).

## Decisões

* **Worker entra na imagem e no compose.** Mesma imagem para os três
  binários (`/api`, `/worker`, `/caixactl`); o serviço escolhe o entrypoint.
  Um usuário não justifica imagem por serviço.
* **`restart: unless-stopped` em tudo**: reboot do host ou panic volta
  sozinho. `stop_grace_period` respeita o shutdown gracioso.
* **DSN keyword em vez de URL** no compose: `host=postgres user=caixa
  password=${CAIXA_BD_SENHA} ...` — URL exigiria percent-encoding da senha
  e `@`/`#`/`/` quebrariam a conexão em silêncio (achado da auditoria).
  Restrição documentada: senha sem espaço nem aspas simples.
* **Healthcheck HTTP fica no Caddy** (proxy só encaminha para upstream são);
  distroless não tem shell para healthcheck de container na API. Postgres
  mantém `pg_isready`.
* **Caddyfile**: TLS automático (Let's Encrypt), HSTS, encaminhamento do
  WebSocket, compressão. HSTS mora aqui — a API não manda HSTS em texto
  puro no dev.
* **Runbook `docs/deploy.md`**: Oracle ARM (`GOOS=linux GOARCH=arm64` sem
  toolchain C) ou VPS, envs obrigatórias, backup `pg_dump` diário com
  retenção, restauração testável, atualização.
* **Fase 9 (SQS/S3) não será feita** — decisão registrada: o valor seria
  provar que as portas existiam antes do segundo adaptador, e isso já está
  provado (fakes em memória + Postgres + Telegram implementam as mesmas
  portas). YAGNI vence o apêndice.

## Verificação

CI: `docker build` (já roda) + `docker compose config` validando o
arquivo. Runtime completo no servidor do autor (Docker local segue
quebrado; pendência aberta no runbook).
