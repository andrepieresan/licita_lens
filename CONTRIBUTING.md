# Contributing to LicitaLens

Thank you for helping improve LicitaLens.

## Before opening a pull request

1. Keep changes focused and document behavior that affects users or operators.
2. Do not include secrets, `.env` files, database dumps, tokens, or private customer data.
3. Preserve the product boundary: scores are commercial indicators, not legal qualification or procurement decisions.
4. Run the relevant checks.

Backend:

```bash
cd licitalens-backend
go test ./...
go vet ./...
go build ./cmd/...
```

Mobile:

```bash
cd licitalens-mobile
bun install --frozen-lockfile
bun run typecheck
```

Infrastructure:

```bash
cd licitalens-backend
docker compose config --quiet
helm lint deploy/helm/licitalens
```

## Pull requests

Explain the problem, the proposed change, how it was validated, and any remaining limitation. Add tests when changing business rules or security-sensitive behavior.

For security vulnerabilities, follow [SECURITY.md](SECURITY.md) instead of opening a public issue.

