# Project Overview

This project now exposes a modular AIOps pipeline composed of seven
cooperating modules:

```
Sensor → Analyst → Planner → Gatekeeper → Executor → Librarian → Orchestrator
```

Each module has its own REST endpoint under `/api/v1` and exchanges
CloudEvents over NATS (commands) and Redpanda (events). Common headers
such as `X-Request-Id` and `traceparent` enable end-to-end tracing, while
`Idempotency-Key` prevents duplicate work.

The workflow extends the earlier "OPS Agent 思维链条": identify abnormal
signals, analyze anomalies, generate and evaluate remediation plans,
execute actions, capture knowledge and orchestrate case state
transitions.

Most modules are still implemented as placeholders. Future work will
connect real data collectors (Prometheus, Loki, Jaeger/Tempo), rule
engines and reporting sinks such as Slack or Grafana.
