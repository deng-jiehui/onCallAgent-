# 可观测性与评测基准设计

## 1. 目标与范围

本阶段只实现两个方向：

1. 为 Eino Agent、知识库召回、工具调用和大模型请求建立可关联的 trace、metrics 和结构化日志。
2. 建立可重复运行的离线评测基准，分别衡量检索质量、工具选择和最终回答质量。

本阶段不实现多模态和多租户，但所有请求级 telemetry 都预留 `tenant_id` 字段，避免后续重新设计上下文传播。

## 2. 当前项目证据

- 聊天图位于 `internal/ai/agent/chat_pipeline`，包含输入转换、Milvus retriever、Prompt 和 ReAct Agent。
- 知识库图位于 `internal/ai/agent/knowledge_index_pipeline`，包含文件加载、Markdown 切分和 Milvus indexer。
- `utility/log_call_back/log_call_back.go` 当前只在 callback start/end 打印文本，没有耗时、错误、token 或 trace 记录。
- `internal/ai/retriever/retriever.go` 从 Milvus 获取 ID 后再查询文档，但没有把 `SearchResult.Scores` 传递给 `schema.Document`。
- Eino 0.6 的 model callback 提供 `TokenUsage`，retriever callback 提供 query/topK/docs，tool callback 提供工具参数和响应。
- 项目已有 OpenTelemetry 间接依赖，但没有初始化 tracer、meter 或 exporter；Docker Compose 当前只包含 Milvus 依赖。
- 当前没有项目测试文件；`go test ./...` 还会被 `internal/logic/sse/sse.go` 的格式化检查和 `temp` 下两个 `main` 文件重名阻断。

## 3. 方案选择

### 3.1 可观测性方案

采用 **Eino callback + OpenTelemetry SDK + Prometheus exporter + OTLP Collector**：

- Eino callback 是统一采集点，覆盖 Graph Node、Retriever、Tool 和 ChatModel，避免在每个 controller 中重复埋点。
- OpenTelemetry span 负责一次请求内的因果关系和跨组件关联。
- Prometheus counter/histogram 负责低成本聚合和告警。
- 应用通过 OTLP/gRPC 将 traces 和 metrics 发送给 Collector；Collector 将 traces 转发到 Jaeger，并通过 Prometheus exporter 暴露聚合端点；Prometheus 只抓取 Collector；Grafana 提供基础 dashboard。

不采用仅日志方案，因为它无法稳定关联一次请求中的召回、工具和模型步骤；不直接绑定第三方观测平台，因为当前项目尚未确定数据出境和供应商策略。

### 3.2 评测方案

采用独立 `eval` CLI 和 JSONL golden dataset：

- 检索层直接调用 retriever，计算 Recall@K、MRR、nDCG、分数摘要和空召回率。
- Agent 层调用 chat pipeline，计算 Exact Match、Token F1 和工具选择准确率。
- LLM Judge 只作为可选扩展，用于相关性、忠实度和完整性，不作为日常 CI 的唯一门禁。

## 4. 可观测性设计

### 4.1 Context 约定

HTTP/SSE 请求创建或继承 `trace_id`，并把以下字段放入 context：

- `request_id`：单次 HTTP/SSE 请求 ID。
- `conversation_id`：当前对话 ID。
- `tenant_id`：当前阶段可为空，后续多租户直接复用。
- `user_id`：若认证层提供则记录低基数标识，不记录原始凭据。

所有 callback 和自定义 retriever/tool 从 context 读取这些字段，作为 span attributes 和 metrics labels。高基数的 query、完整回答和文件内容不得作为 metrics label。

### 4.2 Span 层级

一次聊天请求的逻辑层级为：

```text
HTTP / ChatStream
└── Eino Graph: ChatAgent
    ├── InputToRag
    ├── MilvusRetriever
    ├── ChatTemplate
    └── ReactAgent
        ├── ChatModel (one span per model call)
        └── Tool (one span per tool invocation)
```

知识库索引请求使用同一机制记录 Loader、Transformer、Embedding 和 Indexer。

### 4.3 指标

应用指标名称使用 `superbizagent_` 前缀：

- `superbizagent_requests_total{route,mode,status}`
- `superbizagent_request_duration_seconds{route,mode}`
- `superbizagent_component_duration_seconds{component,type}`
- `superbizagent_component_errors_total{component,type,error_type}`
- `superbizagent_tool_calls_total{tool,status}`
- `superbizagent_retrieval_documents{collection}`
- `superbizagent_retrieval_score{collection}`，记录 score histogram
- `superbizagent_retrieval_empty_total{collection}`
- `superbizagent_llm_tokens_total{model,kind}`，`kind` 为 prompt、completion、total
- `superbizagent_llm_first_token_seconds{model}`（只有流式模型能提供时记录）

不得把原始 query、prompt、工具 JSON 或回答放入 metrics label。

### 4.4 召回分数

Milvus search 结果中的 score 必须与返回文档按位置对齐，并写入文档 metadata，例如 `"_retrieval_score"`。同时保留 `_retrieval_rank` 和 collection 名称。由于当前使用 HAMMING 距离，评测和 dashboard 默认把数值称为 distance/score，不强行转换为相似度概率；需要阈值时使用配置化 distance threshold。

### 4.5 脱敏与采样

- API key、Authorization、DSN 密码和文件内容永不进入 telemetry。
- 默认只记录 query/参数长度、hash、文档 ID、错误类型和截断摘要。
- Debug 模式允许本地开发采样完整输入，但必须显式开启且禁止在生产配置中打开。
- span status 在错误返回或 callback error 时设为 Error，并记录经过分类的错误类型。

## 5. 评测基准设计

### 5.1 数据集格式

数据集文件位于 `eval/datasets/*.jsonl`，每行一个样本：

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

`relevant_doc_ids` 支持多个正确文档；`reference_answer` 和 `expected_tools` 可为空，以便构造只评测召回或只评测回答的样本。

Golden label 依赖稳定文档 ID。当前 Markdown splitter 使用随机 UUID，实施时必须改成基于规范化 source 和 split index 的 SHA-256 稳定 ID，并在切换后重新索引知识库。否则每次重建索引都会使 `relevant_doc_ids` 失效，Recall@K、MRR 和 nDCG 无法跨版本比较。

### 5.2 指标定义

- Recall@K：前 K 个文档中命中的相关文档数 / 相关文档总数。
- MRR：第一个相关文档排名的倒数；没有命中时为 0。
- nDCG@K：按相关文档集合计算的归一化折损累计增益。
- Exact Match：规范化空白、大小写后，回答与参考答案完全相等。
- Token F1：对规范化后的中英文 token/字符序列计算 precision、recall、F1。
- 工具选择准确率：期望工具集合与实际调用工具集合完全匹配的样本比例。
- LLM Judge：输出相关性、忠实度、完整性 0~1 分及理由，单独记录模型和版本。

### 5.3 运行与输出

`eval` runner 必须支持指定数据集、topK、模型配置和输出路径，输出 JSON：

- 每条样本的检索文档、分数、工具调用、回答、指标和错误。
- 聚合指标、样本数、成功数、失败数。
- embedding/model/topK 配置和 git revision（若仓库有 git）。

评测默认不依赖真实外部 API：retriever/model/tool 使用 fake 实现完成单测；真实评测命令显式通过配置开启外部服务。

## 6. 失败处理与兼容性

- telemetry 初始化失败不得阻断聊天请求；应用降级为 no-op tracer/meter，并打印一次启动错误。
- callback 序列化或 exporter 失败不得改变 Agent 输出；错误仅增加内部计数。
- Milvus 没有 score 或 score 数量异常时，保留文档返回并记录 `score_missing`，评测样本标记为不可评分而不是伪造分数。
- token usage 缺失时不估算真实 token，记录 usage_missing；指标非空率按真实返回值统计。
- 流式响应中途断开时关闭 span，记录 SSE 中断和已发送 token/字符数。

实现补充：当前版本使用 OTLP/gRPC exporter，应用将 traces/metrics 发送到 Collector；Collector
的 Prometheus exporter 由 Prometheus 抓取。启动失败时应用记录错误并继续使用 no-op provider。

## 7. 测试与验收

实施顺序：

1. 修复测试基线，隔离 `temp` 探针并处理 SSE 格式化问题，使目标业务包可重复测试。
2. 为 telemetry callback、metric 聚合、retriever score 对齐和评测指标添加纯单元测试。
3. 使用 fake graph/model/retriever/tool 做 callback 集成测试，验证 span 名称、属性、错误状态和 token 采集。
4. 添加 10~30 条人工审核的初始 golden 数据和离线 smoke test。
5. 添加 Collector、Prometheus、Grafana、Jaeger Compose 配置和运行手册。

首期验收门槛：

- 关键 Graph/Tool/Retriever/Model 节点 trace 覆盖率 100%。
- 工具耗时、召回文档数/分数分布、模型 token 在 dashboard 可查询。
- token usage 非空率达到 95% 以上；缺失情况可见且不被伪造。
- 评测 runner 可重复运行，输出每条样本明细和聚合结果。
- telemetry 依赖不可用时，聊天主流程仍能正常返回业务错误或结果。

## 8. 后续扩展点

- 多模态：沿用 Eino `schema.Message.UserInputMultiContent`，复用 model span 和 token metrics。
- 多租户：在 context 中启用 `tenant_id`，将其用于 Milvus partition/filter、文件目录和权限中间件；高基数 tenant 只在 trace 中记录，metrics 需限制租户维度。
- 线上反馈闭环：从脱敏 trace 采样新问题，人工审核后追加到 JSONL golden dataset，并保留数据集版本。

## 9. 实现结果

已落地文件包括 `internal/observability`、`internal/evaluation`、`cmd/eval`、
`deploy/observability` 以及 `docs/observability-evaluation.md`。稳定 chunk ID、召回距离元数据、
Eino callback、OTLP provider、确定性评测指标和 CLI smoke 已完成；真实 Collector 启动验证和
带 `relevant_doc_ids` 的 golden dataset 需要部署 Milvus/Docker 后继续完成。
