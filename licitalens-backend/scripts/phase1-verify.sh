#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE=(docker compose -f "${ROOT}/compose.yaml" -f "${ROOT}/compose.commercial.yaml")

WAIT=false
if [[ "${1:-}" == "--wait" ]]; then
  WAIT=true
fi

min_pncp() {
  "${COMPOSE[@]}" exec -T postgres psql -U licitalens -d licitalens -tAc \
    "SELECT count(*)::int FROM procurement.opportunities WHERE source = 'pncp'" | tr -d '[:space:]'
}

TRIES=0
MAX_TRIES=36
COUNT=0
while true; do
  COUNT="$(min_pncp || echo 0)"
  if [[ "${COUNT:-0}" -ge 1 ]]; then
    break
  fi
  if [[ "$WAIT" != true ]]; then
    echo "Nenhuma oportunidade PNCP no banco ainda (count=${COUNT:-0})." >&2
    echo "Confira: ${COMPOSE[*]} logs ingestion procurement" >&2
    exit 1
  fi
  TRIES=$((TRIES + 1))
  if [[ $TRIES -gt $MAX_TRIES ]]; then
    echo "Timeout: nenhuma oportunidade PNCP após ~3 min." >&2
    exit 1
  fi
  sleep 5
done

echo "→ Postgres: ${COUNT} oportunidade(s) com source=pncp"

SAMPLE=$("${COMPOSE[@]}" exec -T postgres psql -U licitalens -d licitalens -tAc \
  "SELECT id FROM procurement.opportunities WHERE source = 'pncp' ORDER BY published_at DESC NULLS LAST LIMIT 1" | tr -d '[:space:]')
echo "→ Exemplo de id: ${SAMPLE}"

HTTP=$(curl -sf -H 'X-Organization-ID: 00000000-0000-0000-0000-000000000001' -H 'X-User-ID: demo-user' \
  "http://localhost:8080/v1/opportunities?limit=5" || true)
if [[ -z "$HTTP" ]]; then
  echo "API /v1/opportunities não respondeu (gateway no ar?)." >&2
  exit 1
fi

if ! echo "$HTTP" | grep -q '"source":"pncp"'; then
  echo "API respondeu, mas sem source=pncp no payload (pode haver só demo antigo)." >&2
  echo "$HTTP" | head -c 400
  exit 1
fi

echo "→ API: /v1/opportunities retorna oportunidades PNCP"
echo "Fase 1 OK."
