# LicitaLens Backend

Backend em Go para descobrir e explicar oportunidades de compras públicas brasileiras. O repositório entrega a primeira vertical slice: API multiempresa, perfil comercial, feed, ranking explicável, cliente PNCP, arquivamento bruto, eventos versionados, CLI, adaptadores OpenAI/Ollama e base de infraestrutura.

## Executar

O propósito do produto e os critérios para fechar a versão comercial estão em [`docs/PRODUCT-ROADMAP.md`](docs/PRODUCT-ROADMAP.md). Em resumo: o LicitaLens é inteligência comercial privada para fornecedores; não é o portal oficial, não envia propostas e não substitui o edital.

Requisitos: Go 1.25+, Docker e, para Kubernetes, Kind e Helm.

```bash
make test
make run
```

Em outro terminal:

```bash
curl -s http://localhost:8080/health/ready
curl -s -H 'X-Organization-ID: 00000000-0000-0000-0000-000000000001' -H 'X-User-ID: demo-user' http://localhost:8080/v1/opportunities
```

Para exercitar uma página real do PNCP e manter o payload em `.data/raw`:

```bash
make sync
```

O ambiente completo sobe com `make compose-up`. PostgreSQL, pgvector, ClickHouse, Redpanda, MinIO, Keycloak e Mailpit ficam disponíveis apenas para desenvolvimento local.

**Fase 1 — dados reais PNCP** (ingestão contínua, sem seed demo):

```bash
make phase1-up
make phase1-verify   # opcional, repetir até OK
```

Detalhes: [`docs/FASE-1-DADOS-REAIS.md`](docs/FASE-1-DADOS-REAIS.md).

**Fase 3 — infra operacional** (stack completa, backup, readiness Postgres, métricas):

```bash
make phase3-up
```

[`docs/FASE-3-INFRA.md`](docs/FASE-3-INFRA.md) · conformidade no app: [`docs/FASE-4-CONFORMIDADE.md`](docs/FASE-4-CONFORMIDADE.md).

Stack comercial (Postgres + admin + billing + seed demo):

```bash
make commercial-up
```

Isso copia `.env.commercial.example` → `.env.commercial.local` (se ainda não existir), sobe Postgres/Keycloak/Gateway, aplica migrations `006`–`008` (histórico, contas SaaS, CRM) em volumes antigos, reaplica o seed demo e imprime URLs. Painel: `http://localhost:8080/admin` com `X-Admin-Key` definido no env local. Encerrar: `make commercial-down`.

Conta SaaS (JWT local + CRM):

- `POST /v1/auth/signup` e `POST /v1/auth/login` — cadastro e sessão (header `Authorization: Bearer` nas demais rotas).
- `GET /v1/me` — organização e status da assinatura.
- `GET/POST /v1/deals`, `PATCH /v1/deals/{id}`, follow-ups em `/v1/deals/{id}/followups` — pipeline comercial (criação idempotente por `opportunity_id`).

App web SaaS: `cd ../licitalens-mobile && bun run commercial:web` (fluxo welcome → cadastro → pipeline Kanban → licitações com análise → perfil → plano).

**Alertas (worker):** o serviço `notifications` lê `notification_preferences`, perfis e oportunidades **publicadas após o último ciclo** (cursor em `tenancy.notification_scan`; primeiro ciclo usa lookback de 24h), envia e-mail via SMTP (Mailpit em dev), push via Expo (`EXPO_ACCESS_TOKEN` opcional) e registra deduplicação em `notifications.deliveries`. Tokens Expo: `PUT /v1/account/push-token`. Follow-ups usam `next_follow_up_at` nos deals. Disparo manual: `POST /v1/admin/notifications/run` ou botão no painel admin. Mailpit: http://localhost:8025

Com `DEMO_MODE=false`, rode o gateway no host para OIDC (`KEYCLOAK_ISSUER` com `localhost:8180`): `make commercial-gateway-host`.

## IA

Sem `AI_PROVIDER`, a aplicação mantém ranking e evidências sem chamar um modelo. Para habilitar:

```bash
AI_PROVIDER=ollama OLLAMA_BASE_URL=http://localhost:11434 make run
AI_PROVIDER=openai OPENAI_API_KEY=... make run
```

O adaptador OpenAI usa `POST /v1/embeddings` e a Responses API conforme a [documentação oficial](https://developers.openai.com/api/reference/cli/resources/responses/methods/create). A chave nunca deve ser versionada.

## Assinatura e cotas

Em produção, configure `STRIPE_WEBHOOK_SECRET`. O endpoint `POST /v1/webhooks/stripe` valida a assinatura, ignora eventos duplicados, rejeita eventos fora de ordem e atualiza a assinatura da organização. O gateway consulta o plano e o status no PostgreSQL; os headers `X-Plan` e `X-Subscription-Status` não são usados para autorizar acesso.

Checkout e portal:

```bash
export STRIPE_SECRET_KEY=sk_test_...
export STRIPE_PRICE_ESSENTIAL=price_...
export STRIPE_PRICE_PRO=price_...
export STRIPE_SUCCESS_URL=http://localhost:8082/plano
export STRIPE_CANCEL_URL=http://localhost:8082/plano
export STRIPE_PORTAL_RETURN_URL=http://localhost:8082/plano
export ADMIN_API_KEY=change-me
```

- `POST /v1/billing/checkout` e `POST /v1/billing/portal` criam sessões Stripe para a organização ativa.
- `GET /v1/billing/history` retorna o histórico persistido em `tenancy.subscription_history`.
- `POST /v1/account/bootstrap` cria organização, membership `owner` e assinatura `trialing`.
- Painel interno em `GET /admin` (APIs `/v1/admin/*`, header `X-Admin-Key`; em `DEMO_MODE=true` a chave é opcional).

Aplique a migration `db/postgres/006_subscription_history.sql` ao subir PostgreSQL.

## Estrutura

- `cmd/`: gateway, serviços e CLI operacional.
- `internal/domain`: regras independentes de infraestrutura.
- `internal/providers`: integrações PNCP e IA.
- `db/`: schemas transacionais, vetoriais e analíticos.
- `api/openapi.yaml`: contrato que gera o SDK TypeScript.
- `deploy/helm`: implantação e autoscaling.
- `docs/`: arquitetura, segurança e operação.

## Limites da primeira entrega

Com `DATABASE_URL`, a API usa PostgreSQL e o serviço `procurement` consome eventos `procurement.discovered.v1` do Kafka/Redpanda. Sem banco disponível, o gateway cai de forma explícita para o armazenamento em memória de demonstração. A ingestão PNCP já publica eventos e arquiva payloads localmente ou em S3 compatível quando configurada.

Ainda faltam, para produção, os itens de fechamento descritos em [`docs/PRODUCT-ROADMAP.md`](docs/PRODUCT-ROADMAP.md): dados completos de itens/contratos/fornecedores, persistência analítica quando o dashboard estiver pronto, provedores de notificação operacionais e infraestrutura definitiva. O gateway já valida a associação do `subject` autenticado à organização ativa no PostgreSQL quando o modo demo está desligado e agora falha cedo se a configuração crítica de produção estiver incompleta.

Não adicione credenciais, tokens ou arquivos `.env` ao repositório.
