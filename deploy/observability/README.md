# 本地可观测性栈

本目录提供独立于 Milvus 的本地监控栈，包含 OpenTelemetry Collector、Jaeger、Prometheus
和 Grafana。

## 启动

```powershell
docker compose -f deploy/observability/docker-compose.yml up -d
docker compose -f deploy/observability/docker-compose.yml ps
```

## 访问地址

- Grafana：<http://localhost:3000>
- Jaeger：<http://localhost:16686>
- Prometheus：<http://localhost:9090>
- Collector 健康检查：<http://localhost:13133>
- Collector Prometheus 指标：<http://localhost:8889/metrics>

Go 应用默认通过 OTLP/gRPC 将数据发送到 `localhost:4317`。如果应用运行在 Docker 中，使用
`SUPERBIZAGENT_OTEL_ENDPOINT` 覆盖 Collector 地址。

## 连通性检查

调用一次 `/api/chat`，然后：

1. 在 Prometheus 查询 `superbizagent_requests_total`。
2. 在 Jaeger 中搜索服务 `superbizagent`。
3. 在 Grafana 打开 `SuperBizAgent Overview` 仪表盘，检查请求、组件、工具、召回和 token 面板。

仪表盘不会展示原始 prompt、工具参数、文档内容或任何密钥。
