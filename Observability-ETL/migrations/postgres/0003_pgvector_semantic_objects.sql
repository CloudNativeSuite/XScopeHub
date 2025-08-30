-- 安装扩展
CREATE EXTENSION IF NOT EXISTS vector;   -- pgvector
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 统一语义对象：日志片段/告警/知识文档/变更记录/Playbook 等
-- 说明：
--   object_type: 'log' | 'alert' | 'doc' | 'trace' | 'metric_anomaly' ...
--   ref_*: 指向源数据（CH/PG）定位键，便于回跳原始记录
CREATE TABLE IF NOT EXISTS semantic_objects (
  id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  object_type  TEXT NOT NULL,
  service      TEXT,
  host         TEXT,
  trace_id     TEXT,
  span_id      TEXT,
  ts           TIMESTAMPTZ NOT NULL DEFAULT now(),
  title        TEXT,
  content      TEXT NOT NULL,           -- 原文（日志片段/文档段落等）
  labels       JSONB,
  embedding    vector(1024) NOT NULL,   -- 选用你的嵌入维度，如 768/1024/1536
  ref_source   TEXT,                    -- 'clickhouse:logs_events' / 'postgres:metrics_points' 等
  ref_key      JSONB                    -- 记录源表定位键（如 {ts, service, trace_id, ...}）
);

-- 典型索引
CREATE INDEX IF NOT EXISTS idx_semobj_ts ON semantic_objects (ts DESC);
CREATE INDEX IF NOT EXISTS idx_semobj_service_ts ON semantic_objects (service, ts DESC);

-- 近似向量索引（pgvector）
-- 选择余弦距离：vector_cosine_ops；lists 需按数据量微调（10~200）
CREATE INDEX IF NOT EXISTS idx_semobj_embed_cosine
ON semantic_objects
USING ivfflat (embedding vector_cosine_ops)
WITH (lists = 100);


