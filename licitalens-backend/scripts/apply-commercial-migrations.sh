#!/usr/bin/env bash
# Aplica migrations comerciais 004–010 em Postgres já no ar (idempotente).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE=(docker compose -f "${ROOT}/compose.yaml" -f "${ROOT}/compose.commercial.yaml")

echo "→ Aplicando migration 004 (canais de notificação), se necessário…"
TABLE=$("${COMPOSE[@]}" exec -T postgres psql -U licitalens -d licitalens -tAc "SELECT to_regclass('notifications.channels')" | tr -d '[:space:]')
if [[ -z "$TABLE" ]]; then
  "${COMPOSE[@]}" exec -T postgres psql -U licitalens -d licitalens -v ON_ERROR_STOP=1 < "${ROOT}/db/postgres/004_notifications.sql"
  echo "   Migration 004 aplicada."
else
  echo "   notifications.channels já existe."
fi

echo "→ Aplicando migration 006 (histórico de assinatura), se necessário…"
TABLE=$("${COMPOSE[@]}" exec -T postgres psql -U licitalens -d licitalens -tAc "SELECT to_regclass('tenancy.subscription_history')" | tr -d '[:space:]')
if [[ -z "$TABLE" ]]; then
  "${COMPOSE[@]}" exec -T postgres psql -U licitalens -d licitalens -v ON_ERROR_STOP=1 < "${ROOT}/db/postgres/006_subscription_history.sql"
  echo "   Migration 006 aplicada."
else
  echo "   tenancy.subscription_history já existe."
fi

echo "→ Aplicando migration 007 (contas SaaS), se necessário…"
TABLE=$("${COMPOSE[@]}" exec -T postgres psql -U licitalens -d licitalens -tAc "SELECT to_regclass('tenancy.accounts')" | tr -d '[:space:]')
if [[ -z "$TABLE" ]]; then
  "${COMPOSE[@]}" exec -T postgres psql -U licitalens -d licitalens -v ON_ERROR_STOP=1 < "${ROOT}/db/postgres/007_saas_accounts.sql"
  echo "   Migration 007 aplicada."
else
  echo "   tenancy.accounts já existe."
fi

echo "→ Aplicando migration 008 (CRM pipeline), se necessário…"
TABLE=$("${COMPOSE[@]}" exec -T postgres psql -U licitalens -d licitalens -tAc "SELECT to_regclass('crm.deals')" | tr -d '[:space:]')
if [[ -z "$TABLE" ]]; then
  "${COMPOSE[@]}" exec -T postgres psql -U licitalens -d licitalens -v ON_ERROR_STOP=1 < "${ROOT}/db/postgres/008_crm_pipeline.sql"
  echo "   Migration 008 aplicada."
else
  echo "   crm.deals já existe."
fi

echo "→ Aplicando migration 009 (preferências de alerta + índice pipeline), se necessário…"
COL=$("${COMPOSE[@]}" exec -T postgres psql -U licitalens -d licitalens -tAc "SELECT column_name FROM information_schema.columns WHERE table_schema='tenancy' AND table_name='organizations' AND column_name='notification_preferences'" | tr -d '[:space:]')
if [[ -z "$COL" ]]; then
  "${COMPOSE[@]}" exec -T postgres psql -U licitalens -d licitalens -v ON_ERROR_STOP=1 < "${ROOT}/db/postgres/009_notification_preferences.sql"
  echo "   Migration 009 aplicada."
else
  echo "   notification_preferences já existe."
fi

echo "→ Aplicando migration 010 (cursor de oportunidades para alertas), se necessário…"
TABLE=$("${COMPOSE[@]}" exec -T postgres psql -U licitalens -d licitalens -tAc "SELECT to_regclass('tenancy.notification_scan')" | tr -d '[:space:]')
if [[ -z "$TABLE" ]]; then
  "${COMPOSE[@]}" exec -T postgres psql -U licitalens -d licitalens -v ON_ERROR_STOP=1 < "${ROOT}/db/postgres/010_notification_scan.sql"
  echo "   Migration 010 aplicada."
else
  echo "   tenancy.notification_scan já existe."
fi
