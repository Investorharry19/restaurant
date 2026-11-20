.PHONY: help dev-db api migrate go-test go-test-race go-vet go-staticcheck go-fmt client-install client-lint client-typecheck client-test client-format client-format-check client-build check

help:
	@echo "Targets: dev-db api migrate go-test go-test-race go-vet go-staticcheck go-fmt client-install client-lint client-typecheck client-test client-format client-format-check client-build check"

dev-db:
	docker compose up -d postgres

api:
	cd server && go run ./cmd/api

migrate:
	cd server && go run ./cmd/migrate

go-test:
	cd server && go test ./...

go-test-race:
	cd server && go test -race ./...

go-vet:
	cd server && go vet ./...

go-staticcheck:
	cd server && staticcheck ./...

go-fmt:
	cd server && gofmt -w ./cmd ./internal

client-install:
	cd client && pnpm install

client-lint:
	cd client && pnpm lint

client-typecheck:
	cd client && pnpm typecheck

client-test:
	cd client && pnpm test

client-format:
	cd client && pnpm format

client-format-check:
	cd client && pnpm format:check

client-build:
	cd client && pnpm build

check: go-test go-vet go-staticcheck client-lint client-typecheck client-test client-format-check client-build
