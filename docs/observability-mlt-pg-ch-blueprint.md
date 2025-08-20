# 概述

“最小但可用”的落地方案，覆盖你要求的四点，并把「CK + 对象存储分层」也并入其中。你可以直接把 DDL 放进 migrations/，把脚本放进 scripts/ 即可跑通。

#  总览（职责边界与协同）

Metrics / Events（热层）：PG/TimescaleDB 存原始点值与直方图，做连续聚合（CAGG）与告警，短期保留（7–30 天）。
Logs & Tracing + 长期 Metrics 聚合（冷层）：ClickHouse 存日志与链路原始数据，外加从 PG 导入的 1 分钟（或 5/60 分钟）下采样聚合，支持对象存储分层（Hot/Warm/Cold）。
协同方式：Writer 按路由写入；PG 侧 CAGG 异步导出到 CH（或双写）；以 trace_id/span_id 等键在 Grafana 跨源联动。
兼容：如已有 metrics_events/logs_events 使用，可保留 logs_events 于 CH；PG 侧提供 metrics_events 兼容视图（由 metrics_points + metric_series 拼出）。

# 1. PG / TimescaleDB（热层：指标 & 业务事件）

## 1.1 扩展与系列表（标签规范化）

```
-- extensions
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 高基数标签去重：把 (metric_name, labels) 规范化成 series_id
CREATE TABLE IF NOT EXISTS metric_series (
  series_id   BIGSERIAL PRIMARY KEY,
  metric_name TEXT NOT NULL,
  labels      JSONB NOT NULL,    -- 建议写入前 key 排序（见下方函数）
  created_at  TIMESTAMPTZ DEFAULT now(),
  UNIQUE (metric_name, labels)
);
```

可选：提供规范化函数确保 labels key 有序，避免重复 series：

```
CREATE OR REPLACE FUNCTION labels_canonical(j JSONB)
RETURNS JSONB LANGUAGE sql IMMUTABLE AS $$
  SELECT jsonb_object_agg(key, value ORDER BY key)
  FROM jsonb_each(j)
$$;
```

## 1.2 指标点值与直方图

```
-- 原子点值（gauge/counter）
CREATE TABLE IF NOT EXISTS metrics_points (
  time        TIMESTAMPTZ NOT NULL,
  series_id   BIGINT NOT NULL REFERENCES metric_series(series_id),
  value       DOUBLE PRECISION NOT NULL
);
SELECT create_hypertable('metrics_points', 'time', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_mp_series_time ON metrics_points (series_id, time DESC);

-- 压缩与保留
ALTER TABLE metrics_points SET (timescaledb.compress);
SELECT add_compression_policy('metrics_points', INTERVAL '7 days');
SELECT add_retention_policy('metrics_points', INTERVAL '30 days');

-- 直方图/分布
CREATE TABLE IF NOT EXISTS metrics_histogram (
  time        TIMESTAMPTZ NOT NULL,
  series_id   BIGINT NOT NULL REFERENCES metric_series(series_id),
  count       BIGINT,
  sum         DOUBLE PRECISION,
  buckets     JSONB           -- 例：{ "0.1":123, "1":45, "+Inf":1 }
);
SELECT create_hypertable('metrics_histogram', 'time', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_mh_series_time ON metrics_histogram (series_id, time DESC);
```

## 1.3 连续聚合（CAGG，下采样导出到 CH）

-- 1 分钟滚动聚合视图（示例）

```
CREATE MATERIALIZED VIEW IF NOT EXISTS metrics_1m
WITH (timescaledb.continuous) AS
SELECT time_bucket('1 minute', mp.time) AS bucket,
       ms.metric_name,
       labels_canonical(ms.labels)       AS labels,  -- 保证 key 有序
       avg(mp.value) AS avg_v,
       max(mp.value) AS max_v,
       min(mp.value) AS min_v
FROM metrics_points mp
JOIN metric_series ms ON ms.series_id = mp.series_id
GROUP BY bucket, ms.metric_name, labels;

SELECT add_continuous_aggregate_policy('metrics_1m',
  start_offset => INTERVAL '2 hours',
  end_offset   => INTERVAL '1 minute',
  schedule_interval => INTERVAL '1 minute');
```

## 1.4 业务事件（可选短保）

```
CREATE TABLE IF NOT EXISTS events_generic (
  id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  ts         TIMESTAMPTZ NOT NULL,
  type       TEXT NOT NULL,                 -- e.g. "deploy", "incident", "release"
  service    TEXT,
  severity   TEXT,                          -- info/warn/error
  payload    JSONB
);
SELECT create_hypertable('events_generic', 'ts', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_events_type_ts ON events_generic (type, ts DESC);
SELECT add_retention_policy('events_generic', INTERVAL '30 days');
```

## 1.5 兼容视图（如需要保留 metrics_events 名称）

```
CREATE OR REPLACE VIEW metrics_events AS
SELECT mp.time, ms.metric_name, ms.labels, mp.value
FROM metrics_points mp
JOIN metric_series ms ON ms.series_id = mp.series_id;
```

# 2. ClickHouse（冷层：日志 + 链路 + 长期指标聚合）

默认启用分层存储策略（Hot/Warm/Cold），7 天热、30 天冷、180 天删除；可按环境参数化。

## 2.1 对象存储与分层策略（config.xml 片段）
```
<clickhouse>
  <storage_configuration>
    <disks>
      <hot>
        <type>local</type>
        <path>/var/lib/clickhouse/hot/</path>
      </hot>
      <warm>
        <type>local</type>
        <path>/var/lib/clickhouse/warm/</path>
      </warm>
      <s3>
        <type>s3</type>
        <endpoint>https://s3.example.com/your-bucket/prefix/</endpoint>
        <access_key_id>YOUR_KEY</access_key_id>
        <secret_access_key>YOUR_SECRET</secret_access_key>
        <metadata_path>/var/lib/clickhouse/disks/s3/</metadata_path>
        <cache_enabled>true</cache_enabled>
        <cache_path>/var/lib/clickhouse/s3_cache/</cache_path>
        <max_cache_size>50Gi</max_cache_size>
      </s3>
    </disks>
    <policies>
      <hot_warm_cold>
        <volumes>
          <hot><disk>hot</disk></hot>
          <warm><disk>warm</disk></warm>
          <cold><disk>s3</disk></cold>
        </volumes>
        <move_factor>0.2</move_factor>
      </hot_warm_cold>
    </policies>
  </storage_configuration>
</clickhouse>
```

## 2.2 日志（logs_events）

```
CREATE TABLE IF NOT EXISTS logs_events (
  timestamp  DateTime64(3, 'UTC'),
  service    LowCardinality(String),
  host       LowCardinality(String),
  level      LowCardinality(String),     -- info/warn/error
  trace_id   String,
  span_id    String,
  message    String,
  labels     Map(String, String)
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
```

## 2.3 链路追踪（spans + span_events）

```
CREATE TABLE IF NOT EXISTS spans (
  trace_id        String,
  span_id         String,
  parent_span_id  String,
  service         LowCardinality(String),
  name            LowCardinality(String),
  start_time      DateTime64(6, 'UTC'),
  end_time        DateTime64(6, 'UTC'),
  duration_ns     UInt64,
  status_code     LowCardinality(String),  -- UNSET/OK/ERROR
  status_message  String,
  attr            Map(String, String)
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
  trace_id    String,
  span_id     String,
  ts          DateTime64(6, 'UTC'),
  name        LowCardinality(String),
  attr        Map(String, String)
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(ts)
ORDER BY (trace_id, span_id, ts)
TTL
  ts + INTERVAL 7 DAY  TO VOLUME 'warm',
  ts + INTERVAL 30 DAY TO VOLUME 'cold',
  ts + INTERVAL 180 DAY DELETE
SETTINGS storage_policy = 'hot_warm_cold';
```

## 2.4 长期指标聚合（从 PG CAGG 导入）
```
CREATE TABLE IF NOT EXISTS metrics_1m (
  bucket      DateTime('UTC'),
  metric_name LowCardinality(String),
  labels      String,      -- PG 侧已按 key 排序后序列化
  avg_v       Float64,
  max_v       Float64,
  min_v       Float64
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(bucket)
ORDER BY (metric_name, labels, bucket)
TTL bucket + INTERVAL 365 DAY DELETE
SETTINGS storage_policy = 'hot_warm_cold';
```

如需去重，可增加导出“幂等水位”（见脚本）而不是依赖 ReplacingMergeTree。

# 3. 写入路由与导出协同

## 3.1 Writer 路由（推荐）

Metrics（点值/直方图）→ PG：metrics_points / metrics_histogram，触发 CAGG。
Logs/Traces → CH：logs_events / spans / span_events。
可选：Writer 内做 1m 窗口聚合并双写 PG/CH（更低延迟，但实现复杂）。

## 3.2 异步导出（PG → CH）（推荐 A）

定时 Job 每分钟从 PG metrics_1m 读取新窗口，写入 CH metrics_1m。
以 bucket 的最大时间做幂等水位控制。

最小脚本（scripts/export_pg_cagg_to_ch.sh）：
```
#!/usr/bin/env bash
set -euo pipefail

# --- Config ---
PG_DSN="${PG_DSN:-postgresql://user:pass@127.0.0.1:5432/db}"
CH_DSN="${CH_DSN:-http://default:@127.0.0.1:8123}"
STATE_FILE="${STATE_FILE:-/var/tmp/pg2ch_metrics_1m.watermark}"  # 记录已导出最大 bucket

# --- Watermark ---
LAST_TS=$(cat "$STATE_FILE" 2>/dev/null || echo "1970-01-01T00:00:00Z")

SQL="
COPY (
  SELECT bucket, metric_name, labels::text, avg_v, max_v, min_v
  FROM metrics_1m
  WHERE bucket > '${LAST_TS}'
  ORDER BY bucket
) TO STDOUT WITH (FORMAT csv)
"

TMPCSV=$(mktemp)
psql "$PG_DSN" -Atc "$SQL" > "$TMPCSV"

if [ -s "$TMPCSV" ]; then
  curl -sS --fail \
    -H 'Content-Type: text/plain' \
    --data-binary @"$TMPCSV" \
    "$CH_DSN/?query=INSERT%20INTO%20metrics_1m%20FORMAT%20CSV"

  # 更新水位（取最后一行的 bucket）
  NEW_TS=$(tail -n1 "$TMPCSV" | cut -d',' -f1)
  echo "$NEW_TS" > "$STATE_FILE"
fi

rm -f "$TMPCSV"
```

运行频率建议：每分钟一次；若需要“迟到修补”，可在 SQL 中把 bucket > LAST_TS 改为 bucket >= LAST_TS - interval '5 minutes' 并在 CH 端接受重复（借助 insert_deduplicate=1 + 分批小窗口）。

# 4. 查询与联动（Grafana）

- 数据源：PostgreSQL（近实时热层） + ClickHouse（长周期与 Logs/Traces）。
- 联动键：trace_id / span_id / service / host / 规范化 labels 的关键维度。
- 体验建议：
  - 指标面板（PG/CH）：异常点 → 添加 data link 到 logs_events（按时间窗口 + service + trace_id）。
  - 日志面板（CH）：点击 trace_id → 跳转链路视图（spans 查询）。
  - 长周期查询优先走 metrics_1m（CH），原始日志/Span 做范围收敛（先按 service/时间过滤）。

# 5. 保留、压缩与 HA

PG：
- metrics_points：压缩 7 天、保留 30 天（按成本调整）。
- CAGG：保留 90 天或更久（同时导出到 CH）。
- events_generic：短保（默认 30 天）。
- 选择合适 chunk_interval（如 1d/6h），批量 COPY 写入。

CH：

- logs_events/spans/span_events：TTL 7 天热 → 30 天冷（S3）→ 180 天删除；index_granularity = 8192。
- metrics_1m：TTL ≥ 365 天（可更长）。

需要多副本：ReplicatedMergeTree + Keeper；对象存储可结合 zero-copy replication（视版本与配置）。

开启 S3 本地缓存（已在 config 示例中），大幅改善冷热穿越扫描体验。

# 6. Writer/Schema 实施要点（不丢内容的关键点）

- 标签规范化：PG 侧以 (metric_name, labels) 唯一约束生成 series_id，写入点值仅带 series_id，降低写放大；CH 侧长留存用排序后的 JSON 文本（labels 列）保证 GROUP BY 稳定。
- Tracing 入口：OTLP → Gateway → CH（spans/span_events）；日志写入携带 trace_id/span_id，实现跨源关联。
- 日志全文检索：ngrambf_v1 或 tokenbf_v1（按需），搭配 set 索引（trace_id）。
- 幂等写入：导出采用水位法；如需更强去重，可在 CH 开启 session insert_deduplicate=1 并保证单批次稳定分块。

性能基线：

- CH ORDER BY 尽量把高选择性列靠前：日志 (service, timestamp, trace_id)，Span (trace_id, start_time)。
- PG 合理索引、并行计划与批量写入；CAGG 滚动策略与 schedule 结合写入节奏调优。

# 7. 目录建议（migrations + scripts)

```
migrations/
  pg/
    0001_extensions.sql
    0002_metric_series.sql
    0003_metrics_points.sql
    0004_metrics_histogram.sql
    0005_cagg_metrics_1m.sql
    0006_events_generic.sql          # 可选
    0007_view_metrics_events.sql     # 兼容视图（可选）
  clickhouse/
    0001_logs_events.sql
    0002_spans.sql
    0003_span_events.sql
    0004_metrics_1m.sql
    # 注意：CH 的 storage policy 在 config.xml，不放 SQL 里
scripts/
  export_pg_cagg_to_ch.sh
```

TL;DR（按你的 4 点复核）

- M/L/T/E 覆盖

Metrics：PG 的 metric_series + metrics_points + metrics_histogram + CAGG；（可通过视图兼容 metrics_events 名称）
Logs：CH logs_events，毫秒精度、合理 ORDER BY、倒排与 ngram 索引
Tracing：CH spans + span_events（必备字段齐全）
Events：PG events_generic（短保、告警友好）

- PG/TimescaleDB（热层）
原始点值 + 直方图 + 连续聚合 + 压缩/保留策略，支持 1m/5m/1h 下采样与告警。

- ClickHouse（冷层）

日志与链路原始、长期指标聚合；启用对象存储分层（Hot/Warm/Cold）降低 TCO。

- 协同

Writer 路由；PG CAGG → CH metrics_1m 的异步导出（或双写）；以 trace_id 等键在 Grafana 做跨源联动。
