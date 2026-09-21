# Fase 3 — Ambiente legado de desenvolvimento e operação

## Subir localmente

```bash
make phase3-up
make phase3-verify
```

Inclui: Postgres, Redpanda, MinIO, ingestão PNCP, procurement, gateway, worker de alertas, Keycloak e Mailpit para desenvolvimento. A distribuição publicável é [`deploy/self-hosted`](../deploy/self-hosted/README.md); não use este Compose legado como modelo de exposição pública.

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

`backup-postgres.sh` protege somente PostgreSQL. Para uma recuperação válida da distribuição, faça backup também de Redpanda e MinIO, restaure em ambiente isolado e grave o marcador de sucesso descrito no guia self-hosted. Teste restore em ambiente isolado pelo menos uma vez por trimestre.

## Domínio e TLS (nginx)

1. DNS: `app.`, `api.`, `auth.` → IP do servidor.
2. **nginx no host:** `deploy/nginx/licitalens.conf.example` → `/etc/nginx/sites-available/`, `certbot --nginx`.
3. **nginx no Docker (legado):** serviço `nginx` em `compose.prod.yaml` + `deploy/nginx/licitalens.docker.conf`.
4. Guia completo: [`deploy/nginx/README.md`](../deploy/nginx/README.md).
5. Variáveis: `.env.prod.example` → `.env.prod.local` (`ALLOWED_ORIGINS`, `KEYCLOAK_ISSUER` com HTTPS público).

## Kubernetes

```bash
helm lint deploy/helm/licitalens
# Ajuste values: image.repository, ingress.host, secretName
```

Ingress TLS e probes de readiness já estão no chart.

## Observabilidade

- Métricas Prometheus: gateway em `GET /metrics`; workers na rede interna em `GET /metrics`.
- Regras e scrape example: `deploy/prometheus/`.
- Runbook: [`runbook.md`](runbook.md).

## Retenção sugerida

| Dado | Retenção |
|------|----------|
| Backups Postgres | 30 dias (diário) + 12 mensais |
| Raw PNCP (MinIO/S3) | 90 dias mínimo para replay |
| Logs aplicação | 14–30 dias centralizados |

Próximo passo comercial: **Fase 2** (`DEPLOYMENT_MODE=cloud`, Stripe e SMTP reais) sobre staging, não sobre este ambiente legado.
