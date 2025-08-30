# XScopeHub

This repository hosts multiple components:

- `observe-bridge/` – core observability bridge services and supporting code.
- `LLM-Agent/` – placeholder for an upcoming LLM-based operations agent.
- `agents/` – external agent integrations (deepflow, node_exporter, process-exporter, vector).
- `openobserve` and `opentelemetry-collector` – external dependencies tracked as submodules.

# architecture

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

