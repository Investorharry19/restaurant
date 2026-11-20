# Restaurant Queue & Order Notification Platform

The product and engineering requirements are in [prd.md](./prd.md). All frontend visual decisions must follow [design.md](./design.md). Use [ROADMAP.md](./ROADMAP.md) to resume and account for work between sessions; see [PRODUCT.md](./PRODUCT.md) for durable product context.

## Applications

- `client/` — Next.js App Router frontend for customers and restaurant staff.
- `server/` — Go modular-monolith API using only `net/http`.
- PostgreSQL — source of truth for application data.

The frontend uses a same-origin `/api/*` proxy to the Go API. It never connects directly to PostgreSQL and must not contain secrets.

## Local prerequisites

- Go 1.26 or later (required by the selected Goose migration-tool release).
- Node.js and pnpm (the client pins its package-manager version).
- PostgreSQL. `compose.yaml` provides a local database service when Docker Compose is available.
- Staticcheck (`go install honnef.co/go/tools/cmd/staticcheck@latest`) for the complete `make check` target.

## Local setup

1. Copy `.env.example` to `.env` and set values for your machine.
2. Copy `client/.env.example` to `client/.env.local`; set `API_INTERNAL_URL` there if the Go API is not at `http://localhost:8080`.
3. Start PostgreSQL, for example with `docker compose up -d postgres` (port `5433` on the host avoids clashing with a local PostgreSQL service), or use an existing local service and update `DATABASE_URL`.
4. In PowerShell, run `.\scripts\dev.ps1 migrate`, then `.\scripts\dev.ps1 api`. The script loads the root `.env` file into the process environment.
5. In another shell, run `pnpm install` and `pnpm dev` from `client/`.

The client uses `API_INTERNAL_URL` for its server-side proxy destination. The example uses `http://localhost:8080` for local development. Do not use a `NEXT_PUBLIC_` prefix for secrets.

## Validation

Run `make help` to see the available checks and development commands. Go tests use the standard library; frontend lint, typecheck, unit tests, formatting, and production build run from `client/`.

The race-detector check is available as `make go-test-race`; on Windows it requires CGO and a C compiler.

The credentials in `compose.yaml` and `.env.example` are for local development only; never reuse them in production.
