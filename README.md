# LicitaLens

LicitaLens is an open-source commercial-intelligence platform that helps Brazilian suppliers discover, prioritize, and organize public procurement opportunities.

It combines official public data, deterministic and explainable ranking, alerts, and a lightweight CRM workflow. LicitaLens is **not** an official procurement portal, does not submit proposals, and does not replace the public notice or professional legal and accounting advice.

> **Project status:** active early-stage development. The repository includes a complete demonstrable flow and production-oriented foundations, but external providers and a production deployment must be configured and validated by each operator.

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

