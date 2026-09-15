#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

READY="$(curl -sf "http://localhost:8080/health/ready" || true)"
if [[ -z "$READY" ]]; then
  echo "Gateway /health/ready indisponível." >&2
  exit 1
fi
echo "→ /health/ready: $READY"
if ! echo "$READY" | grep -q '"postgres":"ok"'; then
  echo "Postgres não está ok no readiness." >&2
  exit 1
fi

METRICS="$(curl -sf "http://localhost:8080/metrics" | head -5 || true)"
if [[ -z "$METRICS" ]] || ! echo "$METRICS" | grep -q licitalens_http_requests_total; then
  echo "/metrics não expõe contadores esperados." >&2
  exit 1
fi
echo "→ /metrics OK"

if ! ls "${ROOT}/.data/backups/"*.sql.gz >/dev/null 2>&1; then
  echo "Nenhum backup em .data/backups/ — rode scripts/backup-postgres.sh" >&2
  exit 1
fi
echo "→ Backup encontrado em .data/backups/"

chmod +x "${ROOT}/scripts/phase1-verify.sh"
if ! "${ROOT}/scripts/phase1-verify.sh"; then
  echo "Pipeline PNCP (Fase 1) ainda não validado." >&2
  exit 1
fi

echo "Fase 3 OK."
