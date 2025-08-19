CREATE TABLE IF NOT EXISTS logs_events (
  timestamp DateTime64(3, 'UTC'),
  service   LowCardinality(String),
  host      LowCardinality(String),
  level     LowCardinality(String),
  trace_id  String,
  span_id   String,
  message   String,
  labels    Map(String, String)
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(timestamp)
ORDER BY (service, timestamp, trace_id)
TTL
  timestamp + INTERVAL 7 DAY  TO VOLUME 'warm',
  timestamp + INTERVAL 30 DAY TO VOLUME 'cold',
  timestamp + INTERVAL 180 DAY DELETE
SETTINGS storage_policy = 'hot_warm_cold', index_granularity = 8192;

ALTER TABLE logs_events
  ADD INDEX IF NOT EXISTS idx_trace trace_id TYPE set(0) GRANULARITY 4;

ALTER TABLE logs_events
  ADD INDEX IF NOT EXISTS idx_msg message TYPE ngrambf_v1(3, 256, 3, 0) GRANULARITY 64;
