#!/usr/bin/env bash
# Fase 3: stack completa (dados PNCP + comercial) com políticas de produção locais.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

COMPOSE=(docker compose -f compose.yaml -f compose.commercial.yaml -f compose.prod.yaml)
ENV_EXAMPLE="${ROOT}/.env.commercial.example"
ENV_LOCAL="${ROOT}/.env.commercial.local"

if [[ ! -f "$ENV_LOCAL" ]]; then
  cp "$ENV_EXAMPLE" "$ENV_LOCAL"
  echo "→ Criado ${ENV_LOCAL}"
fi

export SKIP_DEMO_SEED=1

set -a
# shellcheck disable=SC1090
source "$ENV_LOCAL"
set +a

echo "→ Subindo stack Fase 3 (Postgres, pipeline PNCP, gateway, alertas, Keycloak, Mailpit)…"
"${COMPOSE[@]}" up -d --build \
  postgres redpanda minio ingestion procurement gateway notifications keycloak mailpit

echo "→ Aguardando Postgres…"
TRIES=0
until "${COMPOSE[@]}" exec -T postgres pg_isready -U licitalens -d licitalens >/dev/null 2>&1; do
  TRIES=$((TRIES + 1))
  [[ $TRIES -le 60 ]] || { echo "Postgres timeout." >&2; exit 1; }
  sleep 1
done

chmod +x "${ROOT}/scripts/apply-commercial-migrations.sh"
"${ROOT}/scripts/apply-commercial-migrations.sh"

echo "→ Seed demo omitido (SKIP_DEMO_SEED=1)."

TRIES=0
until curl -sf "http://localhost:8080/health/ready" >/dev/null; do
  TRIES=$((TRIES + 1))
  [[ $TRIES -le 60 ]] || { echo "Gateway timeout." >&2; exit 1; }
  sleep 1
done

chmod +x "${ROOT}/scripts/backup-postgres.sh" "${ROOT}/scripts/phase3-verify.sh"
"${ROOT}/scripts/backup-postgres.sh"
"${ROOT}/scripts/phase3-verify.sh"

cat <<EOF

LicitaLens — Fase 3 (infra operacional local)

  Ready:    curl -s http://localhost:8080/health/ready | jq .
  Métricas: curl -s http://localhost:8080/metrics | head
  Backup:   .data/backups/
  nginx:    deploy/nginx/README.md (host ou Docker compose.prod)
  Helm:     deploy/helm/licitalens (ingress + probes)

  App: cd ../licitalens-mobile && bun run commercial:web
  Doc: docs/FASE-3-INFRA.md

EOF
