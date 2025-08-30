# Dual-Engine Storage Design

This document outlines a minimal PostgreSQL schema to support the "Five Dimensions One Map" concept using **OpenObserve + PostgreSQL + TimescaleDB + Apache AGE + pgvector**.

## Overview

OpenObserve handles ingestion of observability data (metrics, logs, traces) and routes persistence to PostgreSQL. PostgreSQL acts as a unified store extended with:

- **TimescaleDB** for efficient time-series storage.
- **Apache AGE** for graph-based topology modeling.
- **pgvector** for similarity search over knowledge-base embeddings.

The goal is to keep tables small yet performant and extensible.

## Table Blueprint

### 1. Metrics (Time Series)
```sql
CREATE TABLE metrics (
  time        TIMESTAMPTZ NOT NULL,
  metric      TEXT        NOT NULL,
  labels      JSONB,
  value       DOUBLE PRECISION,
  PRIMARY KEY (time, metric)
);
SELECT create_hypertable('metrics', 'time');
CREATE INDEX ON metrics (metric, time DESC);
CREATE INDEX ON metrics USING GIN (labels);
```

### 2. Logs
```sql
CREATE TABLE logs (
  time     TIMESTAMPTZ NOT NULL,
  source   TEXT,
  level    TEXT,
  message  TEXT,
  fields   JSONB
);
SELECT create_hypertable('logs', 'time');
CREATE INDEX ON logs (source, time DESC);
CREATE INDEX ON logs USING GIN (fields);
```

### 3. Traces
```sql
CREATE TABLE traces (
  trace_id    UUID,
  span_id     UUID,
  parent_id   UUID,
  service     TEXT,
  name        TEXT,
  start_time  TIMESTAMPTZ,
  end_time    TIMESTAMPTZ,
  attributes  JSONB
);
SELECT create_hypertable('traces', 'start_time');
CREATE INDEX ON traces (trace_id);
CREATE INDEX ON traces (service, start_time DESC);
```

### 4. Topology (Graph via AGE)
```sql
SELECT create_graph('topology');
CREATE VLABEL topology.resource;
CREATE ELABEL topology.link;
-- Example usage: (:resource)-[:link {role:'depends_on'}]->(:resource)
```
The graph captures relationships between resources such as services, hosts, or containers.

### 5. Knowledge Base / Vectors
```sql
CREATE TABLE kb_docs (
  id        BIGSERIAL PRIMARY KEY,
  content   TEXT,
  embedding VECTOR(768),
  metadata  JSONB
);
CREATE INDEX ON kb_docs USING ivfflat (embedding vector_l2_ops);
```
Embeddings enable semantic search across documents, runbooks, or past incidents.

## Extensibility and Performance

- **Minimal schema**: core columns only; additional attributes stored in `JSONB` to avoid over-design.
- **Indexes**: btree indexes for common queries; GIN/IVFFLAT for JSON and vector searches.
- **TimescaleDB hypertables**: automatic partitioning and compression for time-series data.
- **Graph & vectors**: AGE and pgvector keep topology and knowledge-base features decoupled yet queryable inside PostgreSQL.

This setup forms a unified evidence framework for AIOps while remaining lightweight and scalable.

## Control-Plane Tables

The API specification adds a set of auxiliary tables in PostgreSQL to
track remediation workflow state. Key tables include:

- `ops_case` and `case_timeline` for case management.
- `plan_proposed` / `plan_approved` and `gate_decision` for planning and
  policy evaluation.
- `exec_run`, `exec_step` and `verify_checkpoint` for execution and
  verification.
- `outbox` and `idempotency` to ensure reliable messaging and
  idempotent API semantics.

These tables complement the observability stores above, providing a
cohesive backbone for Sensor → Orchestrator modules.
