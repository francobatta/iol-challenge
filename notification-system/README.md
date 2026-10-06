# Notification system

Lets an app register its users and their endpoints, and send them notifications through
Twilio, Mailchimp, APNs and FCM. The providers are mocked.

The design, including the scaling math, is in
[`docs/superpowers/specs/2026-10-05-notification-delivery-design.md`](../docs/superpowers/specs/2026-10-05-notification-delivery-design.md).
The audience part of the API is described in `2026-10-05-audience-api-design.md` next to it.

```
client -> api -> Postgres (job + fan-out)            api/cmd/server
                     |
          api: fan-out goroutines, polling for fan-outs that are due
                     |
                     v
          notify.send.<provider>.<app_id>            one queue per app and provider
                     |
          worker pool per provider   worker/cmd/worker  ->  provider (worker/cmd/mockprovider)
                     |
                     +--> notify.retry.<5s|30s|2m|10m> -> back to its queue
                     +--> notify.dead

Prometheus <- api, workers, RabbitMQ                 what was queued, sent and failed,
                                                     and what is waiting, per app and provider
```

Accepting a notification is one `INSERT`: the job, and with it a row in `fanouts` that
holds the user cursor. Nothing is published at that point, so the API accepts
notifications while RabbitMQ is down. Each server runs a few goroutines that claim due
fan-outs with `FOR UPDATE SKIP LOCKED` and a 30 second lease, read the audience a page of
2,000 users at a time, endpoints included, in one index-only query, publish the
deliveries, and move the cursor. A fan-out that fails, or whose server dies, becomes due
again when its lease runs out and continues from its cursor. The row is deleted when the
last page is published.

Nothing flows back from the workers. A job in Postgres says how far its dispatch got and
how many deliveries it came to; what became of the deliveries is in the metrics, as
described under [Following deliveries](#following-deliveries).

| Path | What |
|---|---|
| `api/` | Go module: the REST API, with the goroutines that dispatch what it accepts |
| `worker/` | Go module: the worker and the mock provider. No database access |
| `commons/` | Go module both import: the RabbitMQ topology, the messages, the provider names, tracing and metrics, and the HTTP server loop |
| `api/.env`, `worker/.env` | Configuration of the services in each module |
| `compose.yaml` | Everything on one machine, one worker per provider, with a Prometheus |
| `deploy/k8s/` | Deployments, a Service, and KEDA `ScaledObject`s for the worker pools |
| `deploy/prometheus/` | What the Prometheus of compose scrapes |

## How the code is laid out

Every binary under `cmd/` has the same three files:

| File | What |
|---|---|
| `main.go` | Parses the config, builds the dependencies, runs until told to stop |
| `config.go` | The environment variables the binary reads |
| `dependencies.go` | Everything the binary is made of, built in order, in one place |

Below that the layers are always the same, and each only calls the one under it:

| Layer | API module | Worker module |
|---|---|---|
| Transport | `internal/httpapi`: the [chi](https://github.com/go-chi/chi) router and its handlers | `internal/consume`: the pool that takes deliveries off the queues |
| Service | `internal/audience`, `internal/notify`, `internal/dispatch` | `internal/consume`: the handler that sends one delivery |
| Repository and clients | `internal/postgres`, `internal/broker` | `internal/provider`, the publisher in `internal/consume` |

A service declares the `Repository` interface it needs next to itself, and
`postgres.Repository` implements all of them.

## Run it

```sh
docker compose up --build
```

The API is on `localhost:8080`, RabbitMQ's management UI on `localhost:15672`
(`notify` / `notify`), and Prometheus on `localhost:9090`.

```sh
# Create an app. The answer has its token.
curl -s -X POST localhost:8080/v1/apps -H 'X-Admin-Key: dev-admin-key' -d '{"name": "demo"}'
TOKEN=...

# A user with an endpoint on each provider.
curl -s -X PUT localhost:8080/v1/users/ana -H "Authorization: Bearer $TOKEN"
for e in '{"address": "+5491100000000", "channel": "sms", "provider": "twilio"}' \
         '{"address": "ana@example.com", "channel": "email", "provider": "mailchimp"}' \
         '{"address": "ios-device-token", "channel": "push", "provider": "apns"}' \
         '{"address": "android-device-token", "channel": "push", "provider": "fcm"}'; do
  curl -s -X POST localhost:8080/v1/users/ana/endpoints -H "Authorization: Bearer $TOKEN" -d "$e"
done

# Send. The answer is the job; poll it until it is dispatched.
curl -s -X POST localhost:8080/v1/notifications -H "Authorization: Bearer $TOKEN" \
  -H 'Idempotency-Key: demo-1' \
  -d '{"user_ids": ["ana"], "priority": "high", "content": {"title": "Hi", "body": "Hello"}}'
curl -s localhost:8080/v1/notifications/JOB_ID -H "Authorization: Bearer $TOKEN"
```

A notification can also target a list (`"list_id": "..."`), alone or together with `user_ids`.

An app's first notification ever can take up to 5 seconds longer than the rest: that is how
often workers look for the queues of new apps.

### Following deliveries

A job goes from `pending` through `dispatching` to `dispatched`, with `queued` counting its
deliveries. From there on the deliveries are followed in Prometheus, per app and provider
rather than per job. Every series below has the labels `provider` and `app_id`:

| Metric | From | What |
|---|---|---|
| `notify_fanout_deliveries_total` | api | Deliveries queued |
| `rabbitmq_detailed_queue_messages_ready` | RabbitMQ | Deliveries waiting for a worker |
| `rabbitmq_detailed_queue_messages_unacked` | RabbitMQ | Deliveries a worker holds |
| `notify_deliveries_total{outcome}` | workers | Deliveries `sent`, `retried` or `failed` |
| `notify_breaker_open` | workers | 1 while a worker has paused the app after repeated failures |

The RabbitMQ series come from its `rabbitmq_prometheus` plugin, on port 15692; the
`provider` and `app_id` labels are cut out of the queue name when they are scraped (see
`deploy/prometheus/prometheus.yml`). For example, in the Prometheus UI:

```
# Sent and failed so far, per app and provider.
sum by (app_id, provider, outcome) (notify_deliveries_total{outcome=~"sent|failed"})

# Still on its way: queued but neither sent nor failed.
sum by (app_id, provider) (notify_fanout_deliveries_total)
  - sum by (app_id, provider) (notify_deliveries_total{outcome=~"sent|failed"})

# Backlog right now.
sum by (app_id, provider) (rabbitmq_detailed_queue_messages_ready)
```

A page that shows this to an app's owner would run these queries, filtered by `app_id`,
next to `GET /v1/notifications`.

### Configuration

Each binary reads its configuration from environment variables, parsed into a `config`
struct by [`caarlos0/env`](https://github.com/caarlos0/env). The struct, in `config.go` next
to each `main.go`, says which variables exist, which are required and what the defaults are.

The values live in one `.env` file per module: `api/.env` for the API,
`worker/.env` for the workers and the mock provider. Compose loads them with `env_file`.
The binaries do not read the files themselves, so to run one outside compose, export the
variables first.

The files hold credentials too. They are placeholders for local development, kept there so
that all the configuration is in one place; a real deployment would move them to a secret
store.

### Making the providers fail

The mock answers after a random 1 ms to 1 s. Two variables make it fail a fraction of the
requests:

```sh
THROTTLE_RATE=0.9 docker compose up -d mockprovider   # 429 nine times in ten
ERROR_RATE=1 docker compose up -d mockprovider        # always 500
docker compose up -d mockprovider                     # healthy again
```

What to look for:

- Throttling opens the app's circuit breaker in each worker (`Circuit breaker opened` in the
  worker logs, `notify_breaker_open` in its metrics). The app's deliveries wait instead of
  burning through their retries, and resume within about 10 seconds of the provider recovering.
- Failed deliveries wait in `notify.retry.*`. With `ERROR_RATE=1` a delivery goes through all
  four tiers, about 13 minutes, then lands in `notify.dead` and counts as `failed` in
  `notify_deliveries_total`.
- `docker compose kill worker-twilio` in the middle of a job loses nothing: the deliveries the
  worker held return to their queue. Start it again with `docker compose up -d worker-twilio`.

## Tests

```sh
docker compose up -d db rabbitmq
cd api    && DATABASE_URL=postgres://audience:audience@localhost:5432/audience \
             AMQP_URL=amqp://notify:notify@localhost:5672/ go test ./...
cd worker  && go test ./...
cd commons && go test ./...
```

Without the two variables the tests that need Postgres or RabbitMQ are skipped. The
database must have the current `api/db/schema.sql`, which compose loads only when it
creates the database: after a schema change, `docker compose down -v` first. One broker
test waits for the 5 second retry tier; `-short` skips it.

After changing `api/db/*.sql` run `sqlc generate` in `api/`, and after changing an interface
that has a mock, `go generate ./...`.

What the services share lives in `commons/`, which `api` and `worker` pull in with a
`replace` directive. That is why both images are built from this directory rather than
from the module's own. The messages they exchange are pinned by
`commons/message/testdata`, and `commons/providers` lists the steps to add a provider.

## Scaling

The worker is I/O-bound, so its capacity is the number of requests it has in progress:

```
sends in progress per pod = min(CONCURRENCY, PREFETCH x apps with a backlog)   = up to 200
notifications per second  = sends in progress / mean provider latency          = up to 400
```

At 16 million notifications per app per day, ten apps average 1,850 per second: 5 pods across
the four pools, 14 at a peak three times that. `deploy/k8s/keda.yaml` scales each pool between
1 and 20 pods on the publish rate and the backlog of its provider's queues; the reasoning for
each number is in the design document and in the comments of that file.

Compose does not autoscale. To try more workers by hand:

```sh
docker compose up -d --scale worker-twilio=3
```

## Kubernetes

The manifests in `deploy/k8s` take their configuration from two ConfigMaps made from the
same `.env` files compose uses:

```sh
kubectl apply -f deploy/k8s/config.yaml   # the namespace
kubectl -n notifications create configmap api-config    --from-env-file=api/.env
kubectl -n notifications create configmap worker-config --from-env-file=worker/.env
kubectl apply -f deploy/k8s
```

The files name Postgres, RabbitMQ and the provider by their compose service names (`db`,
`rabbitmq`, `mockprovider`). Either run Services with those names in the `notifications`
namespace, or edit the URLs before creating the ConfigMaps. After changing a file, recreate
its ConfigMap and restart the Deployments that use it:

```sh
kubectl -n notifications create configmap worker-config --from-env-file=worker/.env \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl -n notifications rollout restart deployment -l app=worker
```

## Priority

A job is `normal` or `high`. Within one app's queue for a provider, high-priority deliveries
are taken ahead of normal ones. How far ahead depends on the RabbitMQ version, which is why
compose pins it.

Measured on `rabbitmq:4.1-management`, with a normal job of 12,000 deliveries already queued
and a high-priority job of the same size sent 3 seconds later: the high job was delivered at
twice the rate of the normal one (about 260 against 130 per second) and finished first, and
the normal job kept moving throughout. That is the 2:1 interleaving of RabbitMQ 4.0 and 4.1.
Later versions document strict ordering, under which a steady stream of high-priority
deliveries would hold an app's normal ones back; check again before upgrading.

Priority never crosses apps: each app has its own queue.

## Things that are deliberately simple

- Delivery is at least once. A crash can send a notification twice; its `message_id` is the
  same both times and is passed to the provider to deduplicate on.
- The `sent` and `failed` counters can run slightly high after a crash, for the same reason.
  They are also per app and provider, not per job: the API does not say when a job has been
  delivered, only when it has been dispatched.
- Counters live in the workers' memory until scraped, so the last few seconds of a worker
  that dies are not counted, and Prometheus must be queried with `sum` and `increase` as
  usual for counters that restart.
- Failed deliveries are in `notify.dead` and in the logs, not in the API.
- The daily quota is checked against usage that lags by a few seconds, so it can be overshot
  by the jobs in flight.
- Fan-out is found by polling: a job waits up to a second for it to start, and up to 30
  seconds to be taken up again after a failure, a full send queue or a server that died.
- A trace starts at fan-out, not at the request that created the job.
- Workers exit when they lose RabbitMQ and rely on being restarted.
