# deploy

Local development and demo deployment artifacts.

## docker-compose

```bash
cd deploy/compose
docker compose up -d --build
```

| service   | url                            | credentials       |
| --------- | ------------------------------ | ----------------- |
| Web (SPA) | <http://localhost:54000>       | via Keycloak      |
| API       | <http://localhost:54001>       | OIDC token (TBD)  |
| Keycloak  | <http://localhost:8080>        | `admin` / `admin` |
| Postgres  | `postgres://stigman:stigman@localhost:5432/stigman` | local only |

The Keycloak realm `stigman` is imported at start-up from
`deploy/keycloak/stigman-realm.json`. Two demo users are provisioned:

| username    | password    | notes                                   |
| ----------- | ----------- | --------------------------------------- |
| `admin`     | `admin`     | realm role `admin`                      |
| `evaluator` | `evaluator` | no realm role; collection grants in app |

> **Note:** the SPA → API → Postgres path is wired and the full
> `/api/*` OpenAPI surface (150+ operations) is generated and mounted,
> but everything except `/api/op/appinfo` and `/api/op/configuration`
> returns `501 Not Implemented` until real handlers land in subsequent
> milestones.

## Kubernetes / production

Helm chart and Kustomize overlays arrive in Milestone 4.
