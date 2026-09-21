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

Stack comercial legada para desenvolvimento (Postgres + admin + billing + seed demo):

```bash
make commercial-up
```

Isso copia `.env.commercial.example` → `.env.commercial.local` (se ainda não existir), sobe Postgres/Keycloak/Gateway, aplica as migrations comerciais versionadas em volumes antigos, reaplica o seed demo e imprime URLs. Painel: `http://localhost:8080/admin` com `X-Admin-Key` definido no env local. Encerrar: `make commercial-down`.

Conta SaaS (JWT local + CRM):

- `POST /v1/auth/signup`, `POST /v1/auth/login` e `POST /v1/auth/logout` — cadastro e sessão. No navegador, a sessão é mantida em cookie `HttpOnly` com cookie CSRF separado; clientes nativos podem usar `Authorization: Bearer`.
- confirmação de e-mail e recuperação de senha usam tokens expiradores de uso único; a interface reconhece os links gerados pela API.
- `GET /v1/account/export` e `DELETE /v1/account` permitem exportar os dados vinculados e excluir a conta mediante confirmação explícita.
- `GET /v1/me` — organização e status da assinatura.
- `GET/POST /v1/deals`, `PATCH /v1/deals/{id}`, follow-ups em `/v1/deals/{id}/followups` — pipeline comercial (criação idempotente por `opportunity_id`).

Cadastros, logins e fluxos de recuperação são limitados a dez tentativas por origem em quinze minutos. A distribuição self-hosted mantém o cadastro público fechado por padrão e fornece um comando local para criar o primeiro proprietário; habilite `PUBLIC_SIGNUP` apenas quando o cadastro aberto for intencional.

App web SaaS: `cd ../licitalens-mobile && bun run commercial:web` (fluxo welcome → cadastro → pipeline Kanban → licitações com análise → perfil → plano).

**Alertas (worker):** o serviço `notifications` lê `notification_preferences`, perfis e oportunidades **publicadas após o último ciclo** (cursor em `tenancy.notification_scan`; primeiro ciclo usa lookback de 24h), envia e-mail via SMTP (Mailpit em dev), push via Expo (`EXPO_ACCESS_TOKEN` opcional) e registra deduplicação em `notifications.deliveries`. SMTP tem timeout integral, modos `auto`, `starttls`, `implicit` e `disabled`, e recusa autenticação remota sem TLS. Tokens Expo: `PUT /v1/account/push-token`. Follow-ups usam `next_follow_up_at` nos deals. Disparo manual: `POST /v1/admin/notifications/run` ou botão no painel admin. Mailpit: http://localhost:8025

Login próprio é o fluxo padrão. Keycloak/OIDC é uma integração opcional, mantida no ambiente comercial legado para desenvolvimento; quando usado localmente, `make commercial-gateway-host` permite que o gateway alcance um emissor em `localhost:8180`.

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
- `POST /v1/admin/billing/reconcile` consulta o estado atual do Stripe e reaplica assinaturas com metadados de organização; use-o por agendamento administrativo para reparar webhooks perdidos. A operação é idempotente por assinatura, plano, status e período.
- O comando `billing` executa essa reconciliação automaticamente no modo `cloud`; configure `STRIPE_RECONCILE_INTERVAL` (padrão: `1h`) e mantenha uma única réplica.
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

Com `DATABASE_URL`, a API usa PostgreSQL e o serviço `procurement` consome eventos `procurement.discovered.v1` do Kafka/Redpanda. Eventos inválidos são persistidos em `procurement.dead-letter.v1` antes da confirmação do offset. Fora do modo `demo`, uma dependência crítica ausente interrompe a inicialização; armazenamento em memória é exclusivo da demonstração. A ingestão PNCP publica eventos e arquiva payloads localmente ou em S3 compatível quando configurada. Leituras do PNCP têm timeout e retry exponencial configuráveis, e os checkpoints de página e de dia avançam somente depois do arquivamento e publicação. Após reinício, o worker recupera automaticamente os dias pendentes até `PNCP_MAX_RECOVERY_DAYS` (ou sem limite quando `0`) e mantém uma janela recente para atualizações atrasadas. A prontidão dos workers consulta o PostgreSQL e o primeiro ciclo aplicável.

Para uma operação cloud ainda são necessários credenciais e validação real dos provedores, domínio/HTTPS, coleta externa das métricas e publicação das imagens da release. O gateway valida a associação do `subject` autenticado à organização ativa no PostgreSQL quando o modo demo está desligado e falha cedo se a configuração crítica de produção estiver incompleta.

O gateway expõe métricas Prometheus em `/metrics`; a distribuição self-hosted documenta também as métricas privadas de ingestão, alertas e idade do backup em [`deploy/self-hosted/README.md`](deploy/self-hosted/README.md).

As regras e a configuração-base de scrape ficam em [`deploy/prometheus`](deploy/prometheus). O workflow [`release.yml`](../.github/workflows/release.yml) gera imagens `amd64` e `arm64` no GHCR e uma GitHub Release apenas quando uma tag `v*` for enviada para o repositório público.

Não adicione credenciais, tokens ou arquivos `.env` ao repositório.
