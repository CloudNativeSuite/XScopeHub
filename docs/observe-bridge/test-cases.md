# 测试 observe-bridge 服务

可以在服务启动后：

- 健康检查 curl http://localhost:8080/healthz 预期返回 “OK” 或类似的健康状态信息。
- 指标页面 curl http://localhost:8080/metrics 应返回 Prometheus 格式的指标数据。
- OpenAPI 文档 curl http://localhost:8080/openapi.yaml 用于查看或下载 API 文档。
- 其他 HTTP 客户端 也可以使用浏览器、Postman 或其他工具访问上述接口，检查响应是否符合预期。

如果需要更深入的测试，可编写集成测试或使用 go test（需在代码中提供相应的测试用例）
