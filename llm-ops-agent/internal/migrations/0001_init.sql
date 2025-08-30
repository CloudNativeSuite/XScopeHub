-- Enable TimescaleDB
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- Raw points (simple demo schema)
CREATE TABLE IF NOT EXISTS metrics_point (
  ts        TIMESTAMPTZ NOT NULL,
  metric    TEXT NOT NULL,                  -- e.g. http_req_latency
  service   TEXT,
  labels    JSONB DEFAULT '{}'::jsonb,
  value     DOUBLE PRECISION NOT NULL
);
SELECT create_hypertable('metrics_point', 'ts', if_not_exists => TRUE);

-- Continuous aggregate (1-minute buckets)
DROP MATERIALIZED VIEW IF EXISTS metrics_1m CASCADE;
CREATE MATERIALIZED VIEW metrics_1m
WITH (timescaledb.continuous) AS
SELECT time_bucket('1 minute', ts) AS tb,
       metric,
       service,
       avg(value) AS avg_val,
       max(value) AS max_val
FROM metrics_point
GROUP BY tb, metric, service
WITH NO DATA;

-- Policy to refresh last 2h every 1m
SELECT add_continuous_aggregate_policy('metrics_1m',
    start_offset => INTERVAL '2 hours',
    end_offset   => INTERVAL '1 minute',
    schedule_interval => INTERVAL '1 minute');

-- TopK per minute for latency
DROP MATERIALIZED VIEW IF EXISTS hot_services_1m CASCADE;
CREATE MATERIALIZED VIEW hot_services_1m
WITH (timescaledb.continuous) AS
SELECT tb, service, avg_val,
       ROW_NUMBER() OVER (PARTITION BY tb ORDER BY avg_val DESC) AS rk
FROM (
  SELECT time_bucket('1 minute', ts) AS tb,
         service,
         avg(value) AS avg_val
  FROM metrics_point
  WHERE metric = 'http_req_latency'
  GROUP BY tb, service
) t
WITH NO DATA;

SELECT add_continuous_aggregate_policy('hot_services_1m',
    start_offset => INTERVAL '2 hours',
    end_offset   => INTERVAL '1 minute',
    schedule_interval => INTERVAL '1 minute');

-- Incidents & Actions audit
CREATE TABLE IF NOT EXISTS incidents (
  id BIGSERIAL PRIMARY KEY,
  fingerprint TEXT UNIQUE,
  service TEXT,
  status TEXT,          -- open/mitigating/resolved
  sev INT,
  hypothesis JSONB,
  plan JSONB,
  created_at TIMESTAMPTZ DEFAULT now(),
  closed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS actions_audit (
  id BIGSERIAL PRIMARY KEY,
  incident_id BIGINT REFERENCES incidents(id),
  step INT,
  action TEXT,
  params JSONB,
  dryrun BOOL DEFAULT TRUE,
  result JSONB,
  ts TIMESTAMPTZ DEFAULT now()
);

-- Demo helper: seed 20 minutes of synthetic latency for a service
CREATE OR REPLACE FUNCTION seed_latency(service_name TEXT, base_ms DOUBLE PRECISION, jitter DOUBLE PRECISION)
RETURNS VOID AS $$
DECLARE
  t TIMESTAMPTZ := now() - interval '20 minutes';
BEGIN
  WHILE t < now() LOOP
    INSERT INTO metrics_point(ts, metric, service, value)
    VALUES (t, 'http_req_latency', service_name, base_ms + (random()-0.5)*jitter);
    t := t + interval '10 seconds';
  END LOOP;
END;
$$ LANGUAGE plpgsql;

-- Demo helper: recent average latency
CREATE OR REPLACE FUNCTION recent_latency_avg(service_name TEXT, minutes INT)
RETURNS DOUBLE PRECISION AS $$
DECLARE
  v DOUBLE PRECISION;
BEGIN
  SELECT avg(avg_val) INTO v
  FROM metrics_1m
  WHERE service = service_name AND metric='http_req_latency'
    AND tb >= now() - (minutes::text || ' minutes')::interval;
  RETURN v;
END;
$$ LANGUAGE plpgsql;

-- Seed latency samples for demos
CREATE OR REPLACE FUNCTION seed_latency(svc TEXT, base INT, count INT) RETURNS VOID AS $$
DECLARE
    i INT;
BEGIN
    FOR i IN 1..count LOOP
        INSERT INTO metrics(time, name, value)
        VALUES (now() - (count - i || ' seconds')::interval, svc, base + random()*50);
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- Return true if last 5 min average improved >=10% vs previous window
CREATE OR REPLACE FUNCTION recent_latency_improved(svc TEXT) RETURNS BOOLEAN AS $$
DECLARE
    prev_avg FLOAT;
    curr_avg FLOAT;
BEGIN
    SELECT COALESCE(avg(avg_value),0) INTO prev_avg
      FROM metrics_1m
     WHERE name=svc AND bucket > now() - interval '10 minutes'
       AND bucket <= now() - interval '5 minutes';

    SELECT COALESCE(avg(avg_value),0) INTO curr_avg
      FROM metrics_1m
     WHERE name=svc AND bucket > now() - interval '5 minutes';

    IF prev_avg = 0 THEN
        RETURN FALSE;
    END IF;
    RETURN (prev_avg - curr_avg) / prev_avg >= 0.1;
END;
$$ LANGUAGE plpgsql;
