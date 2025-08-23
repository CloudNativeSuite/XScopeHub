# Architecture

## Overview

```
[边缘节点/主机/Pod]
  ├─ Vector（Filelog/Prom/进程指标→统一转 OTLP）
  │    └─ 本地内存/磁盘缓冲 + 背压
  │         → OTLP/gRPC →  区域 OTel Gateway（持久化队列/WAL）
  │         ↘（可选旁路）→ Kafka.<region>.logs_raw / metrics_raw / traces_raw
  └─ 应用/SDK Trace → 直接 OTLP → 区域 OTel Gateway

[区域网关（多副本，LB 前置）]
  ├─ OTelcol（接 OTLP/Kafka；file_storage + sending_queue）
  │    └─ Exporter 扇出：
  │        → OpenObserve（检索/告警/可视化）
  │        → Kafka.<region>（旁路重放与 ETL 真源）

[存储与分析]
  ├─ OpenObserve（对象存储/本地盘 + WAL；Logs/Metrics/Traces）
  ├─ Kafka（RF≥3）→ ETL（Benthos/Redpanda Connect/Flink）
  │        → PostgreSQL（明细 & 汇总：Timescale/pgvector/AGE/HLL）
  └─ （可选）cLoki/qryn 或 LGTM 替代检索面
```

## Module Responsibilities
- `gateway`: receive writes, route to PG/CH, validate schema, expose health checks.
- `registry`: dynamic table registration and JSONB schema metadata management.
- `pg-writer`: batch writes to TimescaleDB for metrics/events with short retention.
- `ch-writer`: high-throughput writes to ClickHouse for logs/traces and archival.

## Development & Deployment Notes
- PoC: use `deployments/docker-compose/poc.yaml` to launch PostgreSQL, ClickHouse, Grafana, and the four core services.
- Evolution: Helm chart separates Deployment and Service for horizontal scaling; introduce Kafka/NATS when buffering is needed.
- Query: access data via Grafana; PostgreSQL serves real-time curves/aggregations, ClickHouse handles logs and analytical queries.
- Schema management: all tables are registered via the schema registry, with gateway-side validation and lazy table creation.

## Storage Retention
- ClickHouse supports tiered storage via `storage_policy` allowing hot data on local SSD and cold data on S3/OSS.
- Logs and traces typically keep 7 days hot, move to cheaper disks after 7 days, offload to object storage after 30 days and delete after ~180 days using `TTL ... TO VOLUME`.
- Aggregated metrics (e.g., `metrics_1m`) can retain 30 days on hot disks and archive up to 365 days on object storage.
- An example policy:

  ```xml
  <storage_configuration>
    <disks>
      <hot><type>local</type><path>/var/lib/clickhouse/hot/</path></hot>
      <warm><type>local</type><path>/var/lib/clickhouse/warm/</path></warm>
      <s3>
        <type>s3</type>
        <endpoint>https://s3.example.com/bucket/</endpoint>
        <access_key_id>KEY</access_key_id>
        <secret_access_key>SECRET</secret_access_key>
        <cache_enabled>true</cache_enabled>
        <cache_path>/var/lib/clickhouse/s3_cache/</cache_path>
      </s3>
    </disks>
    <policies>
      <hot_warm_cold>
        <volumes>
          <hot><disk>hot</disk></hot>
          <warm><disk>warm</disk></warm>
          <cold><disk>s3</disk></cold>
        </volumes>
      </hot_warm_cold>
    </policies>
  </storage_configuration>
  ```

This setup extends retention (7–90+ days) while keeping recent data fast and older data cost‑efficient.

## Reference Schemas

### PostgreSQL / TimescaleDB
```sql
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
```

### ClickHouse
```sql
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
```

The SQL above is provided under `migrations/postgres` and `migrations/clickhouse` for initialization.
