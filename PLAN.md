# STIG Manager (React 19 + ShadCN + Go) — Project Plan

> **Status:** Proposal for review. No code written yet. Reference repos:
> upstream [NUWCDIVNPT/stig-manager](https://github.com/NUWCDIVNPT/stig-manager) and
> docs at [stig-manager.readthedocs.io](https://stig-manager.readthedocs.io/en/latest/).
> Target repo: [Exonical/stig-manager-react](https://github.com/Exonical/stig-manager-react)
> (currently empty — README only).

## 1. Goal

Recreate the full functionality of upstream STIG Manager OSS (`v1.6.10`) as a modern
two-tier application:

- **Backend:** Go service exposing the existing OpenAPI 3.0.1 surface (paths and
  schemas defined in `api/source/specification/stig-manager.yaml`, 10,482 lines).
- **Frontend:** React 19 SPA built with Vite + TypeScript + Tailwind v4 + shadcn/ui
  (Radix primitives), replacing the upstream ExtJS-based vanilla-JS client.
- **Database:** PostgreSQL 18 (replaces upstream's MySQL 8.x). The store layer is
  written against `pgx` v5 and Postgres-native types (`jsonb`, `numeric`,
  `timestamptz`, `uuid`).
- **Auth:** OIDC / OAuth2 JWT bearer (Keycloak / Okta / Azure Entra ID), PKCE flow,
  identical scope set (`stig-manager:collection`, `stig-manager:stig`,
  `stig-manager:user`, `stig-manager:op` and `:read` variants).

The goal is **feature parity**, not a fork. We re-implement from the OpenAPI spec,
the SQL migrations, and the readthedocs feature inventory.

---

## 2. Differentiation from sibling project

This is intentionally **separate from `Exonical/stig-manager-next`** (Next.js + Go).
Differences:

| Aspect            | `stig-manager-next` (existing)        | `stig-manager-react` (this project)   |
| ----------------- | ------------------------------------- | -------------------------------------- |
| Frontend runtime  | Next.js 15 / RSC + Server Actions     | React 19 SPA (Vite) — no SSR           |
| Component lib     | Next.js conventions + Tailwind        | shadcn/ui (Radix + Tailwind v4)        |
| Hosting model     | Embeddable Next build + Go backend    | Static SPA served from Go (or CDN)     |
| Routing           | App Router (file-based)               | TanStack Router (typed routes)         |
| Data fetching     | RSC + Server Actions                  | TanStack Query against Go REST API     |

Both can coexist; this PR/repo focuses solely on the SPA + shared Go backend.

---

## 3. Source inventory (what we must recreate)

Pulled directly from upstream:

### 3.1 API surface (OpenAPI v1) — 21 path families
```
/assets                    /collections           /jobs            /op/appdata
/op/appdata/tables         /op/appinfo            /op/configuration
/op/definition             /op/details            /op/state        /op/state/sse
/stigs                     /user                  /user/web-preferences
/users                     /user-groups
```
Sub-paths nest deeply under `/collections/...` (reviews, grants, metrics, labels,
stigs, assets, history, exports, CKL/CKLB/XCCDF imports, POA&M, clone, etc.).

### 3.2 Controllers (Express) — to be replaced by Go handler packages
```
Asset.js (599)   Collection.js (1287)   Job.js (171)   Metrics.js (115)
Operation.js (164)  Review.js (455)  STIG.js (271)  User.js (380)
```

### 3.3 Services (data + business logic) — to be replaced by Go service packages
```
AssetService (1783)   CollectionService (2839)   JobService (327)
MetricsService (679)  OperationService (1015)    ReviewService (1470)
STIGService (1468)    UserService (571)          utils (896)
```
≈11k LoC of JS service logic — most of it SQL composition. The Go port will use
sqlc (typed queries) plus hand-written query builders for the dynamic projections.

### 3.4 Database migrations — 47 numbered scripts (MySQL upstream)
`0000.js … 0046.js` plus a `sql/` directory with seed/lib scripts. We do **not**
port these incrementally — we synthesize the cumulative end-state schema as
**one Postgres baseline migration** (`0001_baseline.sql`) using goose. Type
translations: `BIGINT UNSIGNED` → `bigint`, `TINYINT(1)` → `boolean`,
`JSON` → `jsonb`, `DATETIME` → `timestamptz`, `MEDIUMTEXT`/`LONGTEXT` → `text`,
`CHAR(36)` UUIDs → `uuid`. Generated-column projections that MySQL used are
rebuilt as Postgres generated columns or views.

### 3.5 Upstream client modules (43 files in `client/src/js/SM/`)
These map cleanly to React feature folders:
```
Acl, ActivityHandler, Ajax, ApiState, AppData, AppInfo, AssetSelection,
Attachments, BatchReview, Cache, Checklist, Classification, CollectionClone,
CollectionPanel, ColumnFilters, Error, EventDispatcher, Exports, FindingsPanel,
FlexboxLayout, Global, Grant, Inventory, Job, Library, LogStream, MainPanel,
Manage, MetaPanel, NavTree, Review, ReviewsImport, RowEditorToolbar,
SelectingGridToolbar, ServiceWorker, StackTrace, State, StigRevision,
TipContent, TransferAssets, User, UserGroup, WhatsNew
```

### 3.6 Parsers (security-critical, must port faithfully)
- **XCCDF** (DISA STIG / SCAP) — `parsers.js` builds a `Benchmark` from XCCDF XML.
- **CKL** (StigViewer XML) — older format.
- **CKLB** (StigViewer JSON) — newer format.
- **POA&M xlsx** generation — two templates (`poam-template.xlsx`,
  `poam-template-mccast.xlsx`).

### 3.7 Cross-cutting features (from readthedocs)
- Collection-based organization, RMF mapping, customizable labels.
- Real-time dashboards: completion %, severity counts, CORA risk scoring.
- Review handling across STIG revision changes (Rule Version + Check Content
  hashing — the "republished rules" workflow).
- Bulk operations: Collection Review workspace, batch update, import/export.
- Role-based access: Restricted / Full / Manage / Owner.
- Review Accept / Reject validation workflow.
- Rule exceptions, review age tracking.
- POA&M export (eMASS).
- API-first: same OpenAPI for client and integrations.

---

## 4. Architecture overview

```
                                    ┌───────────────────────────────┐
                                    │  OIDC Provider                │
                                    │  (Keycloak / Okta / Entra ID) │
                                    └───────────────┬───────────────┘
                                                    │ JWT (PKCE)
┌────────────────────────────┐                      │
│  Browser (React 19 SPA)    │  ───── REST /api ──► │
│  Vite • TS • shadcn/ui     │  ◄──── SSE /op ───── │
│  TanStack Router/Query     │                      │
└────────────────────────────┘                      │
            ▲                                       ▼
            │ static assets                ┌──────────────────────┐
            └──────────────────────────────│  Go backend          │
                                           │  chi router          │
                                           │  oapi-codegen types  │
                                           │  service packages    │
                                           │  sqlc + driver       │
                                           └──────────┬───────────┘
                                                      │
                                                ┌─────▼──────┐
                                                │  MySQL 8.x │
                                                └────────────┘
```

The Go binary serves both the API (`/api/*`) and the SPA static bundle (`/`),
mirroring the upstream's single-process deployment story. Docs (Sphinx HTML) and
swagger-ui are mounted at `/docs` and `/api-docs` like upstream.

---

## 5. Repository layout

```
stig-manager-react/
├── api/                       # Go backend
│   ├── cmd/stigman/main.go
│   ├── internal/
│   │   ├── config/            # env-driven config (mirrors upstream env vars)
│   │   ├── auth/              # JWT validation, JWKS cache, scope/role checks
│   │   ├── httpx/             # router, middleware, error mapping, SSE
│   │   ├── handlers/          # asset, collection, review, stig, job, user, op
│   │   ├── service/           # business logic (one pkg per domain)
│   │   ├── store/             # sqlc-generated queries + dynamic builders
│   │   ├── parser/            # xccdf, ckl, cklb, poam (xlsx)
│   │   ├── migrate/           # goose migrations (baseline + forward)
│   │   └── jobs/              # background scheduler (cron-like, persisted)
│   ├── api/openapi.yaml       # spec (forked, kept in lockstep)
│   ├── migrations/            # *.sql goose files
│   └── go.mod
│
├── web/                       # React 19 SPA
│   ├── src/
│   │   ├── app/               # router config, providers, layout
│   │   ├── features/          # collections/, assets/, reviews/, stigs/,
│   │   │                      # users/, grants/, jobs/, metrics/, library/
│   │   ├── components/        # shadcn-generated primitives + shared
│   │   ├── lib/api/           # generated TS client (from openapi.yaml)
│   │   ├── lib/auth/          # oidc-client-ts wrapper, PKCE flow
│   │   ├── lib/parsers/       # client-side CKL/CKLB preview (optional)
│   │   └── styles/
│   ├── package.json           # pnpm; React 19 + Vite + Tailwind v4
│   └── tsconfig.json
│
├── docs/                      # User & admin docs (Markdown via Astro Starlight
│                              # OR ported Sphinx; decision in §15)
├── deploy/
│   ├── docker/                # Dockerfile (multi-stage: web build → go build)
│   ├── compose/               # docker-compose for local dev (MySQL + Keycloak)
│   └── helm/                  # optional, follows kubevirt-management pattern
├── .github/workflows/         # ci.yaml, release.yaml, docker.yaml
├── Makefile
├── PLAN.md
└── README.md
```

---

## 6. Backend (Go) design

### 6.1 Tech choices

| Concern              | Choice                              | Why                                  |
| -------------------- | ----------------------------------- | ------------------------------------ |
| HTTP router          | `go-chi/chi` v5                     | Stable, std-lib compatible, fast     |
| OpenAPI binding      | `oapi-codegen` (types + chi server) | Generates types directly from spec   |
| Validation           | `kin-openapi` middleware            | Request/response validation          |
| DB driver            | `github.com/jackc/pgx/v5`           | Postgres 18 native driver            |
| Query layer          | `sqlc` (engine: postgresql) + hand-written builders | Typed simple queries; builders for projections |
| Migrations           | `pressly/goose`                     | SQL-first, embed-able                |
| JWT                  | `github.com/lestrrat-go/jwx/v2`     | JWKS, kid rotation, alg allowlist    |
| Logging              | `log/slog` + JSON handler           | std lib, structured                  |
| Config               | env-only, mirroring upstream vars   | Drop-in compatibility                |
| SSE / WS             | std `http.Flusher` + `gorilla/websocket` for log socket | Matches upstream `/op/state/sse` + log socket |
| Excel (POA&M)        | `xuri/excelize/v2`                  | Read upstream xlsx templates         |
| XML parsing          | `encoding/xml` + custom decoder for SCAP/XCCDF | Replaces fast-xml-parser  |
| Archive (CKL bundle) | `archive/zip` (std)                 | Replaces archiver/jszip              |

### 6.2 Configuration parity

Upstream environment variables are preserved (same names) where applicable.
DB-specific renames (kept for clarity, with old names accepted as fallbacks):
`STIGMAN_DB_HOST`, `STIGMAN_DB_PORT` (default `5432`),
`STIGMAN_DB_NAME` (was `STIGMAN_DB_SCHEMA`, fallback honored),
`STIGMAN_DB_USER`, `STIGMAN_DB_PASSWORD`,
`STIGMAN_DB_SSLMODE` (`disable`|`require`|`verify-full`).
Other examples: `STIGMAN_API_ADDRESS/PORT`,
`STIGMAN_OIDC_PROVIDER`, `STIGMAN_JWT_*`, `STIGMAN_CLIENT_*`, `STIGMAN_CLASSIFICATION`,
`STIGMAN_API_MAX_JSON_BODY`, `STIGMAN_API_MAX_UPLOAD`, etc. Documented in
`docs/installation-and-setup/environment-variables.rst` (we port the CSVs).

### 6.3 Service module map (1:1 with upstream)

| Upstream service       | Go package                            |
| ---------------------- | ------------------------------------- |
| `AssetService.js`      | `internal/service/asset`              |
| `CollectionService.js` | `internal/service/collection`         |
| `ReviewService.js`     | `internal/service/review`             |
| `STIGService.js`       | `internal/service/stig`               |
| `MetricsService.js`    | `internal/service/metrics` (CORA)     |
| `JobService.js`        | `internal/service/job`                |
| `OperationService.js`  | `internal/service/op` (appinfo, appdata, SSE state, definition) |
| `UserService.js`       | `internal/service/user` (+ user-groups + grants) |

### 6.4 Auth flow

1. Browser does PKCE login against OIDC provider, receives JWT.
2. Every API request carries `Authorization: Bearer <jwt>`.
3. Middleware validates: signature against JWKS (cached, kid-aware), `iss`,
   `aud` (optional via env), `exp`, alg ∈ allowlist (RS256/ES256), and scopes.
4. Claim mapping (configurable via env, same defaults as upstream):
   - username ← `preferred_username`
   - name ← `name`
   - email ← `email`
   - privileges ← `realm_access.roles` (array)
   - scope ← `scope` (space-separated) or `scp`
   - assertion id ← `jti` (or `uti`)
   - service name ← `clientId`
5. Role check (`Restricted`=1, `Full`=2, `Manage`=3, `Owner`=4) happens in
   service layer based on collection-scoped grants stored in DB.

### 6.5 Background jobs

Upstream's `/jobs` API supports schedulable tasks. We replicate with a persisted
scheduler (rows in `job` + `job_run` + `job_task` tables, leader-elected via a
DB advisory lock if multi-replica).

---

## 7. Frontend (React 19 + shadcn/ui) design

### 7.1 Tech choices

| Concern              | Choice                                       |
| -------------------- | -------------------------------------------- |
| Framework            | React 19 (concurrent, `use()`, transitions)  |
| Bundler              | Vite 6                                       |
| Language             | TypeScript 5 (strict)                        |
| Styling              | Tailwind v4 + CSS variables                  |
| Components           | shadcn/ui (Radix), installed via shadcn MCP  |
| Routing              | TanStack Router (typed, file-based)          |
| Data fetching        | TanStack Query v5 (infinite for grids)       |
| Forms                | React Hook Form + Zod                        |
| Tables               | TanStack Table v8 (server-driven for grids)  |
| Virtualization       | TanStack Virtual (large review tables)       |
| Charts               | Recharts (dashboards)                        |
| OIDC                 | `oidc-client-ts` + custom React adapter      |
| State (cross-cut)    | Zustand (UI state); server state via Query   |
| Date / time          | `date-fns` (no moment)                       |
| File upload          | `react-dropzone` + chunked upload to Go      |
| Diff (review)        | `diff` lib + custom view (replaces diff2html)|
| API client           | Generated from `openapi.yaml` via `openapi-typescript` + custom fetcher |

### 7.2 Feature folders → upstream screens

| React feature           | Upstream module(s)                          | Notes                                  |
| ----------------------- | ------------------------------------------- | -------------------------------------- |
| `collections/`          | CollectionPanel, CollectionClone, MainPanel | List, create, clone, settings, grants  |
| `collection-dashboard/` | MetaPanel, FindingsPanel                    | CORA, severity, completion charts      |
| `assets/`               | Inventory, AssetSelection, TransferAssets   | Asset grid, attach/detach STIGs        |
| `reviews/`              | Review, BatchReview, ReviewsImport          | Single + collection review workspaces  |
| `library/`              | Library, StigRevision, Checklist            | STIG library, revision diff            |
| `users/`                | User, UserGroup, Grant                      | User mgmt, groups, ACLs                |
| `jobs/`                 | Job, LogStream                              | Scheduler UI, live run output          |
| `appinfo/`              | AppInfo, AppData, Manage                    | Admin telemetry, export/import         |
| `auth/`                 | Acl, ApiState                               | Login, token refresh, scope guards     |
| `shared/`               | ColumnFilters, Attachments, Exports, TipContent, WhatsNew, Classification banner | Cross-cutting UI |

### 7.3 Layout

- Persistent left nav rail (collapsible) — Collections, Library, Admin
  (conditional on `stig-manager:op`), User menu.
- Top bar: classification banner (configurable via `STIGMAN_CLASSIFICATION`),
  global search, notifications (job completions), user profile.
- Main pane: route-driven; supports split panes (asset/review side-by-side) via
  `react-resizable-panels`.
- Right inspector drawer for rule details (CCI, control mapping, fix text).

### 7.4 Accessibility

- shadcn/Radix primitives are WAI-ARIA compliant out of the box.
- All grids: keyboard navigation, row selection via space/enter, aria-rowcount.
- Color is never the only signal for severity (icons + text + color).
- Honors `prefers-reduced-motion`.

---

## 8. Data model (baseline)

Replicates upstream schema. Core entities:

```
collection ─┬─< asset ───< asset_stig >─── stig (benchmark + revisions)
            │                                   │
            ├─< grant >─── user_or_user_group   ├─< rule ──< check, fix, cci, control_map
            ├─< label  >── asset_label          └─< review_history (republished rules)
            └─< setting/metadata

review (asset_id, rule_id, result, detail, comment, status, ts_ack, ts_action,
        evaluator_id, reviewer_id, attachments, severity_override, ...)

job ─< job_run ─< job_run_event           audit_log (write-ahead per op)
job_task                                  rule_check_content_hash (matching algorithm)
```

Baseline migration is the synthesized end-state from upstream's 47 migrations
(verified by `mysqldump --no-data` of a freshly-bootstrapped upstream container).
Forward migrations are numbered from `1000_*` onwards to leave room.

---

## 9. Imports, exports, and review matching

### 9.1 Imports (multipart upload → Go parser → DB)
- **XCCDF** (.xml / SCAP zip): produces a Benchmark + Revision; matched against
  existing rules by `ruleId@version` and check content hash.
- **CKL** (.ckl XML): produces Review rows tied to an Asset; partial imports
  allowed.
- **CKLB** (.cklb JSON): same as CKL but JSON.
- **Asset batch CSV** (matches upstream's
  `Stig-Manager-Asset-Batch-Import.csv` shape).

### 9.2 Exports
- **CKL / CKLB** download (per asset, per stig, or bundled .zip).
- **XCCDF results** (XCCDF v1.2 result file).
- **POA&M xlsx** (eMASS-compatible; default + MCCAST template).
- **Collection AppData JSON** (full backup/restore via `/op/appdata`).

### 9.3 Review handling across STIG revisions
We implement the exact algorithm documented in
[review-handling.rst](https://github.com/NUWCDIVNPT/stig-manager/blob/main/docs/user-guide/review-handling.rst):
- A review survives a STIG revision bump if `(ruleVersion, checkContentSha256)`
  matches.
- Otherwise it is orphaned and shown in a "Republished Rules" view.

---

## 10. Operations & observability

- **/op/appinfo**: cluster + node metrics, MySQL status, user/role counts,
  collection grants summary — same shape as upstream `AppInfo` schema.
- **/op/state/sse**: server-sent events for "starting/available/degraded".
- **/op/definition**: returns the embedded OpenAPI definition.
- **/op/configuration**: redacted env summary (booleans + non-secret strings).
- **Log socket**: WebSocket-based live tail of structured JSON logs (admin-only).
- Prometheus `/metrics` endpoint (additive; not in upstream).

---

## 11. Testing strategy

### Backend (Go)
- Unit tests per service package (table-driven; fakes for store).
- Parser corpus tests: a fixtures directory with real DISA XCCDF / CKL / CKLB
  samples (we vendor the same fixtures upstream uses in `test/`).
- Integration tests against a real MySQL via testcontainers-go.
- OpenAPI conformance tests using `kin-openapi` to validate handler responses.

### Frontend (React)
- Vitest + Testing Library for components and hooks.
- Playwright for end-to-end (login → create collection → import CKL → review).
- Storybook (optional) for shadcn-composed components.

### CI (`.github/workflows/ci.yaml`)
1. `go vet`, `staticcheck`, `golangci-lint`.
2. `go test ./... -race -cover`.
3. `pnpm typecheck`, `pnpm lint`, `pnpm test`.
4. `pnpm build` (sanity).
5. (PR only) Playwright smoke against a docker-compose stack.

---

## 12. Deployment

- **Local dev:** `docker compose up` brings Postgres 18 + Keycloak (with a seed
  realm) + the Go API (live reload via `air`) + the Vite dev server (proxied at
  `/api`) + the Astro Starlight docs (`pnpm --filter docs dev`).
- **Single-binary prod:** multi-stage Dockerfile builds the SPA, embeds the
  `dist/` into the Go binary (via `embed.FS`), and produces a < 60 MB image.
- **Helm chart:** optional, mirrors the pattern from `kubevirt-management`.

---

## 13. Milestones (proposed sequencing)

> Each milestone ends in a runnable PR. Smaller PRs preferred; this list is the
> rollup. We commit to feature parity, not all at once.

| # | Milestone                                          | Output                                          |
|---|----------------------------------------------------|-------------------------------------------------|
| 0 | **Docs site (Astro Starlight)** ◄ start here       | `docs/` workspace, full IA, content ported from upstream Sphinx, OpenAPI v1 rendered, CI green, preview deploy. |
| 1 | **Repo scaffold (backend + frontend)**             | Go workspace, Vite SPA, Tailwind v4 + shadcn init, CI, Dockerfile, docker-compose (Postgres 18 + Keycloak). |
| 2 | **OpenAPI lift + types**                           | Vendored upstream openapi.yaml; oapi-codegen wired; openapi-typescript wired; empty handlers returning 501. |
| 3 | **Auth (OIDC) end-to-end**                         | PKCE login on SPA; JWT validation middleware; `/user` returns the caller. |
| 4 | **DB + baseline migration (Postgres 18)**          | goose baseline schema; can boot and pass schema validation against fresh DB. |
| 5 | **STIG library**                                   | `/stigs/*` endpoints; XCCDF upload + parse + persist; library UI screen. |
| 6 | **Collections & assets**                           | `/collections`, `/assets`, grants, labels; Collections list/detail + Inventory screens. |
| 7 | **Reviews (single asset)**                         | `/collections/.../reviews` CRUD; Review workspace UI; check/fix/CCI panels. |
| 8 | **Imports (CKL / CKLB / XCCDF results)**           | Bulk upload pipeline + reconciliation with existing reviews. |
| 9 | **Batch / Collection Review workspace**            | Multi-asset grid for a single Rule; bulk updates. |
| 10 | **Metrics & CORA dashboard**                       | `/collections/.../metrics`; Recharts dashboards. |
| 11 | **Exports (CKL bundle, XCCDF results, POA&M xlsx)** | Excelize templates, archive/zip bundles. |
| 12 | **Users, user-groups, grants UI**                  | Admin screens; ACL evaluation. |
| 13 | **Jobs & log socket**                              | Scheduler + UI + WebSocket live tail. |
| 14 | **AppInfo / AppData / op state SSE**               | Admin telemetry pages; classification banner. |
| 15 | **Review-handling algorithm + Republished Rules**  | Revision diff UI; rule matching tests. |
| 16 | **Hardening**                                      | Rate limits, audit log surface, Playwright e2e, helm chart. |

---

## 14. Risks & open questions for you (Bryce)

Please confirm or correct before I start scaffolding:

**Decisions confirmed by user (2026-05-19):**
- DB: **Postgres 18** (no MySQL fallback).
- API compat: **keep `/api/v1` byte-compatible with upstream**.
- Docs: **Astro Starlight, start with docs first.**

Still open (defaults shown — call out anything to change):
1. **License.** Default: **MIT** (upstream is MIT + federal-employee public
   domain).
2. **Auth providers for testing.** Default: bundle a **Keycloak compose preset**
   with a seed `stigman` realm + demo users + scopes.
3. **Embedding the SPA in the Go binary** vs. serving from a separate static
   host / CDN. Default: **embed** via `embed.FS`; opt-out via env for CDN mode.
4. **Realtime updates** beyond op-state SSE + log-socket. Default: add SSE for
   live review updates **post-v1**.
5. **Test fixtures.** Default: vendor a subset of upstream's `test/` corpus
   (XCCDF, CKL, CKLB samples).

---

## 15. What I will do next

1. Land **Milestone 0 (Docs)** in one PR: Astro Starlight site under `docs/`,
   IA mirroring upstream readthedocs, content ported from Sphinx `.rst` to
   MD/MDX, OpenAPI v1 rendered inside the docs, CI green, preview deploy.
2. Then **Milestone 1 (Scaffold)**: Go + Vite + shadcn + docker-compose (Postgres
   18 + Keycloak) + CI.
3. Then proceed milestone-by-milestone, one PR each.
