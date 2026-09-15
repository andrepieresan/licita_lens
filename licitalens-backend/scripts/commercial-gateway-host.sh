#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

ENV_LOCAL="${ROOT}/.env.commercial.local"
if [[ ! -f "$ENV_LOCAL" ]]; then
  echo "Execute make commercial-up antes." >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
source "$ENV_LOCAL"
set +a

export DATABASE_URL="${DATABASE_URL:-postgres://licitalens:local-only@localhost:5432/licitalens}"
export PORT="${PORT:-8080}"
export DEMO_MODE="${DEMO_MODE:-false}"

echo "→ Gateway local em :${PORT} (DATABASE_URL=${DATABASE_URL})"
echo "  KEYCLOAK_ISSUER=${KEYCLOAK_ISSUER:-}"
exec go run ./cmd/gateway
