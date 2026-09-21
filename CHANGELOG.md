# Changelog

This project follows [Semantic Versioning](https://semver.org/). Until the first stable release, breaking changes may occur in minor versions and are documented here.

## 0.1.0 - Unreleased

### Added

- self-hosted Docker Compose distribution with PostgreSQL, Redpanda, MinIO, migrations, workers and web application;
- deployment modes for demo, self-hosted and cloud operation;
- local sessions with cookie/CSRF protection, e-mail confirmation, password recovery, account export and account deletion;
- durable notification attempts and suppression after repeated delivery failures;
- PNCP ingestion checkpoints, bounded retries, raw archive storage and procurement-event dead-letter topic;
- public configuration endpoint, OpenAPI contract, typed web client, clean-install CI and restore validation guidance;
- local administrative bootstrap with public self-hosted registration closed by default;
- persisted terms/privacy acceptance and configurable operator identity in the web build;
- Stripe reconciliation endpoint with idempotent state repair and out-of-order subscription protection.

### Security

- production-like modes reject known development secrets and require a database-backed store;
- cloud mode requires Stripe settings, verified e-mail and SMTP TLS;
- SMTP delivery has connection deadlines, header validation and refuses remote authentication without TLS;
- worker readiness reports dependent PostgreSQL availability and initial-cycle status.

## Compatibility and upgrade policy

- A release may include forward-only database migrations. Back up PostgreSQL, Redpanda and MinIO before updating.
- Do not roll an application image back after a migration unless the release notes explicitly say the schema remains compatible.
- Restore into isolated volumes first, verify row counts and API readiness, then promote the recovery set.
- API additions preserve existing clients within a minor line; removed or changed behavior is announced in the next major release.
