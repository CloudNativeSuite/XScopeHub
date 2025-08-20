# Roadmap

The following roadmap is organized by development phases. Each phase represents a two‑week
cycle with tasks broken down into minimal deliverables.

## Phase 1: Ingestion and Write Path (2 weeks)
- Stabilize vector→gateway→PG/CH write path
- Implement batching, retry, and idempotency mechanisms

## Phase 2: Grafana Auto Provisioning (1 week)
- Auto‑create PG/CH data sources
- Generate an overview dashboard

## Phase 3: Alert Engine (2 weeks)
- Support SQL/PromQL rule evaluation
- Provide notification plugins (Webhook/Email/IM)

## Phase 4: XInsight Analytics (2 weeks)
- Build operator library (TopN, anomaly detection, causal clue)

## Phase 5: Storage/Computation Separation (2 weeks)
- Integrate Kafka/NATS buffering
- Enable distributed and HA PG/CH

## Phase 6: Security (1 week)
- Implement multi‑tenancy
- Add authentication and auditing

