# deploy

Local development and demo deployment artifacts.

## docker-compose

```bash
cd deploy/compose
docker compose up -d --build
```

| service   | url                            | credentials              |
| --------- | ------------------------------ | ------------------------ |
| Web (SPA) | <http://localhost:54000>       | via Keycloak (PKCE)      |
| API       | <http://localhost:54001>       | OIDC bearer token        |
| Keycloak  | <http://localhost:8080>        | `admin` / `admin`        |
| Postgres  | `postgres://stigman:stigman@localhost:5432/stigman` | local only |

OIDC settings flow through the stack as:

1. The API reaches Keycloak at `http://keycloak:8080/realms/stigman`
   (compose service name) to fetch the JWKS for token validation.
2. The API serves `/js/Env.js` with `oauth.authority` set to
   `http://localhost:8080/realms/stigman` (the URL the browser can
   reach).
3. The SPA loads `/js/Env.js` on boot, constructs an
   `oidc-client-ts` `UserManager`, and starts a PKCE flow when the
   user clicks **Sign in**.
4. Subsequent API calls include `Authorization: Bearer <token>`; the
   API validates the token against Keycloak's JWKS and per-route
   scope.

The Keycloak realm `stigman` is imported at start-up from
`deploy/keycloak/stigman-realm.json`. Two demo users are provisioned:

| username    | password    | notes                                   |
| ----------- | ----------- | --------------------------------------- |
| `admin`     | `admin`     | realm role `admin`                      |
| `evaluator` | `evaluator` | no realm role; collection grants in app |

> **Note:** the SPA → API → Postgres path is wired and the full
> `/api/*` OpenAPI surface (150+ operations) is generated and mounted.
> As of Milestone 4 the implemented operations are:
>
> - `GET /api/op/configuration` (public)
> - `GET /api/op/appinfo` (requires `stig-manager:op:read`)
> - `GET /api/collections` (requires `stig-manager:collection:read`)
> - `GET /api/collections/{id}` (requires `stig-manager:collection:read`)
> - `POST /api/collections` (requires `stig-manager:collection`)
>
> Every other operation still returns `501 Not Implemented`. On
> start-up the API connects to Postgres, runs the goose migrations
> baked into the binary, and exposes the resulting schema version
> via `lastMigration` on `/api/op/configuration`.

## Kubernetes / production

Helm chart and Kustomize overlays arrive in Milestone 4.
