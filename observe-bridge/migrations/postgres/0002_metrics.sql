-- Metric series normalize label sets
CREATE TABLE IF NOT EXISTS metric_series (
  series_id   BIGSERIAL PRIMARY KEY,
  metric_name TEXT NOT NULL,
  labels      JSONB NOT NULL,
  created_at  TIMESTAMPTZ DEFAULT now(),
  UNIQUE (metric_name, labels)
);

-- Atomic metric points
CREATE TABLE IF NOT EXISTS metrics_points (
  time      TIMESTAMPTZ NOT NULL,
  series_id BIGINT NOT NULL REFERENCES metric_series(series_id),
  value     DOUBLE PRECISION NOT NULL
);
SELECT create_hypertable('metrics_points', 'time', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_mp_series_time ON metrics_points (series_id, time DESC);
ALTER TABLE metrics_points SET (timescaledb.compress);
SELECT add_compression_policy('metrics_points', INTERVAL '7 days');
SELECT add_retention_policy('metrics_points', INTERVAL '30 days');

-- Optional histogram support
CREATE TABLE IF NOT EXISTS metrics_histogram (
  time      TIMESTAMPTZ NOT NULL,
  series_id BIGINT NOT NULL REFERENCES metric_series(series_id),
  count     BIGINT,
  sum       DOUBLE PRECISION,
  buckets   JSONB
);
SELECT create_hypertable('metrics_histogram', 'time', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_mh_series_time ON metrics_histogram (series_id, time DESC);

-- Continuous aggregate for 1m downsampling
CREATE MATERIALIZED VIEW IF NOT EXISTS metrics_1m
WITH (timescaledb.continuous) AS
SELECT time_bucket('1 minute', mp.time) AS bucket,
       ms.metric_name,
       ms.labels,
       avg(mp.value) AS avg_v,
       max(mp.value) AS max_v,
       min(mp.value) AS min_v
FROM metrics_points mp
JOIN metric_series ms ON ms.series_id = mp.series_id
GROUP BY bucket, ms.metric_name, ms.labels;

SELECT add_continuous_aggregate_policy('metrics_1m',
  start_offset => INTERVAL '2 hours',
  end_offset   => INTERVAL '1 minute',
  schedule_interval => INTERVAL '1 minute');
