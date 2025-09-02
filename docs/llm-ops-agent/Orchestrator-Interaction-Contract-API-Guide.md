Orchestrator 核心放在中间，说明它与各模块/API的交互边界、方向（同步/异步）、触发时机与幂等/并发约束。给你两种集成样式：事件驱动（推荐）与直连 REST（过渡/兜底）——实际可并存。

1. Orchestrator 的接口面

对外暴露（被其它模块/人调用）

POST /case/create：创建 Case（初始 NEW）

GET /case/{id}：查询 Case（含时间线、当前状态）

PATCH /case/{id}/transition：唯一的状态迁移入口
头部：Idempotency-Key（必选）＋ If-Match: W/"<version>"（建议）
语义：原子迁移 + 写时间线 + 写 outbox（事务内）

对外发布（Orchestrator 主动发）

Outbox → 事件总线（至少一次投递）

evt.case.transition.v1（任何迁移都发）

cmd.plan.generate（进入 PLANNING 时可发）

cmd.gate.eval（计划“准备好”时可发）

cmd.exec.run（进入 EXECUTING 前可发）

cmd.verify.run（进入 VERIFYING 时可发）

原则：所有副作用都经由 Outbox，HTTP 调用仅作兜底或人机界面。

2. 事件驱动对接（推荐）
   2.1 典型正向流（NEW → CLOSED）
   Analyst → Orchestrator → Planner → Gatekeeper → Executor → Verifier

(a) 分析拉起

Analyst 发现异常 → 调用 PATCH /case/{id}/transition 触发 start_analysis

Orchestrator: NEW→ANALYZING，发 evt.case.transition.v1

(b) 产出计划

（可选）Orchestrator 在 ANALYZING→PLANNING 后，发 cmd.plan.generate

Planner 消费 cmd.plan.generate → POST /plan/generate 自己落库 → 发 evt.plan.proposed.v1

Orchestrator 订阅 evt.plan.proposed.v1 → PATCH /case/{id}/transition (plan_ready)
守卫：plan 完整（steps+rollback+verify）→ WAIT_GATE

(c) 审批通过

Orchestrator 发 cmd.gate.eval

Gatekeeper 评估后发 evt.plan.approved.v1

Orchestrator 收到后 PATCH (gate_approved) → EXECUTING

(d) 执行与验收

Orchestrator 发 cmd.exec.run

Executor 逐步执行，流转 evt.exec.step_result.v1；完成发 evt.exec.done|failed.v1

Orchestrator 收到 done → PATCH (exec_done) → VERIFYING; 收到 failed → PATCH (exec_failed) → PARKED

Orchestrator 发 cmd.verify.run

Verifier 发 evt.verify.pass|failed.v1

Orchestrator PATCH (verify_pass|verify_failed) → CLOSED 或 PARKED

任何失败都汇聚到 PARKED，保留人工/重试入口。

2.2 这些事件谁来订阅？

Planner/Gatekeeper/Executor/Verifier 订阅各自的 cmd.\*；

Orchestrator 订阅 evt.plan._、evt.exec._、evt.verify.\*；

所有模块都写自己的表，但状态收敛只能通过 Orchestrator 的 PATCH transition。

3. 直连 REST（兜底/人机界面）

当你尚未接通总线或为了便捷调试时，各模块也可直接调用 Orchestrator：

Planner 生成完成后：PATCH /case/{id}/transition with event=plan_ready

Gatekeeper 审批通过：PATCH /case/{id}/transition with event=gate_approved

Executor 执行完成/失败：PATCH /case/{id}/transition with event=exec_done|exec_failed

Verifier 验证通过/失败：PATCH /case/{id}/transition with event=verify_pass|verify_failed

两种模式可同时启用：事件优先，REST 作为回退与人工操作。

4. 交互契约（简表）
   模块 → Orchestrator 入参（REST/事件） Orchestrator 动作 出参/副作用
   Analyst PATCH /case/{id}/transition start*analysis NEW→ANALYZING evt.case.transition.v1
   Planner evt.plan.proposed.v1 或 PATCH plan_ready 守卫校验后 PLANNING→WAIT_GATE evt.case.transition.v1
   Gatekeeper evt.plan.approved.v1 或 PATCH gate_approved WAIT_GATE→EXECUTING evt.case.transition.v1
   Executor `evt.exec.done failed.v1**或**PATCH exec*\_`	to VERIFYING 或 PARKED
Verifier	`evt.verify.pass failed.v1**或**PATCH verify\_\_` CLOSED 或 PARKED
   Human/UI PATCH force_park / start_analysis 见 FSM 时间线+事件
5. 幂等 & 并发（每次 PATCH transition 都必须）

Idempotency-Key：同一键重复调用→返回首个结果（不重复写时间线/出盒）。

If-Match 版本：不匹配→409/412；防止并发覆盖。

事务原子性：状态更新 + 时间线 + Outbox 同一事务提交；否则回滚。

6. 两个“从端到端”例子（超精简 cURL）

A. 人工直连（演示/调试）

# 1) 创建

CASE=$(curl -sX POST /case/create -d '{"title":"p95 spike","tenant":"t-001"}'|jq -r .data.case_id)

# 2) 开始分析

curl -X PATCH /case/$CASE/transition -H 'Idempotency-Key:a1' \
 -d '{"event":"start_analysis","actor":"analyst@svc"}'

# 3) 计划就绪（Planner 完成后）

curl -X PATCH /case/$CASE/transition -H 'Idempotency-Key:a2' \
 -d '{"event":"plan_ready","actor":"planner@svc"}'

# 4) 审批通过

curl -X PATCH /case/$CASE/transition -H 'Idempotency-Key:a3' \
 -d '{"event":"gate_approved","actor":"gate@svc"}'

# 5) 执行完成

curl -X PATCH /case/$CASE/transition -H 'Idempotency-Key:a4' \
 -d '{"event":"exec_done","actor":"exec@svc"}'

# 6) 验证通过 → CLOSED

curl -X PATCH /case/$CASE/transition -H 'Idempotency-Key:a5' \
 -d '{"event":"verify_pass","actor":"verifier@svc"}'

B. 事件驱动（生产推荐）

Orchestrator 迁移到 PLANNING 时发 cmd.plan.generate

Planner → evt.plan.proposed.v1

Orchestrator 自动 plan_ready → WAIT_GATE，并发 cmd.gate.eval

Gatekeeper → evt.plan.approved.v1 → Orchestrator gate_approved → EXECUTING → 发 cmd.exec.run

Executor → evt.exec.done.v1 → Orchestrator exec_done → 发 cmd.verify.run

Verifier → evt.verify.pass.v1 → Orchestrator verify_pass → CLOSED

7. 与数据层的绑定（关键表）

读/写：ops_case（含 version）、case_timeline、outbox、idempotency

读：plan_proposed / plan_approved、exec_run / exec_step、verify_checkpoint

证据链：event_envelope / evidence_link / oo_locator（时间线里放引用）

一句话总结：
Orchestrator 只做两件事——判定合法迁移（FSM 纯函数） + 一次事务内记录（状态/时间线/outbox）。
其余模块要么通过 事件被它驱动、要么通过 PATCH 告诉它“阶段已完成”。这样既能事件解耦，又保留人工/调试的直连路径。
