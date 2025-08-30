
A compact Go-based orchestration and ETL stack that turns raw observability data (OpenObserve) into queryable aggregates (Timescale/Postgres), an active service-call graph (AGE), and time-aware topology from IaC/Ansible.

1. Overview

Goals

OO→PG (nearline aggregates): logs/metrics/traces → metric_1m, service_call_5m, log_pattern_5m, with log_pattern dictionary and oo_locator evidence.

Active call graph (AGE): maintain a 10-minute active (:Resource)-[:CALLS]->(:Resource) graph based on service_call_5m.

Topology (IaC/Ansible): discover & time-version structural and app dependencies into topo_edge_time using tstzrange intervals.

Orchestration

Single Go binary: window-aligned scheduling, DAG dependencies, idempotent upserts, multi-tenant sharding, event backfills.

Persistence of job runs in PG (etl_job_run) for exactly-once per (job, tenant, window).

2. Project Layout (merged)

├─ cmd/etl/                   # CLI entrypoint
│  └─ main.go
├─ pkg/
│  ├─ scheduler/              # window calc / enqueue
│  ├─ runner/                 # worker pool / retries / DAG fanout
│  ├─ registry/               # Job interface + registry
│  ├─ store/                  # PG queue + run-state + once semantics
│  ├─ window/                 # time alignment helpers
│  ├─ events/                 # HTTP/CloudEvents enqueue
│  ├─ oo/                     # OpenObserve readers (S3/API)
│  ├─ agg/                    # 1m/5m aggregators
│  ├─ patterns/               # log template mining (Drain/RE2)
│  ├─ iac/                    # IaC/Cloud discovery
│  ├─ ansible/                # inventory/roles dependency extraction
│  └─ pgw/                    # Postgres writer (COPY + upserts)
├─ jobs/
│  ├─ ooagg.go                # OO→PG nearline aggregates
│  ├─ age_refresh.go          # active CALLS graph (AGE)
│  ├─ topo_iac.go             # infra topology
│  └─ topo_ansible.go         # app dependency topology
├─ sql/
│  └─ age_refresh.sql         # AGE CALLS refresh
└─ configs/
   └─ etl.yaml                # scheduling & lookback config

3. Data Tables (consolidated)
Domain	Table	Purpose (granularity)	Key/Index Highlights
Dim	dim_tenant	tenants/domains	code unique
Dim	dim_resource	normalized URN per resource	urn unique; type, labels GIN
Locator	oo_locator	backref to OO object + time window	time index; unique composite recommended
Metrics	metric_1m	1m aggregates: avg/max/p95	hypertable on bucket; PK (bucket,tenant,resource,metric)
Calls	service_call_5m	5m A→B rps/err/p50/p95	hypertable; PK (bucket,tenant,src,dst)
Logs	log_pattern	template dictionary	tenant index; pattern trigram GIN
Logs	log_pattern_5m	5m counts per fingerprint/resource	hypertable; PK (bucket,tenant,resource,fingerprint)
Topo	topo_edge_time	temporal edges with tstzrange valid	btree_gist/GIST on range
KB	kb_doc / kb_chunk	runbooks & embeddings	HNSW on embedding
Events	event_envelope	event envelope	time desc index
Events	evidence_link	polymorphic links to PG/OO	unique on (event,dim,ref_pg_hash,coalesce(ref_oo,0))
ETL	etl_job_run	run ledger (once)	unique (job,tenant,window_from,window_to)
4. Pipelines
4.1 OO→PG (Nearline)

Align 1m, Delay 2m, Interval 1m.

Read OO (S3/API) → normalize URN → in-memory 1m/5m aggregation:

metric_1m: avg/max/p95 per (tenant, resource, metric, 1m).

service_call_5m: A→B rps/err/p50/p95 per 5m.

log_pattern_5m: fingerprint counts/errors per 5m, powered by patterns.

oo_locator: one (or few) per window with query hints.

Upsert via pgw.Flush (COPY batches + ON CONFLICT DO UPDATE).

4.2 Active CALLS Graph (AGE)

Align 5m, After oo-agg.

Refresh :CALLS edges using last 10 minutes of service_call_5m (SQL in sql/age_refresh.sql).

Edge props: last_seen, rps, err_rate, p95.

4.3 Topology from IaC & Ansible

IaC (15m): discover infra edges (LB→svc, node→pod, etc.) and upsert temporal edges in topo_edge_time.

Ansible (1h): parse inventory/group_vars/roles to extract DEPENDS_ON edges for app deps.

Temporal diffing: open interval on new edges, close interval on disappeared edges.

5. Scheduling & Orchestration

Exactly-once per (job,tenant,window) using etl_job_run unique index.

Window upper bound: floor(now - Delay, Align).

DAG: age-refresh depends on oo-agg.

Event backfill: POST /events/enqueue creates queued runs for arbitrary windows.

Retry: exponential backoff (2s → … → 5m cap). Optional circuit breaker on repeated failures.

Job interface

type Job interface {
  Name() string
  Interval() time.Duration   // 0 = event-driven only
  Align() time.Duration
  Delay() time.Duration
  Concurrency() int
  After() []string           // upstream job names
  Run(ctx context.Context, tenantID int64, w Window, args map[string]any) error
}

6. Module APIs (selected)

OO reader

type RecordKind string // "metric" | "log" | "trace"
type Record struct {
  Kind RecordKind
  Time time.Time       // UTC
  URN  string          // normalized
  Attrs map[string]any // value/labels or span/log fields
}
func Stream(ctx context.Context, tenant int64, w Window, fn func(Record) error) error


Aggregator

func Reset()
func Feed(rec oo.Record)
type Out struct { Metrics1m []MetricRow; Calls5m []CallRow; LogPatterns5m []LogPatRow; PatternsUpsert []PatternRow; Locators []LocatorRow }
func Drain() Out


PG writer

func Flush(ctx context.Context, tenant int64, w Window, out agg.Out) error
func UpsertTopoEdges(ctx context.Context, tenant int64, edges []iac.Edge) error

7. Data Contracts

Time inclusion: record belongs to window if From ≤ ts < To.

Resource identity: urn:* unique per resource; dim_resource(urn) upserted.

Idempotency: every target table keyed to enable ON CONFLICT DO UPDATE.

Locators: typically 1 per window/dataset; unique composite prevents duplicates.

8. Idempotency & Consistency

Single-writer per (job,tenant,window) via INSERT … ON CONFLICT DO NOTHING.

Agg tables use deterministic recalculation; replays overwrite same primary keys.

Temporal topology uses set-diff to end open intervals and start new ones.

9. Performance Targets

Single tenant, typical window:

1m window: ingest ~5k logs, 500 metrics, 1k spans in <3s end-to-end.

Memory peak per worker <200MB; CPU < 1 core.

COPY batch size: 1–5k rows; commit per 5–10s or lower.

Indices: time-desc compound + GIN(labels); HNSW for KB.

10. Security & Tenancy

Credentials via env/secret (PG_URL, OO_*).

Optional PG RLS by tenant_id.

Mask sensitive label/metadata fields on write.

11. Configuration

configs/etl.yaml

tenants:
  initial_lookback:
    oo-agg:       "24h"
    topo-iac:     "48h"
    topo-ansible: "168h"
jobs:
  oo-agg: { delay: "2m", align: "1m", interval: "1m", concurrency: 2 }
  age-refresh: { interval: "0s" }   # dependency-driven
  topo-iac: { interval: "15m" }
  topo-ansible: { interval: "1h" }

12. Bootstrap (DDL)

Use sql/bootstrap.sql to create extensions, 12 core tables, AGE graph labels, and ETL tables.

Seed dim_tenant(code='default').

13. Runbook (ops)

Start: PG_URL=... ETL_CONFIG=configs/etl.yaml ./bin/etl.

Backfill: POST /events/enqueue with {job,tenant_id,from,to}.

Health:

etl_job_run.status transitions.

Window lag ≈ Delay + Align.

Error budget: <1% failed runs/day, auto-retry ≤3.

14. Testing Strategy

Unit: agg statistics; patterns fingerprint stability.

Integration: mock oo.Stream → pgw.Flush into test PG; verify PK conflicts & upserts.

Perf soak: 10 windows * (5k logs + 1k spans + 500 metrics), steady throughput, memory cap.

Topology diff: same edge set twice (no dup); remove an edge then re-run (interval closes).

15. Codex Tasks

Use these as atomic “create-or-update” tasks for your codegen agent.

T1 — Implement pgw.Flush (COPY + upserts)

Desc: Batch COPY metric_1m, service_call_5m, log_pattern, log_pattern_5m; upsert dim_resource, insert oo_locator and wire sample_ref.

Test: Replay same window twice → row count unchanged; aggregates stable. Conflict rate <5%.

T2 — Execute sql/age_refresh.sql from jobs/age_refresh.go

Desc: Load SQL, bind rows per-tenant, execute. Ensure MERGE semantics in AGE.

Test: Insert sample service_call_5m; run job twice → edge count stable; props updated.

T3 — oo.Stream mock + S3/API adapters

Desc: Provide --mock generator; implement S3 partition walk and time-filtered reads; OO API query fallback.

Test: Mock: 1m window produces expected record counts; S3: list & filter by window correctly.

T4 — agg statistics

Desc: Compute 1m avg/max/p95 (metrics), 5m rps/err/p50/p95 (calls), 5m counts (logs).

Test: Deterministic sample series → expected quantiles; A→B grouping correctness.

T5 — patterns fingerprint mining

Desc: Drain/RE2-based templating; severity inference; configurable ignore tokens.

Test: Same template different vars → same fingerprint; error lines counted in count_error.

T6 — UpsertTopoEdges temporal diff

Desc: Set-diff against open intervals; insert opens, close vanished.

Test: Two identical runs → no new rows; remove edge → upper bound set to now().

T7 — Events backfill endpoint

Desc: POST /events/enqueue enqueues windows; respects Delay upper bound.

Test: Backfill historical windows execute; future windows stay queued until eligible.

T8 — Config-driven initial lookback

Desc: Use tenants.initial_lookback when no prior success window.

Test: Clean slate → start from configured lookback; modify config & reload (optional).

T9 — Metrics/observability

Desc: Export counters and histograms (ingested records, batch time, failures, lag).

Test: Under steady load window lag ≈ Delay + Align; alerts on repeated failures.

16. Mermaid Overview
flowchart LR
  subgraph Ingest
    OO[OpenObserve (S3/API)]
  end
  subgraph ETL
    R[oo.Stream]
    A[agg 1m/5m]
    W[pgw.Flush]
  end
  subgraph PG
    M[metric_1m]
    C[service_call_5m]
    L[log_pattern_5m]
    D[log_pattern]
    O[oo_locator]
    T[topo_edge_time]
  end
  subgraph AGE
    G[(:Resource)-[:CALLS]->(:Resource)]
  end
  OO --> R --> A --> W
  W --> M
  W --> C
  W --> L
  W --> D
  W --> O
  C -->|10m active| G


Name: Observability ETL Suite
Scope: OO→PG aggregates, AGE active graph, IaC/Ansible topology — all orchestrated in one Go binary with windowed, idempotent ETL runs.
