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
