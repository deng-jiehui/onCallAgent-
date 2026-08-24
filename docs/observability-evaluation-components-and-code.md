# 可观测性与评测基准：技术组件和核心代码说明

本文用于说明本次改造使用的技术组件、运行时数据流和核心代码职责。它适合开发者了解代码结构，也适合运维人员了解本地监控栈如何部署。

## 1. 总体架构

系统由业务应用、AI 编排、知识库、可观测性和离线评测五部分组成：

```text
                         ┌──────────────────────────────┐
                         │          Go 应用              │
                         │  HTTP / SSE / Eino Agent     │
                         └──────────────┬───────────────┘
                                        │ OTLP/gRPC
                         ┌──────────────▼───────────────┐
                         │ OpenTelemetry Collector       │
                         └──────────┬───────────┬────────┘
                                    │           │
                             traces │           │ metrics
                                    ▼           ▼
                               Jaeger      Prometheus
                                    └───────┬───┘
                                            ▼
                                         Grafana

  文档上传 ──> Loader ──> Markdown Splitter ──> Embedding ──> Milvus
  用户问题 ──> Retriever ──> ChatTemplate ──> ReAct Agent ──> Tool / ChatModel

  JSONL 数据集 ──> eval CLI ──> 检索指标 / 回答指标 / 工具指标 / 可选 LLM Judge
```

## 2. 技术组件介绍

### 2.1 Go 和 GoFrame

项目使用 Go 1.24，HTTP 服务和配置主要由 GoFrame 提供：

- `g.Server()` 创建 HTTP 服务。
- `g.Cfg()` 读取 `config.yaml`。
- `ghttp` 提供请求、响应、SSE 和路由中间件。
- `g.Log()` 负责应用级日志。

入口位于 [main.go](../main.go)。应用启动时先初始化 observability，再注册聊天路由和统一响应中间件。

### 2.2 Eino

Eino 是本项目的 Agent 编排框架，负责把模型、Prompt、Retriever 和 Tool 组合成可执行图。

本项目使用的主要 Eino 能力：

- `compose.Graph`：定义节点和边。
- `compose.Runnable`：编译后的可执行图，支持 `Invoke` 和 `Stream`。
- `react.Agent`：执行模型推理和工具调用循环。
- `schema.Message`：统一表示用户、助手、系统和工具消息。
- Eino callback：在组件 start/end/error/stream 生命周期采集 telemetry。

版本：`github.com/cloudwego/eino v0.6.0`。

### 2.3 Milvus

Milvus 用于保存知识块向量和原始内容。当前结构包含：

- `id`：稳定的知识块 ID。
- `vector`：二进制向量。
- `content`：知识块文本。
- `metadata`：来源、标题和召回证据等 JSON metadata。

项目使用 `HAMMING` 距离。这个值表示距离，不是 0 到 1 的相似度概率，因此代码中使用名称
`_retrieval_distance`，评测和 dashboard 也按 distance 展示。

### 2.4 OpenTelemetry

OpenTelemetry 负责统一 trace 和 metrics：

- Trace：查看一次请求从 HTTP、Eino 节点、召回到工具/模型的完整因果链。
- Metrics：按低基数标签聚合请求量、延迟、错误、工具调用、召回距离和 token。
- OTLP/gRPC：应用向 Collector 的传输协议。
- `BatchSpanProcessor` 和周期性 metric reader：异步导出，避免导出失败阻塞业务请求。

版本：`go.opentelemetry.io/otel v1.35.0`。

### 2.5 OpenTelemetry Collector

Collector 位于应用和后端之间，配置文件是 [otel-collector.yaml](../deploy/observability/otel-collector.yaml)：

- `otlp` receiver 接收 gRPC `4317` 和 HTTP `4318`。
- `memory_limiter` 防止 Collector 内存失控。
- `batch` 合并小批量数据，降低网络和后端压力。
- `otlp/jaeger` 将 trace 发送到 Jaeger。
- `prometheus` exporter 在 `8889` 暴露 metrics。
- `health_check` 在 `13133` 提供健康检查。

### 2.6 Jaeger、Prometheus、Grafana

- Jaeger：保存并检索 trace，地址为 <http://localhost:16686>。
- Prometheus：抓取 Collector 的 metrics，地址为 <http://localhost:9090>。
- Grafana：连接 Prometheus 和 Jaeger，提供统一 dashboard，地址为 <http://localhost:3000>。

三者由 [docker-compose.yml](../deploy/observability/docker-compose.yml) 独立管理，不和 Milvus Compose 强绑定。

### 2.7 评测组件

评测不依赖在线平台，核心是 Go 实现的离线 runner：

- JSONL：保存问题、参考答案、相关文档 ID、期望工具和标签。
- 确定性指标：Recall@K、MRR、nDCG、Exact Match、Token F1、工具集合准确率。
- LLM Judge：可选，只用于相关性、忠实度和完整性，不作为唯一 CI 门禁。
- CLI：`cmd/eval` 负责加载数据集、组装依赖、运行评测和原子写入 JSON 报告。

## 3. 核心代码说明

### 3.1 聊天 Agent 图

文件：[internal/ai/agent/chat_pipeline/orchestration.go](../internal/ai/agent/chat_pipeline/orchestration.go:10)

入口函数：

```go
func BuildChatAgent(ctx context.Context) (compose.Runnable[*UserMessage, *schema.Message], error)
```

图中节点和边：

```text
START
 ├─> InputToRag ──> MilvusRetriever ──┐
 └─> InputToChat ─────────────────────┤
                                      ▼
                                ChatTemplate
                                      ▼
                                  ReactAgent
                                      ▼
                                     END
```

- `InputToRag`：从 `UserMessage` 提取 query。
- `MilvusRetriever`：查询知识库并输出 `documents`。
- `InputToChat`：准备用户问题、历史消息和当前时间。
- `ChatTemplate`：把系统提示、历史、用户问题和召回文档组合成模型输入。
- `ReactAgent`：根据模型判断是否调用工具，直到生成最终回答。

### 3.2 知识索引图

文件：[internal/ai/agent/knowledge_index_pipeline/orchestration.go](../internal/ai/agent/knowledge_index_pipeline/orchestration.go:10)

入口函数：

```go
func BuildKnowledgeIndexing(ctx context.Context) (compose.Runnable[document.Source, []string], error)
```

处理顺序：

```text
FileLoader ──> MarkdownSplitter ──> MilvusIndexer
```

- `FileLoader` 读取文件并设置 `_source`、文件名和扩展名 metadata。
- `MarkdownSplitter` 按标题切分文档。
- `MilvusIndexer` 调用 embedding，将内容和向量写入 Milvus。

### 3.3 稳定 chunk ID

文件：[internal/ai/agent/knowledge_index_pipeline/transformer.go](../internal/ai/agent/knowledge_index_pipeline/transformer.go:32)

核心函数：

```go
func stableChunkID(originalID string, splitIndex int) string
```

函数先统一路径分隔符，再计算：

```text
SHA-256(normalized_source + ":" + split_index)
```

这样同一个文件重复索引时，知识块 ID 不会随机变化。评测集中的 `relevant_doc_ids` 依赖这个稳定性。

注意：从旧 UUID 迁移到稳定 ID 后，必须重新构建 Milvus `biz` collection，再人工核验评测标签。

### 3.4 Milvus Retriever 和召回证据

文件：[internal/ai/retriever/retriever.go](../internal/ai/retriever/retriever.go:26)

主要函数：

- `NewMilvusRetriever`：创建 Milvus client 和 embedding。
- `Retrieve`：embedding query、执行 Milvus search、查询文档字段并恢复搜索顺序。
- `searchHits`：把 `SearchResult.IDs`、`Scores` 转成带排名的 `searchHit`。
- `documentsForHits`：按照 hit 顺序组织文档并写入召回 metadata。

返回文档中的召回证据：

```json
{
  "_retrieval_distance": 8,
  "_retrieval_rank": 1,
  "_retrieval_collection": "biz"
}
```

`searchHits` 会严格检查 ID 数量和 score 数量是否一致。缺失分数时返回错误，不用 0 填充，避免评测得到虚假结果。

### 3.5 Observability 配置和 provider

文件：

- [config.go](../internal/observability/config.go:11)
- [provider.go](../internal/observability/provider.go:20)
- [context.go](../internal/observability/context.go:5)
- [metrics.go](../internal/observability/metrics.go:11)

关键接口：

```go
func LoadConfig(ctx context.Context) Config
func Init(ctx context.Context, cfg Config) (func(context.Context) error, error)
func WithRequestInfo(ctx context.Context, info RequestInfo) context.Context
func RequestInfoFromContext(ctx context.Context) RequestInfo
```

`LoadConfig` 读取 `config.yaml` 默认值，并允许环境变量覆盖：

- `SUPERBIZAGENT_OTEL_ENABLED`
- `SUPERBIZAGENT_OTEL_ENDPOINT`
- `SUPERBIZAGENT_OTEL_SAMPLE_RATIO`
- `SUPERBIZAGENT_OTEL_DEBUG_CONTENT`

`Init` 完成 resource、tracer provider、meter provider 和全局 propagator 初始化。关闭函数使用 `sync.Once`，因此应用多个退出路径调用也不会重复关闭 provider。

### 3.6 Eino Callback

文件：[internal/observability/eino_callback.go](../internal/observability/eino_callback.go:29)

入口函数：

```go
func EinoHandler(metricsSet *Metrics) callbacks.Handler
```

生命周期处理：

1. `OnStart` 根据 `RunInfo.Component` 创建 span。
2. `OnEnd` 按 ChatModel、Retriever、Tool 类型提取结构化输出。
3. `OnError` 设置 span Error 状态，记录错误类型和组件错误计数。
4. `OnEndWithStreamOutput` 复制 stream reader，独立统计首 token，不影响业务消费。

不同组件记录不同信息：

| 组件 | 记录内容 |
| --- | --- |
| ChatModel | model、prompt/completion/total token、usage missing、首 token时间 |
| Retriever | 文档数、距离 min/max/avg/p50/p95、空召回、分数缺失 |
| Tool | 工具名、参数长度、参数 SHA-256、耗时、成功/失败 |
| 其他 Eino 节点 | 组件名、类型、耗时、错误状态 |

完整 query、prompt、工具参数和模型回答不会写入 span。

### 3.7 HTTP 请求根 Span

文件：[internal/observability/request.go](../internal/observability/request.go:16)

核心接口：

```go
func StartRequest(ctx context.Context, route, mode, conversationID string) (context.Context, func(error))
```

它负责：

- 生成 `request_id`。
- 保存 `conversation_id`、`tenant_id`、`user_id` 等请求信息。
- 创建 `HTTP <route>` 根 span。
- 记录请求数量和端到端延迟。
- 在 finish 函数中记录业务错误，并保证 span 只结束一次。

聊天入口在以下文件中使用它：

- [chat_v1_chat.go](../internal/controller/chat/chat_v1_chat.go)
- [chat_v1_chat_stream.go](../internal/controller/chat/chat_v1_chat_stream.go)
- [chat_v1_file_upload.go](../internal/controller/chat/chat_v1_file_upload.go)

### 3.8 评测数据集和指标

文件：

- [dataset.go](../internal/evaluation/dataset.go:24)
- [metrics.go](../internal/evaluation/metrics.go:15)

JSONL 样本格式：

```json
{
  "id": "alert-001",
  "question": "服务下线的常见原因是什么？",
  "relevant_doc_ids": ["doc-123"],
  "reference_answer": "参考答案",
  "expected_tools": ["query_internal_docs"],
  "tags": ["knowledge", "alert"]
}
```

`LoadJSONL` 会校验 ID、问题和重复样本。指标函数均为纯函数，方便在没有外部服务的情况下测试：

- `RecallAtK`：前 K 个结果命中的相关文档比例。
- `MRR`：第一个相关文档排名的倒数。
- `NDCGAtK`：按排名折损的归一化增益。
- `ExactMatch`：规范化后答案完全相等。
- `TokenF1`：中英文混合 token 的 precision、recall 和 F1。
- `ToolSetAccuracy`：实际工具集合与期望集合完全一致。

### 3.9 评测 Runner 和 LLM Judge

文件：

- [runner.go](../internal/evaluation/runner.go:74)
- [judge.go](../internal/evaluation/judge.go:32)

`Run` 将每条样本分成三个可选阶段：

```text
检索 ──> 计算 Recall/MRR/nDCG
Agent ──> 记录工具 + 计算 Exact Match/Token F1
Judge ──> 计算相关性/忠实度/完整性
```

阶段之间错误隔离：检索失败不会抹掉 Agent 已得到的回答指标，Judge 失败也不会抹掉确定性指标。每个聚合指标同时保存有效样本数 `count`，避免把不可评分样本当成 0 分。

`ModelJudge.Evaluate` 要求模型返回唯一 JSON 对象，并拒绝：

- JSON 外的解释文本。
- 未知字段。
- 小于 0 或大于 1 的分数。
- 空的 `reason`。

### 3.10 评测 CLI

文件：[cmd/eval/main.go](../cmd/eval/main.go:28)

入口：

```go
func run(args []string, stdout, stderr io.Writer) int
```

常用命令：

```powershell
go run ./cmd/eval `
  -dataset eval/datasets/smoke.jsonl `
  -output eval/results/latest.json `
  -top-k 5
```

退出码：

- `0`：所有选中样本完成。
- `1`：至少一个样本失败。
- `2`：参数、数据集或外部依赖初始化错误。

`writeReport` 先写入 `<output>.tmp`，成功关闭后再原子重命名，避免进程中断留下半份报告。

## 4. 监控指标如何解释

### 请求和组件

- `superbizagent_requests_total`：按 route、mode、status 统计请求量。
- `superbizagent_request_duration_seconds`：HTTP/SSE 端到端延迟。
- `superbizagent_component_duration_seconds`：Eino 组件耗时。
- `superbizagent_component_errors_total`：按组件和错误类型统计失败。

### 工具和召回

- `superbizagent_tool_calls_total`：工具调用次数及成功/失败。
- `superbizagent_retrieval_documents`：每次召回返回的文档数。
- `superbizagent_retrieval_distance`：Milvus HAMMING distance 分布。
- `superbizagent_retrieval_empty_total`：空召回次数。

距离越小通常表示 HAMMING 空间中越接近，但阈值需要结合 embedding、数据集和人工验证设定，不能直接当成百分比。

### 大模型

- `superbizagent_llm_tokens_total{kind="prompt"}`：输入 token。
- `superbizagent_llm_tokens_total{kind="completion"}`：输出 token。
- `superbizagent_llm_tokens_total{kind="total"}`：总 token。
- `superbizagent_llm_first_token_seconds`：流式响应首 token 延迟。
- `superbizagent_llm_usage_missing_total`：模型没有返回真实 usage 的次数。

## 5. 扩展指南

### 增加新的 Eino 组件指标

1. 在 `eino_callback.go` 的 `setInputSummary` 或 `setOutputSummary` 增加类型转换。
2. 在 `metrics.go` 中创建低基数 instrument。
3. 为正常输出、错误和缺失数据分别添加单元测试。
4. 更新 `docs/observability-evaluation.md` 和 Grafana dashboard。

### 增加新的评测指标

1. 在 `internal/evaluation/metrics.go` 添加纯函数。
2. 先写包含边界条件的表驱动测试。
3. 在 `CaseMetrics`、`addCaseAggregates` 和报告 JSON 中接入。
4. 在本说明和评测指南中写清公式、有效样本分母和空输入行为。

### 接入多租户

后续可以复用现有 `RequestInfo.TenantID`：

- 在认证中间件中写入 `tenant_id`。
- Retriever 使用 tenant filter 或 Milvus partition。
- 文件上传按 tenant 隔离目录。
- trace 可以记录 tenant，metrics 必须限制 tenant 标签基数。

## 6. 当前限制

- 本地环境尚未实际启动 Docker 监控栈；已验证 Compose 配置可解析。
- Windows 环境缺少 `gcc`，`go test -race` 需要安装 MinGW 或 LLVM C 编译器。
- smoke 数据集暂不包含 `relevant_doc_ids`，完成稳定 ID 重索引和人工核验后才能启用检索质量指标。
- `config.yaml` 仍需将明文 API Key 迁移到环境变量或 Secret 管理系统。
