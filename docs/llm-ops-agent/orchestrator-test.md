本地启动 Orchestrator 服务

准备数据库

按 docs/llm-ops-agent/postgres-init.md 说明初始化 PostgreSQL，并执行仓库里的迁移脚本

编译并运行服务

cd llm-ops-agent
make build                      # 生成 ./bin/xopsagent
make run                        # 读取 ../config/XOpsAgent.yaml 并监听 8100 端口
默认监听地址和数据库连接可在 config/XOpsAgent.yaml 中修改

# curl 验证接口可用性

- 健康检查 curl -i http://localhost:8100/healthz
- 创建工单（带幂等键和操作者）

```bash
curl -X POST http://localhost:8100/case/create \
     -H "Content-Type: application/json" \
     -H "Idempotency-Key: create-1" \
     -H "X-Actor: tester" \
     -d '{"tenant_id":1,"title":"p95 spike"}'
```
首次返回 case_id、status: NEW 与 version；重复发送同一 Idempotency-Key 会返回相同结果

- 状态迁移（带版本与幂等）

```
curl -X PATCH http://localhost:8100/case/<case_id>/transition \
     -H "Content-Type: application/json" \
     -H "If-Match: 1" \
     -H "Idempotency-Key: trans-1" \
     -H "X-Actor: tester" \
     -d '{"event":"start_analysis"}'
```

成功后返回 status: ANALYZING 与新的 version，响应头 ETag 亦携带版本, 再次使用相同 Idempotency-Key 将直接复用第一次的响应

- 非法迁移验证

```bash
# NEW 状态直接执行 plan_ready 会触发 409
curl -X PATCH http://localhost:8100/case/<case_id>/transition \
     -H "Content-Type: application/json" \
     -H "If-Match: 2" \
     -d '{"event":"plan_ready"}'
```

返回 409 Conflict，错误为 “illegal transition” 或 “guard not satisfied”，源于纯函数 FSM 的判定

- 版本冲突验证

```
curl -X PATCH http://localhost:8100/case/<case_id>/transition \
     -H "Content-Type: application/json" \
     -H "If-Match: 0" \
     -d '{"event":"start_analysis"}'
```

版本不匹配时返回 412 Precondition Failed

# 接口摘要

- POST /case/create 生成初始状态为 NEW 的工单
- PATCH /case/{id}/transition 驱动状态机事件（如 start_analysis、analysis_done 等）

以上步骤即可用 curl 在本地验证 Orchestrator 核心的幂等写入、乐观并发控制及错误处理逻辑，同时确认接口可用性与状态迁移行为。
