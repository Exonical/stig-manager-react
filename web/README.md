# web

React 19 SPA for [stig-manager-react](https://github.com/Exonical/stig-manager-react).

- Vite 6 + React 19 + TypeScript (strict)
- Tailwind CSS v4 (`@tailwindcss/vite`)
- shadcn/ui components (copied into `src/components/ui`)
- ESLint flat config + Prettier

## Local development

```bash
pnpm install                # from repo root
pnpm --filter web dev       # http://localhost:54000
```

The dev server proxies `/api/*` to the Go backend (default
`http://localhost:54001`). Override with `VITE_API_PROXY_TARGET`.

## Scripts

| script              | description                          |
| ------------------- | ------------------------------------ |
| `pnpm dev`          | Vite dev server (port 54000)         |
| `pnpm build`        | Production build to `dist/`          |
| `pnpm preview`      | Serve the production build           |
| `pnpm typecheck`    | `tsc -b --noEmit`                    |
| `pnpm lint`         | ESLint                               |
| `pnpm format`       | Prettier (write)                     |

## Layout

```
web/
├── public/              static assets
├── src/
│   ├── components/
│   │   ├── ui/          shadcn primitives
│   │   └── ...
│   ├── lib/             utils, API client
│   ├── styles/          Tailwind / global CSS
│   ├── App.tsx
│   └── main.tsx
├── vite.config.ts
├── tsconfig*.json
└── package.json
```
