# LicitaLens Mobile

Cliente Expo/React Native mobile-first. O contrato TypeScript é gerado do OpenAPI mantido no backend.

```bash
bun install
bun run generate:sdk
EXPO_PUBLIC_API_URL=http://localhost:8080 bun start
```

Em aparelho físico, use o IP local da máquina no lugar de `localhost`. Fora do modo demo, o app usa OIDC/PKCE (Keycloak) ou `EXPO_PUBLIC_ACCESS_TOKEN` para desenvolvimento, provisiona a organização no primeiro acesso e expõe checkout, portal e histórico na aba Plano.

```bash
EXPO_PUBLIC_KEYCLOAK_ISSUER=http://localhost:8180/realms/licitalens
EXPO_PUBLIC_KEYCLOAK_CLIENT_ID=licitalens-mobile
EXPO_PUBLIC_API_URL=http://localhost:8080
bun start
```

## Demonstração sem integrações

O aplicativo inclui dados locais para validar toda a experiência sem backend, login, pagamento ou provedores externos:

```bash
bun run demo
bun run demo:web
```

O fluxo inclui onboarding, perfil editável, feed pesquisável, filtros por UF, detalhe da licitação, análise explicável, consumo do plano e preferências persistidas no dispositivo.

### Ver tudo interligado (app + API + admin)

```bash
# Terminal 1 — backend, Postgres, Keycloak, painel admin
cd ../licitalens-backend && make commercial-up

# Terminal 2 — app web contra a API (não usa mock local)
cd licitalens-mobile && bun run commercial:web
```

Abra duas abas:

| Aba | URL | O que valida |
|-----|-----|----------------|
| **Cliente** | Expo Web (ex.: http://localhost:8081) | Login/org (se necessário), onboarding, feed, análise, plano, ajustes |
| **Admin** | http://localhost:8080/admin | Organizações, planos vigentes, histórico de assinaturas |

Chave admin (padrão local): `local-commercial-admin` — campo no topo do painel.

Só UI do cliente, sem backend: `bun run demo:web`.
