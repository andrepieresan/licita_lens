#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE=(docker compose -f "${ROOT}/compose.yaml" -f "${ROOT}/compose.commercial.yaml" -f "${ROOT}/compose.prod.yaml")
BACKUP_DIR="${ROOT}/.data/backups"
STAMP="$(date -u +%Y%m%d-%H%M%S)"
OUT="${BACKUP_DIR}/licitalens-${STAMP}.sql.gz"

mkdir -p "$BACKUP_DIR"

echo "→ Backup Postgres → ${OUT}"
"${COMPOSE[@]}" exec -T postgres pg_dump -U licitalens -d licitalens --no-owner --no-acl | gzip -9 > "$OUT"
echo "OK ($(du -h "$OUT" | awk '{print $1}'))"
