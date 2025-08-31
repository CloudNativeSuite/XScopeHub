# XOpsAgent (TimescaleDB + OpenObserve)

一个最小可跑的闭环示例：Alertmanager → （OPS Agent）→ GitHub PR →（可选）ArgoCD 健康检测 → TimescaleDB 指标验证。

核心 API 模块依次为 Sensor → Analyst → Planner → Gatekeeper → Executor →
Librarian → Orchestrator，接口细节见 [docs/api.md](docs/api.md)。

## 目录结构

```
XOpsAgent/
├── docker-compose.yml
├── configs/
│   ├── otelcol.yaml
│   └── github/
│       └── pr_template.md
├── db/
│   └── 001_schema.sql
├── cmd/
│   └── main.go
├── scripts/
│   └── load_schema.sh
├── go.mod
├── README.md
└── .env.example
```

## 快速开始

### 1) 启动 TimescaleDB + OpenObserve + OTel Collector
```bash
docker compose up -d
```

OpenObserve UI: http://localhost:5080  （默认账号 admin@example.com / ComplexPass123!）

OTLP 入口：http://localhost:5080/api/default/

### 2) 初始化数据库
```bash
scripts/load_schema.sh
```

### 3) 配置环境变量并运行 Agent

创建 `.env`（或直接 export 环境变量）：

```bash
export PG_URL="postgres://postgres:postgres@127.0.0.1:5432/ops?sslmode=disable"
export LISTEN_ADDR=":8080"

# GitHub PR 所需（使用你的仓库）
export GITHUB_TOKEN="<ghp_xxx>"
export GITHUB_OWNER="your-github-user-or-org"
export GITHUB_REPO="your-gitops-repo"
export GITHUB_BASE_BRANCH="main"
export GITHUB_FILE_PATH="charts/app/values.yaml"   # 该文件需存在
export FLAG_PATH="featureFlags.recommendation_v2"  # 要切的布尔开关路径

# 可选：ArgoCD
export ARGOCD_URL="https://argocd.example.com"
export ARGOCD_TOKEN="<argocd.jwt>"
export ARGOCD_APP="your-app"
```

运行（前台调试示例）：
```bash
go run ./cmd --daemon=false
```

### 4) 发送告警（模拟 Alertmanager Webhook）
```bash
curl -XPOST http://localhost:8080/alertmanager -H 'Content-Type: application/json' -d '{
  "status": "firing",
  "commonLabels": { "service": "checkout" },
  "alerts": [ { "labels": { "service": "checkout" }, "annotations": { "summary": "p95 latency high" } } ]
}'
```

返回类似：
```json
{"incident_id":1,"pr_url":"https://github.com/<owner>/<repo>/pull/123","verified":false}
```

### 5) Timescale 验证（演示数据）

先生成 20 分钟样本：
```sql
SELECT seed_latency('checkout', 400, 120);
REFRESH MATERIALIZED VIEW CONCURRENTLY metrics_1m;
```

PR 合并&下发（或直接再次生成“更好”的最近 5 分钟样本）：
```sql
-- 让最近 5 分钟平均值更低，模拟“生效后”好转
SELECT seed_latency('checkout', 250, 60);
REFRESH MATERIALIZED VIEW CONCURRENTLY metrics_1m;
```

Agent 的 `/alertmanager` 接口在提交 PR、（可选）等待 ArgoCD Healthy 后，会调用 `recent_latency_improved()` 对比“最近 5 分钟”与“之前 5 分钟”，降幅 ≥10% 视为成功并关闭 incident。

## OTel → OpenObserve

`configs/otelcol.yaml` 已配置 otlphttp/openobserve 导出器，开箱即上报主机指标/日志。

你也可以把应用的 OTLP 指标/日志/链路打到 `http://localhost:5080/api/default/`。

## 生产化提示

- 真实环境建议把验证指标改用 p95/p99。
- GitOps 优先，直连操作需 RBAC + 审计。
- 把“规则+RAG 计划器”放在独立 `planner/` 模块；本 PoC 仅演示“关闭开关”。

## 常见问题

- PR 失败：检查 `GITHUB_*` 变量、`values.yaml` 路径是否存在、Token 权限（repo:contents, pull_request）。
- ArgoCD 跳过：不配置 `ARGOCD_*` 就会直接跳过等待环节。
- Timescale 没数据：先执行 `seed_latency()` 或把业务指标写入 `metrics_point`

## CI

直接用 API 触发（高级用法）

GitHub REST API v3:

curl -X POST \
  -H "Accept: application/vnd.github+json" \
  -H "Authorization: Bearer <YOUR_TOKEN>" \
  https://api.github.com/repos/<OWNER>/<REPO>/actions/workflows/build-test-release.yml/dispatches \
  -d '{"ref":"main","inputs":{"build":"true","unit":"true","integration":"false","fixtures":"false","release":"false"}}'

