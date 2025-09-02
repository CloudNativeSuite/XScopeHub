# API Reference

本仓库提供 AIOps 服务基础骨架，包含健康检查 `/healthz` 与指标端点 `/metrics`，并接入 Gin、OpenAPI、NATS、Redpanda、TimescaleDB 与 OpenTelemetry。

本项目遵循模块化架构，关键路径如下：

Sensor → Analyst → Planner → Gatekeeper → Executor → Librarian → Orchestrator

下表描述了各模块的职责、输入输出及接口定义：

| 模块         | 职责       | 输入             | 输出                    | 存储   | 接口             | SLO            |
| :----------- | :--------- | :--------------- | :---------------------- | :----- | :--------------- | :------------- |
| Sensor       | 接入信号   | OTLP、Prom、logs | OO 明细                 | OO、PG | `/ingest/*`      | 写入 p99<2s    |
| Analyst      | 异常检测   | OO 明细、PG      | 聚合、analysis.findings | PG     | `/analyze/run`   | 10min 完成聚合 |
| Planner      | 生成计划   | kb_chunk、证据   | plan.proposed           | PG     | `/plan/generate` | <30s 首版计划  |
| Gatekeeper   | 策略评估   | plan.proposed    | plan.approved           | PG     | `/gate/eval`     | 自动评估<1s    |
| Executor     | 执行动作   | plan.approved    | exec.step.result        | OO、PG | `/adapter/exec`  | 单步 15m 超时  |
| Librarian    | 知识沉淀   | 日志、diff       | kb_doc、kb_chunk        | PG     | `/kb/ingest`     | 5min 内可检索  |
| Orchestrator | 状态机调度 | 各类事件         | case.updates            | PG     | `/case/*`        | 状态原子迁移   |

# Orchestrator 中心化后：接口总览（UI vs 内部）

下面把人机 UI 可见的 HTTP API，与内部调用接口（服务间 REST & 事件总线）分开列出，并给出对应模块与函数/处理器建议名。读完即可据此建路由与订阅器。

# UI / 外部可见的 HTTP API（给人机或外部系统用）

Endpoint Method 模块 职责 关键语义 Go 处理器/服务函数（建议名）
/case/create POST Orchestrator 创建 Case（初始 NEW） 仅建档，不改其他表 api.CreateCaseHandler → orchestrator.CreateCase(ctx, in)
/case/{id} GET Orchestrator 查询 Case 返回状态、版本、标签等 api.GetCaseHandler → orchestrator.GetCase(ctx, id)
/case/{id}/timeline GET Orchestrator 时间线查询 只读 api.ListTimelineHandler → orchestrator.ListTimeline(ctx, id)
/case/{id}/transition PATCH Orchestrator 唯一状态迁移入口 要求 Idempotency-Key，支持 If-Match api.TransitionHandler → orchestrator.Transition(ctx, id, event, meta)
/plan/{id} GET Planner 读取已生成的 Plan 只读（DSL/元数据） api.GetPlanHandler → planner.Get(ctx, id)
/exec/{id} GET Executor 执行状态/步骤 只读（流水/日志引用） api.GetExecHandler → executor.Status(ctx, id)
/kb/search GET Librarian 知识检索 只读 api.KBSearchHandler → librarian.Search(ctx, q, topk)
/kb/ingest POST Librarian 知识入库 可对外开放或内用 api.KBIngestHandler → librarian.Ingest(ctx, doc)
/healthz GET All 存活检查 200/非200 api.HealthHandler
/metrics GET All Prom 指标 Text/Prom 由 exporter 暴露

原则：只有 Orchestrator 的 PATCH /transition 能改 Case 状态；其他模块的 UI API 均为只读或产生“自己的领域数据”，不写 ops_case。

# 内部服务间 REST（可选，调试/回退通道）

Endpoint Method 模块 职责 备注 处理器/函数
/plan/generate POST Planner 基于输入生成计划（写 plan_proposed） 不改 Case 状态 plannerapi.GenerateHandler → planner.Generate(ctx, in)
/gate/eval POST Gatekeeper 策略评估（写 gate_decision/plan_approved） 不改 Case 状态 gateapi.EvalHandler → gatekeeper.Evaluate(ctx, plan)
/adapter/exec POST Executor 启动执行（写 exec_run/exec_step） 返回 202+exec_id execapi.RunHandler → executor.Execute(ctx, planID, opts)
/adapter/exec/{exec_id} GET Executor 查询执行进度 只读 execapi.StatusHandler → executor.Status(ctx, id)
/verify/run POST Verifier 触发验证（写 verify_checkpoint） 不改 Case 状态 verifyapi.RunHandler → verifier.Run(ctx, execID)
/analyze/run POST Analyst 触发一次分析任务 产出 findings 事件 analystapi.RunHandler → analyst.Run(ctx, in)
/ingest/hint POST Sensor 记录 OO 索引 写 oo_locator sensorapi.HintHandler → sensor.WriteHint(ctx, in)

这些内部 REST 最终都通过事件让 Orchestrator 执行 transition；当总线未就绪时可用 REST 直连作为临时方案。

事件总线（推荐的内部主通道）

命名约定：命令 cmd._（NATS/JetStream），事件 evt._（Redpanda/Kafka）

Subject/Topic 方向 模块 语义 Orchestrator 对应动作 订阅/发布函数（建议名）
evt.case.transition.v1 发布 Orchestrator 任何状态迁移的事实 （供审计/订阅） orchestrator.PublishCaseTransition(ctx, msg)
cmd.plan.generate 发布 Orchestrator 请求 Planner 产出计划 等待 evt.plan.proposed.v1 orchestrator.EmitPlanGenerate(ctx, caseID)
evt.plan.proposed.v1 订阅 Planner 计划已生成（含 DSL） PATCH transition(plan_ready) orchestrator.OnPlanProposed(ctx, evt)
cmd.gate.eval 发布 Orchestrator 请求 Gatekeeper 评估 等待 evt.plan.approved.v1 orchestrator.EmitGateEval(ctx, planID)
evt.plan.approved.v1 订阅 Gatekeeper 审批通过/拒绝 gate_approved / gate_rejected orchestrator.OnPlanApproved(ctx, evt)
cmd.exec.run 发布 Orchestrator 请求 Executor 执行 等待执行结果 orchestrator.EmitExecRun(ctx, planID)
evt.exec.step_result.v1 订阅 Executor 步骤产出 （更新 exec 表，非必需迁移） orchestrator.OnExecStep(ctx, evt)
evt.exec.done.v1 / evt.exec.failed.v1 订阅 Executor 执行完成/失败 exec_done / exec_failed orchestrator.OnExecDone(ctx, evt)
cmd.verify.run 发布 Orchestrator 请求 Verifier 校验 等待校验事件 orchestrator.EmitVerifyRun(ctx, execID)
evt.verify.pass.v1 / evt.verify.failed.v1 订阅 Verifier 验证结果 verify_pass / verify_failed orchestrator.OnVerifyResult(ctx, evt)
evt.analysis.findings.v1 订阅 Analyst 异常发现（证据） 通常驱动 start_analysis → PLANNING orchestrator.OnFindings(ctx, evt)

所有 evt.\* 到达 Orchestrator 后，最终都通过其本地事务调用 Transition()，写 ops_case + case_timeline + outbox。

# 模块与核心函数（汇总）

模块 关键函数（服务层） 主要读写表
Orchestrator CreateCase, GetCase, ListTimeline, Transition（调用 workflow.Decide 纯函数） ops_case, case_timeline, outbox, idempotency
Planner Generate（产出 DSL 与工件意图） plan_proposed
Gatekeeper Evaluate（写决策&批准） gate_decision, plan_approved
Executor Execute, Status（step runner + 适配器） exec_run, exec_step（日志引用指向 oo_locator）
Verifier Run（SLO 校验并记录检查点） verify_checkpoint, 读 metric_1m
Analyst Run（窗口聚合与规则） 读 metric_1m/log_pattern_5m，写 event_envelope/evidence_link
Sensor WriteHint oo_locator
Librarian Ingest, Search kb_doc, kb_chunk
最小规范（适用于所有“改状态/落地副作用”的写接口）

Idempotency-Key：必传；命中直接返回首个结果。

If-Match（或 expected_version）：并发冲突返回 409/412。

单事务：状态更新 + 时间线 + outbox 同一事务提交，失败全回滚。

审计：每次迁移发布 evt.case.transition.v1，并把 actor/reason/correlation_id 写入 timeline。

有了这三张表 + 两组函数名，你就能把路由/订阅器一次性搭好，随后逐模块替换内部实现即可。

简述最新的接口设计和Orchestrator：

三类集成测试全绿；Prom 指标：case_transition_total / conflict_total / idempotent_replay_total 上报；Outbox 消费无重复副作用。

    •	Planner：plan_proposed 入库，DSL 经 schema 校验；守卫能挡住不完整计划。
    •	Gatekeeper：策略可配置（本地文件/内存），自动审批延时 <1s；拒绝能落 PARKED 并记录原因。
    •	Executor：最少 2 个 Adapter；支持 step 超时/重试；失败能回写 exec_failed 并落 PARKED。
    •	Verifier：SLO 查询正确；误报率可控（可配置阈值/窗口）。
    •	Analyst：能从 metric_1m 发现简单异常并自动开 Case。
    •	Sensor：locator 能被 evidence_link 回链；写入 p99 < 2s。
    •	Librarian：topK 语义检索返回可用于 plan 注释/回滚提示。的关系

# 聚焦边界、事件与达标条件（DoD）。

Orchestrator（状态唯一事实源，SSOT）

唯一改状态入口：PATCH /case/{id}/transition（Idempotency-Key + If-Match）。

发布（Outbox）：evt.case.transition.v1、cmd.plan.generate、cmd.gate.eval、cmd.exec.run、cmd.verify.run。

订阅：evt.plan.proposed.v1、evt.plan.approved.v1、evt.exec.done|failed.v1、evt.verify.pass|failed.v1、evt.analysis.findings.v1。

DoD：三类集成测全绿（非法迁移409 / 幂等重放 / 失败落PARKED）；Prom 指标上报
case_transition_total、conflict_total、idempotent_replay_total；Outbox 消费无重复副作用（至少一次投递 + 消费者幂等）。

模块 → 接口/事件 → 与 Orchestrator 的关系（含 DoD）

Planner

接口：POST /plan/generate（入库 plan_proposed，DSL 通过 schema 校验）。

事件：发 evt.plan.proposed.v1。

关系：Orchestrator 收到后触发 plan_ready 守卫；不完整计划被守卫挡下（不准入 WAIT_GATE）。

Gatekeeper

接口：POST /gate/eval（可配置策略：本地文件/内存）。

事件：发 evt.plan.approved.v1（或拒绝原因）。

关系：Orchestrator 据此 gate_approved → EXECUTING；拒绝→PARKED 且时间线记录原因。

DoD：自动审批延时 <1s。

Executor

接口：POST /adapter/exec，GET /adapter/exec/{exec_id}。

事件：evt.exec.step_result.v1、evt.exec.done|failed.v1。

关系：Orchestrator 收到 done/failed → exec_done/failed；失败一律汇聚到 PARKED。

DoD：≥2 个 Adapter（如 k8s、script）；支持 step 超时/重试；stdout/stderr 落 OO（exec_step.\*\_ref）。

Verifier

接口：POST /verify/run。

事件：evt.verify.pass|failed.v1。

关系：Orchestrator verify_pass → CLOSED / verify_failed → PARKED。

DoD：SLO 查询正确；阈值/窗口可配置，误报率可控。

Analyst

接口：POST /analyze/run 或定时任务。

事件：evt.analysis.findings.v1。

关系：驱动新 Case（/case/create）或现有 Case start_analysis → ANALYZING → 触发 cmd.plan.generate。

DoD：能从 metric_1m 发现简单异常并自动开 Case。

Sensor

接口：POST /ingest/hint（写 oo_locator）。

关系：被 evidence_link.ref_oo 回链，用于证据检索。

DoD：写入 p99 < 2s。

Librarian

接口：/kb/ingest、/kb/search（pgvector）。

关系：Planner 在生成计划与回滚提示时调用检索，不直接改 Case 状态。

DoD：返回 topK 语义检索结果可用于计划注释/回滚建议。

端到端主流程（事件驱动）

Analyst → Orchestrator(start*analysis) → Planner(plan_proposed) → Orchestrator(plan_ready) → Gatekeeper(approved) → Orchestrator(gate_approved→EXECUTING) → Executor(done/failed) → Orchestrator(exec*\* → VERIFYING/PARKED) → Verifier(pass/failed) → Orchestrator(CLOSED/PARKED)

规则：状态只经 Orchestrator 改变；各子模块只产出“事实/决定/结果”事件。守卫保证计划完备性，策略决定执行资格，执行与验证给出结果，Orchestrator 负责原子落库与审计。
