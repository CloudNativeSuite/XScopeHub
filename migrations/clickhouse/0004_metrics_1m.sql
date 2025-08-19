CREATE TABLE IF NOT EXISTS metrics_1m (
  bucket      DateTime('UTC'),
  metric_name LowCardinality(String),
  labels      String,
  avg_v       Float64,
  max_v       Float64,
  min_v       Float64
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(bucket)
ORDER BY (metric_name, labels, bucket)
TTL
  bucket + INTERVAL 30 DAY TO VOLUME 'cold',
  bucket + INTERVAL 365 DAY DELETE
SETTINGS storage_policy = 'hot_warm_cold';
