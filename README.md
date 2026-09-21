# LicitaLens

LicitaLens is an open-source commercial-intelligence platform that helps Brazilian suppliers discover, prioritize, and organize public procurement opportunities.

It combines official public data, deterministic and explainable ranking, alerts, and a lightweight CRM workflow. LicitaLens is **not** an official procurement portal, does not submit proposals, and does not replace the public notice or professional legal and accounting advice.

> **Project status:** active early-stage development. The self-hosted flow has been validated locally from a clean installation, including bootstrap, ingestion with controlled PNCP data, session revocation, and PostgreSQL restoration. External providers and a public production deployment must still be configured and validated by each operator.

## What is included

- PNCP ingestion and raw-payload archiving;
- multi-organization Go API;
- explainable opportunity ranking;
- PostgreSQL and pgvector schemas;
- optional Redpanda/Kafka, MinIO/S3, ClickHouse, and Keycloak infrastructure;
- notification worker with SMTP and Expo Push adapters;
- subscription and billing flows prepared for Stripe;
- Expo application for web, Android, and iOS;
- demo mode for evaluating the product without external services.

## Repository structure

| Directory | Description |
| --- | --- |
| [`licitalens-backend`](licitalens-backend) | Go API, workers, ingestion, database schemas, Docker Compose, and Helm chart |
| [`licitalens-mobile`](licitalens-mobile) | Expo/React Native client for web and mobile |

## Quick start: demo

Requirements: [Bun](https://bun.sh/) and a current browser.

```bash
cd licitalens-mobile
bun install
bun run demo:web
```

This mode uses illustrative local data and does not require a backend, login, payment provider, or cloud account.

## Run your own instance

The self-hosted distribution includes PostgreSQL, Redpanda, MinIO, PNCP ingestion, the API, notification worker, and the web app. It does not require Stripe, Keycloak, or an AI provider.

```bash
cd licitalens-backend/deploy/self-hosted
./setup.sh
docker compose up -d --build
```

Public registration is closed by default. Create the first owner with the local bootstrap command documented in the self-hosted guide, then open `http://localhost:8080`. Before exposing the instance publicly, configure its legal operator identity, public URLs, HTTPS-only cookies and SMTP; enable `PUBLIC_SIGNUP` only when open registration is intentional.

See the [self-hosted guide](licitalens-backend/deploy/self-hosted/README.md) for updates, backups, SMTP, HTTPS, and the cloud-mode requirements.

## Monitoring

Prometheus collects gateway and worker health, ingestion, notification, queue, and backup-age metrics. Grafana is the optional dashboard layer. Start with the examples in [`licitalens-backend/deploy/prometheus`](licitalens-backend/deploy/prometheus) and import [`licitalens-backend/deploy/grafana/licitalens-dashboard.json`](licitalens-backend/deploy/grafana/licitalens-dashboard.json) after configuring a Prometheus data source. Do not expose worker metrics ports directly to the internet.

## Run the integrated commercial stack locally

Requirements: Go 1.25+, Docker with Compose, and Bun.

```bash
cd licitalens-backend
make commercial-up
```

In another terminal:

```bash
cd licitalens-mobile
bun install
bun run commercial:web
```

See the [backend guide](licitalens-backend/README.md), [mobile guide](licitalens-mobile/README.md), and [product roadmap](licitalens-backend/docs/PRODUCT-ROADMAP.md) for architecture, production requirements, and known boundaries.

## Releases and upgrades

Release notes and compatibility rules live in [CHANGELOG.md](CHANGELOG.md). Before every update, back up PostgreSQL, Redpanda and MinIO together, apply the published migrations, and restore a copy into isolated volumes before treating the backup as recoverable. Images are intentionally not published from this checkout until the public repository and registry owner are defined.

When a public repository is ready, tagging a validated version as `v*` triggers the checked-in release workflow: it publishes multi-architecture backend and web images to GHCR and generates the GitHub Release.

## Validate changes

```bash
cd licitalens-backend
go test ./...
go build ./cmd/...

cd ../licitalens-mobile
bun install --frozen-lockfile
bun run typecheck
```

## Security and responsible operation

Never commit `.env` files, database backups, credentials, private procurement data, or provider tokens. Demo records must remain clearly identified as illustrative, and users must verify opportunities against the official source and public notice.

Please report vulnerabilities according to [SECURITY.md](SECURITY.md).

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) before opening an issue or pull request.

## License

LicitaLens is available under the [MIT License](LICENSE).
