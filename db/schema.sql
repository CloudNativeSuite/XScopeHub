-- PostgreSQL schema initialization for OPSAgent
-- Generated according to PG data model overview.

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'timescaledb') THEN
    CREATE EXTENSION IF NOT EXISTS timescaledb;
  ELSE
    RAISE NOTICE 'timescaledb extension is not available, skipping.';
  END IF;
END $$;

CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- gen_random_uuid
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'vector') THEN
    CREATE EXTENSION IF NOT EXISTS vector;
  ELSE
    RAISE NOTICE 'vector extension is not available, skipping.';
  END IF;
END $$;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'age') THEN
    CREATE EXTENSION IF NOT EXISTS age;       -- graph extension for service call graph
    BEGIN
      EXECUTE 'LOAD ''age''';
    EXCEPTION WHEN OTHERS THEN
      RAISE NOTICE 'age library could not be loaded, skipping.';
    END;
  ELSE
    RAISE NOTICE 'age extension is not available, skipping.';
  END IF;
END $$;

CREATE EXTENSION IF NOT EXISTS btree_gist; -- temporal topology range index

CREATE TABLE IF NOT EXISTS dim_tenant (
  tenant_id   BIGSERIAL PRIMARY KEY,
  code        TEXT UNIQUE NOT NULL,
  name        TEXT NOT NULL,
  labels      JSONB DEFAULT '{}'::jsonb,
  created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS dim_resource (
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
CREATE INDEX IF NOT EXISTS idx_res_type ON dim_resource(type);
CREATE INDEX IF NOT EXISTS idx_res_labels_gin ON dim_resource USING GIN(labels);

CREATE TABLE IF NOT EXISTS oo_locator (
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
CREATE INDEX IF NOT EXISTS idx_oo_time ON oo_locator(dataset, t_from, t_to);

CREATE TABLE IF NOT EXISTS metric_1m (
  bucket       TIMESTAMPTZ NOT NULL,
  tenant_id    BIGINT REFERENCES dim_tenant(tenant_id),
  resource_id  BIGINT REFERENCES dim_resource(resource_id),
  metric       TEXT NOT NULL,
  avg_val      DOUBLE PRECISION,
  max_val      DOUBLE PRECISION,
  p95_val      DOUBLE PRECISION,
  labels       JSONB DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_metric_key ON metric_1m(resource_id, metric, bucket DESC);
CREATE INDEX IF NOT EXISTS idx_metric_labels ON metric_1m USING GIN(labels);
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
    PERFORM create_hypertable('metric_1m','bucket',chunk_time_interval => interval '7 days');
    EXECUTE 'ALTER TABLE metric_1m SET (
      timescaledb.compress,
      timescaledb.compress_segmentby = ''resource_id, metric'',
      timescaledb.compress_orderby   = ''bucket''
    )';
    PERFORM add_compression_policy('metric_1m', INTERVAL '7 days');
    PERFORM add_retention_policy   ('metric_1m', INTERVAL '180 days');
  ELSE
    RAISE NOTICE 'timescaledb extension not installed; skipping hypertable and policies for metric_1m.';
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS service_call_5m (
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
CREATE INDEX IF NOT EXISTS idx_call_src_dst ON service_call_5m(src_resource_id, dst_resource_id, bucket DESC);
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
    PERFORM create_hypertable('service_call_5m','bucket',chunk_time_interval => interval '30 days');
    PERFORM add_retention_policy('service_call_5m', INTERVAL '365 days');
  ELSE
    RAISE NOTICE 'timescaledb extension not installed; skipping hypertable and policies for service_call_5m.';
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS log_pattern (
  fingerprint_id  BIGSERIAL PRIMARY KEY,
  tenant_id       BIGINT REFERENCES dim_tenant(tenant_id),
  pattern         TEXT NOT NULL,
  sample_message  TEXT,
  severity        TEXT,
  attrs_schema    JSONB DEFAULT '{}'::jsonb,
  first_seen      TIMESTAMPTZ,
  last_seen       TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_logpat_tenant ON log_pattern(tenant_id);
CREATE INDEX IF NOT EXISTS idx_logpat_pattern_trgm ON log_pattern USING GIN (pattern gin_trgm_ops);

CREATE TABLE IF NOT EXISTS log_pattern_5m (
  bucket         TIMESTAMPTZ NOT NULL,
  tenant_id      BIGINT REFERENCES dim_tenant(tenant_id),
  resource_id    BIGINT REFERENCES dim_resource(resource_id),
  fingerprint_id BIGINT REFERENCES log_pattern(fingerprint_id),
  count_total    BIGINT NOT NULL,
  count_error    BIGINT NOT NULL DEFAULT 0,
  sample_ref     BIGINT REFERENCES oo_locator(id),
  PRIMARY KEY(bucket, tenant_id, resource_id, fingerprint_id)
);
CREATE INDEX IF NOT EXISTS idx_logpat5m_res ON log_pattern_5m(resource_id, bucket DESC);
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
    PERFORM create_hypertable('log_pattern_5m','bucket',chunk_time_interval => interval '30 days');
    PERFORM add_retention_policy('log_pattern_5m', INTERVAL '180 days');
  ELSE
    RAISE NOTICE 'timescaledb extension not installed; skipping hypertable and policies for log_pattern_5m.';
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS topo_edge_time (
  tenant_id       BIGINT REFERENCES dim_tenant(tenant_id),
  src_resource_id BIGINT REFERENCES dim_resource(resource_id),
  dst_resource_id BIGINT REFERENCES dim_resource(resource_id),
  relation        TEXT NOT NULL,
  valid           tstzrange NOT NULL,  -- [from, to)
  props           JSONB DEFAULT '{}'::jsonb,
  PRIMARY KEY(tenant_id, src_resource_id, dst_resource_id, relation, valid)
);
CREATE INDEX IF NOT EXISTS idx_topo_valid ON topo_edge_time USING GIST (tenant_id, src_resource_id, dst_resource_id, valid);

CREATE TABLE IF NOT EXISTS kb_doc (
  doc_id     BIGSERIAL PRIMARY KEY,
  tenant_id  BIGINT REFERENCES dim_tenant(tenant_id),
  source     TEXT,
  title      TEXT,
  url        TEXT,
  metadata   JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ DEFAULT now()
);

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector') THEN
    CREATE TABLE IF NOT EXISTS kb_chunk (
      chunk_id   BIGSERIAL PRIMARY KEY,
      doc_id     BIGINT REFERENCES kb_doc(doc_id) ON DELETE CASCADE,
      chunk_idx  INT NOT NULL,
      content    TEXT NOT NULL,
      embedding  vector(1536) NOT NULL,
      metadata   JSONB DEFAULT '{}'::jsonb
    );
    CREATE INDEX IF NOT EXISTS idx_kb_chunk_doc ON kb_chunk(doc_id, chunk_idx);
    CREATE INDEX IF NOT EXISTS idx_kb_chunk_meta ON kb_chunk USING GIN(metadata);
    CREATE INDEX IF NOT EXISTS idx_kb_vec_hnsw ON kb_chunk USING hnsw (embedding vector_l2_ops);
  ELSE
    RAISE NOTICE 'vector extension not installed; skipping kb_chunk table.';
  END IF;
END $$;

-- 8) Events & evidence chain (2)
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'severity') THEN
    CREATE TYPE severity AS ENUM ('TRACE','DEBUG','INFO','WARN','ERROR','FATAL');
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS event_envelope (
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
CREATE INDEX IF NOT EXISTS idx_event_time ON event_envelope(tenant_id, detected_at DESC);

-- Note: primary key cannot use expression, switch to serial + unique index (ref_pg hash + ref_oo)
CREATE TABLE IF NOT EXISTS evidence_link (
  evidence_id BIGSERIAL PRIMARY KEY,
  event_id    UUID NOT NULL REFERENCES event_envelope(event_id) ON DELETE CASCADE,
  dim         TEXT NOT NULL,  -- metric/log/trace/topo/kb
  ref_pg      JSONB,          -- {"table":"...","keys":{...}}
  ref_oo      BIGINT REFERENCES oo_locator(id),
  note        TEXT,
  ref_pg_hash TEXT GENERATED ALWAYS AS (md5(coalesce(ref_pg::text, ''))) STORED
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_evidence_unique
  ON evidence_link(event_id, dim, ref_pg_hash, coalesce(ref_oo, 0));
CREATE INDEX IF NOT EXISTS idx_evidence_event ON evidence_link(event_id);

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'age') THEN
    PERFORM create_graph('ops');
    PERFORM create_vlabel('ops', 'Resource');
    PERFORM create_elabel('ops', 'CALLS');
  ELSE
    RAISE NOTICE 'age extension not installed; skipping graph initialization.';
  END IF;
END $$;

