# Repository Structure

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

This document outlines the top-level layout of the repository. Each directory contains
components described in the inline comments above.

