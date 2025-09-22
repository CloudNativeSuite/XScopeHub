# Observe Gateway 部署与运维指南

本指南面向需要部署与维护 `observe-gateway` 统一查询网关的工程师，介绍运行架构、配置项、部署方式以及测试校验流程。网关通过单一的 POST /api/query 接口代理 PromQL、LogQL 与 TraceQL 查询，并提供 JWT 鉴权、租户限流、查询缓存和审计日志等能力。

## 组件与依赖

| 组件 | 说明 |
| ---- | ---- |
| Observe Gateway | 本仓库 `observe-gateway/cmd/gateway` Go 二进制，提供查询网关能力 |
| OpenObserve | 主查询后端，PromQL/LogQL/TraceQL 调用均优先转发到 OpenObserve |
| PostgreSQL | OpenObserve 元数据/租户信息存储，网关通过适配器查询 Org 及日志/链路表 |
| VM/Mimir (可选) | 当 PromQL 在 OpenObserve 上不兼容时的旁路 Prometheus API 兼容实现 |
| Redis (可选) | 启用租户滑动窗口限流时需要；同时可用于后续扩展缓存集群 |

所有可选组件在配置中默认关闭，对应特性需要显式启用。

## 构建与运行

```bash
# 1. 进入仓库根目录
cd XScopeHub

# 2. 构建网关二进制
cd observe-gateway
go build -o bin/gateway ./cmd/gateway

# 3. 准备配置文件（见下文配置示例）
cat <<'CFG' > ./config.yaml
# 根据实际环境调整以下字段
server:
  address: ":8080"
  read_timeout: 15s
  write_timeout: 15s
  idle_timeout: 60s
CFG

# 4. 启动服务
go run ./cmd/gateway -config ./config.yaml
# 或使用已构建的二进制
./bin/gateway -config ./config.yaml
```

生产环境推荐使用 systemd、supervisord 或容器编排系统（Kubernetes/Nomad 等）托管进程，并配合反向代理或服务网格暴露服务端口。

## 配置说明

网关采用 YAML 配置，通过 `-config` 命令行参数传入。未提供配置文件时使用 `internal/config/defaultConfig` 中的默认值。

### 完整配置示例

```yaml
server:
  address: ":8080"
  read_timeout: 15s
  write_timeout: 15s
  idle_timeout: 60s

auth:
  enabled: true
  jwks_url: "https://issuer.example.com/.well-known/jwks.json"
  audience: ["xscopehub"]
  issuer: "https://issuer.example.com"
  tenant_claim: "tenant"
  user_claim: "email"
  cache_ttl: 1h
  insecure_tls: false

rate_limiter:
  enabled: true
  requests_per_second: 20
  burst: 40
  window: 1m
  redis_addr: "redis:6379"
  redis_username: ""
  redis_password: ""
  redis_db: 0
  redis_tls_insecure: false
  redis_tls_ca: ""
  redis_tls_skip_verify: false

cache:
  enabled: true
  num_counters: 50000
  max_cost: 268435456
  buffer_items: 64
  ttl: 1m

audit:
  enabled: true

backends:
  openobserve:
    base_url: "https://openobserve.example.com"
    org: "default"
    api_key: "${O2_API_KEY}"
    timeout: 30s
    prom_query_endpoint: "/api/%s/promql/query"
    prom_range_endpoint: "/api/%s/promql/query_range"
    log_search_endpoint: "/api/%s/_search"
    trace_search_endpoint: "/api/%s/traces"
    log_table: "logs"
    trace_table: "traces"
  fallback:
    enabled: true
    base_url: "https://mimir.example.com"
    api_key: "${MIMIR_TOKEN}"
    timeout: 20s
    query_endpoint: "/api/v1/query"
    range_endpoint: "/api/v1/query_range"
  metadata:
    enabled: true
    dsn: "postgres://o2:o2pass@postgres:5432/openobserve?sslmode=disable"
    max_connections: 10
    max_conn_idle_time: 5m
    tenant_lookup_query: "SELECT org, log_table, trace_table FROM tenant_metadata WHERE tenant = $1"
```

### 关键配置项解释

- **server**：HTTP 监听地址与超时设置。
- **auth**：JWT 鉴权配置；启用后会根据 JWKs 校验令牌，并从指定的 `tenant_claim` / `user_claim` 中提取租户与用户。
- **rate_limiter**：按租户限流配置；需要 Redis。当 `redis_addr` 为空时限流自动降级为关闭。
- **cache**：查询结果缓存配置，基于 Ristretto，键由 `lang+query+range+tenant` 组成。
- **audit**：是否输出 JSON 审计日志。
- **backends.openobserve**：OpenObserve 的基础地址、默认 Org、日志/链路默认表名及各类查询的 API 路径模板。
- **backends.fallback**：PromQL 兼容后端（如 VM/Mimir），在 OpenObserve 返回 4xx/5xx 且启用时触发。
- **backends.metadata**：PostgreSQL 连接配置，网关会执行 `tenant_lookup_query` 获取租户 Org 与日志/链路表；若查询不到则使用 `openobserve.log_table` 与 `openobserve.trace_table` 默认值。

建议将敏感信息（API Key、Redis 密码等）通过外部 Secret 管理（Kubernetes Secret、环境变量注入等）。

## 部署建议

1. **健康检查**：
   - 监听端口可通过 `GET /health`（由 `internal/server` 暴露）进行存活检测。
2. **日志与审计**：
   - 审计日志默认输出到 STDOUT，可收集至日志平台；生产环境建议将应用日志和审计日志区分处理。
3. **TLS/反向代理**：
   - 可以在网关前部署 Nginx/Envoy/Traefik，负责 TLS 终止与访问控制。
4. **扩容**：
   - 网关为无状态应用，支持通过水平扩容增加并发能力；限流需指向同一 Redis 以保证全局配额。

## 测试与校验

### 单元测试

```bash
cd observe-gateway
go test ./...
```

### 本地集成验证

1. 启动依赖服务（OpenObserve、可选 Redis/Mimir）。
2. 配置 `config.yaml` 并启动网关。
3. 使用 `curl` 验证各语言查询：
   ```bash
   curl -X POST http://localhost:8080/api/query \
     -H 'Content-Type: application/json' \
     -d '{"lang":"promql","query":"up"}'
   ```
4. 检查日志中是否输出审计条目，同时验证限流 / 缓存命中等特性。

### CI/CD 建议

- 将 `go test ./...` 加入 CI 流水线，结合 `golangci-lint` 等静态检查提升质量。
- 发布镜像或二进制前，通过 staging 环境做一次端到端回归，确认 PromQL/LogQL/TraceQL 转发路径正常。

## 故障排查

- **401/403**：检查 JWT 是否可被 JWKs 校验，租户 Claim 是否存在。
- **429**：表明命中限流，可调整 `requests_per_second`、`burst` 或确认 Redis 可用性。
- **5xx**：查看后端 OpenObserve 或 fallback 服务状态，必要时启用更多日志。

如需更多架构细节，请参考规划文档 `docs/xscopehub-query-gateway.md`。
### 连接 OpenObserve 与 PostgreSQL

- **OpenObserve**：`base_url`、`api_key` 指向 O2 API，`org` 用于填充接口中的 `%s` 占位符（若 PostgreSQL 返回租户专属 Org 则自动覆盖）。
- **PostgreSQL**：`metadata.dsn` 建议使用 `postgres://user:pass@host:5432/db` 格式，开启 TLS 时追加 `?sslmode=require`；网关启动时会建立连接池并执行 `SELECT org, log_table, trace_table FROM tenant_metadata WHERE tenant = $1` 查表，可根据实际数据模型调整 `tenant_lookup_query`。
- **租户表结构示例**：
  ```sql
  CREATE TABLE tenant_metadata (
      tenant TEXT PRIMARY KEY,
      org TEXT NOT NULL,
      log_table TEXT DEFAULT 'logs',
      trace_table TEXT DEFAULT 'traces'
  );
  ```
- 当 PostgreSQL 中不存在对应租户时，查询将自动回落到配置文件中的默认 Org/表名。
