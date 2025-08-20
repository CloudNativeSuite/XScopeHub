# XInsight

XInsight combines vector similarity search with graph analytics to assist incident
response. PostgreSQL with pgvector stores semantic objects such as log segments
or knowledge base fragments, while Apache AGE maintains service dependency
relationships.

## Schema

Migration `0003_pgvector_semantic_objects.sql` creates the `semantic_objects`
table and its indices. `0004_age_init.sql` initializes the AGE graph named
`xinsight`.

## Queries

### Top-K Similarity

```sql
SELECT id, object_type, service, ts, title,
       1 - (embedding <=> :qvec) AS score
FROM semantic_objects
WHERE object_type IN ('alert', 'log', 'doc')
ORDER BY embedding <=> :qvec
LIMIT 20;
```

### Service Dependency

```sql
SELECT * FROM cypher('xinsight', $$
  MATCH p = (s:Service {name: $svc})-[:CALLS*1..3]->(t:Service)
  RETURN p
$$) AS (p agtype);
```

These building blocks allow implementing a search → discovery → root cause
analysis workflow across metrics, logs and traces.

*XInsight（深度分析引擎）**的可落地设计：在 PostgreSQL 里用 pgvector 做向量检索，在 AGE（Apache AGE） 做属性图查询，并和 PG/CH 的现有观测数据联动。

目标

用 pgvector 做相似度检索（日志/告警/知识库/变更记录等的语义搜索、相似事故召回）。

用 AGE（PG 图扩展，openCypher）做服务依赖、根因路径、传播链等图分析。

与现有 TimescaleDB（热指标）、ClickHouse（日志/链路/长期指标）协同，形成检索 → 发现 → 溯源闭环。

一、PG 向量库（pgvector）— 语义检索
1) 扩展 & 统一向量表
-- 安装扩展
CREATE EXTENSION IF NOT EXISTS vector;   -- pgvector
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 统一语义对象：日志片段/告警/知识文档/变更记录/Playbook 等
-- 说明：
--   object_type: 'log' | 'alert' | 'doc' | 'trace' | 'metric_anomaly' ...
--   ref_*: 指向源数据（CH/PG）定位键，便于回跳原始记录
CREATE TABLE IF NOT EXISTS semantic_objects (
  id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  object_type  TEXT NOT NULL,
  service      TEXT,
  host         TEXT,
  trace_id     TEXT,
  span_id      TEXT,
  ts           TIMESTAMPTZ NOT NULL DEFAULT now(),
  title        TEXT,
  content      TEXT NOT NULL,           -- 原文（日志片段/文档段落等）
  labels       JSONB,
  embedding    vector(1024) NOT NULL,   -- 选用你的嵌入维度，如 768/1024/1536
  ref_source   TEXT,                    -- 'clickhouse:logs_events' / 'postgres:metrics_points' 等
  ref_key      JSONB                    -- 记录源表定位键（如 {ts, service, trace_id, ...}）
);

-- 典型索引
CREATE INDEX IF NOT EXISTS idx_semobj_ts ON semantic_objects (ts DESC);
CREATE INDEX IF NOT EXISTS idx_semobj_service_ts ON semantic_objects (service, ts DESC);

-- 近似向量索引（pgvector）
-- 选择余弦距离：vector_cosine_ops；lists 需按数据量微调（10~200）
CREATE INDEX IF NOT EXISTS idx_semobj_embed_cosine
ON semantic_objects
USING ivfflat (embedding vector_cosine_ops)
WITH (lists = 100);


备注：若使用 pgvector 新版支持 HNSW，可改为 USING hnsw(embedding vector_cosine_ops) WITH (m=16, ef_construction=200)；不确定版本时就先用 ivfflat。

2) 语义检索（Top-K）
-- 传入查询向量 :qvec（由服务端生成）
-- 限定服务 + 时间窗，减少搜索空间
SELECT id, object_type, service, ts, title, content, labels,
       1 - (embedding <=> :qvec) AS score   -- 余弦相似度的“相似度分数”
FROM semantic_objects
WHERE service = :svc
  AND ts >= now() - interval '7 days'
ORDER BY embedding <=> :qvec               -- 越小越近
LIMIT 50;

3) 混合检索（向量 + 关键词/全文）
-- 使用全文索引可选：为 content 建 tsvector
ALTER TABLE semantic_objects
  ADD COLUMN fts tsvector GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED;
CREATE INDEX IF NOT EXISTS idx_semobj_fts ON semantic_objects USING GIN(fts);

-- 混合：先向量召回TopN，再按文本分值重排（简单示例）
WITH v AS (
  SELECT id, 1 - (embedding <=> :qvec) AS vscore
  FROM semantic_objects
  WHERE ts >= now() - interval '30 days'
  ORDER BY embedding <=> :qvec
  LIMIT 200
)
SELECT s.id, s.object_type, s.service, s.ts, s.title, s.content,
       v.vscore,
       ts_rank(s.fts, plainto_tsquery(:qtext)) AS tscore
FROM v JOIN semantic_objects s USING(id)
ORDER BY (vscore * 0.7 + tscore * 0.3) DESC
LIMIT 50;

4) 与 CH/PG 源数据回跳

ref_source + ref_key 保存可定位原始行的信息：

日志（CH logs_events）：{ "timestamp": "...", "service": "...", "trace_id": "..." }

指标异常（PG/Timescale）或 metrics_1m（CH）同理。

前端/Grafana Data Links 用它们拼接跳转 URL。

二、PG 图扩展（Apache AGE）— 依赖&根因图
1) 安装与建图
CREATE EXTENSION IF NOT EXISTS age;
LOAD 'age';

-- 创建图空间
SELECT * FROM create_graph('xinsight');

2) 节点/边（建议的属性图模型）

节点（label）：

Service{ name, team, tier, labels }

Host{ name, az, labels }

Endpoint{ path, method, svc }

Incident{ id, ts, severity, summary }

Trace{ trace_id, ts, svc }

边（type）：

(:Service)-[:RUNS_ON]->(:Host)

(:Service)-[:CALLS]->(:Service)（由 Trace 拓扑学习或离线拓扑导入）

(:Service)-[:EXPOSES]->(:Endpoint)

(:Incident)-[:AFFECTS]->(:Service)

(:Trace)-[:BELONGS_TO]->(:Service)

(:Service)-[:RELATED_LOGS]->(:LogTemplate)（可选，若把日志模板也当节点）

3) 基础写入示例（openCypher）
-- 新建服务与依赖
SELECT * FROM cypher('xinsight', $$
  MERGE (a:Service {name: 'checkout'})
  MERGE (b:Service {name: 'payment'})
  MERGE (a)-[:CALLS]->(b)
  RETURN a,b
$$) AS (a agtype, b agtype);

4) 典型图查询

影响半径（k跳以内）

SELECT * FROM cypher('xinsight', $$
  MATCH p = (s:Service {name: $svc}) -[:CALLS*1..3]-> (t:Service)
  RETURN p
$$) AS (p agtype);


结合指标异常：定位最可能根因服务

-- 思路：取异常服务 S，向外扩 CALLS；结合近 10 分钟内各服务错误日志/Span 错误计数，排序返回最可疑路径
-- 图部分（路径） + 外部聚合（CH/PG）在应用层拼接；或把计数结果写回 AGE 为边/点属性再查。


服务与主机映射

SELECT * FROM cypher('xinsight', $$
  MATCH (s:Service)-[:RUNS_ON]->(h:Host)
  WHERE s.name = $svc
  RETURN s, h
$$) AS (s agtype, h agtype);

三、与 Timescale / ClickHouse 的协同
写入与同步

Metrics 原始点值/直方图 → PG/Timescale（热层），用于实时查询与告警。

Logs & Spans → CH（高吞吐、长留存，配合对象存储分层）。

XInsight 同步：

从 CH/PG 抽取“语义对象”（异常窗口内的日志片段、告警摘要、变更说明、Playbook 片段），编码为 embedding → 落 PG semantic_objects。

从 Trace/依赖生成 CALLS 图，周期性合并到 AGE（或增量更新）。

查询与联动

第一跳：用户输入自然语言/关键字 → semantic_objects 召回“相似事故/日志/知识片段”。

第二跳：对召回结果关联 trace_id/service → 图上跑最短/最密路径、扇出/扇入、k-hop 影响。

第三跳：拉取 PG/CH 原始数据（面板联动）验证假设。

四、仓库落地（新增/调整）
xscopehub/
├─ cmd/insight/                         # XInsight API: 统一封装 语义检索 + 图查询
├─ internal/analytics/vector/           # pgvector DAO、TopK/HYBRID 查询、重排
├─ internal/analytics/graph/            # AGE DAO、常用 Cypher 模板
├─ migrations/postgres/
│  ├─ 0003_pgvector_semantic_objects.sql
│  └─ 0004_age_init.sql                 # create_graph / 初始 schema 脚本（可选）
├─ scripts/
│  ├─ embed_log_segments.py             # 从 CH 抽取日志片段，生成嵌入，写 PG
│  └─ build_call_graph.go               # 从 Traces 构建 CALLS 边，写 AGE
├─ configs/insight.yaml                 # 索引参数、模型维度、检索权重、时间窗等
└─ docs/insight.md                      # 使用说明与查询示例


迁移脚本示例（0003_pgvector_semantic_objects.sql）

CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS semantic_objects (
  id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  object_type  TEXT NOT NULL,
  service      TEXT,
  host         TEXT,
  trace_id     TEXT,
  span_id      TEXT,
  ts           TIMESTAMPTZ NOT NULL DEFAULT now(),
  title        TEXT,
  content      TEXT NOT NULL,
  labels       JSONB,
  embedding    vector(1024) NOT NULL,
  ref_source   TEXT,
  ref_key      JSONB
);
CREATE INDEX IF NOT EXISTS idx_semobj_ts ON semantic_objects (ts DESC);
CREATE INDEX IF NOT EXISTS idx_semobj_service_ts ON semantic_objects (service, ts DESC);
CREATE INDEX IF NOT EXISTS idx_semobj_embed_cosine
  ON semantic_objects USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);


AGE 初始化示例（0004_age_init.sql）

CREATE EXTENSION IF NOT EXISTS age;
LOAD 'age';
SELECT * FROM create_graph('xinsight');
-- 节点/边在业务侧用 cypher 动态创建（更灵活）

五、查询范式（开箱即用）

A. 找“这次告警”最像的历史事故/日志

-- :qvec 来自告警上下文拼接后的文本 embedding
SELECT id, object_type, service, ts, title, 1 - (embedding <=> :qvec) AS score
FROM semantic_objects
WHERE object_type IN ('alert','log','doc')
  AND ts >= now() - interval '180 days'
ORDER BY embedding <=> :qvec
LIMIT 20;


B. 围绕异常服务的根因路径（3 跃点内）

SELECT * FROM cypher('xinsight', $$
  MATCH p = (s:Service {name: $svc})-[:CALLS*1..3]->(t:Service)
  RETURN p
  ORDER BY length(p) ASC
  LIMIT 50
$$) AS (p agtype);


C. 将 A 的结果聚焦到图上邻域（按服务聚类）

把 A 里召回的 service 做频次统计，取 Top-N 服务作为图查询的起点/终点，再跑 B。

六、实施建议

嵌入维度统一（如 1024/1536），便于索引与批处理；不同模型可拆多表。

近似索引参数：lists/probes（ivfflat）或 m/ef_search（hnsw）要压测后定。

语料切分：日志按模板/窗口切片（如 5–20 行），文档按段落；避免向量过长。

一致性：semantic_objects 的 ref_key 必须能唯一定位 CH/PG 原始行（设计好主键组合）。

安全多租户：把 tenant_id 加入 semantic_objects 与 AGE 节点属性，并在查询层过滤。
