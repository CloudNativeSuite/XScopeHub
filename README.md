# XScopeHub

Unified observability platform for metrics, logs, and traces.

## Overview

XScopeHub combines a scalable ingestion gateway, alerting engine, and the optional XInsight analytics module. Data is written to TimescaleDB (hot layer) and ClickHouse (cold/OLAP).

## Documentation

- [Architecture](docs/architecture.md)
- [Deployment](docs/deployment.md)
- [Roadmap](docs/roadmap.md)
- [Grafana](docs/grafana.md)
- [API](docs/api.md)

## Repository Structure

```
xscopehub/
├─ cmd/                            # service entrypoints
│  ├─ gateway/                     # unified API (REST/gRPC/OTel/Flight)
│  ├─ registry/                    # schema registry
│  ├─ pg-writer/                   # TimescaleDB writer
│  ├─ ch-writer/                   # ClickHouse writer
│  ├─ alert/                       # alert engine
│  └─ insight/                     # analysis service (XInsight)
│
├─ internal/                       # internal libraries
│  ├─ api/ ingest/ registry/ writer/
│  ├─ storage/ {postgres, clickhouse}
│  ├─ alerting/ analytics/ o11y/ config/ utils/
│
├─ proto/                          # gRPC/Protobuf definitions
├─ flight/                         # Arrow Flight module
├─ migrations/                     # database migrations
├─ agents/                         # collector examples
├─ grafana/                        # data sources & dashboards
├─ deployments/                    # docker-compose & helm
├─ configs/                        # service config examples
├─ scripts/                        # helper scripts
├─ docs/                           # documentation
├─ .github/workflows/              # CI workflows
├─ go.work  Makefile  .env.example  LICENSE  README.md
```

