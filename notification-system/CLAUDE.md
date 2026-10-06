# Notification system

Apps register users and endpoints, and send notifications through Twilio, Mailchimp, APNs
and FCM (mocked). `README.md` only says how to run it; `DESIGN.md` is the user's to write.
Older specs are in `../docs/superpowers/specs/`.

## Flow

```
client -> api -> Postgres (job + fanouts row)        nothing is published on accept
          api fan-out goroutines poll due fan-outs   FOR UPDATE SKIP LOCKED, 30s lease,
                     |                               pages of 2,000 users, cursor in the row
          notify.send.<provider>.<app_id>            one quorum queue per app and provider
                     |
          worker pool per provider -> provider       reject = broker's delayed retry (10s x failures)
                     +--> notify.dead                4xx at once, or after 10 failures
```

- Nothing flows back from workers. A job is `pending` -> `dispatching` -> `dispatched` with
  `queued`; what became of deliveries is only in Prometheus, per app and provider, not per job.
- Delivery is at least once; `message_id` is stable for the provider to deduplicate on.
- Workers have no database access, exit when they lose RabbitMQ, and find queues of new
  apps every 5s. Each has a circuit breaker per app.
- Priority (`normal`/`high`) is strict within one app's queue on RabbitMQ 4.3, never across apps.
- `GET /v1/metrics?range=` runs fixed PromQL restricted to the token's `app_id`
  (`api/internal/insight`); the browser never talks to Prometheus. 503 without Prometheus.

## Layout

| Path | What |
|---|---|
| `api/` | Go module: REST API and the fan-out. `cmd/server` |
| `worker/` | Go module: `cmd/worker` and `cmd/mockprovider` |
| `commons/` | Go module both use via `replace`: topology, messages, providers, telemetry, health, HTTP server loop. Why images build from this directory |
| `web/` | Console: Vite, React, TypeScript, Tailwind, shadcn/ui, TanStack Query, Recharts. nginx passes `/v1` to the api (`nginx.conf.template`). API docs are Redoc over `public/openapi.yaml` |
| `compose.yaml`, `api/.env`, `worker/.env` | Local run; placeholder credentials. One worker per provider, no autoscaling |
| `deploy/k8s/` | kustomize: `base/` (api, web, `workers/pool` + one directory per provider), `infra/` (Postgres, RabbitMQ, mock provider, monitoring; label `tier=infra`), `overlays/k3s/` |
| `deploy/*.sh` | `cluster-up.sh`, `loadtest.sh`, `cluster-down.sh`, shared `lib.sh` |
| `deploy/{prometheus,grafana,tempo,loki,alloy}/` | Monitoring config for compose |

Every `cmd/*` has `main.go`, `config.go` (env vars, `caarlos0/env`) and `dependencies.go`.
Layers, each calling only the one below:

| Layer | api | worker |
|---|---|---|
| Transport | `internal/httpapi` (chi) | `internal/consume` pool |
| Service | `internal/audience`, `notify`, `dispatch`, `insight` | `internal/consume` handler |
| Repository, clients | `internal/postgres`, `broker`, `prom` | `internal/provider` |

A service declares the `Repository` interface it needs; `postgres.Repository` implements all.

## Commands

```sh
docker compose up --build -d              # local: console :3000, api :8080, grafana :3001,
                                          # prometheus :9090, rabbitmq :15672 (notify/notify)
docker compose down -v                    # also needed after a schema or queue-argument change
THROTTLE_RATE=0.9 docker compose up -d mockprovider   # or ERROR_RATE=1; plain `up -d` heals it

deploy/cluster-up.sh                      # k3s in Docker + KEDA; rerun to rebuild and redeploy
deploy/loadtest.sh                        # USERS=20000 default -> 80,000 deliveries, PASS/FAIL
deploy/cluster-down.sh                    # cluster ports: console :13000, api :18080,
                                          # grafana :13001, prometheus :19090, rabbitmq :25672
export KUBECONFIG=$PWD/deploy/.kube/config   # namespace: notifications
```

Admin key everywhere: `dev-admin-key` (`X-Admin-Key`). Apps authenticate with a bearer token.

## Tests and generated code

```sh
docker compose up -d db rabbitmq
cd api && DATABASE_URL=postgres://audience:audience@localhost:5432/audience \
          AMQP_URL=amqp://notify:notify@localhost:5672/ go test ./...   # -short skips a 10s test
cd worker && go test ./...
cd commons && go test ./...
cd web && npm test -- --run        # also: npm run lint, npm run build
```

- Without the two variables, the Postgres and RabbitMQ tests are skipped.
- After changing `api/db/*.sql`: `sqlc generate` in `api/`. After changing a mocked
  interface: `go generate ./...`.
- `commons/message/testdata` pins the wire format; `commons/providers` lists the steps to
  add a provider.
- Go follows the `google-go-style` skill.
- Lint each Go module from inside it: `go fix ./... && golangci-lint run --fix ./...`
  (v2, config in `.golangci.yml`: the standard linters plus gofmt and goimports).

## Things that bite

- Compose loads `api/db/schema.sql` only when the database is created.
- RabbitMQ refuses to redeclare a queue with other arguments; delayed retry needs 4.3.
- The scripts must stay portable: Bash 3.2, Git Bash on Windows, only docker, kubectl and
  curl; relative paths only, no jq, no temp files (see the header of `deploy/lib.sh`). `*.sh` is LF.
- `kubectl kustomize` needs `--load-restrictor LoadRestrictionsNone`: the overlay reads
  `api/db/schema.sql`.
- `RABBITMQ_API_URL` must be the full service name, or KEDA (another namespace) never scales.
- Images keep their tag on rebuild, so `cluster-up.sh` restarts the Deployments.
- api readiness checks Postgres only: it accepts notifications while RabbitMQ is down.
- KEDA: 1 to 20 pods a pool in `base`, 6 in the k3s overlay, with smaller requests and a
  30s cooldown. A pod does up to 200 sends at once (`CONCURRENCY`), 50 per app (`PREFETCH`).
- Monitoring keeps nothing across restarts; Grafana is provisioned from `deploy/grafana`
  only. Traces start at fan-out and are dropped under load; metrics stay exact.
