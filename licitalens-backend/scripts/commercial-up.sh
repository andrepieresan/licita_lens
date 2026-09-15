#!/usr/bin/env bash
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

# Exporta variáveis para substituição no compose.commercial.yaml
set -a
# shellcheck disable=SC1090
source "$ENV_LOCAL"
set +a

echo "→ Subindo Postgres, Keycloak, Mailpit e Gateway (stack comercial)…"
"${COMPOSE[@]}" up -d postgres keycloak mailpit gateway notifications

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

if [[ "${SKIP_DEMO_SEED:-}" != "1" ]]; then
  echo "→ Reaplicando seed demo (idempotente)…"
  "${COMPOSE[@]}" exec -T postgres psql -U licitalens -d licitalens -v ON_ERROR_STOP=1 < "${ROOT}/db/postgres/005_demo_seed.sql" >/dev/null
else
  echo "→ Seed demo omitido (SKIP_DEMO_SEED=1)."
fi

echo "→ Rebuild do gateway (rotas SaaS recentes)…"
"${COMPOSE[@]}" up -d --build gateway >/dev/null

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

ADMIN_KEY="${ADMIN_API_KEY:-local-commercial-admin}"
ORG_ID="00000000-0000-0000-0000-000000000001"

cat <<EOF

LicitaLens — stack comercial no ar

  API:        http://localhost:8080/health/ready
  Admin:      http://localhost:8080/admin
              Header X-Admin-Key: ${ADMIN_KEY}
  Keycloak:   http://localhost:8180  (admin/local-only)
              Realm licitalens · user demo / demo123
  Mailpit:    http://localhost:8025  (e-mails de alerta do worker)

  Worker de alertas: serviço docker \`notifications\` (SMTP → Mailpit).
  Disparo manual: POST http://localhost:8080/v1/admin/notifications/run  (X-Admin-Key)

  DEMO_MODE=${DEMO_MODE:-true}
  Organização demo: ${ORG_ID}

Mobile (outro terminal):

  cd ${ROOT}/../licitalens-mobile
  EXPO_PUBLIC_API_URL=http://localhost:8080 \\
  EXPO_PUBLIC_ORGANIZATION_ID=${ORG_ID} \\
  bun start

Demo completo no app (sem backend):

  cd ${ROOT}/../licitalens-mobile && bun run demo:web

Plataforma inteira interligada (app + API + admin):

  # terminal 1 (se ainda não estiver no ar)
  cd ${ROOT} && make commercial-up

  # terminal 2 — app web apontando para o gateway
  cd ${ROOT}/../licitalens-mobile && bun run commercial:web

  # abas no navegador
  #   • App cliente (Expo, geralmente http://localhost:8081)
  #   • Admin operacional http://localhost:8080/admin  (X-Admin-Key no painel)

Roteiro de telas no app (bun run commercial:web): boas-vindas → cadastro/login → ativação → onboarding → Pipeline Kanban/lista → Licitações → Plano → Conta.

OIDC no mobile (DEMO_MODE=false no .env.commercial.local):

  make commercial-gateway-host
  EXPO_PUBLIC_KEYCLOAK_ISSUER=http://localhost:8180/realms/licitalens \\
  EXPO_PUBLIC_KEYCLOAK_CLIENT_ID=licitalens-mobile \\
  EXPO_PUBLIC_API_URL=http://localhost:8080 bun start

Stripe: preencha STRIPE_* em .env.commercial.local e reinicie o gateway:
  ${COMPOSE[*]} up -d gateway

Parar stack: make commercial-down

EOF
