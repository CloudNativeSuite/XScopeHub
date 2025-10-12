# MCP Server Integration Design

## Purpose

The MCP server introduces a workflow-aware control plane that orchestrates
infrastructure, deployment, observability, and agent automation across the
XScopeHub ecosystem. It replaces the earlier "Codex" naming so that the
repository consistently references the `mcp-server/` service that now pairs with
our LLM agents.

```
mcp-server/
├── cmd/
│   ├── mcp/                        # 主 CLI 入口
│   │   ├── main.go
│   │   ├── serve.go                # 启动 Hub Server
│   │   ├── run.go                  # 执行 Workflow
│   │   ├── deploy.go               # IAC 一键部署
│   │   └── version.go              # 版本信息
│   └── iac-mcp-server/             # 可选独立 IAC Server（Terraform/Pulumi）
│       └── main.go
│
├── internal/
│   ├── mcp/                        # MCP 基础协议与通道
│   │   ├── client.go               # MCP Client（下游 JSON-RPC）
│   │   ├── server.go               # MCP Server（对上 JSON-RPC）
│   │   ├── registry.go             # 统一路径注册与调度
│   │   ├── protocol.go             # Request/Response 定义
│   │   ├── auth.go                 # token/env 验证
│   │   └── logger.go               # 通用日志封装
│   │
│   ├── hub/                        # Hub 业务编排层
│   │   ├── hub.go                  # 读取配置，注册插件
│   │   ├── workflow.go             # YAML 工作流执行器
│   │   ├── state.go                # 状态保存与断点续跑
│   │   ├── audit.go                # 审计日志 / 执行轨迹
│   │   ├── policy.go               # allow/deny 策略控制
│   │   └── metrics.go              # Prometheus 指标
│   │
│   └── plugins/                    # MCP 插件适配层
│       ├── chrome.go               # 浏览器自动化
│       ├── ansible.go              # 远程部署
│       ├── github.go               # SCM / CI
│       ├── iac.go                  # Terraform / Pulumi
│       ├── monitor.go              # Prometheus / Grafana
│       ├── llm.go                  # LLM Agent / RAG
│       └── k8s.go                  # (未来) K8S MCP
│
├── pkg/                            # 公共辅助包
│   ├── executil/                   # 执行外部命令（带日志/超时）
│   ├── fileutil/                   # 读写 YAML/JSON/模板
│   ├── templating/                 # Go Template 引擎
│   └── ui/                         # CLI 输出格式化（颜色/进度条）
│
├── configs/
│   ├── hub.yaml                    # 全局 Hub 配置（端口、下游 MCP）
│   ├── logging.yaml                # 日志格式/级别/路径
│   ├── policies.yaml               # 权限与白名单控制
│   └── workflows/
│       ├── dev-ci-pr.yaml          # 开发流水线（GitHub + Chrome）
│       ├── ops-deploy-ansible.yaml # 运维自动化（Ansible + Chrome + GitHub）
│       ├── iac-deploy-cloud.yaml   # IaC 部署（Terraform + Chrome + GitHub）
│       └── rollback.yaml           # 回滚任务
│
├── scripts/                        # 工具脚本
│   ├── install.sh                  # 快速安装
│   ├── run_dev.sh                  # 本地调试启动
│   └── docker-entrypoint.sh        # 容器启动
│
├── Makefile                        # 构建/测试/打包
├── go.mod                          # Go 模块声明
├── go.sum
├── README.md                       # 项目说明
└── LICENSE
```

## Relation to XScopeHub Components

| MCP Server Layer    | Role in XScopeHub | Key Integrations |
|---------------------|-------------------|------------------|
| `cmd/mcp` CLI       | Operator entry point aligned with `scripts/run_dev.sh` and `Makefile` tooling. | Triggers hub workflows that in turn call into `observe-bridge/`, `llm-ops-agent`, and `llm-code-agent` tasks. |
| `internal/mcp`      | Translates MCP JSON-RPC into unified dispatch. | Bridges the protocol to downstream agents already listed under `agents/` and to the dedicated orchestrators in both LLM agent repositories. |
| `internal/hub`      | Workflow and policy engine. | Persists execution state alongside the observability databases, enabling audit trails that correlate with existing metrics/log indexes. |
| `internal/plugins`  | Concrete adapters for Chrome automation, GitHub, Ansible, IaC, monitoring, and LLM/K8s capabilities. | Reuses `observe-bridge` data to drive monitor plugins while LLM plugins reuse shared prompt workflows from `llm-ops-agent` and `llm-code-agent`. |
| `pkg/*`             | Shared utilities (command execution, templating, UI). | Align with agent orchestrator contracts where templated prompts or shell commands are required. |
| `configs/workflows` | Declarative workflow definitions. | Extend repository automation alongside existing YAML/SQL specifications documented in `docs/architecture.md` and `docs/insight.md`. |

## Workflow Embedding

1. **Observation-driven Automation**: Metrics/alerts processed by `observe-bridge` can emit events into the MCP server via the `monitor` plugin. Workflows fan out to GitHub issue automation, Chrome-based runbook steps, or IaC remediation.
2. **LLM-Assisted Operations**: The `llm` plugin targets the `llm-ops-agent` and `llm-code-agent` services. It uses their API contracts (`docs/llm-ops-agent/api.md`, definitions under `llm-code-agent/api/`) to provide context injection and conversational guidance inside MCP sessions.
3. **Infrastructure as Code (IaC)**: `internal/plugins/iac.go` ties Terraform/Pulumi stacks to hub workflows, with `cmd/mcp deploy` exposing one-click rollouts that rely on repository IaC manifests under `deploy/`.
4. **Audit & Policy**: The hub's `audit.go` and `policy.go` modules deliver the governance hooks requested in `docs/roadmap.md`, ensuring automated actions remain auditable and permission-bound.

## Repository Linking

- **MCP Server ↔ LLM Ops Agent**: The `llm` plugin forwards MCP tool calls to the gRPC/HTTP endpoints defined in `llm-ops-agent/cmd/` so that operational runbooks can invoke reasoning and remediation routines.
- **MCP Server ↔ LLM Code Agent**: Code generation or review requests are dispatched to `llm-code-agent` APIs defined under `llm-code-agent/api/`. Workflow definitions reference the agent through service bindings in `configs/hub.yaml`, ensuring MCP sessions can pivot between ops and code tasks seamlessly.
- **Shared Credentials & Policies**: Token and role configuration is centralised in `configs/policies.yaml`, keeping consistent RBAC across all three services.

## Deployment & Operations

- **Serve Mode**: `cmd/mcp serve` hosts the MCP server, exposing JSON-RPC endpoints that upstream orchestrators (including the LLM agents) call.
- **Run Mode**: `cmd/mcp run` executes ad-hoc workflows, mirroring how operators currently use scripts in `scripts/` but with policy, logging, and state persistence baked in.
- **Policies**: `configs/policies.yaml` establishes allow/deny matrices aligned with the repository's RBAC discussions in `docs/insight.md`.
- **Metrics**: `internal/hub/metrics.go` exports Prometheus metrics. These are scraped by the observability stack deployed via `observe-gateway` and visualised in Grafana dashboards from `docs/grafana.md`.

## Roadmap Alignment

- **Unified Agent Registry**: `internal/mcp/registry.go` maintains a catalog for Chrome, GitHub, IaC, monitoring, and LLM agents, matching roadmap themes for “federated automation”.
- **Resilience**: Workflow `state.go` supports resume/replay semantics similar to the "不丢包" guarantees described in `docs/architecture.md` for telemetry pipelines.
- **Extensibility**: The plugins folder outlines where additional MCP bridges (e.g., Kubernetes, advanced monitoring) will land, ensuring the repository remains the central integration point for both data plane and control plane automation.

## Next Steps

1. Implement the MCP server/client scaffolding under `internal/mcp/` and expose it through `cmd/mcp serve`.
2. Define initial workflows in `configs/workflows/` that automate the existing ETL jobs plus LLM code/ops triage tasks.
3. Connect `internal/plugins/monitor.go` to the observability metrics emitted by `observe-bridge`.
4. Document end-to-end runbooks showing how MCP-driven automation closes the loop between detection, reasoning, code fixes, and remediation.
