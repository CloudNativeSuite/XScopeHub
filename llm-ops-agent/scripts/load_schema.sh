#!/usr/bin/env bash
set -euo pipefail

PGHOST="${PGHOST:-127.0.0.1}"
PGPORT="${PGPORT:-5432}"
PGDATABASE="${PGDATABASE:-ops}"
PGUSER="${PGUSER:-postgres}"
PGPASSWORD="${PGPASSWORD:-postgres}"

export PGPASSWORD

echo "Applying schema to postgresql://${PGUSER}@${PGHOST}:${PGPORT}/${PGDATABASE} ..."
psql "host=$PGHOST port=$PGPORT dbname=$PGDATABASE user=$PGUSER" -v ON_ERROR_STOP=1 -f db/001_schema.sql

echo "Done."
