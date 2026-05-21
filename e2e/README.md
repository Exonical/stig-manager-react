# End-to-end tests

Playwright tests that drive the live compose stack — `postgres + keycloak + api + web` — and exercise both the SPA and the JSON API as a real user would.

## What the tests cover

`tests/auth.setup.ts` performs the real OIDC code flow (click *Sign in* → Keycloak login form → redirect back) once per run, then persists the resulting browser state for the rest of the projects.

`tests/api.spec.ts`:
- `/health` returns 200 anonymously.
- `/js/Env.js` is served and contains the OIDC config.
- `/api/op/state` reports `db=true, oidc=true`.
- `/api/collections` rejects anonymous reads.
- Authenticated flow: `POST /api/collections` → `GET /api/collections` (verifies the new row) → `GET /api/op/audit-log` (verifies the audit middleware captured the POST asynchronously).

`tests/web.spec.ts`:
- SPA renders the signed-in header after `auth.setup`.
- *Fetch app info* button renders the API response.
- *List collections* button renders the API response.

## Running locally

The tests assume the compose stack is up. Bring it up from `deploy/compose/`:

```sh
docker compose -f deploy/compose/docker-compose.yaml up -d --build --wait
```

Then from `e2e/`:

```sh
pnpm install
pnpm install:browsers   # one-off: download Chromium
pnpm test
```

Override any of the URLs via the environment when running against a non-default stack:

```sh
WEB_URL=https://demo.example.invalid \
API_URL=https://demo.example.invalid \
pnpm test
```

After a failed run, `pnpm test:report` opens the HTML report. Traces and screenshots for failed tests are emitted under `test-results/`.

## Notes

- The setup project performs a real browser login. Keycloak is configured for `directAccessGrantsEnabled: false`, so we cannot short-circuit the flow with a password-grant token — and the browser flow is what we actually want to exercise.
- Tests run serially (`workers: 1`) because they share `app_user` rows + create real database state in the API.
