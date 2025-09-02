ALTER TABLE ops_case ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS case_timeline (
  id        BIGSERIAL PRIMARY KEY,
  case_id   UUID REFERENCES ops_case(case_id) ON DELETE CASCADE,
  ts        TIMESTAMPTZ DEFAULT now(),
  actor     TEXT,
  event     TEXT,
  payload   JSONB
);

CREATE INDEX IF NOT EXISTS idx_case_tl_case_time ON case_timeline(case_id, ts DESC);
