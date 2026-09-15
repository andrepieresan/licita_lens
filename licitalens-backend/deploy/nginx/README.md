# nginx — proxy do LicitaLens

Dois modos:

## 1. nginx no host (VPS)

Recomendado quando o Docker publica portas em `127.0.0.1`.

```bash
sudo cp licitalens.conf.example /etc/nginx/sites-available/licitalens
sudo nano /etc/nginx/sites-available/licitalens   # domínio + SSL
sudo ln -sf /etc/nginx/sites-available/licitalens /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
sudo certbot --nginx -d app.seudominio.com -d api.seudominio.com -d auth.seudominio.com
```

Upstreams padrão: app `:8081`, api `:8080`, auth `:8180`.

## 2. nginx no Docker (`compose.prod.yaml`)

```bash
cd licitalens-mobile
EXPO_PUBLIC_API_URL=https://api.seudominio.com npx expo export --platform web

cd ../licitalens-backend
# Edite deploy/nginx/licitalens.docker.conf (server_name)
docker compose -f compose.yaml -f compose.commercial.yaml -f compose.prod.yaml up -d nginx
```

Portas **80/443** no container `nginx`. App estático em `licitalens-mobile/dist`.

TLS: monte `deploy/nginx/certs/` (fullchain + privkey) e duplique os blocos `listen 443 ssl` de `licitalens.conf.example` em `licitalens.docker.conf`, ou termine TLS no nginx do host apontando para `localhost:80` do container.

## Variáveis do backend

```env
ALLOWED_ORIGINS=https://app.seudominio.com
KEYCLOAK_ISSUER=https://auth.seudominio.com/realms/licitalens
```

Keycloak atrás de proxy (compose):

```yaml
keycloak:
  environment:
    KC_HOSTNAME: auth.seudominio.com
    KC_PROXY_HEADERS: xforwarded
    KC_HTTP_ENABLED: "true"
```

Ajuste `redirectUris` no realm para `https://app.seudominio.com/*`.
