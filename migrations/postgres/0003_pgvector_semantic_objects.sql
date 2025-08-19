CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS semantic_objects (
  id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  object_type  TEXT NOT NULL,
  service      TEXT,
  host         TEXT,
  trace_id     TEXT,
  span_id      TEXT,
  ts           TIMESTAMPTZ NOT NULL DEFAULT now(),
  title        TEXT,
  content      TEXT NOT NULL,
  labels       JSONB,
  embedding    vector(1024) NOT NULL,
  ref_source   TEXT,
  ref_key      JSONB
);
CREATE INDEX IF NOT EXISTS idx_semobj_ts ON semantic_objects (ts DESC);
CREATE INDEX IF NOT EXISTS idx_semobj_service_ts ON semantic_objects (service, ts DESC);
CREATE INDEX IF NOT EXISTS idx_semobj_embed_cosine
  ON semantic_objects USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

