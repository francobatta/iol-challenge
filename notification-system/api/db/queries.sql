-- Queries compiled by sqlc into internal/postgres/queries.
-- Every query is scoped by app_id. Collections page by primary key: rows after @after, in order.

-- name: CreateApp :one
INSERT INTO apps (name) VALUES (@name) RETURNING *;

-- name: InsertUser :execrows
INSERT INTO users (app_id, user_id) VALUES (@app_id, @user_id) ON CONFLICT DO NOTHING;

-- name: User :one
SELECT * FROM users WHERE app_id = @app_id AND user_id = @user_id;

-- name: Users :many
SELECT * FROM users
WHERE app_id = @app_id AND user_id > @after
ORDER BY user_id
LIMIT @max_rows;

-- name: KnownUsers :many
SELECT user_id FROM users WHERE app_id = @app_id AND user_id = ANY(@user_ids::text[]);

-- name: DeleteUser :execrows
DELETE FROM users WHERE app_id = @app_id AND user_id = @user_id;

-- name: CreateEndpoint :one
INSERT INTO endpoints (app_id, user_id, address, channel, provider)
VALUES (@app_id, @user_id, @address, @channel, @provider)
RETURNING *;

-- name: Endpoint :one
SELECT * FROM endpoints WHERE app_id = @app_id AND endpoint_id = @endpoint_id;

-- name: Endpoints :many
SELECT * FROM endpoints
WHERE app_id = @app_id AND user_id = @user_id AND endpoint_id > @after
ORDER BY endpoint_id
LIMIT @max_rows;

-- name: UpdateEndpoint :one
UPDATE endpoints
SET address = @address, channel = @channel, provider = @provider
WHERE app_id = @app_id AND endpoint_id = @endpoint_id
RETURNING *;

-- name: DeleteEndpoint :execrows
DELETE FROM endpoints WHERE app_id = @app_id AND endpoint_id = @endpoint_id;

-- name: CreateList :one
INSERT INTO lists (app_id, name, description)
VALUES (@app_id, @name, @description)
RETURNING *;

-- name: List :one
SELECT * FROM lists WHERE app_id = @app_id AND list_id = @list_id;

-- name: Lists :many
SELECT * FROM lists
WHERE app_id = @app_id AND list_id > @after
ORDER BY list_id
LIMIT @max_rows;

-- name: UpdateList :one
UPDATE lists
SET name = @name, description = @description
WHERE app_id = @app_id AND list_id = @list_id
RETURNING *;

-- name: DeleteList :execrows
DELETE FROM lists WHERE app_id = @app_id AND list_id = @list_id;

-- name: AddMembers :exec
INSERT INTO list_members (app_id, list_id, user_id)
SELECT @app_id::uuid, @list_id::uuid, unnest(@user_ids::text[])
ON CONFLICT DO NOTHING;

-- name: RemoveMember :exec
DELETE FROM list_members WHERE app_id = @app_id AND list_id = @list_id AND user_id = @user_id;

-- name: Members :many
SELECT user_id, added_at FROM list_members
WHERE app_id = @app_id AND list_id = @list_id AND user_id > @after
ORDER BY user_id
LIMIT @max_rows;

-- name: InsertJob :one
-- Stores a job together with the fan-out it is owed. Returns no row when the app has
-- already used the idempotency key.
WITH job AS (
    INSERT INTO jobs (app_id, priority, user_ids, list_id, title, body, idempotency_key)
    VALUES (@app_id, @priority, @user_ids, @list_id, @title, @body, @idempotency_key)
    ON CONFLICT (app_id, idempotency_key) DO NOTHING
    RETURNING *
), fanout AS (
    INSERT INTO fanouts (app_id, job_id) SELECT job.app_id, job.job_id FROM job
)
SELECT * FROM job;

-- name: JobByIdempotencyKey :one
SELECT * FROM jobs WHERE app_id = @app_id AND idempotency_key = @idempotency_key;

-- name: Job :one
SELECT * FROM jobs WHERE app_id = @app_id AND job_id = @job_id;

-- name: Jobs :many
SELECT * FROM jobs
WHERE app_id = @app_id AND job_id > @after
ORDER BY job_id
LIMIT @max_rows;

-- name: Quota :one
SELECT a.daily_quota, COALESCE(u.queued, 0)::bigint AS used
FROM apps a
LEFT JOIN usage_daily u ON u.app_id = a.app_id AND u.day = current_date
WHERE a.app_id = @app_id;

-- name: ClaimFanout :one
-- Takes the fan-out that has been due longest and keeps the others off it for the lease.
-- SKIP LOCKED makes dispatchers that claim at the same moment take different ones.
WITH claimed AS (
    UPDATE fanouts f
    SET run_after = now() + make_interval(secs => @lease_secs)
    WHERE (f.app_id, f.job_id) = (
        SELECT d.app_id, d.job_id FROM fanouts d
        WHERE d.run_after <= now()
        ORDER BY d.run_after
        LIMIT 1
        FOR UPDATE SKIP LOCKED
    )
    RETURNING f.app_id, f.job_id, f.user_cursor
)
SELECT c.app_id, c.job_id, c.user_cursor, j.priority, j.user_ids, j.list_id, j.title, j.body
FROM claimed c
JOIN jobs j ON j.app_id = c.app_id AND j.job_id = c.job_id;

-- name: FanoutPage :many
-- The next @max_users users of a job's audience after @after, each once, with their
-- endpoints: one row per endpoint, and a row without one for a user that has none, so
-- that the caller sees every user the page covers.
--
-- It is three index range scans and no more. The members come in order from the primary
-- key of list_members, the named users need no table at all, and the endpoints of the
-- page are one range of endpoints_fanout_idx. Each branch is limited on its own so that
-- it stops after one page.
WITH page AS (
    SELECT t.user_id FROM (
        (SELECT m.user_id FROM list_members m
         WHERE m.app_id = @app_id AND m.list_id = @list_id AND m.user_id > @after
         ORDER BY m.user_id LIMIT @max_users)
        UNION
        (SELECT u.user_id FROM unnest(@user_ids::text[]) AS u(user_id)
         WHERE u.user_id > @after
         ORDER BY u.user_id LIMIT @max_users)
    ) t
    ORDER BY t.user_id
    LIMIT @max_users
)
SELECT p.user_id::text AS user_id, e.endpoint_id, e.channel, e.provider, e.address
FROM page p
LEFT JOIN endpoints e ON e.app_id = @app_id AND e.user_id = p.user_id
ORDER BY p.user_id, e.endpoint_id;

-- name: AdvanceFanout :execrows
-- One statement, so the cursor, the job's count and the app's usage move together. It
-- also renews the lease. Nothing changes, and no row is affected, unless the cursor is
-- still at @previous_cursor: a dispatcher that lost its lease must not count a page twice.
WITH advanced AS (
    UPDATE fanouts f
    SET user_cursor = @user_cursor, run_after = now() + make_interval(secs => @lease_secs)
    WHERE f.app_id = @app_id AND f.job_id = @job_id AND f.user_cursor = @previous_cursor
    RETURNING f.app_id, f.job_id
), counted AS (
    UPDATE jobs j
    SET status = 'dispatching', queued = j.queued + @published, updated_at = now()
    FROM advanced a
    WHERE j.app_id = a.app_id AND j.job_id = a.job_id
)
INSERT INTO usage_daily (app_id, day, queued)
SELECT a.app_id, current_date, @published FROM advanced a
ON CONFLICT (app_id, day) DO UPDATE SET queued = usage_daily.queued + EXCLUDED.queued;

-- name: FinishFanout :execrows
-- AdvanceFanout for the last page: instead of moving the cursor it deletes the fan-out.
WITH finished AS (
    DELETE FROM fanouts f
    WHERE f.app_id = @app_id AND f.job_id = @job_id AND f.user_cursor = @previous_cursor
    RETURNING f.app_id, f.job_id
), counted AS (
    UPDATE jobs j
    SET status = 'dispatched', queued = j.queued + @published, updated_at = now()
    FROM finished f
    WHERE j.app_id = f.app_id AND j.job_id = f.job_id
)
INSERT INTO usage_daily (app_id, day, queued)
SELECT f.app_id, current_date, @published FROM finished f
ON CONFLICT (app_id, day) DO UPDATE SET queued = usage_daily.queued + EXCLUDED.queued;
