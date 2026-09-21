# LicitaLens self-hosted

This distribution runs the complete LicitaLens application for an operator's own organization or customers. It is not connected to LicitaLens Cloud and does not require Stripe.

## Requirements

- Docker Engine with Docker Compose;
- at least 4 GB RAM and 20 GB free disk for a small installation;
- public internet access from the ingestion service to PNCP;
- an SMTP provider if e-mail alerts are required.

## First start

```bash
cd licitalens-backend/deploy/self-hosted
./setup.sh
# Review public URLs, legal identity, signup policy and SMTP values in .env.
docker compose up -d --build
docker compose ps
```

`setup.sh` refuses to overwrite an existing `.env` and generates the database, object-store, JWT and administrative secrets with OpenSSL. You can copy `.env.example` manually instead, but example values are rejected by the application outside demo mode.

Create the first owner locally. Keep the password out of the command history:

```bash
read -s BOOTSTRAP_PASSWORD && export BOOTSTRAP_PASSWORD
docker compose --profile tools run --rm admin \
  --email owner@example.com \
  --name "Owner Name" \
  --organization "Organization Name" \
  --accept-legal \
  --terms-version 2026-03-01 \
  --privacy-version 2026-03-01
unset BOOTSTRAP_PASSWORD
```

Use the legal version displayed by the web app if it differs from the example. The command creates a verified owner account and never prints its password. Open `http://localhost:8080` and sign in. The database schema is applied by the `migrate` service; the distribution never inserts demonstration opportunities. The first PNCP synchronization can take several minutes.

The initial synchronization reads the previous seven days by default (`PNCP_INITIAL_DAYS`). The worker persists a durable day cursor and automatically catches up days left pending after a restart, then revisits the latest two days (`PNCP_LOOKBACK_DAYS`) to cover delayed PNCP updates. `PNCP_MAX_RECOVERY_DAYS` (default `30`) bounds automatic catch-up; set it to `0` for an unlimited recovery window when recovering a long interruption.

Public registration is closed by default. Set `PUBLIC_SIGNUP=true` only when any visitor should be allowed to create a new organization; turn it off again after the intended registration window.

## Operations

Only the `web` service is published. PostgreSQL, Redpanda, MinIO, gateway, ingestion, procurement, and notifications stay on the private Compose network.

For e-mail alerts, configure all `SMTP_*` values. When SMTP is absent, the worker reports delivery failures instead of pretending that messages were sent.

For HTTPS, place a reverse proxy in front of `web`, terminate TLS there, forward `X-Forwarded-Proto: https`, and set `ALLOWED_ORIGINS` and `APP_BASE_URL` to the final `https://` application URL. Also set `COOKIE_SECURE=true` so browser session cookies always receive the `Secure` flag. Do not expose database, Kafka, or MinIO ports to the internet.

## Account security and external configuration

Browser sessions use an `HttpOnly` session cookie plus a CSRF cookie. `POST /v1/auth/logout` revokes locally-issued sessions in the database, so an old session token cannot be reused. Native clients continue to use the bearer token returned on login and should call the same logout endpoint when ending a session.

For an existing database, apply `db/postgres/011_account_sessions.psql` and `db/postgres/012_legal_acceptance.psql` before deploying this version. New self-hosted installations already receive both changes through `001_schema.psql`.

Before opening registration to third parties, fill `LEGAL_OPERATOR_NAME`, `LEGAL_OPERATOR_DOCUMENT`, `LEGAL_SUPPORT_EMAIL`, and `LEGAL_PRIVACY_EMAIL` in `.env`, review the bundled legal template with qualified counsel, and rebuild `web`. The accepted terms and privacy versions, together with the server-side acceptance timestamp, are stored on each account and included in account export. Leaving those variables empty produces neutral operator wording suitable only for local evaluation, not an official public service.

Set `SMTP_HOST`, `SMTP_PORT`, `SMTP_FROM`, `SMTP_USER` and `SMTP_PASSWORD` together for e-mail delivery. `SMTP_TLS_MODE` accepts `auto`, `starttls`, `implicit` or `disabled`; use `starttls` or `implicit` on an internet-facing installation. `SMTP_TIMEOUT` bounds the complete SMTP exchange. Authentication over an unencrypted remote connection is rejected, and the worker records an error when e-mail is requested without a usable configuration.

`APP_BASE_URL` is the public web address used in confirmation and password-recovery links. Cloud mode requires `APP_BASE_URL`, `SMTP_HOST` and `SMTP_FROM`; new cloud accounts cannot receive a session or access protected APIs before confirming their e-mail. Self-hosted mode keeps e-mail confirmation optional so an installation without SMTP remains usable.

Notification delivery attempts are persisted in `notifications.deliveries`. Failures are retried on later worker cycles; after five failed attempts the delivery is marked `suppressed` for operator review, preserving the last provider error instead of silently advancing the notification cursor. Re-enable or repair the corresponding channel before manually reprocessing a suppressed delivery.

Invalid procurement events are copied to the durable `procurement.dead-letter.v1` topic with their original topic, partition, offset, key, payload and validation error. The original message is acknowledged only after the DLQ accepts that copy. Inspect pending records with `docker compose exec redpanda rpk topic consume procurement.dead-letter.v1 --num 10`; after correcting a payload, republish it to `procurement.discovered.v1` with the original opportunity key. DLQ entries use the source topic/partition/offset as a stable identifier, so operators can deduplicate ambiguous retries.

PNCP access requires no secret. Configure `PNCP_INITIAL_DAYS`, `PNCP_LOOKBACK_DAYS`, `PNCP_RECENT_PAGES` and `PNCP_MODALITIES` according to the desired initial coverage and provider rate limits. `PNCP_REQUEST_TIMEOUT`, `PNCP_MAX_ATTEMPTS` and `PNCP_RETRY_DELAY` control bounded exponential retries for transient reads; a checkpoint only advances after archive and publication complete. Keep MinIO and Redpanda volumes in the backup because they contain raw archives and pending events.

## Backup and restore

Back up the PostgreSQL, Redpanda, and MinIO volumes together. A PostgreSQL backup alone does not preserve raw PNCP archives or messages pending in the queue.

```bash
docker compose exec -T postgres pg_dump -U licitalens -d licitalens | gzip > licitalens.sql.gz
```

Restore into a fresh isolated instance first, then verify a user can sign in, see profiles, deals, and recent opportunities. Do not restore over a running production database without a separately tested recovery procedure.

The gateway exposes Prometheus metrics at `/api/metrics`. It reports `licitalens_backup_configured` and `licitalens_backup_last_success_timestamp_seconds` from `BACKUP_STATUS_FILE`. The default Compose mount maps `BACKUP_STATUS_DIR` (`./backups`) to `/backups`. Only after copying the complete recovery set, record its completion time:

```bash
mkdir -p backups
date -u +%Y-%m-%dT%H:%M:%SZ > backups/last-success
```

Do not write this marker after a PostgreSQL-only dump: it is evidence for the database, Redpanda, and MinIO recovery set together.

The CI installation test restores its PostgreSQL dump into a separate `pgvector/pgvector:pg17` container and checks that the schema, bootstrapped account, and ingested opportunity survive restoration. This automated check does not validate an operator's Redpanda and MinIO volume backup. Keep production dumps encrypted outside the application server and periodically rehearse restoration of the complete recovery set.

## Updates

Read the release notes and back up all persistent volumes. This source distribution builds its images locally, so update the checked-out release and run:

```bash
docker compose up -d --build
```

The release workflow also publishes versioned multi-architecture images to GHCR after all validation gates pass. The default Compose intentionally remains build-from-source until the final public repository owner defines stable image names. The `migrate` service is idempotent. An application rollback does not automatically undo database changes; restore the compatible backup if a database rollback is needed.

## Monitoring

Each worker exposes `/health/live`, `/health/ready`, and `/metrics` on its private service port. Scrape the gateway through `/api/metrics`, and scrape `ingestion:8080/metrics`, `procurement:8080/metrics`, and `notifications:8080/metrics` from an internal monitoring agent. Alert on a stale `licitalens_ingestion_last_success_timestamp_seconds`, nonzero `licitalens_ingestion_failures_total`, a growing `licitalens_procurement_consumer_lag` or `licitalens_procurement_dead_letters_total`, a stale `licitalens_notifications_last_success_timestamp_seconds`, increases in `licitalens_notifications_failures_total`, or a stale/missing backup timestamp. Do not expose worker ports directly to the internet.

Use `deploy/prometheus/prometheus.yml.example` and `deploy/prometheus/licitalens-alerts.yml` as the monitoring baseline. Import `deploy/grafana/licitalens-dashboard.json` into Grafana after configuring Prometheus as a data source.

The bundled Compose installation was exercised from empty volumes with an isolated database restore, account bootstrap, protected web session, and a controlled PNCP fixture. The public PNCP endpoint remains an external dependency; its availability and rate limits must be monitored in each operator environment.

## Cloud mode

Use `DEPLOYMENT_MODE=cloud` only for the official hosted service. It requires a real `AUTH_JWT_SECRET`, `ADMIN_API_KEY`, `ALLOWED_ORIGINS`, and full Stripe configuration. The gateway refuses to start if those values are absent or if the JWT secret is the known development default.

Cloud registrations receive a fourteen-day trial. The API rejects an expired `trialing` subscription until Stripe marks it active.

The cloud operator must additionally provide `STRIPE_SECRET_KEY`, `STRIPE_PRICE_ESSENTIAL`, `STRIPE_PRICE_PRO`, `STRIPE_WEBHOOK_SECRET`, `STRIPE_SUCCESS_URL` and `STRIPE_CANCEL_URL`. Configure the Stripe webhook to call `/v1/webhooks/stripe`; run that integration in Stripe test mode before using a production key.
