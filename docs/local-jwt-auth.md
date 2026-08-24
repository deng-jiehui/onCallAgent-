# 本地 JWT 认证

当前阶段提供本地账号认证，用于开发和测试多用户改造。认证模块只负责确认身份并签发 JWT；会话归属、租户检索过滤和并发控制将在后续阶段接入。

## 环境变量

启动服务前设置：

```text
SUPERBIZ_JWT_SECRET
SUPERBIZ_AUTH_PASSWORD_HASH
SUPERBIZ_AUTH_USERNAME       # 可选，默认 admin
SUPERBIZ_AUTH_USER_ID        # 可选，默认 local-admin
SUPERBIZ_AUTH_TENANT_ID      # 可选，默认 local-tenant
SUPERBIZ_AUTH_ROLES          # 可选，逗号分隔，默认 operator
SUPERBIZ_JWT_TTL             # 可选，Go duration，默认 1h
```

`SUPERBIZ_JWT_SECRET` 至少 32 个字符。`SUPERBIZ_AUTH_PASSWORD_HASH` 必须是 bcrypt 哈希，不能填写明文密码。

请使用组织批准的 bcrypt 工具生成哈希，并通过环境变量注入；不要把密码或哈希提交到仓库。开发机也可以使用一次性 Go 小程序调用 `bcrypt.GenerateFromPassword` 生成哈希，生成后立即删除小程序和明文密码。

## 登录

```http
POST /api/auth/login
Content-Type: application/json

{"username":"admin","password":"change-me"}
```

成功响应包含 `access_token`、`token_type`、`expires_in`、`user_id` 和 `tenant_id`。访问聊天接口时携带：

```http
Authorization: Bearer <access_token>
```

当前受保护接口包括 `/api/chat`、`/api/chat_stream`、`/api/upload` 和 `/api/ai_ops`。

## 生产限制

本地用户列表和 HS256 密钥只适合开发/测试。生产环境应替换为 PostgreSQL 用户仓库或企业 OIDC/JWT 验证，并增加密钥轮换、撤销、审计和刷新令牌策略。客户端传入的 `id` 仍不是用户身份，后续会话层必须使用认证上下文中的 `tenant_id + user_id + conversation_id`。
