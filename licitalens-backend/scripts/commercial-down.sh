#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

docker compose -f compose.yaml -f compose.commercial.yaml down

echo "Stack comercial encerrada."
