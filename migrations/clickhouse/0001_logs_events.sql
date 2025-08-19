CREATE TABLE IF NOT EXISTS logs_events (
  timestamp DateTime,
  app       String,
  host      String,
  trace_id  String,
  message   String,
  labels    Nested(k String, v String)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (timestamp, app);
