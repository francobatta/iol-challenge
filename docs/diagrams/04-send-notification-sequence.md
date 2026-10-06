# Sending a notification

The typical case: an app notifies a list, and every delivery succeeds at the first try.

```mermaid
sequenceDiagram
    autonumber
    actor App as App (client)
    participant API as api: HTTP
    participant DB as Postgres
    participant Fan as api: fan-out goroutine
    participant MQ as RabbitMQ
    participant W as worker (one per provider)
    participant P as provider

    Note over App,DB: Accept. One request, no broker involved.
    App->>API: POST /v1/notifications (Bearer token, Idempotency-Key)
    API->>DB: list exists? usage today below daily_quota?
    API->>DB: INSERT job (pending) with its fanouts row
    API-->>App: 202 Accepted, job_id, status pending

    Note over DB,MQ: Fan out. Starts within 1 second, in any api pod.
    Fan->>DB: claim a due fan-out (FOR UPDATE SKIP LOCKED, 30s lease)
    loop each page of 2,000 users
        Fan->>DB: read users and their endpoints after the cursor
        Fan->>MQ: publish one delivery per endpoint to notify.send.PROVIDER.APP_ID
        MQ-->>Fan: confirms, for the whole page
        Fan->>DB: move cursor, add to queued and usage, renew lease (dispatching)
    end
    Fan->>DB: last page: delete the fanouts row, job is dispatched

    Note over MQ,P: Deliver. Each provider's pool works on its own queues.
    par for each delivery, up to 200 at once per pod
        MQ->>W: deliver (at most 50 unacked per queue)
        W->>P: send, with message_id
        P-->>W: 2xx
        W->>MQ: ack
        Note right of W: counted in notify_deliveries_total, outcome sent
    end

    Note over App,DB: Follow up.
    App->>API: GET /v1/notifications/{job_id}
    API->>DB: read job
    API-->>App: 200, status dispatched, queued N
```

## What to notice

- **202 means stored, not sent.** Accepting a job writes one row and its fan-out, in one
  statement. Nothing is published yet, so the API accepts notifications while RabbitMQ
  is down.
- **Nobody announces the job.** The fan-out goroutines find it by polling `fanouts`, so
  there is no message to lose between accepting and dispatching.
- **Fan-out is resumable.** The cursor is saved after every page. If the pod dies, a
  publish fails or a queue is full, the lease runs out after 30 s and any api pod
  continues from the cursor. The page in progress is published again; `message_id`
  (`job_id:endpoint_id`) is the same, so the provider can drop the copies.
- **`dispatched` is the last status.** It means every delivery is queued, not delivered.
  Workers report nothing back: what was sent or failed is in the metrics, per app and
  provider. See [the observability diagram](03-kubernetes-and-observability.md#observability-stack).
- **The same `Idempotency-Key` twice** returns the job created the first time, without
  creating another.

## When it does not go well

| What happens | Result |
|---|---|
| The app is over its daily quota | 429 at step 2, no job |
| A send queue holds 100,000 deliveries | The publish is rejected, and the fan-out resumes after its lease |
| The provider answers 429 or 5xx, or times out | The worker rejects the delivery, and the broker delivers it again after 10 s x failures |
| The provider refuses the notification (other 4xx), or it has failed 10 times | It goes to `notify.dead` |
| A worker dies holding deliveries | The broker returns them to their queue |

The delivery path in full is in [the queue topology](02-queue-topology.md#what-becomes-of-one-delivery).
