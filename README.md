# iol-challenge

A Go REST API that manages who a notifications service can reach: apps, their users, each
user's endpoints, and app-owned lists of users. The design, with the full route table, is in
[docs/superpowers/specs/2026-10-05-audience-api-design.md](docs/superpowers/specs/2026-10-05-audience-api-design.md).

## Layout

- `notification-system/` holds the whole system and its `compose.yaml`.
- `notification-system/api/` is this API: a Go module with its own `Dockerfile`.

## Run

```sh
cd notification-system
docker compose up --build
```

This starts Postgres 18 with `api/db/schema.sql` loaded and the API on `localhost:8080`, using
the development secrets in `compose.yaml`.

```sh
# Create an app; the response carries its token.
curl -X POST localhost:8080/v1/apps -H 'X-Admin-Key: dev-admin-key' -d '{"name": "my app"}'

# Everything else is called with that token.
curl -X PUT localhost:8080/v1/users/ana -H "Authorization: Bearer $TOKEN"
```

## Test

```sh
cd notification-system/api
go test ./...
```

The Postgres store tests are skipped unless `DATABASE_URL` points to a database with the
schema loaded:

```sh
docker compose -f ../compose.yaml up -d --wait db
DATABASE_URL=postgres://audience:audience@localhost:5432/audience go test ./...
```

## Regenerate code

In `notification-system/api`, after changing the `audience.Store` interface run
`go generate ./...` to rebuild the gomock mock in `internal/audience/audiencetest`.

## Change the SQL

In `notification-system/api`, edit `db/schema.sql` or `db/queries.sql`, then run
`sqlc generate` to rebuild `internal/postgres/queries`.
