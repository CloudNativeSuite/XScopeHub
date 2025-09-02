-- Base extensions and severity enum are created in db/schema.sql

CREATE TABLE IF NOT EXISTS ops_case (
  case_id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id  BIGINT NOT NULL,
  title      TEXT NOT NULL,
  severity   severity NOT NULL DEFAULT 'INFO',
  status     TEXT NOT NULL,
  resource_id BIGINT,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now(),
  labels     JSONB DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_case_tenant_time ON ops_case(tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS outbox (
  id           BIGSERIAL PRIMARY KEY,
  aggregate    TEXT,
  aggregate_id TEXT,
  topic        TEXT,
  payload      JSONB NOT NULL,
  created_at   TIMESTAMPTZ DEFAULT now(),
  published    BOOLEAN DEFAULT FALSE,
  published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_outbox_unpub ON outbox(published) WHERE published = FALSE;

CREATE TABLE IF NOT EXISTS idempotency (
  idem_key   TEXT PRIMARY KEY,
  request    JSONB,
  response   JSONB,
  created_at TIMESTAMPTZ DEFAULT now(),
  ttl        TIMESTAMPTZ
);
