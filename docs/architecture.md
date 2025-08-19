# Architecture

## Core Features
- Real-time monitoring: high-concurrency writes to TimescaleDB with second-level aggregation queries.
- Intelligent alerting: extensible engine supporting SQL/PromQL rules and notifications via Webhook/Email/Feishu/Slack.
- Layered storage: PostgreSQL hot layer plus ClickHouse cold layer for large-scale OLAP.
- Deep analysis: optional XInsight module providing exploratory analytics and AI assistance.
- Unified ingestion: REST, gRPC, OpenTelemetry and Arrow Flight interfaces.
- Open extension: Go-first implementation with optional Rust submodules (Flight channel).

## Architecture Overview

```mermaid
graph TD
  subgraph Collectors
    N1[node_exporter]
    N2[process_exporter]
    N3[DeepFlow Agent]
    N4[journald/syslog]
    V[Vector Agent]
    N1 --> V
    N2 --> V
    N3 --> V
    N4 --> V
  end

  subgraph XScopeHub
    API[Unified API (REST/gRPC/OTel/Flight)]
    REG[Schema Registry]
    ALERT[Alerting Engine]
    API --> REG
    API --> ALERT
  end

  V --> API

  subgraph Storage
    PG[(PostgreSQL · TimescaleDB)]
    CH[(ClickHouse · OLAP)]
    API --> PG
    API --> CH
  end

  subgraph XInsight
    G[Grafana Dashboards]
    A[Deep Analysis / AI]
    PG --> G
    CH --> G
    PG --> A
    CH --> A
  end
```

## Module Responsibilities

- `gateway`: receive writes, route to PG/CH, validate schema, expose health checks.
- `registry`: dynamic table registration and JSONB schema metadata management.
- `pg-writer`: batch writes to TimescaleDB for metrics/events with short retention.
- `ch-writer`: high-throughput writes to ClickHouse for logs/traces and archival.
- `alert`: rule parsing and notifications.
- `insight`: analysis APIs serving Grafana and upstream applications.

