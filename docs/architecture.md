# Architecture

## Overview

```mermaid
graph TD
  subgraph Client
    A[Vector Agent]
  end

  subgraph Transport
    A --> B[Unified API<br/>(REST / gRPC / OTel / Flight)]
  end

  subgraph Storage
    B --> C1[PostgreSQL<br/>(TimescaleDB + JSONB)]
    B --> C2[ClickHouse<br/>(OLAP 聚合)]
  end

  subgraph Query
    C1 --> D[Grafana / SQL]
    C2 --> D
  end
```

## Module Responsibilities
- `gateway`: receive writes, route to PG/CH, validate schema, expose health checks.
- `registry`: dynamic table registration and JSONB schema metadata management.
- `pg-writer`: batch writes to TimescaleDB for metrics/events with short retention.
- `ch-writer`: high-throughput writes to ClickHouse for logs/traces and archival.

## Development & Deployment Notes
- PoC: use `deployments/docker-compose/poc.yaml` to launch PostgreSQL, ClickHouse, Grafana, and the four core services.
- Evolution: Helm chart separates Deployment and Service for horizontal scaling; introduce Kafka/NATS when buffering is needed.
- Query: access data via Grafana; PostgreSQL serves real-time curves/aggregations, ClickHouse handles logs and analytical queries.
- Schema management: all tables are registered via the schema registry, with gateway-side validation and lazy table creation.
