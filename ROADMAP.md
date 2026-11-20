# Delivery Roadmap

This is the durable progress record for implementing `prd.md` and `design.md`. Work one milestone at a time; update this file before ending each implementation session.

## Current position

- **Active milestone:** Phase 0 — Repository inspection and foundation
- **Status:** In progress — code foundation verified; database-backed exit gate remains open.
- **Next task:** Configure a usable local PostgreSQL connection, run the migration command, start the API, and verify `/healthz` and `/readyz`.
- **Decision rule:** The PRD defines behavior and engineering constraints; `design.md` governs every frontend visual decision. Ask before changing either source of truth.

## Project shape

- `client/`: Next.js App Router, TypeScript, customer and staff experiences.
- `server/`: Go modular monolith; standard-library `net/http`; PostgreSQL persistence and migrations.
- `server/internal/`: focused configuration, database, HTTP, and domain packages. Keep business rules out of handlers.
- `server/migrations/`: ordered SQL migrations.
- `PRODUCT.md`: durable product facts; `prd.md` and `design.md` remain the detailed specifications.

The browser talks to the Go API through a same-origin `/api/*` proxy. The Go API is the only layer that owns business rules and database access.

## Milestones

### Phase 0 — Repository inspection and foundation

**Status: In progress**

- [x] Inspect the existing Next.js app, Go directory, package scripts, and local toolchain.
- [x] Confirm Next.js + Go `net/http` + PostgreSQL as the project stack.
- [x] Add validated Go environment configuration, PostgreSQL connectivity, and a migration runner.
- [x] Add the `net/http` server, middleware, bounded timeouts, graceful shutdown, health/readiness endpoints, and standard API envelopes.
- [x] Add the same-origin API proxy and a typed, runtime-checked frontend API client.
- [x] Add shared local-development, lint, formatting, test, and typecheck commands with setup documentation.
- [x] Verify backend tests, frontend lint/typecheck/tests/build, and static analysis.
- [ ] Verify database connectivity, migrations, and the running API against a configured PostgreSQL database.
- [ ] Run `go test -race ./...` on a host with CGO and a C compiler.

**Exit gate:** Both apps start, PostgreSQL connects, migrations run, and the foundation tests pass.

**Verification record:**

- `make check` passes: Go tests, `go vet`, Staticcheck, frontend ESLint, TypeScript, Vitest, Prettier, and production build.
- The local PostgreSQL service is running, but it rejected the example role/password. No Docker command is available in this shell, so the database-backed migration and API-start checks remain open.
- `go test -race ./...` could not run because `CGO_ENABLED=0` and no C compiler is installed. The `go-test-race` target is available for a configured host.
- No product tables exist yet; their first migration belongs to Phase 1. The migration command reports this explicitly and exits successfully when the migration directory contains no SQL migrations.

### Phase 1 — Restaurant and staff foundation

**Status: Not started**

- Restaurant profile and membership schema, migrations, and management services.
- Staff accounts, Argon2id password hashing, server-side sessions, login/logout, CSRF, and permission checks.
- Restaurant tenant scoping and audit log foundation.

**Exit gate:** An owner/manager can authenticate and access only their restaurant; tenant isolation is tested.

### Phase 2 — Customer QR and queue tickets

**Status: Not started**

- Public restaurant lookup and QR destination.
- Customer sessions, table/pickup queue creation, ticket numbering, secure ticket tokens, and ticket status.
- Server-calculated queue position and customer ticket UI.

**Exit gate:** A customer can securely join a queue and retrieve their ticket after a refresh.

### Phase 3 — Staff queue and table operations

**Status: Not started**

- Staff queue dashboard, call/arrive/no-show/cancel/complete actions.
- Table CRUD, availability, safe assignment/release, and audit events.
- Concurrency-safe queue/table operations and tests.

**Exit gate:** Staff can manage a table queue through seating without duplicate calls or assignments.

### Phase 4 — Orders and pickup

**Status: Not started**

- Order creation/listing, valid status transitions, pickup tickets, and kitchen view.
- Ready and picked-up actions.

**Exit gate:** Staff can manage an order from acceptance through pickup.

### Phase 5 — Real-time updates

**Status: Not started**

- Transactional outbox and publisher.
- Authorized customer and staff SSE streams.
- Frontend reconnect and refetch behavior.

**Exit gate:** Customer and staff screens update without manual refresh and recover correctly after reconnect.

### Phase 6 — Web Push notifications

**Status: Not started**

- Customer permission UX, service worker, subscription management, VAPID configuration.
- Asynchronous notification jobs, retries, invalid-subscription handling, and history.

**Exit gate:** An opted-in supported device receives the appropriate ticket/order notification; failures are observable and retryable.

### Phase 7 — Hardening and deployment

**Status: Not started**

- Security, privacy, tenant-isolation, and concurrency review.
- Rate limiting, observability, production health checks, and end-to-end acceptance.
- Container/deployment configuration and production environment documentation.

**Exit gate:** The MVP acceptance criteria in `prd.md` pass and production setup is documented.

## Session handoff protocol

At the start of each work session:

1. Read this file and resume the active milestone's first unchecked item.
2. Read the corresponding PRD sections and `design.md` before frontend changes.
3. Inspect the affected code and tests; preserve work already in progress.

Before ending a session:

1. Run the smallest relevant checks and record the exact results.
2. Update checkboxes, milestone status, and the next concrete task here.
3. Record blockers or decisions rather than silently skipping requirements.
4. Mark a milestone complete only when its exit gate is verified.
