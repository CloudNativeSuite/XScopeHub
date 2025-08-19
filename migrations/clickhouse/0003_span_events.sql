CREATE TABLE IF NOT EXISTS span_events (
  trace_id String,
  span_id  String,
  ts       DateTime64(6, 'UTC'),
  name     LowCardinality(String),
  attr     Map(String, String)
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(ts)
ORDER BY (trace_id, span_id, ts)
TTL
  ts + INTERVAL 7 DAY  TO VOLUME 'warm',
  ts + INTERVAL 30 DAY TO VOLUME 'cold',
  ts + INTERVAL 180 DAY DELETE
SETTINGS storage_policy = 'hot_warm_cold';
