#!/usr/bin/env bash
# Fase 1: PNCP → ingestão → Kafka → procurement → Postgres (sem seed demo).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

COMPOSE=(docker compose -f compose.yaml -f compose.commercial.yaml)
ENV_EXAMPLE="${ROOT}/.env.commercial.example"
ENV_LOCAL="${ROOT}/.env.commercial.local"

if [[ ! -f "$ENV_LOCAL" ]]; then
  cp "$ENV_EXAMPLE" "$ENV_LOCAL"
  echo "→ Criado ${ENV_LOCAL} a partir do exemplo."
fi

set -a
# shellcheck disable=SC1090
source "$ENV_LOCAL"
set +a

echo "→ Subindo Postgres, Redpanda, MinIO, ingestão, procurement e gateway…"
"${COMPOSE[@]}" up -d --build postgres redpanda minio ingestion procurement gateway

echo "→ Aguardando Postgres…"
TRIES=0
until "${COMPOSE[@]}" exec -T postgres pg_isready -U licitalens -d licitalens >/dev/null 2>&1; do
  TRIES=$((TRIES + 1))
  if [[ $TRIES -gt 60 ]]; then
    echo "Postgres não ficou pronto a tempo." >&2
    exit 1
  fi
  sleep 1
done

chmod +x "${ROOT}/scripts/apply-commercial-migrations.sh"
"${ROOT}/scripts/apply-commercial-migrations.sh"

echo "→ Seed demo omitido (dados reais via PNCP)."

echo "→ Aguardando gateway…"
TRIES=0
until curl -sf "http://localhost:8080/health/ready" >/dev/null; do
  TRIES=$((TRIES + 1))
  if [[ $TRIES -gt 60 ]]; then
    echo "Gateway não respondeu em http://localhost:8080/health/ready" >&2
    exit 1
  fi
  sleep 1
done

echo "→ Aguardando primeira sincronização PNCP (até 3 min)…"
chmod +x "${ROOT}/scripts/phase1-verify.sh"
if ! "${ROOT}/scripts/phase1-verify.sh" --wait; then
  echo ""
  echo "A ingestão ainda pode estar em andamento. Logs: ${COMPOSE[*]} logs -f ingestion procurement"
  echo "Repita: make phase1-verify"
  exit 1
fi

cat <<EOF

LicitaLens — Fase 1 (dados reais PNCP)

  API:     http://localhost:8080/health/ready
  Radar:   GET http://localhost:8080/v1/opportunities  (modo demo: headers X-Organization-ID / X-User-ID)
  App:     cd ../licitalens-mobile && bun run commercial:web

  Verificar: make phase1-verify
  Logs PNCP: ${COMPOSE[*]} logs -f ingestion procurement

EOF
