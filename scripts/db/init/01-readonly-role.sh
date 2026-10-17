#!/bin/sh
# Runs once, on first initialisation of the Postgres data volume.
#
# Creates a separate READ-ONLY role for the MCP server (see AGENTS.md, section
# "Database"). The agent's database tooling connects as this role only, so a
# stray statement cannot write. statement_timeout caps runaway scans.
set -eu

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
	CREATE ROLE "${POSTGRES_RO_USER}" LOGIN PASSWORD '${POSTGRES_RO_PASSWORD}';
	ALTER ROLE "${POSTGRES_RO_USER}" SET statement_timeout = '10s';

	GRANT CONNECT ON DATABASE "${POSTGRES_DB}" TO "${POSTGRES_RO_USER}";
	GRANT USAGE ON SCHEMA public TO "${POSTGRES_RO_USER}";

	-- Tables are created later, by the migrations, as ${POSTGRES_USER}.
	-- Default privileges keep them readable by the read-only role.
	ALTER DEFAULT PRIVILEGES FOR ROLE "${POSTGRES_USER}" IN SCHEMA public
		GRANT SELECT ON TABLES TO "${POSTGRES_RO_USER}";
EOSQL
