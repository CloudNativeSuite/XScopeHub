# XScopeHub Full-Stack Observability Design

## Overview

XScopeHub already ships the classical observability triad by funnelling logs, metrics, and traces from edge agents into OpenObserve through the `observe-gateway` and down-stream ETL services described in the existing architecture notes.【F:docs/architecture.md†L7-L59】  This document extends that baseline with a topology layer and a context/intelligence layer so that the platform can reason about relationships (service ↔ infrastructure ↔ user) and high-level knowledge (alerts, incidents, runbooks, change records) in addition to raw telemetry.

The intent of this design is twofold:

1. Capture the current code capabilities and extension points that make the three core signals available.
2. Specify how topology and knowledge resources should be modelled, stored, and surfaced via the MCP server so that downstream agents gain a full-site view.

## Signal Pillars (Logs / Metrics / Traces)

### Gateway responsibilities

The `observe-gateway` service already provides unified query entry points for PromQL, LogQL, and TraceQL. Prometheus-style queries are executed via OpenObserve with optional fallbacks, while logs and traces are translated into SQL before dispatching to OpenObserve search endpoints.【F:observe-gateway/internal/backend/client.go†L20-L118】  Tenant-aware metadata lookups decide which tables and organisations to target for each call.【F:observe-gateway/internal/backend/metadata.go†L16-L88】  Configuration for auth, rate limiting, caching, auditing, and backend endpoints is centralised in `internal/config`, ensuring ingestion/query flows can be tuned independently per tenant.【F:observe-gateway/internal/config/config.go†L11-L123】

### Persistent storage & analytics

The ETL workloads inside `observe-bridge` are designed to hydrate PostgreSQL (Timescale, pgvector, AGE) and ClickHouse so that raw OTLP payloads become structured aggregates, graph relationships, and semantic objects as documented in the architecture guide.【F:docs/architecture.md†L61-L156】  The analytics packages in `internal/analytics` already expose primitives for interacting with AGE graphs (service dependencies) and pgvector-backed semantic objects, which we will reuse for higher layers.【F:observe-bridge/internal/analytics/graph/graph.go†L1-L27】【F:observe-bridge/internal/analytics/vector/vector.go†L8-L79】

## Topology Layer

### Goals

* Maintain continuously refreshed graphs describing how services, containers, networks, and infrastructure elements interact.
* Provide topology snapshots and historical deltas that can be cross-linked with telemetry (trace IDs, host IDs, deployment hashes).

### Data model

We extend the AGE-backed `Service` graph with additional vertex/edge labels representing deployment units (`Deployment`), runtime containers (`Pod`, `Node`), network artefacts (`Network`, `Flow`), and infrastructure (`Subnet`, `Zone`, `Account`). Each edge records the relationship type (e.g., `DEPLOYED_ON`, `ROUTES_TO`, `BELONGS_TO`) plus the time window when it was observed. The existing DAO already merges `Service` nodes; we will introduce helper methods for the other node types and for directional edges that include timestamps and metadata payloads.

### Sources & ingestion

* **Deploy / Service topology** – ingest from GitOps manifests and Kubernetes APIs. The ETL jobs in `observe-bridge` can watch the Git/Ansible inputs listed in the architecture diagram and translate them into graph updates with `MERGE`/`SET` semantics.【F:docs/architecture.md†L20-L44】  Service call edges continue to be derived from trace spans via the current AGE DAO.【F:observe-bridge/internal/analytics/graph/graph.go†L18-L27】
* **Network topology** – integrate eBPF flow collectors (agents/deepflow) and map flow tuples into `Flow` edges connecting `Pod` ↔ `Service` or `Node` ↔ `Subnet`. Retain flow statistics as properties so they can be aggregated later.
* **Infrastructure topology** – ingest cloud inventory or Terraform state snapshots, linking `Account` → `Zone` → `Subnet` → `Node`. The repository already contains `deploy/helm` and infrastructure bootstrap scripts that can seed these inventories.【F:deploy/helm/values.yaml†L1-L120】
* **User session topology** – stream RUM/session replay metadata (to be added) into a `Session` vertex class referencing `Service` and `Route` nodes.

### Query surfaces

Topology resources will be materialised as:

* **Graph snapshots** – periodic exports of subgraphs keyed by tenant/environment for fast client consumption.
* **Graph traversal APIs** – reusing the DAO to answer k-hop dependency questions (`ServiceDependencies`) and to support RCA workflows.

## Context & Intelligence Layer

### Knowledge objects

The semantic vector DAO already persists structured objects with embeddings, labels, and references to trace/span identifiers.【F:observe-bridge/internal/analytics/vector/vector.go†L8-L79】  We will standardise object types across:

* Alerts & incidents (linked to topology nodes and trace IDs).
* Runbooks & remediation guides (linked to service/deployment nodes).
* Change records & config commits (linked via Git metadata and tenant IDs).
* CMDB entities (unifying asset data with topology vertices).

Audit logging in the gateway can track user queries, latency, cache hits, and backend errors, providing behavioural context for RCA and compliance reports.【F:observe-gateway/internal/audit/audit.go†L11-L52】  Metadata lookups, rate limiting, and auth settings already provide the necessary hooks to enforce tenant scoping and identity mapping for these knowledge objects.【F:observe-gateway/internal/config/config.go†L11-L123】

### Processing pipeline

1. **Ingestion** – Alerts and incidents stream from OpenObserve (via webhook/queue) into the ETL jobs, where they are normalised and stored both in Timescale (time series) and as semantic objects with embeddings for similarity clustering.
2. **Correlation** – Trace IDs and host IDs join alerts with topology edges, enabling near-real-time graph walks to propose likely root causes.
3. **Knowledge augmentation** – GitOps commits, CMDB updates, and runbook markdown files are embedded and persisted via the vector DAO. These objects are linked back to topology vertices to provide context-aware retrieval for the LLM Ops agent.

### Outputs

* **SLO & budget tracking** – Computed from Timescale continuous aggregates described in the architecture doc.【F:docs/architecture.md†L104-L136】
* **Incident timeline** – Materialised by ordering alerts, change events, and topology deltas on a unified timeline persisted in PostgreSQL.
* **RAG surface** – The `llm-ops-agent` will query the vector DAO for the top-K relevant knowledge items when assisting with incidents.

## Integration & Resource Model

The MCP skeleton needs to expose the richer dataset so agent clients can discover every layer. We extend the resource catalogue to include `traces`, `topology`, and `knowledge` alongside the existing `logs` and `metrics`. Each resource returns structured payloads:

* `logs` – recent log entries by service and severity.
* `metrics` – key SLO metrics aggregated from OpenObserve.
* `traces` – exemplar spans with latency and dependency metadata.
* `topology` – service, network, container, and infrastructure relationships, plus recent graph deltas.
* `knowledge` – alerts, incidents, runbooks, CMDB/config entities, and change records linked by topology identifiers.

Downstream tooling can map these resources into dashboards, automated RCA pipelines, or conversational agents. The manifest and registry changes that accompany this document ensure clients discover the new resources immediately.

## Implementation Roadmap

1. **Extend ETL jobs** to populate new vertex/edge types and knowledge object schemas, reusing the AGE and pgvector DAOs.
2. **Expose APIs** from `observe-bridge` to query graph snapshots and semantic objects for downstream services (gateway, agents).
3. **Update MCP server** to load real data providers for `traces`, `topology`, and `knowledge`, replacing the current static stubs.
4. **Instrument agents** (DeepFlow, Vector, Kubernetes watchers) to push topology deltas and change events into the bridge.
5. **Build UI/LLM experiences** on top of the enriched resources for RCA, anomaly detection, and runbook automation.
