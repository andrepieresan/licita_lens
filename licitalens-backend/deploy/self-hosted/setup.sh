#!/usr/bin/env sh
set -eu

if [ -e .env ]; then
  echo ".env already exists; refusing to overwrite it." >&2
  exit 1
fi
if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl is required to generate installation secrets." >&2
  exit 1
fi

postgres_password="$(openssl rand -hex 24)"
minio_password="$(openssl rand -hex 24)"
jwt_secret="$(openssl rand -hex 32)"
admin_key="$(openssl rand -hex 24)"

while IFS= read -r line || [ -n "$line" ]; do
  case "$line" in
    POSTGRES_PASSWORD=*) printf 'POSTGRES_PASSWORD=%s\n' "$postgres_password" ;;
    MINIO_ROOT_PASSWORD=*) printf 'MINIO_ROOT_PASSWORD=%s\n' "$minio_password" ;;
    AUTH_JWT_SECRET=*) printf 'AUTH_JWT_SECRET=%s\n' "$jwt_secret" ;;
    ADMIN_API_KEY=*) printf 'ADMIN_API_KEY=%s\n' "$admin_key" ;;
    *) printf '%s\n' "$line" ;;
  esac
done < .env.example > .env

echo "Created .env with unique local secrets. Review public URLs, legal operator identity, signup policy and SMTP settings before starting."
