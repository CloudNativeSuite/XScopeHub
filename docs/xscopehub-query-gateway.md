# XScopeHub Query Gateway 规划文档

## 目录
1. [背景与目标](#背景与目标)
2. [技术选型](#技术选型)
3. [架构设计](#架构设计)
4. [查询语言映射](#查询语言映射)
5. [功能模块](#功能模块)
6. [API 设计](#api-设计)
7. [迭代规划](#迭代规划)
8. [部署与集成](#部署与集成)
9. [后续扩展](#后续扩展)

---

## 背景与目标
当前日志 (LogQL)、指标 (PromQL)、链路 (TraceQL) 的查询入口各自分离，导致：
- 多套鉴权与租户隔离逻辑，维护成本高；
- 查询语言、接口风格不统一，前端集成复杂；
- SLO/SLA 面板无法实现跨信号“一键查询”。

**目标**：构建一个 **统一的查询网关** (Query Gateway)：
- 统一入口：PromQL / LogQL / TraceQL；
- 单用户/多租户支持，安全可控；
- 后端对接 **OpenObserve (O2)** 与 PostgreSQL 元数据；
- 支持限流、配额、缓存与审计；
- 无缝集成 Grafana 与 Insight Workbench。

---

## 技术选型
- **后端存储**：OpenObserve (O2)
  - 元数据存储：PostgreSQL
  - 数据存储：对象存储 (MinIO/S3/GCS)
- **网关实现**：Go
  - Web 框架：`chi`
  - PromQL 解析：`prometheus/promql/parser`
  - 限流：`x/time/rate` + Redis
  - 缓存：`ristretto` + Redis
  - Observability：OTel SDK，Prometheus RED 指标

---

## 架构设计

```mermaid
flowchart LR
    Client[Grafana / UI / API] --> Gateway[Go Query Gateway]
    Gateway -->|PromQL| O2Prom[OpenObserve PromQL API]
    Gateway -->|LogQL→SQL翻译| O2Log[OpenObserve Logs SQL API]
    Gateway -->|TraceQL→SQL翻译| O2Trace[OpenObserve Traces API]
    Gateway --> PG[(PostgreSQL 元数据)]
    Gateway --> Minio[(对象存储)]

查询语言映射
入口语言	示例	O2 对应实现
PromQL	rate(http_requests_total[5m])	调用 O2 PromQL API（若不兼容则旁路 VM/Mimir）
LogQL	`{app="api"}	= "error"`
TraceQL	`from default	where span.kind="server" and duration>200ms`
功能模块

鉴权

JWT (OIDC JWK 校验)

mTLS (可选)

多租户

Header X-Tenant / JWT claims

注入 Label、路由到对应后端池

限流与配额

每租户 QPS、样本点数、日志字节数、trace 数量限制

查询规划

规范化时间窗与 step

拦截危险查询（过大 lookback、offset 滥用）

Fanout / Hedging / Retry 策略

缓存

Instant 查询短 TTL 缓存

Range 查询分片缓存 + 拼接

审计日志

记录 JSON 格式日志（tenant, user, lang, query, cost, duration）

可观测性

OTel trace

Prometheus RED 指标

API 设计
请求示例

POST /api/query

{
  "lang": "promql|logql|traceql",
  "query": "...",
  "start": "2025-09-22T00:00:00Z",
  "end": "2025-09-22T01:00:00Z",
  "step": "30s",
  "normalize": true
}

响应示例
{
  "lang": "logql",
  "tenant": "gold",
  "result": {
    "logs": [
      { "ts": 1690000000.123, "line": "..." }
    ]
  },
  "stats": {
    "backend": "oo-logs",
    "cached": false,
    "duration_ms": 120
  }
}

# 迭代规划

- MVP（单租户）

  - PromQL/LogQL/TraceQL 基础代理
  - JWT 鉴权、查询超时、限流

## 多租户增强
- 标签注入、分区路由
- Redis 分布式限流与配额

## 缓存/优化
- instant / range 分片缓存
- Hedging / Retry

## 集成

- Grafana DataSource → Query Gateway
- Insight Workbench → 内嵌查询 API

# 部署与集成

部署

- VM部署：bin + systemd
- 容器化：Dockerfile + K8s Deployment
— ConfigMap 挂载配置文件 config.yaml

集成

Grafana 配置 DataSource 指向网关
Insight Workbench 调用 /api/query

安全
Ingress 层启用 mTLS 或 JWT 验证
O2 API 基于 Basic Auth / Token 调用
