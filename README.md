# stig-manager-react

A modern re-implementation of
[NUWCDIVNPT/stig-manager](https://github.com/NUWCDIVNPT/stig-manager):

- **Frontend:** React 19 + Vite + TypeScript + [shadcn/ui](https://ui.shadcn.com).
- **Backend:** Go (`net/http` + `chi`), OpenAPI 3.0.1 v1 — byte-compatible with upstream.
- **Database:** PostgreSQL 18.
- **Docs:** [Astro Starlight](https://starlight.astro.build/) (this repo's `docs/`).

The project is staged across a series of milestones; the current PR adds
Milestone 0, the documentation site.

## Repository layout

```
.
├── docs/                 Astro Starlight docs site (Milestone 0 — this PR)
├── api/                  Go backend                     (Milestone 1+)
├── web/                  React 19 SPA                   (Milestone 1+)
├── deploy/               Docker / Compose / Helm        (Milestone 1+)
├── tools/                supporting scripts             (later)
├── .github/workflows/    CI
└── pnpm-workspace.yaml   pnpm workspace definition
```

## Quick start (docs)

```bash
pnpm install
pnpm --filter docs dev
# open http://127.0.0.1:4321
```

To build the static site:

```bash
pnpm --filter docs build
# output: docs/dist/
```

## Quick start (application)

*(Available from Milestone 1 onward.)*

```bash
docker compose -f deploy/compose/docker-compose.yaml up -d
# SPA: http://localhost:54000
# Docs: http://localhost:54000/docs
```

## Status

See the roadmap in [`docs/`](./docs/src/content/docs/project/roadmap.mdx)
for the up-to-date milestone status.

## License

MIT — see [`LICENSE`](./LICENSE). Portions of the OpenAPI spec and
documentation are carried forward from the upstream NUWCDIVNPT/stig-manager
project; refer to upstream's `LICENSE.md` and `INTENT.md` for their
applicable terms.
