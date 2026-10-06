# Deploy: compose for development, k3s to prove production

Approved in conversation on 2026-10-06.

## Goal

- `docker compose up` stays the one-command local stack and is known to work from a clean
  checkout.
- Kubernetes is self-contained: its own ConfigMaps and Secret, every Deployment the system
  needs, KEDA, and a single-node k3s cluster to run it on.
- One script creates the cluster and applies everything. A second runs a load test, checks
  that KEDA scaled the workers, and asserts the job completed by probing Postgres, RabbitMQ,
  the metrics, the API and Kubernetes.
- Scripts are Bash and run on Windows (Git Bash), Debian/Ubuntu and macOS with only
  `docker`, `kubectl` and `curl`.

## Compose

`compose.yaml`, `api/.env` and `worker/.env` stay in place and become compose-only: the
Kubernetes configuration no longer derives from them.

## Probes

Every Go binary answers the two standard probes:

| Path | Answers 200 when |
|---|---|
| `GET /healthz` | the process serves HTTP |
| `GET /readyz` | it can do its work: the API reaches Postgres, a worker holds its RabbitMQ connection, the mock always |

The API does not make RabbitMQ part of readiness: it accepts notifications while the broker
is down, by design. API and worker serve the probes on the metrics port (`:9090`), the mock
on its only port. `commons/health` holds the handler. Postgres, RabbitMQ, Prometheus and
nginx use their own probes.

## Bulk import

`POST /v1/users/import?list_id=<optional>`, body `text/csv`, one endpoint per row, no
header:

```
user_id,channel,provider,address
```

- Registers each user, gives it the endpoint, and adds it to the list if one is named.
- The body is streamed and written in batches of 1,000 rows, each batch one statement, so
  each batch is all-or-nothing. Existing users, endpoints and memberships are left as they
  are: posting the same file twice is safe.
- A bad row answers 400 naming the row; batches before it stay.
- Body limit 64 MiB, for this route only.
- Answers `{"rows": n, "users": created, "endpoints": created}`.

Layers as elsewhere: handler (CSV is the wire format) -> `audience.Service.Import`
(numbering, validation, batching) -> `postgres.Repository.ImportEndpoints` (sqlc).

## Kubernetes layout

```
deploy/k8s/
  base/           the system: namespace, api, web, ConfigMaps
    workers/      one pool (Deployment + ScaledObject), instantiated per provider
  infra/          what the system needs around it: postgres, rabbitmq, prometheus, mockprovider
  overlays/k3s/   base + infra + dev Secret + image tags + single-node patches
```

- Credentials (`DATABASE_URL`, `AMQP_URL`, `RABBITMQ_API_URL`, `JWT_SECRET`, `ADMIN_KEY`)
  are a Secret; the rest is ConfigMaps. A real deployment writes its own overlay without
  `infra/`.
- KEDA triggers keep the production targets (300 msg/s and 400 queued per pod). The k3s
  overlay caps a pool at 6 replicas, lowers CPU requests so that 24 workers fit on one node,
  and shortens scale-down stabilization to 30s.

## Scripts, in `deploy/`

- `cluster-up.sh`: one `rancher/k3s` container (traefik off), kubeconfig written to a
  gitignored file, images built and imported, pinned KEDA manifest, the overlay, wait for
  rollouts. Safe to re-run. Host ports differ from compose: API 18080, console 13000,
  RabbitMQ management 25672, Prometheus 19090.
- `cluster-down.sh`: removes the container and its volume.
- `loadtest.sh`: creates an app and a list, imports 20,000 users with an endpoint on each
  of the four providers (80,000 deliveries), sends one notification to the list, watches
  job status, backlog and replicas, then asserts:

| Probe | Assertion |
|---|---|
| API | job `dispatched`, `queued` = expected |
| Postgres | job row matches, no `fanouts` row left, `usage_daily` counted |
| RabbitMQ | the app's send queues, the retry queues and `notify.dead` are empty |
| Metrics | `GET /v1/metrics` answers 200; Prometheus has `sent` >= expected and no `failed` |
| Kubernetes | every pool peaked above 1 replica and returned to 1; ScaledObjects Ready; Deployments Available; no restarts |

Each assertion prints PASS or FAIL; any FAIL exits non-zero.
