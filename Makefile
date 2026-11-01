GO ?= go
SQLC ?= sqlc
OAPI ?= oapi-codegen
MIGRATE ?= migrate

.PHONY: fmt test test-integration vet build generate migrate-up seed check web-check

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

test-integration:
	$(GO) test -v ./internal/database

migrate-up:
	$(MIGRATE) -path db/migrations -database "$(DATABASE_URL)" up

seed:
	$(GO) run ./cmd/seed

vet:
	$(GO) vet ./...

build:
	$(GO) build ./cmd/...

generate:
	$(SQLC) generate
	$(OAPI) --config oapi-codegen.yaml api/openapi.yaml
	cd web && npm run generate

web-check:
	cd web && npm run typecheck && npm run lint && npm run test -- --run && npm run build

check: fmt test vet build web-check
