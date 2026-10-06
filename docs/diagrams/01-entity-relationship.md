# Entity relationship diagram

Every table of Postgres, from [`api/db/schema.sql`](../../api/db/schema.sql).

```mermaid
erDiagram
    apps ||--o{ users : "has"
    apps ||--o{ lists : "owns"
    apps ||--o{ jobs : "sends"
    apps ||--o{ usage_daily : "is metered by"
    users ||--o{ endpoints : "is reached at"
    users ||--o{ list_members : "is in"
    lists ||--o{ list_members : "contains"
    jobs ||--o| fanouts : "is still owed"
    lists |o..o{ jobs : "is the audience of (no FK)"

    apps {
        uuid app_id PK
        text name
        bigint daily_quota "default 20,000,000"
        timestamptz created_at
    }
    users {
        uuid app_id PK, FK
        text user_id PK "chosen by the app"
        timestamptz created_at
    }
    endpoints {
        uuid app_id PK, FK
        uuid endpoint_id PK
        text user_id FK
        text address "phone, email or device token"
        text channel "email, sms or push"
        text provider "twilio, mailchimp, apns or fcm"
    }
    lists {
        uuid app_id PK, FK
        uuid list_id PK
        text name "unique per app"
        text description
        timestamptz created_at
    }
    list_members {
        uuid app_id PK, FK
        uuid list_id PK, FK
        text user_id PK, FK
        timestamptz added_at
    }
    jobs {
        uuid app_id PK, FK
        uuid job_id PK
        text status "pending, dispatching or dispatched"
        text priority "high or normal"
        text_array user_ids "audience, with the list"
        uuid list_id "audience, not a foreign key"
        text title
        text body
        text idempotency_key "unique per app"
        bigint queued "deliveries published so far"
        timestamptz created_at
        timestamptz updated_at
    }
    fanouts {
        uuid app_id PK, FK
        uuid job_id PK, FK
        text user_cursor "last user published for"
        timestamptz run_after "the lease, claimable once past"
    }
    usage_daily {
        uuid app_id PK, FK
        date day PK
        bigint queued "checked against daily_quota"
    }
```

## What to notice

- **Everything belongs to an app.** `app_id` leads every primary key, so one app's rows
  sit together and no query can cross apps by accident. Deleting an app cascades to all of it.
- **A job is one row however many people it reaches.** Deliveries are never stored: they
  live in RabbitMQ, and their outcome is counted in Prometheus.
- **`fanouts` is the work queue of the dispatchers.** A row is created with its job and
  deleted when the last delivery is published, so the table holds only jobs in progress.
- **`jobs.list_id` is not a foreign key.** A job keeps its record after its list is deleted.
- **IDs are `uuidv7()`** (Postgres 18), so they sort by creation time. `user_id` is the
  app's own identifier.

## Indexes beyond the keys

| Index | For |
|---|---|
| `endpoints (app_id, user_id) INCLUDE (endpoint_id, channel, provider, address)` | Fan-out reads a page of an audience as one index-only range scan |
| `endpoints UNIQUE (app_id, user_id, channel, address)` | Registering the same endpoint twice does nothing |
| `list_members (app_id, user_id)` | Deleting a user finds its memberships without scanning every list |
| `fanouts (run_after)` | A dispatcher finds the fan-out that has waited longest |
