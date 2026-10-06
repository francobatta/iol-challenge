# Monitoring: metrics, traces and logs in Grafana

## Goal

An operator can see how the notification system is doing from one frontend: request
rate, errors and latency of the API, the delivery pipeline, the processes, the traces of
a notification from request to provider call, and the logs of every container.

Today the only views are the raw Prometheus UI, the console's per-app dashboard and
`docker logs`. Tracing is wired in the code but exports nowhere, so it does nothing.

## Decisions

| Question | Decision |
|---|---|
| Tracing: remove or use | Keep it and make it used, with Tempo |
| API request metrics | A Prometheus middleware in the API, not metrics derived from traces |
| Logs | Seen in Grafana through Loki. Not kept: no volume, no retention or rotation settings |
| Frontend | Grafana, provisioned from files |

## What is added

Four containers, in compose and in `deploy/k8s/infra`. Each is one small instance with
no persistent volume, like the Prometheus that is already there: restarting one empties it.

| Component | Role | Reached as |
|---|---|---|
| Grafana | The frontend | `grafana:3000`; host `localhost:3001` (compose), `localhost:13001` (k3s) |
| Tempo | Stores traces; receives OTLP/HTTP | `tempo:4318` (OTLP), `tempo:3200` (queries). Not published to the host |
| Loki | Stores logs | `loki:3100`. Not published to the host |
| Alloy | Reads container logs and pushes them to Loki | Nothing calls it |

Image tags are pinned, as the existing ones are. The current stable release of each is
looked up when the files are written.

```
api, workers --OTLP/HTTP--> Tempo  <--+
api, workers, RabbitMQ <--scrape-- Prometheus <--+-- Grafana <-- operator
all containers --stdout--> Alloy --> Loki  <--+
```

## Traces

No span is added or changed. The spans that exist only lack somewhere to go. They make
two traces per notification, as the README already says under "Things that are
deliberately simple": `notifications.send` for the request, and `fanout` in the API's
dispatch goroutines with the workers' `deliver` spans under it, joined through the
`traceparent` header of the messages. The two are not linked, because the job waits in
Postgres between them.

- `deploy/tempo/tempo.yaml`: a single-binary Tempo with local storage and the OTLP/HTTP
  receiver on 4318. Compose and the k3s manifests read the same file.
- `OTEL_EXPORTER_OTLP_ENDPOINT=http://tempo:4318` in `api/.env`, `worker/.env`, and the
  `api-config` and `worker-config` ConfigMaps of `deploy/k8s/base`. The variable stays
  optional in the code: empty still exports nothing.
- Every trace is sampled. Under the load test (80,000 deliveries) the exporter's queue
  may fill, and it then drops spans rather than slow the service down. That is accepted
  and written in the README; no sampling setting is added.

## API request metrics

A middleware in `api/internal/httpapi/instrument.go`, registered on the registry the API
already serves on `/metrics`:

| Metric | Labels | What |
|---|---|---|
| `notify_http_requests_total` | `method`, `route`, `status` | Requests answered |
| `notify_http_request_duration_seconds` | `method`, `route` | Histogram of how long they took |

- `route` is the chi route pattern (`/v1/users/{user_id}`), never the path, so the number
  of series does not grow with the data. A request that matches no route is counted
  under the route `unmatched`; one whose path has a route for another method, under that
  route with its 405.
- `status` is the code written, or 200 if the handler wrote none.
- `httpapi.NewMetrics(reg)` builds it, as `dispatch.NewMetrics` does, and `NewRouter`
  takes the result. The middleware wraps the whole router, so requests refused by
  authentication are counted too.
- Tests: table-driven, against a private registry: a matched route, a path parameter
  (one series for two IDs), an unmatched path, a 4xx and a 5xx.

No scrape configuration changes: both Prometheus files already scrape the API.

## Logs

The services keep writing to stdout and stderr, in the format they have now. Nothing in
the Go code changes.

- `deploy/loki/loki.yaml`: a single-binary Loki storing on the container's own
  filesystem. Shared by compose and k3s.
- Alloy finds the containers and tails them. How it finds them differs, so there are two
  configurations, as there are two `prometheus.yml`:
  - `deploy/alloy/config.alloy` (compose): the Docker socket, mounted read-only,
    restricted to the containers of this compose project.
  - `deploy/k8s/infra/config.alloy` (k3s): the Kubernetes API, for the pods of the
    namespace. It runs as one Deployment, with a Role that may read `pods` and
    `pods/log`. No DaemonSet and no host path.
- Both give every line the label `service`: `api`, `worker`, `rabbitmq`, `db`, `web`,
  `mockprovider`, and the monitoring containers themselves. Lines of a worker also carry
  `provider`. On k3s `pod` is added.

Mounting the Docker socket lets Alloy read about every container on the machine. That is
acceptable for a development compose file and is said in a comment next to the mount.

## Grafana

Everything comes from files in `deploy/grafana/`; nothing is set up by hand and nothing
needs to survive a restart.

- `provisioning/datasources/datasources.yaml`: Prometheus (default), Tempo and Loki.
- `provisioning/dashboards/dashboards.yaml`: loads `dashboards/*.json`, read-only.
- Access: anonymous, as an editor, because Grafana lets only editors open Explore, where
  traces are read. The dashboards stay read-only: they are provisioned. The admin login
  is the placeholder `admin` / `admin`, like the other development credentials.

| Dashboard | Panels |
|---|---|
| API | Request rate by route; share of 5xx; p50, p95 and p99 latency by route; fan-out pages, deliveries queued and pages rejected |
| Delivery pipeline | Queued against sent, retried and failed, per provider; provider latency; sends in flight; open breakers; backlog ready and unacked; workers per pool |
| Processes | `up` per target; CPU; memory; goroutines; GC pause |
| Logs | Lines per service over time; lines at the level error or warn, as Loki detects it; the log itself, with a `service` variable and a text filter |

Traces are read in Explore with the Tempo datasource; there is no trace dashboard.

On k3s: `deploy/k8s/infra/grafana.yaml`, `tempo.yaml`, `loki.yaml` and `alloy.yaml`,
their ConfigMaps generated from the files above, and a NodePort 30030 for Grafana in
`overlays/k3s/nodeports.yaml`. `lib.sh` gains `GRAFANA_PORT=13001` and `cluster-up.sh`
publishes it and prints the URL. A cluster that already exists was created without that
port: it has to be removed with `cluster-down.sh` and created again.

## What is removed

Nothing. With tracing kept, an audit found no unused observability code or
infrastructure: every metric the services expose is read by `api/internal/insight`, and
the three scrape jobs are all used by it and by the load test.

## Left as it is

- The console dashboard and `GET /v1/metrics`. They serve an app's owner, restricted to
  that app; Grafana serves the operator and shows every app.
- The format of the logs.

## Out of scope

Alert rules, metrics derived from traces and the service graph, links from metrics or
logs to traces (exemplars, trace IDs in log lines), a Postgres exporter, authentication
for Grafana beyond the placeholder, and keeping any of the data.

## Documentation

- README: a Monitoring section (what runs, where Grafana is, what each dashboard shows,
  how to find the trace of a notification and the logs of a service, that sampling may
  drop spans under load, that nothing is kept); the new metrics in the table under
  "Following deliveries"; Grafana in the tables of addresses and directories.
- Comments in `compose.yaml`, the two `.env` files and `deploy/k8s/infra/kustomization.yaml`.

## Verification

1. `go test ./...` in `api`, `worker` and `commons`.
2. `docker compose up -d --build`, then:
   - Grafana answers `GET /api/health`, and each of the three datasources passes its
     health check through Grafana's API.
   - After a few API calls, `notify_http_requests_total` is in Prometheus with the route
     patterns as labels.
   - After one notification is sent, Tempo returns a trace holding spans of both
     `notification-api` and a `notification-worker-*` service.
   - Loki returns lines for `service="api"` and for a worker.
3. `deploy/loadtest.sh` gains checks under "Metrics": Grafana is healthy, and the API's
   request counter is above zero. It is run only if a k3s cluster is recreated for it.
