# stig-manager-react

A modern re-implementation of
[NUWCDIVNPT/stig-manager](https://github.com/NUWCDIVNPT/stig-manager):

- **Frontend:** React 19 + Vite + TypeScript + [shadcn/ui](https://ui.shadcn.com).
- **Backend:** Go 1.26 (`net/http` + `chi`), OpenAPI 3.0.1 v1 — byte-compatible with upstream.
- **Database:** PostgreSQL 18 (via pgx v5).
- **Auth:** OIDC / OAuth 2.0 PKCE (Keycloak / Okta / Azure Entra ID).
- **Docs:** [Astro Starlight](https://starlight.astro.build/) (this repo's `docs/`).

## Repository layout

```
.
├── api/                  Go backend (chi, pgx, slog)
├── web/                  React 19 SPA (Vite, Tailwind v4, shadcn/ui)
├── docs/                 Astro Starlight docs site
├── deploy/               docker-compose + Keycloak realm import
├── .github/workflows/    CI (web, api, docs)
├── pnpm-workspace.yaml   pnpm workspace (docs + web)
└── pnpm-lock.yaml        single lockfile at repo root
```

## Prerequisites

- **Node.js 24** and **pnpm 9** (via [Corepack](https://nodejs.org/api/corepack.html)).
- **Go 1.26** for the API.
- **Docker / Docker Compose** for the integrated dev stack.

## Quick start (everything via compose)

```bash
docker compose -f deploy/compose/docker-compose.yaml up -d --build
# SPA:      http://localhost:54000
# API:      http://localhost:54001
# Keycloak: http://localhost:8080  (admin / admin)
```

## Quick start (host development)

From the repo root, install JS deps:

```bash
pnpm install
```

In three terminals:

```bash
# 1. Go API on :54001
cd api && cp -n .env.example .env && go run ./cmd/stigman

# 2. React SPA on :54000 (proxies /api/* to the Go API)
pnpm --filter @stig-manager-react/web dev

# 3. Docs site on :4321
pnpm --filter docs dev
```

## Status

See the roadmap in [`docs/`](./docs/src/content/docs/project/roadmap.mdx)
for the up-to-date milestone status.

## License

MIT — see [`LICENSE`](./LICENSE). Portions of the OpenAPI spec and
documentation are carried forward from the upstream NUWCDIVNPT/stig-manager
project; refer to upstream's `LICENSE.md` and `INTENT.md` for their
applicable terms.
