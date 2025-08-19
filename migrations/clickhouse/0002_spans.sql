CREATE TABLE IF NOT EXISTS spans (
  trace_id       String,
  span_id        String,
  parent_span_id String,
  service        LowCardinality(String),
  name           LowCardinality(String),
  start_time     DateTime64(6, 'UTC'),
  end_time       DateTime64(6, 'UTC'),
  duration_ns    UInt64,
  status_code    LowCardinality(String),
  status_message String,
  attr           Map(String, String)
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(start_time)
ORDER BY (trace_id, start_time)
TTL
  start_time + INTERVAL 7 DAY  TO VOLUME 'warm',
  start_time + INTERVAL 30 DAY TO VOLUME 'cold',
  start_time + INTERVAL 180 DAY DELETE
SETTINGS storage_policy = 'hot_warm_cold';
