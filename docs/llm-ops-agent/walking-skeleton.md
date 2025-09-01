
**从零到可跑（Walking Skeleton）**的实现顺序与切分方式。核心思路：先打通一条“最短闭环”（人工触发 → 计划 → 审批 → 执行 → 验证 → 归档），其余模块逐步替换/加深。每一步都能运行、有度量、有回滚。

一、建议实现顺序（Why this order）

Orchestrator 核心（必须先做）

目标：把“合法性判定（纯函数 FSM）”与“原子副作用（DB 状态 + 时间线 + Outbox + 幂等）”打牢。

交付：

/case/create、PATCH /case/{id}/transition（含 409 非法迁移、If-Match 版 OCC、Idempotency-Key）。

表：ops_case / case_timeline / outbox / idempotency（你已就位），以及发布器（outbox→NATS/Redpanda）。

集成测：非法迁移 409 / 幂等重放 / 失败落 PARKED（你提出的 3 条验收）。

意义：后续 Planner / Gate / Exec 都是通过 outbox 事件与这个核心解耦。

Planner（最小可用）

目标：让状态机能从 PLANNING → WAIT_GATE。

交付：

纯函数计划生成器：plans.BuildChangePlan、plans.BuildIncidentPlan（已给骨架）。

/plan/generate：把 DSL（YAML/JSON）写入 plan_proposed，并发布 evt.plan.proposed.v1。

守卫对接：只有当 steps+rollback+verify 俱全时，Orchestrator 才允许 EPlanReady。

意义：无需 KB/Embedding 也能先输出“模板计划”，形成流转。

Gatekeeper（最小策略/自动审批）

目标：让状态机能从 WAIT_GATE → EXECUTING。

交付：

内置规则（本地 OPA/Cedar 之后再接）：变更窗口、环境白名单、风险分数阈值。

/gate/eval：写 gate_decision、plan_approved，发 evt.plan.approved.v1；触发 Orchestrator 的 EGateApproved。

意义：形成“可控”的执行入口，先用自动审批，后续再接人审。

Executor（最小执行器 + Step Runner）

目标：打通 EXECUTING → VERIFYING。

交付：

端口接口：Adapter（k8s / gateway / script），先实现Echo/Script与K8s rollout两个最小适配器。

/adapter/exec：写 exec_run / exec_step，逐步驱动执行；stdout/stderr 落 OO 或本地并在表里存 ref。

事件：evt.exec.step_result.v1、结束发 evt.exec.done|failed.v1；Orchestrator 消费后触发 EExecDone/EExecFailed。

意义：这是第一个“能动真格”的地方——哪怕先跑 echo/script，也能闭环。

Verifier（SLO 校验，或并入 Executor 的收尾步骤）

目标：VERIFYING → CLOSED 的自动判定。

交付：

简化版校验：查询 metric_1m 做 p95/错误率门槛；写 verify_checkpoint，通过则触发 EVerifyPass，否则 EVerifyFailed（回 PARKED）。

意义：闭环“完成/未完成”的客观标准。

Analyst（最小异常检测）

目标：非人工入口：由指标/日志触发 Case。

交付：

/analyze/run + 一个定时 Job：读取 metric_1m / log_pattern_5m，用简单规则产出 evt.analysis.findings.v1；

监听 findings：NEW→ANALYZING→PLANNING（自动拉起 Planner）。

意义：把系统从“命令式”推进到“信号驱动”。

Sensor（最小化接入）

目标：面向外部接入的“索引桥”。

交付：

/ingest/hint 写 oo_locator；OTLP/Logs 可先透传或 stub；真正的 OO/Loki/Prom 接入后置。

意义：即使没有完整明细，也能通过 locator 实现“证据链回查”。

Librarian（KB/向量，非 MVP 必需）

目标：支撑 Planner 做“相似案例/Runbook 拼装”。

交付：

/kb/ingest → kb_doc/kb_chunk（pgvector）；/kb/search。

意义：把 Planner 从“模板计划”升级为“基于证据的方案检索+拟合”。

顺序要点：1→4→5 打通“人工闭环”；6→7→8 增强智能化与知识化。

二、每个阶段的 DoD（Definition of Done）

Orchestrator：三类集成测试全绿；Prom 指标：case_transition_total / conflict_total / idempotent_replay_total 上报；Outbox 消费无重复副作用。

Planner：plan_proposed 入库，DSL 经 schema 校验；守卫能挡住不完整计划。

Gatekeeper：策略可配置（本地文件/内存），自动审批延时 <1s；拒绝能落 PARKED 并记录原因。

Executor：最少 2 个 Adapter；支持 step 超时/重试；失败能回写 exec_failed 并落 PARKED。

Verifier：SLO 查询正确；误报率可控（可配置阈值/窗口）。

Analyst：能从 metric_1m 发现简单异常并自动开 Case。

Sensor：locator 能被 evidence_link 回链；写入 p99 < 2s。

Librarian：topK 语义检索返回可用于 plan 注释/回滚提示。

三、首要实现的 API & 事件（最小集）

REST：

POST /case/create，PATCH /case/{id}/transition（含 Idempotency-Key/If-Match）。

POST /plan/generate，POST /gate/eval，POST /adapter/exec。

事件（Redpanda/NATS）：

evt.case.transition.v1（任何迁移）、evt.plan.proposed.v1、evt.plan.approved.v1、evt.exec.step_result.v1、evt.exec.done|failed.v1、evt.analysis.findings.v1。

四、第一条“可演示闭环”脚本（建议这样打通）

POST /case/create → NEW

PATCH /case/{id}/transition with start_analysis → ANALYZING

POST /plan/generate（场景1：canary or restart）→ 写 plan_proposed

PATCH ... EPlanReady（守卫：plan 完整）→ WAIT_GATE

POST /gate/eval（自动通过）→ 写 plan_approved + 触发 EGateApproved → EXECUTING

POST /adapter/exec（echo/k8s）→ 步骤完成后发 evt.exec.done.v1 → VERIFYING

Verifier 读 metric_1m 判定 → EVerifyPass → CLOSED（或失败→PARKED）

五、建议的代码分层与依赖

workflow/：纯函数 FSM（已给）

plans/：纯函数 Plan 生成（已给）

adapters/：执行适配器（k8s、gateway、script）

ports/：Outbox、Timeline、Idempotency、Repo（PG）接口

services/：Orchestrator/Planner/Gate/Executor/Analyst 聚合服务

api/：Gin/Fiber + OpenAPI；中间件（Tracing、Auth、Idem-Key）

internal/pubsub/：NATS/Redpanda 客户端（至少一次投递 + 去重）

六、风险与避坑（按优先级）

事务一致性：迁移、时间线、出盒必须在同一事务；外部 IO 禁止在 FSM 回调内做。

幂等：所有写接口必须支持 Idempotency-Key；执行器对 message_id 去重。

并发：SELECT … FOR UPDATE + OCC；重复迁移→409/412。

守卫松紧度：先“宽后紧”，避免卡死流程；策略放 Gatekeeper。

可观测：每模块先打指标再写业务；否则排障困难。

七、落地清单（可以直接拆 Issue/PR）

 workflow 与 plans 包落库；生成 OAS 草图与 mocks。

 Orchestrator：/case/* + outbox publisher + 三类测试。

 Planner：/plan/generate + plan_proposed 入库 + 守卫接线。

 Gatekeeper：/gate/eval + 本地策略 + plan_approved。

 Executor：/adapter/exec + adapter(script/k8s) + step runner + 事件回写。

 Verifier：查询 metric_1m → verify_checkpoint → 触发 EVerifyPass/Failed。

 Analyst：定时规则引擎最小实现 + /analyze/run。

 Sensor：/ingest/hint 写 oo_locator。

 Librarian：/kb/ingest + /kb/search（非 MVP 但可并行）。

结论：先做 Orchestrator → Planner → Gatekeeper → Executor → Verifier，用最少适配器跑通人工闭环；再补 Analyst/Sensor/Librarian，把闭环升级为“被动→主动→知识化”。这样每一步都有“可演示、可观测、可回滚”的价值增量，能快速把 AI OPS Agent 从概念落到可运行的产品骨架。
