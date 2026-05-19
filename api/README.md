# api

Go HTTP API server for [stig-manager-react](https://github.com/Exonical/stig-manager-react).

- Go 1.23, [`chi`](https://github.com/go-chi/chi) router, structured logging
  via `log/slog`.
- [`pgx`](https://github.com/jackc/pgx) v5 for Postgres 18.
- OpenAPI v1 surface (currently stubbed: `/api/v1/op/appinfo`,
  `/api/v1/op/appdata/tables`). Code-generated handlers from
  `docs/openapi/stig-manager.yaml` arrive in Milestone 2.

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
curl -s http://localhost:54001/api/v1/op/appinfo | jq
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
│   ├── config/                env-based configuration
│   ├── handlers/              HTTP handlers (scaffold)
│   ├── server/                router wiring
│   └── store/                 Postgres data layer (scaffold)
├── go.mod
└── Dockerfile                 multi-stage distroless build
```

## Environment

| variable                  | default                                              | description                                  |
| ------------------------- | ---------------------------------------------------- | -------------------------------------------- |
| `STIGMAN_HTTP_ADDR`       | `:54001`                                             | HTTP listen address                          |
| `STIGMAN_DATABASE_URL`    | _(empty during scaffold)_                            | Postgres DSN (`postgres://…`)                |
| `STIGMAN_ALLOWED_ORIGINS` | `http://localhost:54000,http://127.0.0.1:54000`      | comma-separated CORS allow-list              |
| `STIGMAN_LOG_LEVEL`       | `info`                                               | `debug` \| `info` \| `warn` \| `error`       |
