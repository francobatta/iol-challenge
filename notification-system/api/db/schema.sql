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
