# SuperBizAgent

SuperBizAgent 是一个面向告警处理和 AI 运维场景的 Go 智能助手。它使用 Eino 编排 RAG、ReAct Agent、模型调用和工具调用，并通过 Milvus 检索内部运行手册和故障知识。

当前版本已经包含本地 JWT 认证基础模块，为后续多用户会话、租户隔离和高并发改造提供身份上下文。

## 功能概览

- 普通对话：`POST /api/chat`
- 流式对话：`POST /api/chat_stream`
- 文件上传：`POST /api/upload`
- AI 运维入口：`POST /api/ai_ops`
- 本地 JWT 登录：`POST /api/auth/login`
- Milvus 向量检索和知识库索引
- MCP 日志工具、Prometheus 告警工具、MySQL 工具和内部文档工具
- OpenTelemetry、Prometheus、Grafana、Jaeger 可观测性配置
- 离线评估数据集和评估 CLI

## 架构

```text
客户端
  -> GoFrame HTTP/SSE 接入层
  -> JWT 认证和请求上下文
  -> Eino ChatAgent Graph
       -> RAG / Milvus Retriever
       -> ReAct Agent
       -> LLM / MCP / 业务工具
  -> 模型、知识库和可观测性基础设施
```

当前会话历史仍使用进程内内存。PostgreSQL/Redis 共享会话、租户过滤、同会话串行和运行时资源复用已记录在 [多用户改造路线图](docs/superpowers/plans/2026-08-25-local-jwt-auth-and-multitenant-roadmap.md) 中，尚未全部实现。

## 环境要求

- Go 1.24.4 或兼容的 Go 1.24 工具链
- Docker Desktop 或 Docker Engine（运行 Milvus 时需要）
- Python 3 或 Node.js（仅运行静态前端时需要）
- 可访问的模型 API；生产环境建议使用密钥管理服务

## 配置

1. 复制配置模板：

   ```powershell
   Copy-Item config.example.yaml config.yaml
   ```

2. 编辑 `config.yaml`，把模型 API Key、Embedding API Key、`file_dir` 和 MCP 地址替换为本地值。`config.yaml` 已被 `.gitignore` 忽略，不应提交。

3. 设置本地 JWT 环境变量。密码必须是 bcrypt 哈希，不能填写明文：

   ```powershell
   $env:SUPERBIZ_JWT_SECRET = "replace-with-at-least-32-random-characters"
   $env:SUPERBIZ_AUTH_PASSWORD_HASH = "<bcrypt-hash>"
   $env:SUPERBIZ_AUTH_USERNAME = "admin"
   $env:SUPERBIZ_AUTH_USER_ID = "local-admin"
   $env:SUPERBIZ_AUTH_TENANT_ID = "local-tenant"
   $env:SUPERBIZ_AUTH_ROLES = "operator,admin"
   ```

   bcrypt 哈希应使用组织批准的密码工具生成。不要把密码、哈希或 JWT 密钥写入 Git。

更多认证说明见 [docs/local-jwt-auth.md](docs/local-jwt-auth.md)。

## 启动依赖服务

### Milvus

在项目根目录执行：

```powershell
docker compose -f manifest/docker/docker-compose.yml up -d
```

Milvus 默认监听 `localhost:19530`。首次启动可能需要等待几十秒。

### MCP 日志服务（可选）

该服务需要镜像和腾讯云 CLS 凭据：

```powershell
$env:TENCENTCLOUD_SECRET_ID = "..."
$env:TENCENTCLOUD_SECRET_KEY = "..."
docker compose -f deploy/mcp/docker-compose.yml up -d
```

如果 MCP 不可用，Agent 会继续启动，但对应日志工具不可用。

### 可观测性栈（可选）

```powershell
docker compose -f deploy/observability/docker-compose.yml up -d
```

常用地址：Jaeger `http://localhost:16686`、Prometheus `http://localhost:9090`、Grafana `http://localhost:3000`。

## 启动后端

确保 `config.yaml` 和 JWT 环境变量已经设置，然后执行：

```powershell
go mod download
go run .
```

后端默认监听 `http://localhost:6872`。

也可以先构建再运行：

```powershell
go build -o superbizagent.exe .
./superbizagent.exe
```

服务启动时会校验 JWT 配置；缺少 `SUPERBIZ_JWT_SECRET` 或 `SUPERBIZ_AUTH_PASSWORD_HASH` 会直接退出。

## 验证登录和聊天

先登录获取 Token：

```powershell
$body = @{ username = "admin"; password = "your-password" } | ConvertTo-Json
$login = Invoke-RestMethod `
  -Uri http://localhost:6872/api/auth/login `
  -Method Post `
  -ContentType "application/json" `
  -Body $body

$token = $login.data.access_token
```

调用普通聊天接口：

```powershell
$headers = @{ Authorization = "Bearer $token" }
$chatBody = @{ id = "conversation-local-001"; question = "服务下线后应该先检查什么？" } | ConvertTo-Json

Invoke-RestMethod `
  -Uri http://localhost:6872/api/chat `
  -Method Post `
  -Headers $headers `
  -ContentType "application/json" `
  -Body $chatBody
```

没有 Token 的受保护接口会返回 `401`。

## 启动前端

前端是一个无构建步骤的静态页面：

```powershell
cd SuperBizAgentFrontend
py -m http.server 8080
```

然后打开 `http://localhost:8080`。

注意：当前前端尚未接入 JWT 登录和 `Authorization` 请求头，因此直接从前端发起聊天、上传或 AI 运维请求会被后端拒绝为 `401`。后续多用户阶段会增加前端登录、Token 存储、过期处理和请求注入。

## 测试和构建

```powershell
go test ./...
go build ./...
```

认证专项测试：

```powershell
go test ./internal/auth ./internal/controller/auth ./internal/observability -count=1
```

## 目录说明

```text
api/                         HTTP 请求和响应契约
internal/auth/               本地 JWT 签发、校验和中间件
internal/controller/         GoFrame 控制器
internal/ai/                 Agent、RAG、模型、Embedding 和工具
internal/observability/      OpenTelemetry、指标和回调
internal/evaluation/         离线评估逻辑
deploy/                      MCP 和可观测性部署文件
manifest/docker/             Milvus Docker Compose
SuperBizAgentFrontend/       静态前端
docs/                        架构、部署和评估文档
```

## 安全说明

- 不要提交 `config.yaml`、数据库 DSN、模型 API Key、JWT 密钥或 bcrypt 密码哈希。
- 当前本地 JWT 账号只适合开发和测试；生产环境应替换为企业 OIDC/JWT 或 PostgreSQL 用户仓库。
- `mysql_crud` 工具当前仍需要进一步做数据源白名单、租户权限和只读限制，不能直接作为生产数据库代理。
- 当前仓库历史配置曾出现过凭据，相关 Key 应立即轮换。
