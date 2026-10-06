-- Schema of the audience API. Requires Postgres 18 for uuidv7(), which makes
-- server-generated IDs sort by creation time.

CREATE TABLE apps (
    app_id     uuid        PRIMARY KEY DEFAULT uuidv7(),
    name       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    app_id     uuid        NOT NULL REFERENCES apps ON DELETE CASCADE,
    user_id    text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (app_id, user_id)
);

CREATE TABLE endpoints (
    app_id      uuid NOT NULL,
    endpoint_id uuid NOT NULL DEFAULT uuidv7(),
    user_id     text NOT NULL,
    address     text NOT NULL,
    channel     text NOT NULL CHECK (channel IN ('email', 'sms', 'push')),
    provider    text NOT NULL,
    PRIMARY KEY (app_id, endpoint_id),
    FOREIGN KEY (app_id, user_id) REFERENCES users ON DELETE CASCADE,
    UNIQUE (app_id, user_id, channel, address)
);

-- Everything fan-out needs of an endpoint, in user order. A page of an audience reads its
-- endpoints as one range of this index, without visiting the table.
CREATE INDEX endpoints_fanout_idx ON endpoints (app_id, user_id) INCLUDE (endpoint_id, channel, provider, address);

CREATE TABLE lists (
    app_id      uuid        NOT NULL REFERENCES apps ON DELETE CASCADE,
    list_id     uuid        NOT NULL DEFAULT uuidv7(),
    name        text        NOT NULL,
    description text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (app_id, list_id),
    UNIQUE (app_id, name)
);

CREATE TABLE list_members (
    app_id   uuid        NOT NULL,
    list_id  uuid        NOT NULL,
    user_id  text        NOT NULL,
    added_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (app_id, list_id, user_id),
    FOREIGN KEY (app_id, list_id) REFERENCES lists ON DELETE CASCADE,
    FOREIGN KEY (app_id, user_id) REFERENCES users ON DELETE CASCADE
);

-- Lets deleting a user find its memberships without scanning every list.
CREATE INDEX list_members_user_idx ON list_members (app_id, user_id);

-- The most notifications an app may queue in one day.
ALTER TABLE apps ADD COLUMN daily_quota bigint NOT NULL DEFAULT 20000000;

-- A job is one request to notify some users. It is a single row however many people it
-- reaches: deliveries live in RabbitMQ, and what becomes of them is counted by the
-- workers' metrics, not here.
CREATE TABLE jobs (
    app_id          uuid        NOT NULL REFERENCES apps ON DELETE CASCADE,
    job_id          uuid        NOT NULL DEFAULT uuidv7(),
    status          text        NOT NULL DEFAULT 'pending'
                                CHECK (status IN ('pending', 'dispatching', 'dispatched')),
    priority        text        NOT NULL CHECK (priority IN ('high', 'normal')),
    -- The audience: these users, plus the members of the list if there is one.
    user_ids        text[]      NOT NULL DEFAULT '{}',
    list_id         uuid,
    title           text        NOT NULL DEFAULT '',
    body            text        NOT NULL,
    idempotency_key text,
    queued          bigint      NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (app_id, job_id),
    UNIQUE (app_id, idempotency_key)
);

-- A fanout is the work a job is still owed: turning its audience into deliveries. It is
-- stored with the job and deleted when the last delivery has been published, so the table
-- holds only the jobs in progress and the dispatchers that poll it read next to nothing.
CREATE TABLE fanouts (
    app_id      uuid        NOT NULL,
    job_id      uuid        NOT NULL,
    -- The last user deliveries have been published for; fan-out resumes after it.
    user_cursor text        NOT NULL DEFAULT '',
    -- No dispatcher takes the fan-out before this time. Taking it moves the time forward,
    -- which is what hands the work of a dispatcher that died or failed to another.
    run_after   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (app_id, job_id),
    FOREIGN KEY (app_id, job_id) REFERENCES jobs ON DELETE CASCADE
);

-- Lets a dispatcher find the fan-out that has waited longest.
CREATE INDEX fanouts_run_after_idx ON fanouts (run_after);

-- Deliveries queued per app and day, which the daily quota is checked against.
CREATE TABLE usage_daily (
    app_id uuid   NOT NULL REFERENCES apps ON DELETE CASCADE,
    day    date   NOT NULL,
    queued bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (app_id, day)
);
