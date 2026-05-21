# stig-manager Helm chart

A production-ready chart for the STIG Manager API + SPA workloads.

The chart deploys:

| Component | Image                                  | Listens | Notes                                           |
| --------- | -------------------------------------- | ------- | ----------------------------------------------- |
| `api`     | `ghcr.io/exonical/stig-manager-api`    | `:54001` | Distroless Go binary; runs as non-root.        |
| `web`     | `ghcr.io/exonical/stig-manager-web`    | `:80`    | nginx serving the SPA + reverse-proxying `/api`. |

External dependencies (NOT installed by this chart, by design):

- **PostgreSQL ≥ 18** — connection string supplied via `secrets.databaseUrl` or `secrets.existing.name`.
- **OIDC provider** (Keycloak, Auth0, Cognito, Azure AD, …) — issuer URL supplied via `config.oidc.issuer`.

## Quick start

```bash
helm install stigman ./deploy/helm/stig-manager \
  --namespace stigman --create-namespace \
  --set secrets.databaseUrl='postgres://stigman:s3cret@pg-rw.databases:5432/stigman?sslmode=require' \
  --set config.oidc.issuer='https://idp.example.com/realms/stigman' \
  --set config.oidc.audience='stig-manager' \
  --set config.allowedOrigins='https://stig.example.com' \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set 'ingress.hosts[0].host=stig.example.com' \
  --set 'ingress.hosts[0].paths[0].path=/' \
  --set 'ingress.hosts[0].paths[0].pathType=Prefix'
```

Once it is up, verify with the chart's bundled smoke test:

```bash
helm test stigman -n stigman
```

The test Pod curls `/health` on the API Service and the SPA root on the web Service.

## Configuration

Every value is documented inline in [`values.yaml`](./values.yaml). The headline knobs:

### Images

```yaml
api:
  image:
    repository: ghcr.io/exonical/stig-manager-api
    tag: ""              # defaults to .Chart.AppVersion
web:
  image:
    repository: ghcr.io/exonical/stig-manager-web
    tag: ""
```

### OIDC

```yaml
config:
  oidc:
    issuer: https://idp.example.com/realms/stigman
    # Override only when the API's in-cluster discovery URL must differ
    # from the issuer claim on tokens (e.g. a per-cluster Keycloak with
    # a different hostname). Uses go-oidc's InsecureIssuerURLContext.
    discoveryUrl: ""
    audience: stig-manager
    clientId: stig-manager
```

### Database

Inline:

```yaml
secrets:
  databaseUrl: postgres://user:pass@host:5432/db?sslmode=require
```

External (e.g. external-secrets, sealed-secrets, ArgoCD bootstrap):

```yaml
secrets:
  existing:
    name: stigman-db
    databaseUrlKey: dsn
```

### Ingress

```yaml
ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt
  hosts:
    - host: stig.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: stigman-tls
      hosts:
        - stig.example.com
```

The Ingress points at the **web** Service; nginx in the web pod proxies `/api/*` and `/js/Env.js` to the api Service. This keeps the SPA, JSON API, and runtime OIDC config behind a single hostname (which is the only configuration the OIDC provider needs to accept as a redirect URI).

### Horizontal scaling

`api.autoscaling.enabled=true` provisions an HPA targeting CPU (and optionally memory). `api.podDisruptionBudget.enabled` is on by default for any deployment with `replicaCount > 1`. The same knobs exist under `web`.

### NetworkPolicy

`networkPolicy.enabled=true` creates two NetworkPolicies that restrict pod-to-pod traffic to the chart's own Services + DNS. Add Postgres / OIDC egress entries to `networkPolicy.apiEgress`:

```yaml
networkPolicy:
  enabled: true
  apiEgress:
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: databases
      ports:
        - protocol: TCP
          port: 5432
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: idp
      ports:
        - protocol: TCP
          port: 8080
```

## Linting and templating

The chart is validated on every PR via `helm lint` and `helm template` against the fixtures in [`ci/`](./ci/). To run locally:

```bash
helm lint deploy/helm/stig-manager --strict \
  --values deploy/helm/stig-manager/ci/default-values.yaml
helm template stigman deploy/helm/stig-manager \
  --values deploy/helm/stig-manager/ci/full-values.yaml \
  --namespace stigman \
  --debug
```
