-- TimescaleDB 扩展（如已启用可忽略）
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Schema registry
CREATE TABLE IF NOT EXISTS table_registry (
  table_name  TEXT PRIMARY KEY,
  schema      JSONB NOT NULL,
  created_at  TIMESTAMPTZ DEFAULT now()
);
