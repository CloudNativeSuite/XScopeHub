#!/usr/bin/env bash
set -euo pipefail

PG_DSN=${PG_DSN:-"postgres://postgres:postgres@localhost:5432/xscope?sslmode=disable"}
CH_HOST=${CH_HOST:-"localhost"}

psql "$PG_DSN" -c "COPY (SELECT bucket, metric_name, labels, avg_v, max_v, min_v FROM metrics_1m WHERE bucket > now() - interval '1 minute') TO STDOUT WITH CSV" \
  | clickhouse-client --host "$CH_HOST" --query="INSERT INTO metrics_1m FORMAT CSV"
