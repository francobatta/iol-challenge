# Notification delivery design

Date: 2026-10-05

> Superseded in part on 2026-10-06: there is no separate dispatcher, `notify.dispatch` queue
> or sweeper any more. The API server stores a `fanouts` row with each job and its own
> goroutines claim and fan those out. There is no `notify.results` queue or aggregator
> either: jobs end at `dispatched`, and the workers count what they sent and failed in
> Prometheus, per app and provider. See the flow at the top of
> `notification-system/README.md`; the rest of this document still holds.

## Goal

Let an app send a notification to some of its users and have it delivered through the right
provider, at 16 million notifications per tenant per day, with at-least-once delivery, bounded
backlogs, tenant fairness, retries, and a status the app can poll.

This builds on the audience API (`2026-10-05-audience-api-design.md`), which already stores
apps, users, endpoints and lists.

Two simpler designs were rejected:

- **A global table of pending messages polled by a job.** Correct, but every tenant's workers
  scan and update the same table, and at this volume that table is the bottleneck.
- **One queue per channel.** A tenant with a large campaign starves the others, and a slow or
  throttling provider account blocks unrelated traffic.

## Decisions taken in the interview

| Topic | Decision |
|---|---|
| Broker | RabbitMQ, quorum queues, persistent messages, publisher confirms |
| Queues | One per `(provider, app_id)`: `notify.send.<provider>.<app_id>` |
| Workers | One pool (Deployment) per provider, consuming every tenant queue of that provider |
| Targets | `user_ids` (1 to 1000) and/or one `list_id`; fan-out is asynchronous |
| Priority | `high` or `normal`, set by the app per request |
| Status | Job-level counters only: `queued`, `sent`, `failed` |
| Counter path | Workers publish result events; an aggregator batches them into Postgres |
| Limits | Per-app daily quota at the API. No worker rate limiter |
| Provider errors | Circuit breaker per `(app, provider)`; exponential backoff through retry tiers; then a dead-letter queue |
| Layout | New `worker` module with no database access; `cmd/dispatcher` in the `api` module |
| Providers | `twilio`, `mailchimp`, `apns`, `fcm`, mimicked by a mock HTTP server |
| Observability | Structured logs, Prometheus metrics, OpenTelemetry trace context end to end |
| Design load | 10 tenants, 3x daily peak |

## Architecture

```
client
  | POST /v1/notifications
  v
api ---------> Postgres: jobs row (pending)
  |
  +----------> notify.dispatch                      job reference
                   |
              dispatcher: fan-out                   pages users/list members and endpoints
                   |
                   v
              notify.send.<provider>.<app_id>       one delivery per endpoint
                   |
              worker pool for <provider>  ------->  provider HTTP API
                   |
                   +-- sent ---------------------->  notify.results
                   +-- retryable failure --------->  notify.retry.<tier> --TTL--> back to its send queue
                   +-- permanent / exhausted ----->  notify.dead, and a failed result to notify.results

              dispatcher: aggregator  <-----------  notify.results    batched counter updates
              dispatcher: sweeper                    re-publishes jobs stuck in pending
```

Three processes: the existing API server, a dispatcher, and one worker pool per provider.

## RabbitMQ topology

All queues are durable quorum queues; all publishes are persistent and confirmed.

| Name | Purpose | Arguments |
|---|---|---|
| `notify.dispatch` | Jobs waiting for fan-out | |
| `notify.send.<provider>.<app_id>` | Deliveries for one tenant on one provider | `x-max-length: 100000`, `x-overflow: reject-publish`, `x-delivery-limit: 20`, dead-letter to `notify.dead`, `x-dead-letter-strategy: at-least-once` |
| `notify.retry.<tier>` | Delayed retries; tiers `5s`, `30s`, `2m`, `10m` | A topic exchange and a queue bound with `#`; fixed `x-message-ttl`; dead-letter to the default exchange, `x-dead-letter-strategy: at-least-once`, `x-overflow: reject-publish` |
| `notify.results` | Outcome of each delivery | |
| `notify.dead` | Deliveries that will not be retried | |

**Retry routing.** A worker publishes a failed delivery to the exchange `notify.retry.<tier>`
with the routing key set to the name of the queue it came from. When the tier's TTL expires
the message is dead-lettered to the default exchange with that same routing key, which puts it
back in its own queue. Fixed-TTL tiers are used because a per-message TTL only expires at the
head of a queue.

A retry tier dead-letters at least once, so a message whose delay is over stays in the tier
while the queue it returns to is full, instead of being dropped. RabbitMQ only offers that
together with `reject-publish`, which is why the tiers set it.

**Send queues are created on demand.** The dispatcher declares a tenant's queue before its
first publish to it and remembers that it did. The other queues and the exchanges are declared
by every process that uses them, with identical arguments, so the API, the dispatcher and the
workers can start in any order.

**Bounded backlog.** A send queue holds about 100,000 messages. Past that the broker rejects
publishes, the dispatcher stops fanning out that job and puts it off, and the remaining
audience waits as a cursor in Postgres rather than as messages in RabbitMQ.

The limit is approximate. The broker tells publishers that a queue is full a moment after it
fills, so a burst that is already on its way gets in: measured against a queue limited to 2
messages, a batch of 20 was accepted whole and the next one rejected. The overshoot is at most
the deliveries of one fan-out page per dispatcher.

**Priority.** `high` is published with AMQP priority 9; `normal` leaves the priority unset.
How strongly `high` overtakes `normal` depends on the RabbitMQ version: 4.0 treats priorities
above 4 as high and interleaves at 2:1, and the current documentation describes strict
ordering over 32 levels. The image version is pinned in compose at 4.1, where the 2:1
interleaving was measured; the measurement is in the README. A message that is requeued loses its priority.

Priority acts in the broker. Deliveries a worker has already taken, up to its prefetch per
tenant, are not overtaken by a high-priority delivery that arrives later.

## Messages

Both are JSON. Each module defines its own struct, and a golden file copied into both modules
is checked by a test in each, so a change on one side fails the other's tests.

**Delivery** (`notify.send.*`):

```json
{
  "message_id": "<job_id>:<endpoint_id>",
  "job_id": "...",
  "app_id": "...",
  "provider": "twilio",
  "address": "+5491100000000",
  "content": {"title": "...", "body": "..."}
}
```

AMQP properties: `message-id`, `priority`, `delivery-mode: persistent`. Headers: `traceparent`
and `x-attempt` (0 on first publish).

`message_id` is the same on every redelivery and retry, and is sent to the provider as its
idempotency key.

**Result** (`notify.results`): `{"app_id": "...", "job_id": "...", "outcome": "sent" | "failed"}`.

## HTTP API

Added under `/v1`, behind the same bearer-token authentication as the audience routes.

| Method and path | Success | Notes |
|---|---|---|
| `POST /notifications` | 202 job | accepts the job; delivery is asynchronous |
| `GET /notifications/{job_id}` | 200 job | |
| `GET /notifications` | 200 page | ordered by `job_id`, same paging as the audience API |

Request body:

```json
{
  "user_ids": ["u1", "u2"],
  "list_id": "...",
  "priority": "normal",
  "content": {"title": "...", "body": "..."}
}
```

At least one of `user_ids` (1 to 1000) and `list_id` is required. `priority` defaults to
`normal`. `content.body` is non-empty. Every endpoint of every targeted user receives the
notification; a user named twice, or named and also in the list, is notified once per
endpoint.

An optional `Idempotency-Key` header makes the request safe to retry: a second request with
the same key returns the first job with 200 and creates nothing.

Job:

```json
{
  "job_id": "...",
  "status": "dispatching",
  "priority": "normal",
  "queued": 120000,
  "sent": 80211,
  "failed": 14,
  "created_at": "..."
}
```

| Status | Meaning |
|---|---|
| `pending` | Accepted; fan-out has not started |
| `dispatching` | Fan-out in progress; `queued` is still growing |
| `dispatched` | Every delivery has been published |
| `completed` | `sent + failed >= queued` |

New errors:

| Status | Code | When |
|---|---|---|
| 429 | `quota_exceeded` | the app has used its daily quota |
| 404 | `not_found` | the `list_id` or the job does not exist in this app |

Unknown `user_ids` are not an error: they are skipped during fan-out.

**Endpoint providers.** `provider` on an endpoint is no longer free-form. It must be `twilio`
for `sms`, `mailchimp` for `email`, and `apns` or `fcm` for `push`.

## Data model

Added to `db/schema.sql`:

```
apps         + daily_quota bigint NOT NULL DEFAULT 20000000

jobs         (app_id, job_id uuid, status, priority,
              user_ids text[], list_id uuid NULL,          -- the audience
              title, body,                                 -- the content
              idempotency_key text NULL, fanout_cursor text NOT NULL DEFAULT '',
              queued bigint, sent bigint, failed bigint, created_at, updated_at)
                                                      PK (app_id, job_id)
                                                      UNIQUE (app_id, idempotency_key)

usage_daily  (app_id, day date, queued bigint)        PK (app_id, day)
```

A partial index on `jobs (updated_at) WHERE status = 'pending'` serves the sweeper.

There is no row per message. A job is one row no matter how many people it reaches, and its
counters are updated at most once per aggregator batch.

## Quota

`POST /notifications` reads today's `usage_daily.queued` for the app and rejects the request
when it has reached `apps.daily_quota`. Usage is written by the dispatcher as it fans out, so
the check lags by a few seconds and a job that is accepted just under the quota can take the
app over it. That is accepted: the quota is a guard against runaway use, not a billing meter.

## Dispatcher

One binary in the `api` module running three loops. Any number of replicas may run.

**Fan-out.** Consumes `notify.dispatch`. For each job it pages the target users and their
endpoints in key order, publishes one delivery per endpoint, and after each page stores the
cursor and adds the page's count to `jobs.queued` and `usage_daily.queued` in one transaction.
When the audience is exhausted the job becomes `dispatched`.

- If the process dies mid-job the message is redelivered and fan-out resumes from the stored
  cursor. The page in flight may be published twice; the stable `message_id` lets providers
  drop the duplicates.
- If a send queue is full, or fan-out fails for another reason such as the database being
  unreachable, the job is published to a retry tier and acknowledged, so the dispatcher moves
  on to other tenants and comes back to this job later. Jobs use only the two short tiers, 5 s
  and then 30 s. A job is never returned to the dispatch queue directly, because the broker
  discards a message that has been returned too many times.

The audience is paged by user, 500 at a time: the next users after the cursor from the list's
members and the named users together, each once, and then every endpoint of those users.

**Aggregator.** Consumes `notify.results`, collecting up to 1000 events or one second,
whichever comes first. It sums them per job, applies a single `UPDATE ... FROM unnest(...)`,
marks jobs `completed` when their counters reach `queued`, and then acknowledges the batch.
If the update fails it keeps the batch and tries again every second, for the same reason
fan-out does not return jobs to their queue. If it dies before acknowledging, the batch is
redelivered and counted again, so counters can run slightly high after a crash. They are never
low.

**Sweeper.** Every 30 seconds, re-publishes jobs that have been `pending` for more than a
minute, using `FOR UPDATE SKIP LOCKED`. This covers the case where the API stored a job but
failed to publish it, without an outbox table. The API publishes after its insert commits and
still returns 202 if the publish fails.

## Worker

A new Go module, `notification-system/worker`, with no database access. One image runs as
four Deployments, one per provider, selected by `PROVIDER`.

**Consuming.** Every 5 seconds the worker asks the RabbitMQ management API for the queues
named `notify.send.<provider>.*` and subscribes to any it is not yet consuming. A tenant's
first notification ever can therefore wait up to 5 seconds for the workers to find its queue.

Each subscription has a prefetch of 50, so the broker hands a pod at most 50 unacknowledged
messages per tenant and a tenant with a deep backlog cannot occupy the whole pod. A pod-wide
limit of 200 concurrent sends caps memory and sockets when there are many tenants. Deliveries
in hand take the pod's send slots in the order they arrived, so tenants with a backlog share a
busy pod evenly. A pod therefore runs

```
sends in progress = min(CONCURRENCY, PREFETCH x tenants with a backlog)
```

which is the full 200 once four tenants are sending through the provider. Prefetch is a
trade: a larger one lets a lone tenant use more of each pod, and a smaller one keeps fewer
deliveries in the pod's hands, where high-priority deliveries cannot overtake them.

**Sending.** Each provider has a small HTTP client with a 3 second timeout. Responses are
classified as:

| Class | Cases | Action |
|---|---|---|
| ok | 2xx | publish `sent`, acknowledge |
| retryable | 429, 5xx, timeout, connection error | publish to the next retry tier with `x-attempt + 1`, acknowledge |
| permanent | other 4xx | publish to `notify.dead` and a `failed` result, acknowledge |

After the last tier (four retries, about 13 minutes in total) a retryable failure is treated
as permanent. The original message is acknowledged only after the broker confirms what was
published in its place, so a crash at any point leaves the delivery in a queue.

**Circuit breaker.** One breaker per `(app, provider)`, held in the pod. It opens after 5
retryable failures in a row. While it is open the pod sends nothing for that tenant: the
deliveries it holds wait, unacknowledged and without using up an attempt, and since they fill
the tenant's prefetch the broker sends the pod no more. After a 10 second cool-down one
delivery goes through as a probe; if it succeeds the rest follow, and if not the breaker stays
open for another cool-down. Other tenants on the same provider are unaffected.

The waiting deliveries are held rather than returned to the queue because the broker counts
returns against the delivery limit, and a long provider outage would dead-letter them.

**Poison messages.** A message the worker cannot decode goes straight to `notify.dead`. A
message that crashes workers repeatedly is dead-lettered by the broker at the delivery limit.

**Shutdown.** On SIGTERM the worker stops consuming, finishes the sends in flight, and exits.
The pod's termination grace period is longer than the provider timeout.

**Mock provider.** `cmd/mockprovider` in the worker module serves `/twilio`, `/mailchimp`,
`/apns` and `/fcm`. Each request sleeps for a uniformly random 1 ms to 1 s. `ERROR_RATE` and
`THROTTLE_RATE` (0 to 1) make it answer 500 and 429.

## Scaling

**Load.** One tenant sends 16,000,000 / 86,400 s = 185 notifications per second on average.
Ten tenants send 1,850/s. Traffic is not flat, so the system is sized for three times that:
5,550/s.

**Concurrency.** A send holds a goroutine and a connection for the provider's latency and
uses almost no CPU, so capacity is a matter of concurrent sends. With latency uniform between
1 ms and 1 s the mean is 0.5 s, and

```
sends in flight = arrival rate x mean latency
average:  1,850/s x 0.5 s =   925
peak:     5,550/s x 0.5 s = 2,775
```

**Pods.** A pod runs 200 concurrent sends, so it delivers 200 / 0.5 s = 400 notifications
per second. That holds when at least four tenants have a backlog on the provider (see
Consuming); with fewer, a pod delivers 100 per second for each of them.

```
average:  1,850 / 400 =  4.6  ->  5 pods
peak:     5,550 / 400 = 13.9  -> 14 pods, 18 with 25% headroom
```

Those are totals across the four provider pools; each pool scales on its own traffic. A pod
requests 250m CPU and 128Mi. Eighteen pods are 4.5 cores and 2.3 GiB at peak.

**Latency.** The time a notification spends in the worker is the provider call, at most 1 s
here. Time waiting in the queue is what scaling controls, and the targets below aim to keep
it under a second in steady state. During a burst it grows until new pods start, and it is
bounded by the queue cap: 100,000 messages drained at the tenant's share of the pool.

**KEDA.** One `ScaledObject` per provider pool, with two `rabbitmq` triggers over the regex
`^notify\.send\.<provider>\..+` (HTTP protocol, `useRegex`, `operation: sum`). KEDA scales to
the larger of the two.

| Trigger | Target per pod | Why |
|---|---|---|
| `MessageRate` | 300/s | 75% of a pod's 400/s. Sizes the pool for steady traffic |
| `QueueLength` | 400 | One second of a pod's capacity. Adds pods when a backlog forms |

Queue length alone is not enough: when capacity matches arrival the queues are empty, the
pool scales in, a backlog forms, and it scales out again. The rate trigger holds the pool at
the size the traffic needs.

The rate trigger alone is not enough either. It assumes pods at full concurrency, and when
only one or two tenants are sending they are not, so the pool it asks for falls behind. The
backlog that forms is what the length trigger scales on.

Measured on compose, one pod per provider and the mock's 0.5 s mean latency: with two tenants
sending, 40,000 deliveries took 52 s, which is 190 per second per pod against the 200 the
formula gives for two tenants at a prefetch of 50.

`minReplicaCount: 1`, `maxReplicaCount: 20`, and a 120 second stabilization window on
scale-in. A pool of one with no traffic costs 250m CPU; scaling to zero would add a pod's
start-up time to the first notification after a quiet period.

**Broker.** At peak the broker takes about 5,550 deliveries/s and the same number of results,
on 40 send queues. That is within what one three-node cluster does with quorum queues. Worst
case on disk is 40 queues x 100,000 messages x about 0.5 KB = 2 GB.

**Database.** The send path writes one row per job, one small transaction per fan-out page,
and one batched update per second from the aggregator. None of that grows with the number of
messages in flight.

**Beyond this load.** Around 100 tenants (55,000/s at peak) a single cluster is near its
limit. The next step is to assign tenants to one of several clusters and run a worker pool
per cluster. The queue naming and the workers do not change. This is not built.

## Observability

- **Logs.** `slog`, with `app_id`, `job_id` and `message_id` on every line about a delivery.
- **Metrics.** Each process serves Prometheus metrics on `/metrics`: deliveries by provider
  and outcome, provider latency histogram, retries by tier, breaker state by provider,
  in-flight sends, fan-out pages and rejected publishes, aggregator batch size and lag.
  Queue depth and rates come from RabbitMQ's own Prometheus endpoint.
- **Traces.** The API starts a span and puts W3C trace context in the `traceparent` header of
  what it publishes. The dispatcher and the worker continue it, so one trace covers the
  request, the fan-out page, the queue wait and the provider call. Spans are exported over
  OTLP when `OTEL_EXPORTER_OTLP_ENDPOINT` is set and dropped otherwise.

## Packages

`notification-system/api` (existing module):

| Package | Role |
|---|---|
| `cmd/server` | Also wires the notification service and the broker |
| `cmd/dispatcher` | Runs fan-out, the aggregator and the sweeper |
| `internal/notify` | Job types, `Service` (validation, quota), the `Store` and `Publisher` interfaces |
| `internal/notify/notifytest` | Mocks generated by mockgen |
| `internal/dispatch` | The three dispatcher loops |
| `internal/broker` | RabbitMQ topology, confirmed publishing, message types |
| `internal/postgres` | Also implements `notify.Store` |
| `internal/telemetry` | Metrics handler and tracer set-up |

`notification-system/worker` (new module):

| Package | Role |
|---|---|
| `cmd/worker` | Reads config, runs one provider pool |
| `cmd/mockprovider` | The mock provider server |
| `internal/consume` | Queue discovery, subscriptions, retry and dead-letter decisions |
| `internal/provider` | `Sender` and the four provider clients |
| `internal/breaker` | Breakers keyed by app |
| `internal/message` | Delivery and result types |
| `internal/telemetry` | Metrics handler and tracer set-up |

Libraries: `rabbitmq/amqp091-go`, `sony/gobreaker/v2`, `prometheus/client_golang`,
`go.opentelemetry.io/otel`.

## Configuration

| Process | Variables |
|---|---|
| server | adds `AMQP_URL` (required) |
| dispatcher | `DATABASE_URL`, `AMQP_URL` (required); `METRICS_ADDR` (default `:9090`) |
| worker | `PROVIDER`, `AMQP_URL`, `RABBITMQ_API_URL`, `PROVIDER_URL` (required); `CONCURRENCY` (200), `PREFETCH` (50), `METRICS_ADDR` (`:9090`) |
| mockprovider | `ADDR` (`:8081`), `ERROR_RATE` (0), `THROTTLE_RATE` (0) |
| all | `OTEL_EXPORTER_OTLP_ENDPOINT` (optional) |

## Deployment

- `compose.yaml` adds `rabbitmq` (management image, one node), `dispatcher`, `mockprovider`,
  and `worker-twilio`, `worker-mailchimp`, `worker-apns`, `worker-fcm` from one image. There
  is no autoscaling in compose.
- `deploy/k8s/` holds Deployments for the API, the dispatcher and the four pools, a Service
  for the API, four `ScaledObject`s with a `TriggerAuthentication`, and a ConfigMap and Secret
  with placeholder values. RabbitMQ and Postgres are assumed to exist.
- A dispatcher or worker that loses its connection to RabbitMQ exits and is restarted by
  compose or the kubelet. The API reconnects on its next publish.

## Testing

- `notify`: validation and quota rules over the mock store and publisher.
- `api`: the new routes through `httptest`, in the style of the existing table tests.
- `postgres`: job counters, cursor updates, idempotency key, the sweeper query; real database.
- `broker`: topology and the retry round trip against a real RabbitMQ; skipped when `AMQP_URL`
  is unset.
- `dispatch`: fan-out resume from a cursor, back-off on a rejected publish, aggregator
  batching; over mocks.
- `consume`: the ok / retryable / permanent decisions, attempt counting, breaker pause; over
  fakes of the channel and sender.
- `provider`: each client against `httptest`, including timeout and 429.
- Contract: the golden-file test in each module.
- End to end, by hand against compose: a job reaches `completed`; two tenants progress
  together; a killed worker loses nothing; `THROTTLE_RATE` opens breakers; `ERROR_RATE=1` ends
  in `notify.dead`; a small quota returns 429.

## Known limits

- Delivery is at least once. Duplicates are possible after a crash or a redelivery, and are
  suppressed only by the provider honouring the idempotency key.
- Counters can run slightly high after an aggregator or worker crash: a delivery whose
  result was published but not yet acknowledged is sent and counted again. Killing a worker
  in the middle of 16,000 deliveries left the job at 16,001 sent.
- A send queue can exceed its limit by a fan-out page.
- Failed deliveries are visible in `notify.dead` and in logs, not through the API.
- The quota can be overshot by the jobs in flight when it is reached.
- There is no per-provider rate limit; the system relies on 429s, the breaker and backoff.
