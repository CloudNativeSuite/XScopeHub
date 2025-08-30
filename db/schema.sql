-- PostgreSQL schema initialization for OPSAgent
-- Generated according to PG data model overview.

-- 0) Extensions (once)
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- gen_random_uuid
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS age;       -- graph extension for service call graph
LOAD 'age';
CREATE EXTENSION IF NOT EXISTS btree_gist; -- temporal topology range index

-- 1) Dimensions (2)
CREATE TABLE dim_tenant (
  tenant_id   BIGSERIAL PRIMARY KEY,
  code        TEXT UNIQUE NOT NULL,
  name        TEXT NOT NULL,
  labels      JSONB DEFAULT '{}'::jsonb,
  created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE dim_resource (
  resource_id BIGSERIAL PRIMARY KEY,
  tenant_id   BIGINT REFERENCES dim_tenant(tenant_id),
  urn         TEXT UNIQUE NOT NULL,
  type        TEXT NOT NULL,
  name        TEXT NOT NULL,
  env         TEXT,
  region      TEXT,
  zone        TEXT,
  labels      JSONB DEFAULT '{}'::jsonb,
  created_at  TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_res_type ON dim_resource(type);
CREATE INDEX idx_res_labels_gin ON dim_resource USING GIN(labels);

-- 2) Object storage locator (1)
CREATE TABLE oo_locator (
  id          BIGSERIAL PRIMARY KEY,
  tenant_id   BIGINT REFERENCES dim_tenant(tenant_id),
  dataset     TEXT NOT NULL,             -- logs / traces / metrics
  bucket      TEXT NOT NULL,
  object_key  TEXT NOT NULL,
  t_from      TIMESTAMPTZ NOT NULL,
  t_to        TIMESTAMPTZ NOT NULL,
  query_hint  TEXT,
  attributes  JSONB DEFAULT '{}'::jsonb
);
CREATE INDEX idx_oo_time ON oo_locator(dataset, t_from, t_to);

-- 3) Metric aggregates (1m, Hypertable)
CREATE TABLE metric_1m (
  bucket       TIMESTAMPTZ NOT NULL,
  tenant_id    BIGINT REFERENCES dim_tenant(tenant_id),
  resource_id  BIGINT REFERENCES dim_resource(resource_id),
  metric       TEXT NOT NULL,
  avg_val      DOUBLE PRECISION,
  max_val      DOUBLE PRECISION,
  p95_val      DOUBLE PRECISION,
  labels       JSONB DEFAULT '{}'::jsonb
);
SELECT create_hypertable('metric_1m','bucket',chunk_time_interval => interval '7 days');
CREATE INDEX idx_metric_key ON metric_1m(resource_id, metric, bucket DESC);
CREATE INDEX idx_metric_labels ON metric_1m USING GIN(labels);
ALTER TABLE metric_1m SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'resource_id, metric',
  timescaledb.compress_orderby   = 'bucket'
);
SELECT add_compression_policy('metric_1m', INTERVAL '7 days');
SELECT add_retention_policy   ('metric_1m', INTERVAL '180 days');

-- 4) Service level call aggregates (5m, Hypertable)
CREATE TABLE service_call_5m (
  bucket          TIMESTAMPTZ NOT NULL,
  tenant_id       BIGINT REFERENCES dim_tenant(tenant_id),
  src_resource_id BIGINT REFERENCES dim_resource(resource_id),
  dst_resource_id BIGINT REFERENCES dim_resource(resource_id),
  rps             DOUBLE PRECISION,
  err_rate        DOUBLE PRECISION,
  p50_ms          DOUBLE PRECISION,
  p95_ms          DOUBLE PRECISION,
  sample_ref      BIGINT REFERENCES oo_locator(id),
  PRIMARY KEY(bucket, tenant_id, src_resource_id, dst_resource_id)
);
SELECT create_hypertable('service_call_5m','bucket',chunk_time_interval => interval '30 days');
CREATE INDEX idx_call_src_dst ON service_call_5m(src_resource_id, dst_resource_id, bucket DESC);
SELECT add_retention_policy('service_call_5m', INTERVAL '365 days');

-- 5) Log fingerprint and 5m counts (2)
CREATE TABLE log_pattern (
  fingerprint_id  BIGSERIAL PRIMARY KEY,
  tenant_id       BIGINT REFERENCES dim_tenant(tenant_id),
  pattern         TEXT NOT NULL,
  sample_message  TEXT,
  severity        TEXT,
  attrs_schema    JSONB DEFAULT '{}'::jsonb,
  first_seen      TIMESTAMPTZ,
  last_seen       TIMESTAMPTZ
);
CREATE INDEX idx_logpat_tenant ON log_pattern(tenant_id);
CREATE INDEX idx_logpat_pattern_trgm ON log_pattern USING GIN (pattern gin_trgm_ops);

CREATE TABLE log_pattern_5m (
  bucket         TIMESTAMPTZ NOT NULL,
  tenant_id      BIGINT REFERENCES dim_tenant(tenant_id),
  resource_id    BIGINT REFERENCES dim_resource(resource_id),
  fingerprint_id BIGINT REFERENCES log_pattern(fingerprint_id),
  count_total    BIGINT NOT NULL,
  count_error    BIGINT NOT NULL DEFAULT 0,
  sample_ref     BIGINT REFERENCES oo_locator(id),
  PRIMARY KEY(bucket, tenant_id, resource_id, fingerprint_id)
);
SELECT create_hypertable('log_pattern_5m','bucket',chunk_time_interval => interval '30 days');
CREATE INDEX idx_logpat5m_res ON log_pattern_5m(resource_id, bucket DESC);
SELECT add_retention_policy('log_pattern_5m', INTERVAL '180 days');

-- 6) Temporal topology (1)
CREATE TABLE topo_edge_time (
  tenant_id       BIGINT REFERENCES dim_tenant(tenant_id),
  src_resource_id BIGINT REFERENCES dim_resource(resource_id),
  dst_resource_id BIGINT REFERENCES dim_resource(resource_id),
  relation        TEXT NOT NULL,
  valid           tstzrange NOT NULL,  -- [from, to)
  props           JSONB DEFAULT '{}'::jsonb,
  PRIMARY KEY(tenant_id, src_resource_id, dst_resource_id, relation, valid)
);
CREATE INDEX idx_topo_valid ON topo_edge_time USING GIST (tenant_id, src_resource_id, dst_resource_id, valid);

-- 7) Knowledge base / vector (2)
CREATE TABLE kb_doc (
  doc_id     BIGSERIAL PRIMARY KEY,
  tenant_id  BIGINT REFERENCES dim_tenant(tenant_id),
  source     TEXT,
  title      TEXT,
  url        TEXT,
  metadata   JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE kb_chunk (
  chunk_id   BIGSERIAL PRIMARY KEY,
  doc_id     BIGINT REFERENCES kb_doc(doc_id) ON DELETE CASCADE,
  chunk_idx  INT NOT NULL,
  content    TEXT NOT NULL,
  embedding  vector(1536) NOT NULL,
  metadata   JSONB DEFAULT '{}'::jsonb
);
CREATE INDEX idx_kb_chunk_doc ON kb_chunk(doc_id, chunk_idx);
CREATE INDEX idx_kb_chunk_meta ON kb_chunk USING GIN(metadata);
CREATE INDEX idx_kb_vec_hnsw ON kb_chunk USING hnsw (embedding vector_l2_ops);

-- 8) Events & evidence chain (2)
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'severity') THEN
    CREATE TYPE severity AS ENUM ('TRACE','DEBUG','INFO','WARN','ERROR','FATAL');
  END IF;
END $$;

CREATE TABLE event_envelope (
  event_id     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  detected_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  tenant_id    BIGINT REFERENCES dim_tenant(tenant_id),
  resource_id  BIGINT REFERENCES dim_resource(resource_id),
  severity     severity NOT NULL,
  kind         TEXT NOT NULL,     -- anomaly/slo_violation/deploy/incident/...
  title        TEXT,
  summary      TEXT,
  labels       JSONB DEFAULT '{}'::jsonb,
  fingerprints JSONB DEFAULT '{}'::jsonb
);
CREATE INDEX idx_event_time ON event_envelope(tenant_id, detected_at DESC);

-- Note: primary key cannot use expression, switch to serial + unique index (ref_pg hash + ref_oo)
CREATE TABLE evidence_link (
  evidence_id BIGSERIAL PRIMARY KEY,
  event_id    UUID NOT NULL REFERENCES event_envelope(event_id) ON DELETE CASCADE,
  dim         TEXT NOT NULL,  -- metric/log/trace/topo/kb
  ref_pg      JSONB,          -- {"table":"...","keys":{...}}
  ref_oo      BIGINT REFERENCES oo_locator(id),
  note        TEXT,
  ref_pg_hash TEXT GENERATED ALWAYS AS (md5(coalesce(ref_pg::text, ''))) STORED
);
CREATE UNIQUE INDEX ux_evidence_unique
  ON evidence_link(event_id, dim, ref_pg_hash, coalesce(ref_oo, 0));
CREATE INDEX idx_evidence_event ON evidence_link(event_id);

-- AGE graph: initialization (once)
SELECT * FROM create_graph('ops');
SELECT * FROM create_vlabel('ops', 'Resource');
SELECT * FROM create_elabel('ops', 'CALLS');

