# Architecture

## Overview

### 总体设计

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

— Vector 的缓冲与背压可在出口阻塞时把压力“兜”在边缘；OTel Gateway 通过 file_storage（WAL）+ sending_queue 提供持久化发送队列；OO 侧有内置 WAL 与对象存储落地，整体形成“端—边—域”多级持久化。
Vector
OpenTelemetry
OpenObserve

### 多区域设计 / 区域内拓扑

区域内拓扑（A 区示例）

```
[Producers / Agents / Apps]
  └─ (OTLP gRPC/HTTP, Filelog/Prom→OTLP)
        → OTel Gateway (Region-A, N副本, LB)
              ├─ 统一打标签/规范化/生成 event_id
              ├─ 扇出 → OpenObserve.A（近线检索/告警）
              └─ 扇出 → Kafka.A（*_raw 旁路：logs/metrics/traces）
                     ↘（ETL）→ PostgreSQL.A（明细/聚合/关系/向量）
```


多区域（A 主 + B/C 从；统一查询入口）
各区独立：OpenObserve.A/B/C、Kafka.A/B/C、Postgres.A/B/C。
**主区（A）**提供统一查询/汇总（API Gateway / Query Proxy）；默认“就近或主区”路由。
跨区镜像：关键数据（*_norm 或抽样 traces）双写到两个 Region，或以 MirrorMaker2 做 Kafka 跨区镜像到中央集群；中央再做归一化与合并视图。
Amazon Web Services, Inc.
Confluent
红帽文档

### 多级持久化与可重放

“不丢包设计”的专业化表述: 端到端“至少一次（At-least-once）传输语义 + 多级持久化与可重放”

1) 多级持久化链

- 边缘层：Vector 磁盘/内存缓冲 + 背压（出口拥塞时不丢，推高边缘队列）。
Vector
- 区域网关：OTelcol sending_queue + file_storage（WAL）（进程重启/下游不可用时持久化并重试）。
OpenTelemetry
- 旁路总线：Kafka 作为“真源与回放库”（*_raw 主题，RF≥3，acks=all + min.insync.replicas≥2），确保分区内多副本提交后再确认；保留期 ≥ 回放窗口。
- 落地层：OpenObserve 侧 WAL→对象存储（Parquet），降低落地成本并简化横向扩展。 OpenObserve

2) 去重策略

- 区域 OTel Gateway 一写多发：OO（近线检索/告警） + Kafka（回放与 ETL 真源）。
- 在 Gateway 统一生成 event_id（内容指纹/雪花）；PG 侧 UPSERT 幂等入库，允许“重复不缺失”。
- 采样与降噪在 Vector/OTel 侧完成（如仅保留 Error/Warn 或 TopK 热点）。

3) SRE 可观测与演练

- 监控：Exporter 失败率、Gateway 发送队列长度、Vector 缓冲水位、Kafka Lag、PG UPSERT 冲突率。
- 演练：断网 30–60 分钟、重启 Gateway/ETL、下游 429/5xx 限流、历史主题回放验证统计闭合。
- PostgreSQL 二级分析域（“向量-时序-图-TopK”一体）
- 时序：TimescaleDB **连续聚合（Continuous Aggregates）**构建分钟/小时级物化视图，支持分层聚合与长期保留。
- 向量：pgvector（IVFFlat/HNSW）适配告警相似度、日志语义去重/检索等场景。
- 图：Apache AGE（openCypher）记录服务依赖/调用链与实体关系，支持关系追踪/根因查询。
- TopK/近似：HLL（HyperLogLog）与 Timescale Toolkit 支持近似去重/分位数，做高基数聚合与TopK 热点分析。

说明：把“原始海量日志/追踪”长期留在对象存储（OO/ClickHouse 侧更经济），PG 专注结构化明细与多模型分析层。

### 多区域与容灾（DR）设计要点

- 接入与路由 Anycast/GeoDNS 指向就近 OTel Gateway；关键租户可在 边缘或 LB 层做双写两区。
- 跨区复制  Kafka MirrorMaker2：把 *_raw / *_norm 主题跨区镜像到中央或双活集群；主区/中央承担“联邦汇总/统一检索”。
- 阈值与降级 网关/边缘磁盘水位告警（例如 80%）；超阈后策略性降噪（丢 Debug/Info、保 Error/Warn/TopK）。
- 恢复与回放 以 Kafka 为侧路回放源：新规则/新 Schema 上线前，用历史主题重放到沙箱管道做 A/B 验证。

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
