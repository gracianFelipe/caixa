# Deploy do Caixa

Um servidor pequeno basta: dois binários Go, um Postgres e o Caddy. O alvo
preferido é o free tier ARM da Oracle Cloud; qualquer VPS de ~R$ 25/mês serve
de plano B (o próprio plano prevê que o free tier vive sem capacidade).

## O que roda onde

| Serviço | O quê | Porta |
|---|---|---|
| `caddy` | TLS automático + HSTS + proxy | 80/443 públicas |
| `api` | HTTP + PWA embutido + WebSocket | 8080 só em loopback |
| `worker` | outbox → Telegram, agendador do relatório | — |
| `postgres` | dados | 5432 só em loopback |

## Passo a passo (servidor Linux com Docker)

```bash
git clone https://github.com/gracianFelipe/caixa.git && cd caixa/deploy

# 1. segredos (arquivo .env NUNCA commitado; ja coberto pelo .gitignore)
cat > .env <<FIM
CAIXA_BD_SENHA=...            # sem espaco nem aspas simples
CAIXA_USUARIO=felipe
CAIXA_SENHA_HASH=...          # gere na SUA maquina: go run ./cmd/caixactl senha
CAIXA_TELEGRAM_TOKEN=...
CAIXA_TELEGRAM_CHAT_ID=...
CAIXA_ATALHO_TOKEN=...        # 32+ chars aleatorios; vazio desliga o atalho
FIM
chmod 600 .env

# 2. subir tudo (postgres -> migrar -> api + worker)
docker compose up -d --build

# 3. Caddy (fora do compose ou dentro, a gosto). Com o pacote do sistema:
#    edite deploy/Caddyfile com seu dominio e:
sudo cp Caddyfile /etc/caddy/Caddyfile && sudo systemctl reload caddy
```

DNS do domínio apontando para o servidor é pré-requisito do TLS automático.

## Cross-compile do Windows (deploy sem Docker)

Go cruza sem toolchain C — CGO está desligado no projeto inteiro:

```powershell
$env:GOOS="linux"; $env:GOARCH="arm64"   # amd64 para VPS x86
go build -trimpath -o api ./cmd/api
go build -trimpath -o worker ./cmd/worker
go build -trimpath -o caixactl ./cmd/caixactl
```

Copie os três binários; `caixactl migrar` primeiro, depois `api` e `worker`
como serviços systemd (`Restart=always`, envs num `EnvironmentFile` com
`chmod 600`).

## Backup

Diário, com retenção de 14 dias, no cron do host:

```bash
# /etc/cron.daily/caixa-backup
docker exec caixa-postgres pg_dump -U caixa -Fc caixa \
  > /var/backups/caixa/caixa-$(date +%F).dump
find /var/backups/caixa -name 'caixa-*.dump' -mtime +14 -delete
```

Backup que nunca foi restaurado não é backup — ensaio de restauração:

```bash
docker exec -i caixa-postgres pg_restore -U caixa -d caixa_teste --create < caixa-AAAA-MM-DD.dump
```

## Atualização

```bash
git pull && docker compose up -d --build   # migrar roda de novo; e idempotente
```

## Pendências conhecidas

* O Docker Desktop da máquina de desenvolvimento (Windows) está quebrado
  (falha ao criar socket em `AppData\Local`); o `docker build` da CI e este
  runbook são a validação até o primeiro deploy real.
* Smoke pós-deploy: `curl -fsS https://SEU_DOMINIO/saude` e um `/relatorio`
  no Telegram.
