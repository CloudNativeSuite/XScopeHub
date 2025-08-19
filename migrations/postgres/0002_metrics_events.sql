CREATE TABLE IF NOT EXISTS metrics_events (
  time        TIMESTAMPTZ NOT NULL,
  app         TEXT,
  host        TEXT,
  labels      JSONB,
  value       DOUBLE PRECISION,
  trace_id    TEXT,
  level       TEXT
);
SELECT create_hypertable('metrics_events', 'time', if_not_exists => TRUE, migrate_data => TRUE);
