.PHONY: test build run sync backfill compose-up compose-down kind helm-lint commercial-up commercial-down commercial-gateway-host notifications-once phase1-up phase1-verify phase3-up phase3-verify backup-db restore-db

notifications-once:
	SMTP_HOST=localhost SMTP_PORT=1025 DATABASE_URL=postgres://licitalens:local-only@localhost:5432/licitalens NOTIFICATIONS_ONCE=true NOTIFICATIONS_WORKER=true go run ./cmd/notifications

test:
	go test ./...

build:
	go build ./cmd/...

run:
	DEMO_MODE=true go run ./cmd/gateway

sync:
	go run ./cmd/licitalens sync --max-pages=1

backfill:
	go run ./cmd/licitalens backfill --from=2021-04-01 --to=2021-04-01 --max-pages=1

compose-up:
	docker compose up -d --build

compose-down:
	docker compose down

commercial-up:
	@chmod +x scripts/commercial-up.sh scripts/commercial-down.sh scripts/commercial-gateway-host.sh
	@./scripts/commercial-up.sh

commercial-down:
	@chmod +x scripts/commercial-down.sh
	@./scripts/commercial-down.sh

commercial-gateway-host:
	@chmod +x scripts/commercial-gateway-host.sh
	@./scripts/commercial-gateway-host.sh

phase1-up:
	@chmod +x scripts/phase1-up.sh scripts/phase1-verify.sh scripts/apply-commercial-migrations.sh
	@./scripts/phase1-up.sh

phase1-verify:
	@chmod +x scripts/phase1-verify.sh
	@./scripts/phase1-verify.sh

phase3-up:
	@chmod +x scripts/phase3-up.sh scripts/phase3-verify.sh scripts/backup-postgres.sh scripts/restore-postgres.sh scripts/apply-commercial-migrations.sh scripts/phase1-verify.sh
	@./scripts/phase3-up.sh

phase3-verify:
	@chmod +x scripts/phase3-verify.sh scripts/phase1-verify.sh
	@./scripts/phase3-verify.sh

backup-db:
	@chmod +x scripts/backup-postgres.sh
	@./scripts/backup-postgres.sh

restore-db:
	@chmod +x scripts/restore-postgres.sh
	@./scripts/restore-postgres.sh $(FILE)

kind:
	kind create cluster --name licitalens

helm-lint:
	helm lint deploy/helm/licitalens

