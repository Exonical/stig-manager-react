# docs

Astro Starlight documentation site for [stig-manager-react](https://github.com/Exonical/stig-manager-react).

## Local development

```bash
cd docs
pnpm install
pnpm dev  # http://127.0.0.1:4321
```

## Build

```bash
cd docs
pnpm build   # output: docs/dist/
pnpm preview # preview the built site
```

## Structure

```
docs/
├── astro.config.mjs              Astro + Starlight + starlight-openapi config
├── openapi/stig-manager.yaml     OpenAPI v1 spec (rendered at /reference/api)
├── public/                       Static assets
├── src/
│   ├── assets/                   Logo, hero image, etc.
│   ├── content/
│   │   ├── content.config.ts     Starlight content collection config
│   │   └── docs/
│   │       ├── index.mdx         Home (splash)
│   │       ├── features/
│   │       ├── installation/
│   │       ├── user-guide/
│   │       ├── admin-guide/
│   │       ├── reference/
│   │       └── project/
│   └── styles/custom.css         Theme overrides
├── tsconfig.json                 strict TS
└── package.json
```

## CI

`.github/workflows/docs.yaml` builds and link-checks the site on every PR
and deploys to GitHub Pages on `main`.
