# XScopeHub

[English](#english) | [中文](#中文)

## English

This repository hosts multiple components:

- `observe-bridge/` – core observability bridge services and supporting code.
- `llm-ops-agent/` – placeholder for an upcoming LLM-based operations agent.
- `agents/` – external agent integrations (deepflow, node_exporter, process-exporter, vector).
- `openobserve` and `opentelemetry-collector` – external dependencies tracked as submodules.

### Architecture

```text
[exporter]                     [Vector]         [OTel GW]        [OpenObserve]
NE ─┐                      ┌─────────┐      ┌──────────┐     ┌─────────────┐
PE ─┼── metrics/logs ────> │ Vector  │ ───> │ OTel GW  │ ──> │      OO      │
DF ─┤                      └─────────┘      └──────────┘     └─────────────┘
LG ─┘

                       (nearline window ETL: Align=1m · Delay=2m)
                                        │
                                        ▼
                         ┌──────────────────────────────────────┐
 IaC/Cloud  ────────────>│                                      │
                         │   ObserveBridge (ETL JOBS)           │
 Ansible     ───────────>│   • ETL 窗口聚合 / oo_locator        │
                         │   • 拓扑 (IaC/Ansible)               │
 OO 明细(OO→OB)  ───────>│   • AGE 10 分钟活跃调用图刷新        │
                         └──────────────────────────────────────┘

┌─────────────────────────────── Postgres Suite ───────────────────────────────┐
│   PG_JSONB            │   PG Aggregates (Timescale)   │  PG Vector  │  AGE   │
│ (oo_locator/events)   │ (metric_1m / call_5m / log_5m)│ (pgvector)  │ Graph  │
└───────────────┬────────┴───────────────┬──────────────┴─────────────┬────────┘
                │                        │                             │
                │                        │                             │
                ▼                        ▼                             ▼
                         [ llm-ops-agent / 应用消费（查询/检索/推理） ]
```

### Documentation

- [Repository Structure](docs/repository_structure.md)
- [Architecture](docs/architecture.md)
- [API](docs/api.md)
- Observability Bridge
  - [Observability ETL Suite Design (ZH)](docs/observe-bridge/Observability-ETL-Suite-Design-ZH.md)
  - [Observability ETL Suite Design (EN)](docs/observe-bridge/Observability-ETL-Suite-Design-EN.md)
- [Insight](docs/insight.md)
- [Roadmap](docs/roadmap.md)
- [Deployment](docs/deployment.md)
- [Grafana](docs/grafana.md)
- [MCP Server Integration](docs/mcp-server-integration.md)
- LLM-Ops Agent
  - [Overview](docs/llm-ops-agent/overview.md)
  - [API](docs/llm-ops-agent/api.md)
  - [Usage](docs/llm-ops-agent/usage.md)
  - [Docs](docs/llm-ops-agent/docs)
  - [Testing](docs/llm-ops-agent/testing.md)
  - [Dual Engine Design](docs/llm-ops-agent/dual-engine-design.md)
  - [Start](docs/llm-ops-agent/start.md)
  - [Postgres Init](docs/llm-ops-agent/postgres-init.md)

## 中文

本仓库包含多个组件：

- `observe-bridge/` – 核心可观测桥服务及支撑代码。
- `llm-ops-agent/` – 计划中的 LLM 运维代理。
- `agents/` – 外部采集器集成（deepflow、node_exporter、process-exporter、vector）。
- `openobserve` 和 `opentelemetry-collector` – 以子模块方式跟踪的外部依赖。

### 架构

```text
[exporter]                     [Vector]         [OTel GW]        [OpenObserve]
NE ─┐                      ┌─────────┐      ┌──────────┐     ┌─────────────┐
PE ─┼── metrics/logs ────> │ Vector  │ ───> │ OTel GW  │ ──> │      OO      │
DF ─┤                      └─────────┘      └──────────┘     └─────────────┘
LG ─┘

                       (近线窗口 ETL: 对齐=1m · 延迟=2m)
                                        │
                                        ▼
                         ┌──────────────────────────────────────┐
 IaC/Cloud  ────────────>│                                      │
                         │   ObserveBridge (ETL 任务)           │
 Ansible     ───────────>│   • ETL 窗口聚合 / oo_locator        │
                         │   • 拓扑 (IaC/Ansible)               │
 OO 明细(OO→OB)  ───────>│   • AGE 10 分钟活跃调用图刷新        │
                         └──────────────────────────────────────┘

┌─────────────────────────────── Postgres 套件 ───────────────────────────────┐
│   PG_JSONB            │   PG Aggregates (Timescale)   │  PG Vector  │  AGE   │
│ (oo_locator/events)   │ (metric_1m / call_5m / log_5m)│ (pgvector)  │ Graph  │
└───────────────┬────────┴───────────────┬──────────────┴─────────────┬────────┘
                │                        │                             │
                │                        │                             │
                ▼                        ▼                             ▼
                         [ llm-ops-agent / 应用消费（查询/检索/推理） ]
```

### 文档

- [仓库结构](docs/repository_structure.md)
- [架构](docs/architecture.md)
- [API](docs/api.md)
- Observe-Bridge
  - [Observability ETL 套件设计 (中文)](docs/observe-bridge/Observability-ETL-Suite-Design-ZH.md)
  - [Observability ETL Suite Design (English)](docs/observe-bridge/Observability-ETL-Suite-Design-EN.md)
- [XInsight](docs/insight.md)
- [路线图](docs/roadmap.md)
- [部署](docs/deployment.md)
- [Grafana](docs/grafana.md)
- [MCP Server 集成设计](docs/mcp-server-integration.md)
- LLM 运维代理
  - [概览](docs/llm-ops-agent/overview.md)
  - [API](docs/llm-ops-agent/api.md)
  - [使用](docs/llm-ops-agent/usage.md)
  - [文档目录](docs/llm-ops-agent/docs)
  - [测试](docs/llm-ops-agent/testing.md)
  - [双引擎设计](docs/llm-ops-agent/dual-engine-design.md)
  - [启动](docs/llm-ops-agent/start.md)
  - [Postgres 初始化](docs/llm-ops-agent/postgres-init.md)
