# Frontend

The customer and staff experiences share this Next.js App Router application. The Go API owns business rules and persistence; the browser reaches it through the same-origin `/api/*` rewrite.

## Commands

Run from this directory:

```powershell
Copy-Item .env.example .env.local
pnpm install
pnpm dev
pnpm lint
pnpm typecheck
pnpm test
pnpm format:check
pnpm build
```

Set `API_INTERNAL_URL` in the Next.js server environment when the Go API is not at `http://localhost:8080`. It must be an HTTP(S) origin without a path or credentials.

Before creating or changing any UI, read the root `design.md` and the relevant sections of `prd.md`. Keep API requests in `lib/api` and validate response data at runtime.
