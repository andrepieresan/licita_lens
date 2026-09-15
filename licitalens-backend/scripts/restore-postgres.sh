#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Uso: $0 <arquivo.sql.gz>" >&2
  exit 2
fi

FILE="$1"
if [[ ! -f "$FILE" ]]; then
  echo "Arquivo não encontrado: $FILE" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE=(docker compose -f "${ROOT}/compose.yaml" -f "${ROOT}/compose.commercial.yaml" -f "${ROOT}/compose.prod.yaml")

echo "ATENÇÃO: isso substitui o banco licitalens atual."
read -r -p "Digite RESTORE para continuar: " CONFIRM
if [[ "$CONFIRM" != "RESTORE" ]]; then
  echo "Cancelado."
  exit 1
fi

echo "→ Restaurando ${FILE}…"
gunzip -c "$FILE" | "${COMPOSE[@]}" exec -T postgres psql -U licitalens -d licitalens -v ON_ERROR_STOP=1
echo "Restore concluído."
