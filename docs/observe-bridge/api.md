## 模块 API 规划

### 数据流入口层

- **pkg/oo**
  - API: `Stream(ctx, tenant, rule, w, fn)`
  - 对应服务: `GET /oo/stream/{rule}?tenant={id}&from={t1}&to={t2}`
  - 请求参数:
    - `tenant`: 租户 ID。
    - `rule`: 聚合规则名称，例如 `log_error_count`、`cpu_avg`、`latency_p95_ms`。
    - `from`: 窗口起始时间 (RFC3339)。
    - `to`: 窗口结束时间 (RFC3339)。
  - 响应: HTTP `200`，返回以换行符分隔的 JSON 流，每行一个 `{"ts":"...","value":...}` 聚合点。
  - 说明: 提供可配置的聚合流接口，通过配置文件定义规则，避免重复封装查询。

- **pkg/agg**
  - API: `Feed(rec) / Drain()`
  - 内部接口: gRPC/Channel 调用，不直接暴露。
  - 行为: 按租户与窗口缓存聚合结果，`Drain()` 返回 map 供调度器入队，确保每条结果写一次。
  - 输出: 聚合后的指标 (Metrics1m, Calls5m 等)。

### 数据持久层

- **pkg/pgw**
  - API: `Flush(ctx, tenant, w, out)`
  - 对应服务: `POST /pgw/flush`
  - 输入: 聚合结果 out (JSON/Parquet)
  - 输出: 写入 PG (`metric_1m`, `service_call_5m` 等)
  - 幂等: `ON CONFLICT DO UPDATE`

- **pkg/pgw.UpsertTopoEdges**
  - API: `UpsertTopoEdges(ctx, tenant, edges)`
  - 对应服务: `POST /pgw/topo/edges`
  - 输入: IAC/Ansible 边集合
  - 输出: `topo_edge_time` 时态表，支持差分。

### 定时任务

- **jobs/ooagg**
  - 调用链: `pkg/oo → pkg/agg → pkg/pgw.Flush`
  - 调度: 每分钟触发，延迟 2 分钟。
  - 注册 API: `POST /jobs/ooagg/run?tenant={id}&window={w}`

- **jobs/age_refresh**
  - 调度: 每 5 分钟。
  - 动作: 执行 `sql/age_refresh.sql`，更新 AGE 图。
  - 注册 API: `POST /jobs/age_refresh/run?tenant={id}&window={w}`

- **jobs/topo_iac**
  - API: `Run(ctx, tenant, w)`
  - 调度: 每 15 分钟。
  - 对应服务: `POST /jobs/topo/iac/run`

- **jobs/topo_ansible**
  - API: `Run(ctx, tenant, w)`
  - 调度: 每小时。
  - 对应服务: `POST /jobs/topo/ansible/run`

### 配置/调度与事件

- **pkg/events**
  - API: `/events/enqueue`
  - 对应服务: `POST /events/enqueue`
  - 输入: CloudEvents
  - 动作: 状态置 `etl_job_run=queued`

- **pkg/store**
  - API: `EnqueueOnce/Dequeue/MarkDone`
  - 对应服务: 内部库调用
  - 保证: `ux_job_once`，避免重复入队。

- **pkg/scheduler**
  - API: `Tick(ctx)`
  - 对应服务: `POST /scheduler/tick`
  - 动作: 从 `agg` Drain 结果入队并调用 `pgw.Flush` 批量写入，支持重试。

### 基础拓扑发现

- **pkg/iac**
  - API: `Discover(ctx, tenant)`
  - 对应服务: `GET /topo/iac/discover?tenant={id}`
  - 输出: 边集合 []Edge。

- **pkg/ansible**
  - API: `ExtractDeps(ctx, tenant)`
  - 对应服务: `GET /topo/ansible/extract?tenant={id}`
  - 输出: 边集合 []Edge。
