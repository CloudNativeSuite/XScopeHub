测试 observe-bridge 服务，可以在服务启动后：

健康检查

curl http://localhost:8080/healthz
预期返回 “OK” 或类似的健康状态信息。

指标页面

curl http://localhost:8080/metrics
应返回 Prometheus 格式的指标数据。

OpenAPI 文档

curl http://localhost:8080/openapi.yaml
用于查看或下载 API 文档。

其他 HTTP 客户端
也可以使用浏览器、Postman 或其他工具访问上述接口，检查响应是否符合预期。

## 集成测试

在运行集成测试前，需要先启动 OpenObserve 与 PostgreSQL 服务，并设置相关环境变量，例如：

```
export OPENOBSERVE_URL=http://localhost:5080
export OPENOBSERVE_AUTH=$(echo -n "root@example.com:password" | base64)
export DATABASE_URL=postgres://postgres:postgres@localhost:5432/observe?sslmode=disable
```

随后在 `observe-bridge` 目录下执行：

```
make integration-tests
```

该目标会自动运行数据库初始化、迁移、启动服务并执行位于 `integration-test-cases` 目录中的 Go 集成测试。
