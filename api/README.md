# api

Go HTTP API server for [stig-manager-react](https://github.com/Exonical/stig-manager-react).

- Go 1.26, [`chi`](https://github.com/go-chi/chi) router, structured logging
  via `log/slog`.
- [`pgx`](https://github.com/jackc/pgx) v5 for Postgres 18.
- OpenAPI v1 surface served at `/api/*`, generated from
  `docs/openapi/stig-manager.yaml` by [`oapi-codegen`](https://github.com/oapi-codegen/oapi-codegen).
  All 150+ operations are wired up; unimplemented endpoints return 501
  until real handlers land in later milestones.

## Local development

```bash
cd api
cp .env.example .env
export $(grep -v '^#' .env | xargs)
go run ./cmd/stigman
# listens on :54001
```

Smoke test:

```bash
curl -s http://localhost:54001/health
curl -s http://localhost:54001/api/op/appinfo | jq
```

## Test, lint, build

```bash
go vet ./...
go test ./...
go build ./...
```

## Layout

```
api/
├── cmd/stigman/main.go        program entrypoint
├── internal/
│   ├── api/                   oapi-codegen generated types + chi router
│   │   ├── gen.go             go:generate directives
│   │   ├── server-config.yaml oapi-codegen config (chi server)
│   │   ├── types-config.yaml  oapi-codegen config (models)
│   │   ├── server.gen.go      generated chi router
│   │   └── types.gen.go       generated request/response types
│   ├── config/                env-based configuration
│   ├── handlers/              top-level handlers (e.g. /health)
│   ├── server/                router wiring + APIServer overrides
│   └── store/                 Postgres data layer (scaffold)
├── go.mod
└── Containerfile              OCI multi-stage distroless build
```

## Environment

| variable                  | default                                              | description                                  |
| ------------------------- | ---------------------------------------------------- | -------------------------------------------- |
| `STIGMAN_HTTP_ADDR`       | `:54001`                                             | HTTP listen address                          |
| `STIGMAN_DATABASE_URL`    | _(empty during scaffold)_                            | Postgres DSN (`postgres://…`)                |
| `STIGMAN_ALLOWED_ORIGINS` | `http://localhost:54000,http://127.0.0.1:54000`      | comma-separated CORS allow-list              |
| `STIGMAN_LOG_LEVEL`       | `info`                                               | `debug` \| `info` \| `warn` \| `error`       |
