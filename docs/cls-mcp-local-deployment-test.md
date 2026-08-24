# 腾讯云 CLS MCP 本地部署测试记录

## 1. 测试目标

验证腾讯官方 `Tencent/cls-mcp-server` 能否替代已下架的腾讯云托管 MCP，并完成 CLS 服务开通、测试资源创建、日志上传和本地 SSE MCP 连通性验证。

测试日期：2026-08-20。

## 2. 使用组件

- MCP Server：`cls-mcp-server@1.2.0`。
- 官方源码：<https://github.com/Tencent/cls-mcp-server>。
- 腾讯云 CLS Node SDK：`tencentcloud-sdk-nodejs-cls@4.1.297`。
- 腾讯云 CLS Go Producer：`github.com/tencentcloud/tencentcloud-cls-sdk-go@v1.0.14`。
- 本地 MCP 地址：`http://localhost:3100/sse`。
- 健康检查：`http://localhost:3100/health`。

密钥从 Windows 用户级环境变量读取，没有写入仓库：

```text
TENCENTCLOUD_SECRET_ID
TENCENTCLOUD_SECRET_KEY
TENCENTCLOUD_REGION
```

## 3. 腾讯云资源

测试前账号尚未开通 CLS，首次查询返回 `CLS service is unregistered`。随后调用官方 `OpenClsService` API 开通成功。

在 `ap-guangzhou` 创建了以下测试资源：

| 资源 | 名称 | ID |
| --- | --- | --- |
| LogSet | `superbizagent-test` | `dc0d066b-9495-46aa-85c4-15ce1a46831c` |
| Topic | `superbizagent-test-logs` | `6c057ad5-d6ef-477f-a157-09997806348d` |

Topic 使用标准存储，保留期为 7 天。测试资源会产生腾讯云 CLS 使用费用，不再需要时应在控制台删除。

## 4. 测试日志

通过腾讯官方 Go Producer 分两轮上传了测试日志，每轮 9 条，第二轮来自本地 fixture 文件；CLS 主题当前至少包含 18 条测试日志。日志覆盖：

本地可复用的日志文件位于 `eval/logs/cls/`：

```text
service-offline.jsonl
api-failure.jsonl
reconciliation.jsonl
region-mismatch.jsonl
error-codes.jsonl
```

- 服务下线和 Pod 重启。
- `panic`。
- 接口失败和下游超时。
- 对账差异。
- 地域不匹配。
- 错误码 `12000000001`、`12000000002`。
- 健康检查日志。

两轮 Producer 均返回上传成功，第二轮输出：

```text
uploaded 9 logs to topic 6c057ad5-d6ef-477f-a157-09997806348d
```

首次上传后立即查询为空；为 Topic 创建全文索引和动态键值索引并再次上传后，查询已返回日志。当前已验证 CLS 写入和检索可见性。

## 5. MCP 启动结果

Docker 源码构建命令：

```powershell
docker build -t superbizagent-cls-mcp:1.2.0 temp/cls-mcp-server-source
```

首次构建因 Docker Hub 网络连接失败，改用 `docker.m.daocloud.io/library/node:22-alpine` 拉取基础镜像并本地标记后，构建成功：

```text
failed to resolve reference docker.io/library/node:22-alpine
Docker Desktop has no HTTPS proxy
```

最终使用构建出的 Docker 镜像以 SSE 模式启动：

```powershell
docker run -d --name superbizagent-cls-mcp -p 3100:3000 `
  -e TRANSPORT=sse -e PORT=3000 `
  -e TENCENTCLOUD_SECRET_ID="$env:TENCENTCLOUD_SECRET_ID" `
  -e TENCENTCLOUD_SECRET_KEY="$env:TENCENTCLOUD_SECRET_KEY" `
  -e TZ=Asia/Shanghai superbizagent-cls-mcp:1.2.0
```

健康检查通过：

```text
HTTP 200
{"status":"ok","activeSessions":0}
```

使用 SuperBizAgent 自身的 Eino MCP 客户端连接 Docker 容器的 `/sse`，成功发现 20 个工具，并通过 `SearchLog` 查询到 `service-offline.jsonl` 生成的日志。

成功发现的代表性工具包括：

```text
SearchLog
DescribeLogContext
DescribeIndex
DescribeLogHistogram
DescribeTopics
DescribeLogsets
QueryMetric
DescribeAlarms
```

因此 SSE 协议和当前 `GetLogMcpTool` 客户端兼容。

项目配置已更新为：

```yaml
mcp_url: "http://localhost:3100/sse"
```

## 6. 可重复启动命令

Docker Hub 可以访问后重新构建并启动：

```powershell
docker build -t superbizagent-cls-mcp:1.2.0 temp/cls-mcp-server-source

docker run -d `
  --name superbizagent-cls-mcp `
  --restart unless-stopped `
  -p 3100:3000 `
  -e TRANSPORT=sse `
  -e PORT=3000 `
  -e TENCENTCLOUD_SECRET_ID="$env:TENCENTCLOUD_SECRET_ID" `
  -e TENCENTCLOUD_SECRET_KEY="$env:TENCENTCLOUD_SECRET_KEY" `
  -e TZ=Asia/Shanghai `
  superbizagent-cls-mcp:1.2.0
```

验证：

```powershell
Invoke-WebRequest http://localhost:3100/health
```

## 7. 当前结论

- CLS 服务开通：通过。
- LogSet 和 Topic 创建：通过。
- Go Producer 日志发送：通过。
- CLS 日志检索可见性：通过，已查到上传日志。
- npm 方式 SSE MCP 健康检查：通过。
- Eino MCP 客户端工具发现：通过，共 20 个工具。
- Docker 镜像构建：通过，镜像为 `superbizagent-cls-mcp:1.2.0`。
- SuperBizAgent MCP 地址：已改为 `localhost:3100/sse`。

## 8. 安全事项

- 截图中已经显示过腾讯云 SecretId，建议完成测试后立即轮换 SecretId/SecretKey。
- 不要把密钥写入 `config.yaml`、Compose 文件或测试报告。
- 正式账号应使用只具备必要 CLS 权限的 CAM 子账号或角色。
