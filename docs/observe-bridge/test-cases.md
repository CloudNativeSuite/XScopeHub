# 测试 observe-bridge 服务

可以在服务启动后：

- 健康检查 curl http://localhost:8080/healthz 预期返回 “OK” 或类似的健康状态信息。
- 指标页面 curl http://localhost:8080/metrics 应返回 Prometheus 格式的指标数据。
- OpenAPI 文档 curl http://localhost:8080/openapi.yaml 用于查看或下载 API 文档。
- 其他 HTTP 客户端 也可以使用浏览器、Postman 或其他工具访问上述接口，检查响应是否符合预期。

### GET /oo/stream

- 用例描述: 按窗口流式获取指定租户的 OpenObserve 数据。
- 请求示例:
  ```bash
  curl -N "http://localhost:8080/oo/stream?tenant=demo&from=2024-01-01T00:00:00Z&to=2024-01-01T00:05:00Z"
  ```
- 预期结果:
  - 返回状态码 `200`。
  - 响应体为按行分隔的 JSON 流，逐条输出 `oo.Record`。
- 异常场景:
  - 缺少 `tenant`/`from`/`to` 任一参数 -> 返回 `400`。
  - `from` >= `to` 时返回空流或 `204`。

如果需要更深入的测试，可编写集成测试或使用 go test（需在代码中提供相应的测试用例）
