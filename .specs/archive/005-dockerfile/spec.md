# 005 — Dockerfile multi-stage e compose completo

Status: concluída em 2026-09-17 (build validado na CI; runtime fica para a Fase 8)
Fase do plano: 2 (última: 002 ✔ · 003 ✔ · 004 ✔ · **005 Docker**)

## Objetivo

Empacotar `api` e `caixactl` numa imagem mínima e fechar o compose com a
aplicação junto do Postgres — migração roda como serviço one-shot antes da
API subir. Docker é requisito literal da vaga; a imagem é a base do deploy
da Fase 8.

## Decisões

* **Multi-stage**: `golang:1.27` compila com `CGO_ENABLED=0` (binário
  estático — o repo não usa cgo); runtime é
  `gcr.io/distroless/static-debian12:nonroot`: sem shell, sem gerenciador de
  pacotes, usuário não-root, CA certs inclusos (o cliente Telegram da Fase 3
  vai precisar de TLS). tzdata não é necessário na imagem: os binários já
  embutem `time/tzdata`.
* **Migração no compose como serviço one-shot** (`caixactl migrar`) com
  `depends_on: condition: service_completed_successfully` na API — o padrão
  de produção em miniatura, e o migrador já é idempotente e serializado por
  advisory lock.
* **Validação pela CI**, não local: o Docker Desktop desta máquina está
  quebrado (falha ao criar socket em AppData, registrada na spec 001). A CI
  faz `docker build` a cada push; smoke de runtime fica para a Fase 8.

## Entregas

1. `deploy/Dockerfile` — dois estágios, dois binários (`/api`, `/caixactl`),
   `ENTRYPOINT ["/api"]`.
2. `.dockerignore` — contexto mínimo; extratos e segredos nunca entram na
   imagem (Secrets do SEC-CHECK).
3. `deploy/docker-compose.yml` — serviços `postgres` (healthcheck), `migrar`
   (one-shot) e `api` (porta 8080, env `CAIXA_BD_URL` montada com a mesma
   `CAIXA_BD_SENHA`).
4. CI: passo `docker build`.

## Verificação

```powershell
docker compose -f deploy/docker-compose.yml build   # onde houver Docker
docker compose -f deploy/docker-compose.yml up -d   # api responde /saude
```

Nesta máquina: CI verde no passo `docker build` substitui os dois até a
Fase 8.
