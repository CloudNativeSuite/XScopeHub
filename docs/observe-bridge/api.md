## 模块 API 规划

### 数据流入口层
- **pkg/oo**
  - API: `Stream(ctx, tenant, w, fn)`
  - 对应服务: `GET /oo/stream?tenant={id}&from={t1}&to={t2}`
  - 说明: 按窗口流式获取 OO (logs/metrics/traces)，回调 fn(oo.Record)。

- **pkg/agg**
  - API: `Feed(rec) / Drain()`
  - 内部接口: gRPC/Channel 调用，不直接暴露。
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
  - API: `EnqueueOnce/Mark*`
  - 对应服务: 内部库调用
  - 保证: `ux_job_once`，避免重复入队。

- **pkg/scheduler**
  - API: `Tick()`
  - 对应服务: `POST /scheduler/tick`
  - 输入: `dim_tenant & etl_job_run`
  - 输出: 入队窗口任务。

### 基础拓扑发现
- **pkg/iac**
  - API: `Discover(ctx, tenant)`
  - 对应服务: `GET /topo/iac/discover?tenant={id}`
  - 输出: 边集合 []Edge。

- **pkg/ansible**
  - API: `ExtractDeps(ctx, tenant)`
  - 对应服务: `GET /topo/ansible/extract?tenant={id}`
  - 输出: 边集合 []Edge。
