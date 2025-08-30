# API Reference

本仓库提供 AIOps 服务基础骨架，包含健康检查 `/healthz` 与指标端点 `/metrics`，并接入 Gin、OpenAPI、NATS、Redpanda、TimescaleDB 与 OpenTelemetry。

本项目遵循模块化架构，关键路径如下：

Sensor → Analyst → Planner → Gatekeeper → Executor → Librarian → Orchestrator

下表描述了各模块的职责、输入输出及接口定义：

| 模块 | 职责 | 输入 | 输出 | 存储 | 接口 | SLO |
|:----|:----|:----|:----|:----|:----|:----|
| Sensor | 接入信号 | OTLP、Prom、logs | OO 明细 | OO、PG | `/ingest/*` | 写入 p99<2s |
| Analyst | 异常检测 | OO 明细、PG | 聚合、analysis.findings | PG | `/analyze/run` | 10min 完成聚合 |
| Planner | 生成计划 | kb_chunk、证据 | plan.proposed | PG | `/plan/generate` | <30s 首版计划 |
| Gatekeeper | 策略评估 | plan.proposed | plan.approved | PG | `/gate/eval` | 自动评估<1s |
| Executor | 执行动作 | plan.approved | exec.step.result | OO、PG | `/adapter/exec` | 单步 15m 超时 |
| Librarian | 知识沉淀 | 日志、diff | kb_doc、kb_chunk | PG | `/kb/ingest` | 5min 内可检索 |
| Orchestrator | 状态机调度 | 各类事件 | case.updates | PG | `/case/*` | 状态原子迁移 |

所有接口遵循 CloudEvents 协议，返回 JSON 结果。示例：

```bash
curl -XPOST http://localhost:8080/plan/generate -H 'Content-Type: application/json' -d '{}'
```

响应：

```json
{"module":"planner","status":"ok"}
```

# 接口详细设计

（围绕你给出的 SLO 表与 PG 架构），并在结尾把工作拆成Codex 任务提示词（逐条可直接驱动编码）。

## 一、全局约定

### 协议与编解码

外部管理面：REST/JSON（OpenAPI 3.1）；内部高吞吐链路（OTLP、指标、日志）沿用各自原生协议。

事件/命令总线：NATS JetStream（命令总线）+ Redpanda/Kafka（事件日志）。

编码：UTF-8；时间：RFC3339；数值单位显式（ms、rps、bytes）。

版本：Header X-Api-Version: v1；消息 spec.version: "1.0"。

### 追踪与幂等

统一头：X-Request-Id、X-Case-Id、traceparent（W3C TraceContext）。

幂等：请求头 Idempotency-Key（服务端保存 24h 命中即返回首个结果）；命令与事件都带 event_id/command_id (UUID)。

### 安全

内部控制面：mTLS + SPIFFE/SPIRE；外部 API：OIDC JWT（aud=aiops），RBAC（tenant → scope → action）。

策略：OPA（Rego）或 Cedar，决策日志落 PG（gate_decision）。

### 可观测

/metrics（Prometheus）；结构化日志（JSONL）；OpenTelemetry Traces/Logs。

## 二、统一响应与错误码
```json
// 统一响应 Envelope
{
  "request_id": "b3e1-...-9d",
  "code": 0,
  "message": "OK",
  "data": { "...": "..." }
}
```

| code | 含义 |
| ---- | ---- |
| 0 | OK |
| 400x | 校验/幂等/语义错误（如 4001 不合法参数、4002 幂等冲突） |
| 401x | 认证失败 |
| 403x | 授权失败 |
| 404x | 资源不存在 |
| 409x | 状态冲突（不可迁移、审批中） |
| 4290 | 限流 |
| 499x | 客户端主动取消 |
| 500x | 服务内部错误 |
| 503x | 依赖不可用 |
## 三、消息与主题规范（NATS/Redpanda）

### 命名约定

- 命令（NATS）：cmd.<module>.<action>
- 事件（Redpanda）：evt.<bounded-context>.<event-name>.v1
- 例：cmd.analyst.run, evt.case.plan_proposed.v1, evt.exec.step_finished.v1

### 关键消息 Schema（JSON）

evt.obs.event.v1（传感器归一化后的观测事件）

```json
{
  "spec": {"version": "1.0"},
  "event_id": "uuid",
  "tenant": "t-001",
  "resource_urn": "k8s://ns/app/deploy/myapp",
  "ts": "2025-08-29T11:22:33Z",
  "kind": "metric|log|trace|slo_violation",
  "severity": "INFO|WARN|ERROR",
  "attributes": {"metric": "latency_ms", "p95": 812},
  "oo_ref": {"dataset": "metrics", "object_id": 12345}
}
```

cmd.analyst.run（触发分钟级聚合与异常检测）

```json
{
  "command_id": "uuid",
  "tenant": "t-001",
  "window": {"from": "2025-08-29T11:10:00Z", "to": "2025-08-29T11:20:00Z"},
  "targets": ["urn1", "urn2"],
  "rules": ["latency_p95>800ms AND error_rate>1%"],
  "context": {"case_id": "uuid"}
}
```


evt.analysis.findings.v1

```json
{
  "case_id": "uuid",
  "findings": [
    {"type": "latency_spike", "metric": "p95_ms", "value": 812, "baseline": 300,
     "evidence": [{"dim":"metric","ref_pg":{"table":"metric_1m","keys":{"bucket":"...","resource_id":123}}}]}
  ]
}


evt.plan.proposed.v1 / evt.plan.approved.v1 / cmd.exec.run / evt.exec.step_result.v1 类似结构，均包含 case_id/plan_id/step_id、ts、actor、evidence、status 等字段。

## 四、REST 接口（OpenAPI 摘要）

路径前缀 /api/v1；仅列关键字段，细节可据此扩展生成完整 OAS。

1) Sensor（接入/落盘索引）

POST /ingest/otlp（代理 OTLP；反向写入 OO/Prom/Loki，返回 ack）

POST /ingest/logs（JSONL 或 NDJSON 批量）

POST /ingest/hint（写 oo_locator 索引，便于回查）

Req:
```json
{"tenant":"t-001","dataset":"logs","bucket":"oo-bkt","object_key":"...","t_from":"...","t_to":"...","attributes":{}}
```
SLO：写入 p99 < 2s

2) Analyst（异常检测/聚合）

POST /analyze/run

Req:（同 cmd.analyst.run）

Resp:
```json
{"job_id":"uuid","estimated_finish_sec":120}
```
GET /analyze/jobs/{job_id}

Resp：status: PENDING|RUNNING|DONE|FAILED, findings:[...]

SLO：10 分钟内完成窗口聚合（分钟粒度 + TopK）

3) Planner（生成计划）

POST /plan/generate

Req：

```json
{"case_id":"uuid","findings":[...],
 "kb_hints":{"topk":5,"similar_tags":["rollout","latency"]}}
```


Resp：

```json
{"plan_id":"uuid","dsl": "yaml-string", "risk_score":0.62, "rollback":"..."}
```


SLO：< 30s 产出首版 plan.proposed

4) Gatekeeper（策略/审批）

POST /gate/eval

Req：

```json
{"plan_id":"uuid","case_id":"uuid","policies":["slo-baseline","change-window"],"approvers":["ops_lead"]}
```

Resp：

```json
{"approved":true,"constraints":{"change_window":"2025-08-29T12:00Z/13:00Z"},"reason":"auto-pass"}
```

SLO：自动评估 < 1s（OPA/Cedar 本地缓存）

5) Executor（动作）

POST /adapter/exec

Req：

```json
{"plan_id":"uuid","case_id":"uuid","dry_run":false,
 "adapters":[{"type":"gitops","args":{"app":"myapp"}},{"type":"k8s","args":{"namespace":"ns"}}]}
```


Resp：exec_id

GET /adapter/exec/{exec_id} → step 流水、stdout/stderr、status

SLO：单步 15m 超时（超时进入自动回滚分支）

6) Librarian（知识沉淀）

POST /kb/ingest 传入 runbook/日志 diff/postmortem，触发切块、Embedding 写入 kb_doc/kb_chunk

GET /kb/search?q=...&topk=10

SLO：5 分钟内能被检索到

7) Orchestrator（状态机）

POST /case/create

```json
{"title":"p95 spike","tenant":"t-001","resource_urn":"...","severity":"ERROR","labels":{"env":"prod"}}
```


→ 返回 case_id，初始状态 NEW

PATCH /case/{id}/transition

```json
{"event":"ANALYZE" | "PLAN" | "EXECUTE" | "VERIFY" | "CLOSE","reason":"..."}
```


GET /case/{id}（状态、时间线、证据链）

SLO：每次状态迁移原子且可回放（采用事务外盒 Outbox，见下）

## 五、缺失域表（在你现有 PG DDL 基础上补齐）

以下 新增 表补全 Case/Plan/Gate/Exec/Outbox/Idempotency 等“控制面”实体。

```sql
-- 9) Case 主体与时间线
CREATE TABLE ops_case (
  case_id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    BIGINT REFERENCES dim_tenant(tenant_id),
  title        TEXT,
  severity     severity NOT NULL DEFAULT 'INFO',
  status       TEXT NOT NULL, -- NEW|ANALYZING|PLANNING|WAIT_GATE|EXECUTING|VERIFYING|CLOSED|FAILED|PARKED
  resource_id  BIGINT REFERENCES dim_resource(resource_id),
  created_at   TIMESTAMPTZ DEFAULT now(),
  updated_at   TIMESTAMPTZ DEFAULT now(),
  labels       JSONB DEFAULT '{}'::jsonb
);
CREATE INDEX idx_case_tenant_time ON ops_case(tenant_id, created_at DESC);

CREATE TABLE case_timeline (
  id         BIGSERIAL PRIMARY KEY,
  case_id    UUID REFERENCES ops_case(case_id) ON DELETE CASCADE,
  ts         TIMESTAMPTZ DEFAULT now(),
  actor      TEXT, -- system|user:<id>|agent:<name>
  event      TEXT, -- 状态迁移/备注/外部回调
  payload    JSONB
);
CREATE INDEX idx_case_tl_case_time ON case_timeline(case_id, ts DESC);

-- 10) Plan
CREATE TABLE plan_proposed (
  plan_id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id    UUID REFERENCES ops_case(case_id) ON DELETE CASCADE,
  dsl_yaml   TEXT NOT NULL,
  risk_score DOUBLE PRECISION,
  created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE plan_approved (
  plan_id    UUID PRIMARY KEY,
  approver   TEXT,
  approver_type TEXT, -- auto|human
  constraints JSONB DEFAULT '{}'::jsonb,
  approved_at TIMESTAMPTZ DEFAULT now(),
  FOREIGN KEY (plan_id) REFERENCES plan_proposed(plan_id) ON DELETE CASCADE
);

-- 11) Gatekeeper 决策日志
CREATE TABLE gate_decision (
  id         BIGSERIAL PRIMARY KEY,
  case_id    UUID REFERENCES ops_case(case_id) ON DELETE CASCADE,
  plan_id    UUID REFERENCES plan_proposed(plan_id) ON DELETE CASCADE,
  policy     TEXT,
  decision   TEXT, -- allow|deny
  reason     TEXT,
  details    JSONB,
  decided_at TIMESTAMPTZ DEFAULT now()
);

-- 12) 执行流水
CREATE TABLE exec_run (
  exec_id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id    UUID REFERENCES ops_case(case_id) ON DELETE CASCADE,
  plan_id    UUID REFERENCES plan_proposed(plan_id) ON DELETE CASCADE,
  status     TEXT NOT NULL, -- PENDING|RUNNING|PARTIAL|SUCCESS|FAILED|ROLLED_BACK
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ
);

CREATE TABLE exec_step (
  step_id    BIGSERIAL PRIMARY KEY,
  exec_id    UUID REFERENCES exec_run(exec_id) ON DELETE CASCADE,
  idx        INT NOT NULL,
  adapter    TEXT NOT NULL, -- k8s|gitops|db|gateway...
  args       JSONB,
  status     TEXT NOT NULL, -- PENDING|RUNNING|SUCCESS|FAILED|TIMEOUT
  stdout_ref BIGINT REFERENCES oo_locator(id), -- 大日志落 OO，索引回链
  stderr_ref BIGINT REFERENCES oo_locator(id),
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ
);
CREATE INDEX idx_execstep_exec ON exec_step(exec_id, idx);

-- 13) 验证与回滚检查点
CREATE TABLE verify_checkpoint (
  id         BIGSERIAL PRIMARY KEY,
  exec_id    UUID REFERENCES exec_run(exec_id) ON DELETE CASCADE,
  kind       TEXT,          -- sli|synthetic|logpattern
  window     tstzrange,
  result     JSONB,
  passed     BOOLEAN,
  created_at TIMESTAMPTZ DEFAULT now()
);

-- 14) Outbox（事务外盒，保证“先写库后发消息”）
CREATE TABLE outbox (
  id          BIGSERIAL PRIMARY KEY,
  aggregate   TEXT,         -- ops_case|plan|exec...
  aggregate_id TEXT,
  topic       TEXT,         -- evt.*
  payload     JSONB NOT NULL,
  created_at  TIMESTAMPTZ DEFAULT now(),
  published   BOOLEAN DEFAULT FALSE,
  published_at TIMESTAMPTZ
);
CREATE INDEX idx_outbox_unpub ON outbox(published) WHERE published = FALSE;

-- 15) 幂等键
CREATE TABLE idempotency (
  idem_key   TEXT PRIMARY KEY,
  request    JSONB,
  response   JSONB,
  created_at TIMESTAMPTZ DEFAULT now(),
  ttl        TIMESTAMPTZ
);
```

## 六、Plan DSL（最小可用示例）
```yaml
apiVersion: aiops.svc.plus/v1
kind: ChangePlan
metadata:
  planId: ${plan_id}
  caseId: ${case_id}
spec:
  preChecks:
    - type: sli
      expr: "error_rate{svc='myapp',env='prod'} < 0.05"
      timeout: 120s
  steps:
    - name: rollout restart
      adapter: k8s
      args: {namespace: "prod", kind: "Deployment", name: "myapp", action: "rollout_restart"}
      timeout: 15m
    - name: wait-stable
      adapter: k8s
      args: {kind: "Deployment", name: "myapp", condition: "Available"}
      timeout: 10m
  rollback:
    - name: rollout undo
      adapter: k8s
      args: {namespace: "prod", kind: "Deployment", name: "myapp", action: "rollout_undo"}
  verify:
    - type: sli
      expr: "histogram_quantile(0.95, rate(http_request_duration_ms_bucket{svc='myapp'}[5m])) < 500"
      window: "10m"
```

## 七、SLI/SLO 落地度量（摘）

Sensor：ingest_write_latency_seconds{quantile="0.99"} < 2

Analyst：analyze_job_duration_seconds_bucket（分钟粒度完成率）

Planner：planner_first_plan_seconds < 30

Gatekeeper：gate_eval_latency_seconds < 1 + gate_auto_approve_ratio

Executor：exec_step_duration_seconds{le="900"} 覆盖率>99%

Librarian：kb_ingest_to_search_seconds < 300

Orchestrator：case_transition_latency_seconds；outbox_lag_seconds≈0

## 八、Go 侧接口（最小接口定义）
```go
// 以端口接口解耦实现，便于替换适配器
type Analyst interface {
  Run(ctx context.Context, req AnalyzeRequest) (jobID string, err error)
  Get(ctx context.Context, jobID string) (Findings, error)
}
type Planner interface {
  Generate(ctx context.Context, in PlanGenerateInput) (Plan, error)
}
type Gatekeeper interface {
  Evaluate(ctx context.Context, plan Plan, ctxInfo GateContext) (Decision, error)
}
type Executor interface {
  Execute(ctx context.Context, plan Plan, opts ExecOptions) (execID string, err error)
  Status(ctx context.Context, execID string) (ExecStatus, error)
}
type Librarian interface {
  Ingest(ctx context.Context, doc DocIngest) (docID int64, err error)
  Search(ctx context.Context, q string, topk int) ([]ChunkHit, error)
}
type Orchestrator interface {
  Transition(ctx context.Context, caseID string, event CaseEvent) (CaseState, error)
}
```

