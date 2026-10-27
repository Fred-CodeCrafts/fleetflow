# Database

FleetFlow keeps vehicles, drivers, trips, saved runs and hashed API keys in
PostgreSQL. Live congestion state does not live here: reports and route queries
never touch this database, so the routing hot path keeps working even if it goes
away (DESIGN 3.7).

## Schema

`db/migration/` is the only place the schema is defined. Files are applied by
[Flyway](https://documentation.red-gate.com/flyway) in version order and are
immutable once applied — a mistake is fixed by adding the next migration, never
by editing an earlier one or by hand-editing a running database.

Names follow Flyway's convention: `V<version>__<description>.sql`, for example
`V1__fleet_records.sql`. The double underscore separates the version from the
description.

`V10__driver_contract_type.sql` adds the driver's contract type to the stored
profile. Apply it with the same Flyway workflow before deploying code that reads
or writes that field.

Flyway keeps its own bookkeeping in `flyway_schema_history`, which records which
versions ran and whether they succeeded.

## Applying migrations

The database is the `postgres` service in `docker-compose.yml`, and the `migrate`
service applies the migrations. It sits behind a profile so that starting the
database does not silently change its schema:

```sh
docker compose up -d                          # start the database
docker compose --profile tools run --rm migrate
```

Credentials come from the same `.env` the database reads, and they are passed to
Flyway as environment variables rather than command-line flags, so they do not
appear in `ps` output.

Rules that must hold under concurrent requests — one vehicle per driver, and one
unfinished trip per vehicle — are unique constraints and partial indexes in the
migration, not checks in Go. Two racing requests lose at the database, wherever
the service happened to be.

## Running the integration tests

The repository tests in `internal/*/postgres` are skipped unless
`TEST_DATABASE_URL` names a database, so the unit suite needs no server:

```sh
TEST_DATABASE_URL='postgres://fleetflow:...@127.0.0.1:55432/fleetflow' \
  go test ./internal/vehicle/postgres/...
```

`storetest.Reset` empties the fleet tables, and `go test ./...` runs packages in
parallel. Two packages sharing one database will therefore truncate each other's
rows, and the failure looks like a race in the code. Give each package its own
database, or run them one at a time with `-p 1`.
