:q!
概述（ETL-OO2PG & AGE 活跃调用图 & 拓扑/IaC/Ansible）

目标：在单二进制 Go 编排器内，完成三条主干 ETL 闭环：

OO2PG（近线聚合）：从 OpenObserve（S3/API）读取 logs/metrics/traces 明细 → 聚合为 metric_1m、service_call_5m、log_pattern_5m，维护 log_pattern 指纹库与 oo_locator 回查线索。

AGE 活跃调用图：以 service_call_5m 为源，刷新 AGE 图中 10 分钟活跃服务级调用边 (:Resource)-[:CALLS]->(:Resource)。

拓扑 ETL（IaC/Ansible）：从 Terraform/Pulumi/Cloud API 与 Ansible Playbook 抽取结构/应用依赖，以时态方式落入 topo_edge_time（开/关区间），可选同步到 AGE 作为 :STRUCT 边。

调度特性：窗口对齐 + 延迟容忍 + DAG 依赖 + 幂等 Upsert + 分片多租户。
可靠性：PG 唯一索引保证一次性；指数退避重试；失败熔断；事件补数回放。

2. 项目目录（合并版）

├─ etl/cmd/etl/               # 二进制入口/CLI
│  └─ main.go
├─ pkg/
│  ├─ scheduler/              # 调度器（窗口计算/派发）
│  ├─ runner/                 # Worker 池 + 执行/重试/回调
│  ├─ registry/               # Job 接口 + 注册中心 + DAG
│  ├─ store/                  # 状态/一次性保证/简易队列（PG）
│  ├─ window/                 # 时间对齐/窗口工具
│  ├─ events/                 # 事件入口（HTTP/CloudEvents风格）
│  ├─ oo/                     # OO 读取（S3 客户端 + 查询 API 适配）
│  ├─ agg/                    # 聚合器（指标1m / 调用5m / 指纹5m）
│  ├─ patterns/               # 日志指纹挖掘（Drain / RE2）
│  ├─ iac/                    # IaC/Cloud 归一（TF/Pulumi/aws/aliyun…）
│  ├─ ansible/                # Playbook/Inventory 解析与依赖抽取
│  └─ pgw/                    # PG 写入器（COPY 批量 + 幂等 upsert）
├─ jobs/
│  ├─ ooagg.go                # OO → metric_1m / call_5m / log_5m / locator
│  ├─ age_refresh.go          # 近10分钟活跃调用图刷新（依赖 ooagg）
│  ├─ topo_iac.go             # IaC/Cloud → topo_edge_time（时态差分）
│  └─ topo_ansible.go         # Ansible → topo_edge_time（时态差分）
├─ sql/
│  └─ age_refresh.sql         # AGE 刷新 SQL
└─ configs/
   └─ etl.yaml                # 调度/并发/延迟 配置

3. 表/模块总览（合并表）

| 模块        | API/入口                       | SRC（输入）                                | DEST（输出）                                                                 | 窗口/键           | 幂等/唯一约束                       | 备注                                   |
|-------------|--------------------------------|--------------------------------------------|-------------------------------------------------------------------------------|-------------------|-------------------------------------|----------------------------------------|
| pkg/oo      | Stream(ctx, tenant, w, fn)    | OO（S3 分区或查询 API）logs/metrics/traces | 回调 oo.Record                                                                | w=[From,To)       | —                                   | 统一时间/URN；并发读取                 |
| pkg/agg     | Feed(rec) / Drain()           | oo.Record                                  | Metrics1m / Calls5m / LogPatterns5m / PatternsUpsert / Locators               | 1m/5m             | —                                   | 内存桶聚合、指纹抽取                   |
| pkg/pgw     | Flush(ctx, tenant, w, out)    | 聚合结果 out                               | metric_1m、service_call_5m、log_pattern_5m、log_pattern、oo_locator、dim_resource | 1m/5m 主键        | ON CONFLICT DO UPDATE；oo_locator 唯一 | PG 批量 COPY + Upsert                  |
| jobs/ooagg  | Run(ctx, tenant, w)           | pkg/oo → pkg/agg                           | pkg/pgw.Flush                                                                 | Align=1m；Delay=2m | 由目标表主键保证                     | 成功后触发 age-refresh                 |
| sql/age_refresh.sql | cypher('ops', ...)     | service_call_5m 近10分钟                   | AGE 图 CALLS 边                                                               | 10 分钟           | MERGE 唯一匹配                       | e.last_seen/rps/err/p95                |
| jobs/age_refresh | Run(ctx, tenant, w)       | service_call_5m                            | 执行 SQL                                                                     | Align=5m           | —                                   | After()=["oo-agg"]                     |
| pkg/iac     | Discover(ctx, tenant)         | TF/Pulumi/Cloud API                        | 边集合 []Edge                                                                 | —                 | —                                   | 构造 URN、relation                     |
| pkg/ansible | ExtractDeps(ctx, tenant)      | inventory/group_vars/roles                 | 边集合 []Edge                                                                 | —                 | —                                   | 解析 upstream/连接串                   |
| pkg/pgw     | UpsertTopoEdges(ctx, tenant, edges) | iac/ansible 边                           | topo_edge_time（时态）                                                        | valid tstzrange    | PK(tenant,src,dst,rel,valid)         | 差分开/关区间                          |
| jobs/topo_iac | Run(ctx, tenant, w)         | pkg/iac.Discover                           | topo_edge_time                                                                | Align=15m          | —                                   | 结构拓扑                               |
| jobs/topo_ansible | Run(ctx, tenant, w)     | pkg/ansible.ExtractDeps                    | topo_edge_time                                                                | Align=1h           | —                                   | 应用依赖拓扑                           |
| pkg/events  | /events/enqueue               | CloudEvents/HTTP                           | etl_job_run 状态置 queued                                                     | 任意窗口           | ux_job_once                          | 手动补数/回放                          |
| pkg/store   | EnqueueOnce/Mark*             | —                                          | etl_job_run                                                                   | job/tenant/window  | ux_job_once                          | 一次性保证/队列                        |
| pkg/scheduler | Tick()                      | dim_tenant & etl_job_run                   | 入队窗口                                                                      | Align/Delay/Lookback | —                                  | 动态加载配置                           |




相关 PG 表（12 + ETL 元数据）

维度：dim_tenant、dim_resource

定位符：oo_locator

聚合：metric_1m（1m）、service_call_5m（5m）、log_pattern_5m（5m）

指纹：log_pattern

拓扑时态：topo_edge_time

知识：kb_doc、kb_chunk（向量）

事件：event_envelope、evidence_link

ETL 元数据：etl_job_run、etl_job_circuit

AGE：ops 图（Resource、CALLS）

4. 设计要点与边界

窗口推进：upper = floor(now - Delay, Align)，从上次成功窗口末尾或 initial_lookback 起步。

幂等写入：所有聚合表/计数表以主键覆盖；log_pattern 以 pattern hash 或唯一键 upsert；oo_locator 唯一组合避免重复。

资源归一（URN）：标准化 urn:k8s:svc:<ns>/<name> / urn:host:<host> / urn:db:postgres:<cluster>/<db>，并缓存 resource_id。

AGE 图只保活跃 10 分钟：长期时序仍在 service_call_5m 与 topo_edge_time。

拓扑差分：用 tstzrange 开/关区间维护时态；当边消失时关闭上次开区间。

5. Codex Tasks（落地清单）

每个任务包含：name / 描述 / 测试-验证。可直接拆成 Codex 指令执行（create-or-update-files / patch），或作为 PR checklist。

T1 — 完成 pkg/pgw.Flush（COPY + Upsert 批量写）

描述：实现资源 upsert、oo_locator 写入并回填 sample_ref；metric_1m/service_call_5m/log_pattern/log_pattern_5m 批量写入，全部 ON CONFLICT DO UPDATE。

测试/验证：

用本地 mock（见 T3）生成 3 个窗口数据；重复跑同一窗口，行数不增加、统计覆盖一致。

观察 etl_job_run 状态转为 success；冲突率 < 5%。

T2 — 实现 sql/age_refresh.sql 的程序化执行

描述：在 jobs/age_refresh.go 读取并执行 sql/age_refresh.sql，或用参数化 SQL 内嵌实现。

测试/验证：

先插入若干 service_call_5m 行，运行 age-refresh，检查 AGE 中 CALLS 边的 last_seen/rps/err_rate/p95。

连续两次执行，边数量不增长；属性按平均值更新。

T3 — pkg/oo.Stream 的 MOCK 与真实 S3/API 适配

描述：

MOCK：生成固定分布的 logs/metrics/traces；支持 --mock 开关。

S3：按 dataset/yyyy=.../hh=.../mm=... 列表对象；API：按时间窗查询。

测试/验证：

--mock 模式 1m 生成 5k 日志、500 指标点、1k span，端到端落库 < 3s/窗。

切换到 S3，验证窗口边界与对象命名正确。

T4 — pkg/agg 聚合正确性

描述：实现指标 1m avg/max/p95、调用 5m rps/err/p50/p95、日志指纹 5m 计数；接入 patterns.MineTemplate。

测试/验证：

单测：给定样本序列，校验统计量；给定 span/log 对，校验 A→B 聚合。

性能：10 万条日志 + 2 万 span，内存峰值 < 200MB。

T5 — pkg/patterns 指纹抽取（Drain/RE2）

描述：实现日志模板归一；输出 fingerprint、severity；可配置忽略字段。

测试/验证：

对同模式不同变量的日志返回同一指纹；错误日志 count_error 正确累积。

T6 — pkg/iac.Discover + pkg/ansible.ExtractDeps

描述：解析 TF/Pulumi/Cloud 与 inventory/group_vars/roles；映射 URN 与 DEPENDS_ON 边。

测试/验证：

构造样例仓库（或 JSON 快照），跑任务后 topo_edge_time 新增开区间；删除节点后再次运行，旧边区间关闭。

T7 — pkg/pgw.UpsertTopoEdges 时态差分

描述：实现 E_now 与当前开区间 E_prev 的集合差；新增插入开区间，消失关闭区间。

测试/验证：

连续两轮相同输入不新增边；去掉一个边后再次运行，原边 valid 上界从无穷变为 now()。

T8 — pkg/events 事件补数接口

描述：POST /events/enqueue 支持 {job, tenant_id, from, to} 回放窗口；写入 etl_job_run 为 queued 并入队。

测试/验证：

对已成功窗口再次 enqueue，应能覆盖写不冲突；对未来窗口入队，不会被执行（受 Delay 上界限制）。

T9 — 配置加载与初始回看

描述：pkg/config 已有；在 scheduler 注入 Cfg 并使用 tenants.initial_lookback.oo-agg 等。

测试/验证：

删除 etl_job_run 记录后启动，观察从 initial_lookback 开始补跑。

T10 — 观测指标与窗口滞后

描述：导出 Prometheus 指标（自定义 /metrics 或写回 OO）；记录窗口滞后。

测试/验证：

在稳定负载下，window_lag_seconds < Delay + Align；失败重试曲线可见。

附：典型验证 SQL

聚合正确性（1m 指标）

SELECT metric, count(*), min(bucket), max(bucket)
FROM metric_1m
GROUP BY metric ORDER BY count(*) DESC LIMIT 10;


活跃调用图 vs 源一致性

WITH active AS (
  SELECT src_resource_id, dst_resource_id
  FROM service_call_5m
  WHERE bucket >= now() - interval '10 minutes'
  GROUP BY 1,2
)
SELECT count(*) AS edges_in_source FROM active;
-- 再在 AGE 里查同样规模（CALLS 边数量），两者应近似一致（考虑租户过滤）


拓扑时态闭环

-- 当前有效边
SELECT count(*) FROM topo_edge_time WHERE upper_inf(valid);
-- 历史边（已关闭）
SELECT count(*) FROM topo_edge_time WHERE NOT upper_inf(valid);


