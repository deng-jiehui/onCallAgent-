# SuperBizAgent 高并发与多用户上下文隔离设计

## 1. 文档目的

本文说明 SuperBizAgent 在高并发场景下为什么可能发生多用户上下文串扰，以及如何以可逐步实施的方式解决这些问题。

本文面向两类读者：

- 不熟悉并发的产品、运维和业务同学：重点阅读前半部分的概念、例子和请求流程。
- 负责实现和部署的开发同学：重点阅读后半部分的接口、数据结构、错误处理和验收标准。

本文推荐的总体路线是：

> 共享会话存储 + 同一会话串行化 + Agent 资源复用 + 有界并发控制。

这不是一次性重写。设计按阶段落地，先消除串话和请求级资源浪费，再扩展到多副本部署和压力治理。

## 2. 先理解两个词

### 2.1 什么是并发

并发不是“系统一定同时执行很多条指令”，而是系统在同一段时间内同时处理多个请求。

例如：

```text
10:00:00 用户 A 发起聊天请求
10:00:00 用户 B 发起聊天请求
10:00:01 A 等待模型返回
10:00:01 B 查询知识库
10:00:03 A 收到模型结果
10:00:04 B 收到模型结果
```

如果每个请求使用自己的数据、自己的输出通道，并发通常是好事：等待 A 的模型响应时，服务器可以处理 B。

问题出在多个请求不小心共享了本应属于某个用户或某个会话的数据。

### 2.2 什么是上下文串扰

上下文就是模型回答当前问题时能看到的历史消息、用户身份、租户信息和工具调用范围。

上下文串扰是指：

```text
用户 A 的历史或权限信息
        ↓
被错误地带入
        ↓
用户 B 的请求
```

典型后果包括：

- B 看到 A 之前问过的问题；
- B 的问题被模型理解成 A 的后续追问；
- A 和 B 的工具查询使用了错误的租户或地域；
- 同一个会话的两条消息顺序颠倒，模型得到不完整历史。

这类问题通常不是模型“答错了”，而是应用在调用模型之前就给了它错误的输入。

## 3. 当前实现的风险

### 3.1 会话状态只用一个 ID 表示

`Chat` 和 `ChatStream` 使用 `req.Id` 访问内存会话：

```go
History: mem.GetSimpleMemory(id).GetMessages()
```

当前 ID 没有明确区分租户、用户和会话。只要两个请求使用相同 ID，就可能访问同一段历史。即使客户端通常会生成不同 ID，也不应把安全隔离建立在客户端自觉之上。

目标键应当是：

```text
tenant_id + user_id + conversation_id
```

其中 `tenant_id` 和 `user_id` 应来自认证上下文，不能由普通请求体任意伪造。

### 3.2 进程内全局 Map 不适合生产会话存储

`utility/mem/mem.go` 维护了全局 `SimpleMemoryMap`。这会带来四个问题：

1. 多个服务副本各自有一份 Map，请求切换副本后看不到原会话。
2. 服务重启后所有会话历史丢失。
3. 会话数量增长时 Map 会持续占用内存，没有过期和淘汰策略。
4. 锁只保护 Map 查找，不保证一个会话的“读取、调用模型、写回”是完整的原子流程。

### 3.3 同一会话可以同时执行两次

当前流程大致是：

```text
请求 1: 读取历史 H
请求 2: 读取历史 H
请求 1: 调用模型并生成答案 A
请求 2: 调用模型并生成答案 B
请求 1: 写入问题 1、答案 A
请求 2: 写入问题 2、答案 B
```

结果可能是 `问题 1、问题 2、答案 A、答案 B`，也可能因为完成顺序不同变成 `问题 2、答案 B、问题 1、答案 A`。模型下一轮看到的历史不再代表真实对话顺序。

### 3.4 每个请求都创建重资源

`BuildChatAgent` 会构建 Graph、ChatModel、Retriever、Embedding 和 Milvus client。Retriever 初始化还会连接 Milvus、检查数据库和 Collection、加载 Collection。

把这些工作放在请求路径会导致：

- 并发请求同时创建大量连接；
- 重复执行本应只做一次的初始化；
- 首包延迟升高；
- 外部服务更容易触发连接数或 QPS 限制。

### 3.5 SSE 输出缺少明确的背压边界

流式请求需要持续向客户端写数据。当前 `SendToClient` 直接写 HTTP Response，慢客户端可能让 Agent 一直阻塞；如果未来改成无界缓存，又会把慢客户端的数据堆在内存里。

因此必须规定：每个请求允许缓存多少数据、客户端断开后何时取消 Agent、队列满时如何结束请求。

## 4. 设计目标与非目标

### 4.1 目标

- 不同租户、不同用户之间绝不共享会话历史和工具权限上下文。
- 同一会话的请求按协调器接收顺序处理，不出现消息乱序。跨网络到达时间完全相同的请求不承诺客户端发送顺序。
- 不同会话可以并行处理，避免一个慢会话阻塞所有用户。
- 多个服务副本可以共享会话状态，任意副本都能处理请求。
- ChatModel、Embedding、Milvus、Agent Graph 等重资源在进程内复用。
- 对模型、工具、SSE 和外部存储设置并发、超时和内存上限。
- 通过 trace、metrics 和压测验证隔离效果及容量边界。

### 4.2 非目标

- 本阶段不改变模型选择、Prompt 内容和业务工具语义。
- 本阶段不把系统整体改造成事件驱动架构。
- 本阶段不承诺无限并发；超过容量的请求应快速返回限流错误。
- 本阶段不把完整问题、Prompt 或用户隐私写入高基数 metrics label。

## 5. 推荐架构

```text
客户端
  |
  v
HTTP / SSE 接口
  |
  | 认证、身份校验、request_id、超时
  v
会话协调器 Conversation Coordinator
  |
  | 读取历史快照
  | 同一会话排队或版本校验
  v
Agent Runtime
  |
  | 复用已编译 Graph、模型、Embedding、Milvus client
  | 受全局/租户/会话并发限制
  v
会话存储 Redis 或 PostgreSQL
  |
  | 追加消息、版本号、过期时间
  v
SSE 有界输出队列
```

每个组件只负责一件事：

| 组件 | 责任 | 不负责的事情 |
| --- | --- | --- |
| 身份层 | 确认租户和用户身份 | 不保存聊天历史 |
| 会话协调器 | 保证同会话顺序，取得历史快照 | 不创建模型连接 |
| 会话存储 | 持久化和读取消息 | 不决定模型如何回答 |
| Agent Runtime | 执行 RAG、Agent、工具和模型 | 不直接管理用户身份 |
| SSE Writer | 单线程写出流式事件 | 不修改会话历史 |
| 限流器 | 控制资源使用量 | 不改变业务答案 |

## 6. 身份与会话隔离

### 6.1 复合会话键

定义统一的会话身份：

```go
type SessionKey struct {
    TenantID       string
    UserID         string
    ConversationID string
}
```

规范化后的存储键：

```text
conversation:{tenant_id}:{user_id}:{conversation_id}
```

如果当前项目还没有完整认证层，临时开发方案也必须至少使用：

```text
user_id + conversation_id
```

并在接口文档中明确：客户端传来的 `conversation_id` 只表示会话名称，不代表访问权限。

该临时方案不能作为生产隔离方案，因为没有可靠的用户身份就无法证明两个请求属于不同用户。生产上线前必须接入认证层，并从认证结果生成 `tenant_id` 和 `user_id`。

### 6.2 请求上下文

使用自定义 context key，不使用字符串 key：

```go
type requestContextKey struct{}

type RequestIdentity struct {
    RequestID      string
    TenantID       string
    UserID         string
    ConversationID string
}
```

身份信息应在 HTTP 入口完成校验，并传给 Retriever、Tool 和 Observability。工具读取租户范围时必须使用已认证的上下文，而不是从用户问题或 Prompt 中猜测。

### 6.3 访问校验

每次读取或写入会话前检查：

```text
当前用户是否属于 tenant_id
当前用户是否拥有 conversation_id
请求是否已过期或被撤销
```

校验失败统一返回 `403` 或 `404`。对外可以统一使用 `404`，避免泄露某个会话是否存在。

## 7. 会话存储与同会话顺序

### 7.1 推荐使用共享存储

优先选择 Redis 或 PostgreSQL：

- Redis 适合快速读取最近窗口、设置 TTL 和实现短事务；
- PostgreSQL 适合审计、长期保存和复杂查询；
- 如果已有其中一种基础设施，优先复用，不同时引入两套存储。

会话记录至少包含：

```go
type Conversation struct {
    Key       SessionKey
    Version   int64
    Messages  []Message
    UpdatedAt time.Time
}
```

每次进入协调器时生成单调递增的 `turn_seq`，写入 trace 和会话消息元数据。它是服务端定义的处理顺序，用于排查“哪一条先被系统接收”，不用于猜测客户端网络发送时间。

### 7.2 乐观锁是怎样保证顺序的

可以把 `Version` 理解成会话历史的页码：

```text
读取版本 10 -> 处理请求 -> 只有版本仍为 10 才能写成版本 11
```

推荐实现同时使用两道保护，但职责不同：协调器负责正常路径的排队和顺序，版本号负责防止异常路径覆盖（例如实例故障恢复、重复投递或未来新增绕过协调器的入口）。在协调器正常工作时，同一会话不会并行执行，因此不应把版本冲突当成正常的排队机制。

两个请求同时读取版本 10 时，只有一个能成功提交。另一个提交失败后重新读取最新历史，再决定重试或返回冲突。若业务必须保证严格顺序，必须先经过同会话队列确定顺序；单靠版本号只能防止覆盖，不能推断客户端的真实发送先后。

伪代码：

```go
history, version := store.Load(ctx, key)
answer := agent.Invoke(ctx, input.WithHistory(history))
ok := store.AppendIfVersion(ctx, key, version, input, answer)
if !ok {
    return ErrConversationConflict
}
```

对于聊天接口，推荐在协调器中自动重试一次；仍然冲突则返回 `409 conversation_busy`，避免无限重试拖垮系统。

### 7.3 会话锁的适用范围

如果业务要求同一会话严格排队，可以使用 Redis 分布式锁：

```text
lock:conversation:{tenant_id}:{user_id}:{conversation_id}
```

锁必须有租约和续租机制，释放操作必须校验锁持有者，防止一个请求释放另一个请求的锁。

推荐优先使用版本号；只有需要严格排队或暂时无法实现事务更新时才使用分布式锁。单机 `sync.Mutex` 只能作为本地开发阶段的过渡方案。

本设计的推荐组合是“协调器队列 + 条件追加”：队列决定顺序，条件追加做最后一道一致性保护。不要在正常路径同时让多个请求长时间持有分布式锁并等待模型，否则会把外部慢调用时间放大为锁等待时间。

会话过期后，第一次新请求应从空历史开始，并生成新的会话版本；过期不能把旧会话内容返回给新用户。历史达到 `max_messages` 时按完整的用户/助手消息对截断，不能只删掉一条造成角色不匹配。需要长期保留时，应在截断前生成不包含敏感信息的摘要并单独保存，摘要策略不属于本阶段的必需实现。

## 8. Agent Runtime 资源复用

### 8.1 启动时创建

应用启动时创建一次：

1. Milvus client，并完成数据库/Collection/索引检查和加载。
2. Embedding client。
3. ChatModel client。
4. MCP 和其他工具实例。
5. Retriever。
6. 已编译的 ChatAgent Graph。

Controller 通过依赖注入使用这些对象，不再在每个请求中调用 `BuildChatAgent`。

目标结构示例：

```go
type Runtime struct {
    Agent     compose.Runnable[*chat_pipeline.UserMessage, *schema.Message]
    Retriever retriever.Retriever
    Store     ConversationStore
    Limits    Limits
}
```

### 8.2 生命周期

启动失败应让服务启动失败，不能等到第一个用户请求才发现 Milvus 或模型不可用。关闭服务时统一关闭可关闭的 client，并等待正在进行的请求在超时时间内结束。

### 8.3 共享对象的并发要求

复用对象不等于所有状态都能共享。模型和 Milvus client 必须确认 SDK 支持并发调用；请求特有的数据只能放在函数局部变量或请求 context 中，不能写入 Agent 的全局字段。

上线前应通过并发集成测试证明共享 ChatModel、Retriever、Embedding 和工具实例不会保存上一个请求的用户、租户、会话或查询参数。任何包含请求可变字段的对象都必须改为每请求创建的轻量值对象。

## 9. 并发限制、超时与背压

### 9.1 三级并发限制

建议同时设置：

```text
实例级：单个进程最多同时执行 N 个 Agent
集群级：所有副本合计的租户额度由共享限流器控制
会话：同一会话最多 1 个执行中的请求
```

超过限制时返回 `429 too_many_requests`，并在响应中提供重试提示。`conversation_queue_size` 表示“正在执行的 1 个请求之外，最多允许等待的请求数”；等待队列满时立即返回 `429`，等待超时也返回 `429`。只有已经取得执行资格但在提交阶段发现版本不一致时才返回 `409 conversation_busy`。

多副本部署时，实例级信号量只保护单个进程；如果必须保证整个集群的租户额度，租户额度必须使用 Redis 等共享限流器。否则三台副本各自允许 10 个请求，集群实际可能同时运行 30 个请求。

### 9.2 超时传播

请求取消或超时后，必须让下游调用一起停止：

```text
客户端断开
  -> HTTP context 取消
  -> Agent Stream 停止
  -> LLM/Milvus/MCP 使用同一 context 取消
  -> SSE writer 关闭 channel
```

建议分别配置 HTTP、Agent、模型、检索和工具超时，避免某个外部服务长期占用并发名额。

### 9.3 SSE 有界队列

流式输出采用单写协程：

```text
Agent goroutine -> 有界 channel -> SSE writer goroutine -> 客户端
```

规则：

- 一个 Response 只能由一个 writer 写入；
- channel 设置固定容量；
- 客户端断开时立即取消 Agent；
- channel 满时优先终止请求并释放资源；不在服务端无界缓存未发送文本；
- writer 写出失败后记录断开原因，并释放会话和全局并发名额。

## 10. 一次请求的完整流程

```text
1. HTTP 入口验证 token，得到 tenant_id、user_id
2. 生成 request_id，校验 conversation_id 归属
3. 获取会话协调资格；同一会话排队，不同会话继续并行
4. 从 Redis/PostgreSQL 读取历史快照和 version
5. 检查全局/租户 Agent 并发额度
6. 使用复用的 Agent Graph 执行 RAG、工具和模型调用
7. 通过有界 channel 向 SSE writer 发送增量内容
8. 模型完成后先使用 version 条件追加用户消息和助手消息
9. 只有提交成功后才发送 SSE `done`；提交失败则发送 `error`，客户端可使用同一 `idempotency_key` 重试
10. 释放会话资格和并发额度
11. 记录 trace、metrics 和最终状态
```

关键原则是：模型执行期间不修改共享会话对象；只有在结果确定后，才以条件追加方式提交历史。

## 11. 错误处理

| 场景 | 对用户的结果 | 服务端动作 |
| --- | --- | --- |
| 身份无效 | `401` | 不读取会话，不调用模型 |
| 无权访问会话 | 对外统一 `404` | 记录安全审计事件，避免泄露会话存在性 |
| 同会话版本冲突 | `409 conversation_busy` | 自动重试一次，之后结束 |
| 全局/租户额度耗尽 | `429 too_many_requests` | 不创建新的 Agent 执行 |
| LLM/Milvus/MCP 超时 | `504` 或流式 error | 取消下游 context，释放额度 |
| 客户端主动断开 | 无需继续发送 | 停止 Agent 和 SSE writer |
| telemetry 失败 | 不影响业务结果 | 降级为 no-op，并记录一次启动/运行告警 |

任何错误路径都必须释放锁、信号量、连接和 channel。释放逻辑应放在 `defer` 中，并通过测试覆盖。流式请求如果在完成事件前中断，不写入不完整的助手消息；客户端重试必须携带同一个 `idempotency_key`，服务端据此避免重复追加已经提交的结果。

## 12. 分阶段实施计划

### 阶段一：消除单机串话和数据竞争

- 将 `GetMessages` 改为返回快照；使用 `sync.RWMutex`。
- 引入 `SessionKey`，禁止只用裸 `req.Id`。
- 增加单会话锁或本地版本校验。
- 为 Chat 和 ChatStream 补充并发单元测试。

这一阶段的目标是先保证单实例下不会因为两个请求同时操作同一会话而串话。

### 阶段二：复用重资源

- 在启动阶段创建 ChatModel、Embedding、Milvus client、Retriever 和 Agent Graph。
- 通过 Controller 依赖注入 Runtime。
- 将 Collection/Index/Load 检查从请求路径移出。
- 增加启动健康检查和优雅关闭。

这一阶段的目标是降低每请求初始化开销和外部连接压力。

### 阶段三：共享会话和资源治理

- 用 Redis 或 PostgreSQL 替代全局会话 Map。
- 使用版本号条件更新，必要时增加 Redis 锁。
- 增加全局、租户、会话三级并发限制。
- 重构 SSE 为有界队列、单 writer 和取消传播。

这一阶段完成后，服务可以安全地运行多个副本。

### 阶段四：压测和上线治理

- 使用 `go test -race ./...` 检查数据竞争。
- 用 k6、vegeta 或 hey 进行阶梯压测。
- 根据 P95/P99、模型并发、队列等待、429 比例和内存曲线设定容量。
- 配置 Prometheus/Grafana 告警和扩容策略。

## 13. 测试与验收标准

### 13.1 隔离测试

- 用户 A 和用户 B 使用相同 `conversation_id`，不能看到对方历史。
- 同一用户的两个会话并发，历史和工具范围互不影响。
- 不同租户即使 `user_id` 和 `conversation_id` 相同，也不能互相读取。
- 工具从 context 读取的租户范围与认证身份一致，不能被问题文本覆盖。

### 13.2 顺序测试

- 同一会话同时提交两条消息，最终历史顺序与协调器接收顺序一致；测试中应记录 `turn_seq`，不能用客户端网络发送时间猜测顺序。
- 版本冲突只触发有限次数重试，不出现无限循环。
- 一个会话繁忙时，不影响其他会话执行。
- 正常路径由协调器串行化；版本冲突测试应通过模拟重复投递或绕过协调器的写入来验证防覆盖能力。

### 13.3 资源和取消测试

- 100 个不同会话并发时，Agent、LLM、Embedding 和 Milvus 并发数不超过配置上限。
- 慢 SSE 客户端不会导致内存无界增长。
- 客户端断开后，模型、工具和检索调用在规定时间内收到取消。
- 外部服务超时后所有并发名额都能释放。

### 13.4 建议指标

```text
superbizagent_active_requests
superbizagent_active_agent_runs
superbizagent_conversation_wait_seconds
superbizagent_conversation_conflicts_total
superbizagent_limit_rejections_total{scope}
superbizagent_sse_disconnects_total
superbizagent_sse_buffered_bytes
superbizagent_request_duration_seconds
superbizagent_component_errors_total
```

不要把 `request_id`、`conversation_id`、完整问题或 Prompt 作为 metrics label；这些信息只放在 trace 或结构化日志中。

## 14. 附录：实现级接口建议

### 14.1 会话存储接口

```go
type ConversationStore interface {
    Load(ctx context.Context, key SessionKey) (messages []*schema.Message, version int64, err error)
    AppendIfVersion(
        ctx context.Context,
        key SessionKey,
        expectedVersion int64,
        userMessage *schema.Message,
        assistantMessage *schema.Message,
    ) (committed bool, err error)
}
```

实现必须保证 `AppendIfVersion` 是一个原子操作：要么同时追加用户消息和助手消息并递增版本，要么什么都不追加。

Redis 实现可使用 `WATCH/MULTI/EXEC` 或 Lua 脚本完成“检查 version、追加两条消息、递增 version”的原子操作；PostgreSQL 实现应在事务中执行带 `WHERE version = expected_version` 的 `UPDATE`，并检查受影响行数。两种实现都必须保证用户消息和助手消息不会只写入一条。

流式输出只代表“正在生成”，不代表已经提交。客户端在完成事件前断开时，服务端不落库不完整答案；服务端必须在发送 `done` 前完成提交。若提交失败，客户端可能已经看到部分文本，但不会收到成功完成事件；服务端记录可重试状态，并由 `idempotency_key` 防止重复追加。

### 14.2 会话协调接口

```go
type ConversationCoordinator interface {
    Run(ctx context.Context, key SessionKey, fn func(context.Context) error) error
}
```

`Run` 负责同会话顺序和释放锁；业务函数负责读取历史、执行 Agent 和提交结果。不要让 Controller 自己拼接锁的获取和释放逻辑。

### 14.3 配置建议

```yaml
concurrency:
  max_agent_runs: 100
  max_agent_runs_per_tenant: 10
  conversation_queue_size: 1
  acquire_timeout: 3s

request:
  timeout: 120s
  stream_idle_timeout: 30s

conversation:
  backend: redis
  ttl: 24h
  max_messages: 20
  lock_lease: 30s
  lock_renew_interval: 10s

sse:
  buffer_size: 100
  max_buffered_bytes: 1048576
```

具体数值必须通过压测调整；配置示例不是容量承诺。

### 14.4 安全提醒

当前配置文件中存在明文 API Key。上线并发改造前应立即轮换这些 Key，并改为环境变量或密钥管理服务。并发控制不能替代凭据管理。

## 15. 最终结论

高并发并不意味着所有东西都要加锁。正确的做法是：

1. 把每个请求自己的数据放在请求上下文和局部变量中。
2. 把会话历史放到共享、可持久化的存储中。
3. 只对同一个会话排序，不阻塞不同会话。
4. 把模型、Embedding、Milvus 和 Agent Graph 作为可复用的运行时资源。
5. 给外部调用、SSE 和并发执行设置明确上限。
6. 用版本号、trace、metrics 和压测结果证明设计有效。

这样既能避免多用户上下文串扰，也能让系统在达到容量上限时以可预测的方式降级，而不是因为共享状态、慢连接或连接风暴失控。
