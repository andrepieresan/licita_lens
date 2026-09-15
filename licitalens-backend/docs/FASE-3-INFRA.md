# Fase 3 — Infraestrutura e operação

## Subir localmente (paridade com produção)

```bash
make phase3-up
make phase3-verify
```

Inclui: Postgres, Redpanda, MinIO, ingestão PNCP, procurement, gateway, worker de alertas, Keycloak, Mailpit, `restart: unless-stopped`, backup inicial e validação de readiness/métricas/dados PNCP.

## Readiness

`GET /health/ready` retorna:

```json
{"status":"ready","checks":{"api":"ok","postgres":"ok"}}
```

Se o Postgres estiver indisponível: HTTP 503 e `"status":"degraded"`.

## Backups

```bash
make backup-db
make restore-db FILE=.data/backups/licitalens-....sql.gz   # interativo
```

Agende `backup-postgres.sh` no cron do servidor (ex.: diário 03:00 UTC). Teste restore em ambiente isolado pelo menos uma vez por trimestre.

## Domínio e TLS (nginx)

1. DNS: `app.`, `api.`, `auth.` → IP do servidor.
2. **nginx no host:** `deploy/nginx/licitalens.conf.example` → `/etc/nginx/sites-available/`, `certbot --nginx`.
3. **nginx no Docker:** serviço `nginx` em `compose.prod.yaml` + `deploy/nginx/licitalens.docker.conf`.
4. Guia completo: [`deploy/nginx/README.md`](../deploy/nginx/README.md).
5. Variáveis: `.env.prod.example` → `.env.prod.local` (`ALLOWED_ORIGINS`, `KEYCLOAK_ISSUER` com HTTPS público).

## Kubernetes

```bash
helm lint deploy/helm/licitalens
# Ajuste values: image.repository, ingress.host, secretName
```

Ingress TLS e probes de readiness já estão no chart.

## Observabilidade

- Métricas Prometheus: `GET /metrics` no gateway.
- Regras exemplo: `deploy/prometheus/licitalens-alerts.yml`.
- Runbook: [`runbook.md`](runbook.md).

## Retenção sugerida

| Dado | Retenção |
|------|----------|
| Backups Postgres | 30 dias (diário) + 12 mensais |
| Raw PNCP (MinIO/S3) | 90 dias mínimo para replay |
| Logs aplicação | 14–30 dias centralizados |

Próximo passo comercial: **Fase 2** (`DEMO_MODE=false`, Stripe produção) sobre esta base.
