# 可观测性与评测基准实施计划

> **供执行代理使用：** 实现任务时必须逐项执行并保留复选框状态。每个任务都应先写失败测试，再写最小实现，最后运行验证命令。

**目标：** 在不改变现有聊天行为的前提下，加入 Eino Agent 全链路可观测性和可重复运行的检索/Agent 评测基准。

**架构：** Go 进程通过 OTLP/gRPC 将 trace 和 metrics 发送到 OpenTelemetry Collector。Eino callback 记录 Graph、Retriever、Tool 和 ChatModel 活动；自定义 Milvus retriever 将排名距离写入文档 metadata。独立的 `cmd/eval` 程序读取版本化 JSONL 样本，评估检索、回答、工具选择和可选的 LLM Judge 指标。

**技术栈：** Go 1.24、Eino 0.6.0、OpenTelemetry Go 1.35.0、OTLP/gRPC、OpenTelemetry Collector、Prometheus、Grafana、Jaeger、Milvus 2.5。

## 全局约束

- 保持现有 Eino 图结构和公开聊天 API 行为不变。
- telemetry 初始化或导出失败时必须降级为 no-op，不能导致聊天请求失败。
- 不得导出 API Key、Authorization、DSN 密码、完整 prompt、完整工具参数、回答或上传文件内容。
- metrics 标签必须限制基数；`request_id`、`conversation_id`、原始 query、文档 ID 和未来的 `tenant_id` 只能记录在 trace 中。
- Milvus HAMMING 数值是距离，不是相似度概率；按 `retrieval.distance` 暴露，不做反转。
- token 必须来自模型响应 metadata；缺失时记录缺失，不自行估算。
- 真实网络评测必须显式开启；单元测试使用 fake，不依赖 Milvus 或模型凭据。
- 当前目录没有 `.git`；只有恢复仓库元数据后才能执行 commit 步骤。

## 文件职责

- `internal/observability/config.go`：读取 OTLP、服务名、采样率和脱敏配置。
- `internal/observability/provider.go`：创建 tracer/meter provider，并提供幂等关闭函数。
- `internal/observability/context.go`：提供类型安全的请求上下文读写方法。
- `internal/observability/metrics.go`：集中创建 metrics 和有限标签。
- `internal/observability/eino_callback.go`：实现 Eino start/end/error/stream callback。
- `internal/observability/request.go`：创建 HTTP 根 span 和请求计数。
- `internal/ai/retriever/retriever.go`：映射 Milvus 排名结果并保存距离 metadata。
- `internal/evaluation/dataset.go`：JSONL 数据结构和校验。
- `internal/evaluation/metrics.go`：确定性评测指标。
- `internal/evaluation/runner.go`：检索和 Agent 评测编排。
- `internal/evaluation/judge.go`：可选的 Eino ChatModel Judge 和严格 JSON 解析。
- `cmd/eval/main.go`：评测 CLI 和真实依赖组装。
- `deploy/observability/*`：Collector、Prometheus、Grafana、Jaeger 配置。
- `docs/observability-evaluation.md`：运行、评测和故障排查手册。

---

### 任务 1：恢复可靠的测试基线

**文件：**

- 修改：`internal/logic/sse/sse.go:51`
- 修改：`temp/probe_milvus_retrieve.go:1`
- 修改：`temp/inspect_milvus_schema.go:1`
- 新建：`internal/logic/sse/sse_test.go`
- 新建：`docs/observability-evaluation.md`

**步骤：**

- [x] 先添加 `TestFormatEvent`，断言输出包含 `event: message` 和 `data: hello`。
- [x] 运行 `go test ./internal/logic/sse -run TestFormatEvent -v`，确认因 `formatEvent` 不存在而失败。
- [x] 将 SSE 输出抽取到 `formatEvent`，使用 `Response.Write`，避免把动态内容作为 `Writefln` 格式串。
- [x] 为两个 `temp` 探针分别添加 `probe_milvus_retrieve` 和 `inspect_milvus_schema` 构建标签。
- [x] 运行 `go test ./...`，确认全量通过。
- [x] 在实施指南中记录探针的显式运行命令和无密钥规则。

### 任务 2：让知识块 ID 和召回距离可复现

**文件：**

- 修改：`internal/ai/agent/knowledge_index_pipeline/loader.go`
- 修改：`internal/ai/agent/knowledge_index_pipeline/transformer.go`
- 修改：`internal/ai/retriever/retriever.go`
- 新建：`internal/ai/agent/knowledge_index_pipeline/transformer_test.go`
- 新建：`internal/ai/retriever/retriever_test.go`

**步骤：**

- [x] 测试 `stableChunkID` 在相同 source/splitIndex 下稳定，并统一 Windows/Unix 路径分隔符。
- [x] 测试 Milvus `IDs` 和 `Scores` 的位置对齐、排名生成、分数缺失错误。
- [x] 将文件加载器设置为 `UseNameAsID: true`。
- [x] 用规范化 source 和 split index 的 SHA-256 生成 chunk ID。
- [x] 将搜索结果映射为 `searchHit{ID, Distance, Rank}`，严格校验 ID/Score 数量。
- [x] 让 `documentsForHits` 按搜索排名恢复文档顺序，并写入 `_retrieval_distance`、`_retrieval_rank`、`_retrieval_collection`。
- [x] 重新索引说明已写入实施指南；旧 UUID 与新 ID 不兼容。

### 任务 3：实现 OpenTelemetry provider、context 和 metrics

**文件：**

- 修改：`go.mod`、`go.sum`、`config.yaml`
- 新建：`internal/observability/config.go`
- 新建：`internal/observability/context.go`
- 新建：`internal/observability/provider.go`
- 新建：`internal/observability/metrics.go`
- 新建：`internal/observability/*_test.go`

**步骤：**

- [x] 引入 OpenTelemetry `v1.35.0` 的 OTLP trace/metric gRPC exporter。
- [x] 测试默认配置、环境变量覆盖、采样率范围、请求上下文往返和幂等关闭。
- [x] 实现 `LoadConfig(ctx)`、`Init(ctx, cfg)`、`WithRequestInfo`、`RequestInfoFromContext` 和 `Instruments`。
- [x] 创建 `superbizagent_` 前缀的请求、组件、工具、召回、token、首 token 和 SSE 指标。
- [x] 限制 metrics 标签为低基数集合，不把 query、request ID 或文档 ID放入标签。
- [x] 写入 `config.yaml` 的非密钥默认配置：`localhost:4317`、采样率 `1.0`、关闭 debug content。
- [x] 运行 `go test ./internal/observability -v`、`go test ./...`；本机 `-race` 因缺少 `gcc` 无法执行。

### 任务 4：接入 Eino callback 和请求入口

**文件：**

- 新建：`internal/observability/eino_callback.go`
- 新建：`internal/observability/request.go`
- 新建：`internal/observability/*_test.go`
- 修改：`main.go`
- 修改：`internal/controller/chat/chat_v1_chat.go`
- 修改：`internal/controller/chat/chat_v1_chat_stream.go`
- 修改：`internal/controller/chat/chat_v1_file_upload.go`
- 修改：`utility/log_call_back/log_call_back.go`

**步骤：**

- [x] 用内存 span recorder 测试 ChatModel token、Retriever 距离摘要、Tool 脱敏和错误状态。
- [x] 实现 `EinoHandler(metrics)`，建立 `eino.<component>.<name>` span。
- [x] 模型 callback 记录 prompt/completion/total token；缺失时记录 `llm.usage_missing` 和 missing counter。
- [x] Retriever callback 记录文档数、距离 min/max/avg/p50/p95 和空召回。
- [x] Tool callback 只记录参数长度和 SHA-256，绝不记录密钥值。
- [x] 流式 callback 使用独立 reader 副本统计首 token，不消费业务 reader。
- [x] 实现 `StartRequest`，生成 request ID、记录 route/mode/conversation ID，并保证 finish 幂等。
- [x] 启动时注册全局 callback；Chat、ChatStream、FileUpload 创建请求根 span；移除生产环境完整输入日志。
- [x] 运行 `go test ./...` 和 `go vet ./...`。

### 任务 5：实现确定性评测指标和数据集校验

**文件：**

- 新建：`internal/evaluation/dataset.go`
- 新建：`internal/evaluation/metrics.go`
- 新建：`internal/evaluation/dataset_test.go`
- 新建：`internal/evaluation/metrics_test.go`

**步骤：**

- [x] `LoadJSONL(io.Reader)` 校验必填 `id/question`、重复 ID、空行、JSON 错误行号和可选字段。
- [x] 实现 `RecallAtK`、`MRR`、`NDCGAtK`，先去重检索 ID 再计算排名。
- [x] 实现 `ExactMatch`、中文/英文 Token F1 和忽略顺序的 `ToolSetAccuracy`。
- [x] 覆盖空集合、重复 ID、中文标点、英文大小写和工具顺序测试。
- [x] 连续运行 `go test ./internal/evaluation -count=10 -v`，结果稳定；`-race` 等待补齐 C 编译器后运行。

### 任务 6：实现检索/Agent 评测 runner、Judge 和 CLI

**文件：**

- 新建：`internal/evaluation/runner.go`
- 新建：`internal/evaluation/judge.go`
- 新建：`internal/evaluation/runner_test.go`
- 新建：`internal/evaluation/judge_test.go`
- 新建：`cmd/eval/main.go`、`cmd/eval/main_test.go`
- 新建：`eval/datasets/smoke.jsonl`

**步骤：**

- [x] 使用 fake retriever/Agent 测试召回、回答、工具、聚合分母、部分失败、标签筛选和样本顺序。
- [x] 实现 `Run(ctx, dataset, cfg, deps)`，分别记录 retrieval、agent、judge 错误。
- [x] Tool recorder 只保存工具名，不保存参数。
- [x] 实现严格 JSON Judge，拒绝额外 prose、未知字段、越界分数和空 reason。
- [x] CLI 支持 `-dataset`、`-output`、`-top-k`、`-tags`、`-run-retrieval`、`-run-agent`、`-judge`。
- [x] 使用临时文件写报告后原子重命名；退出码 `0/1/2` 分别表示成功、样本失败、参数/初始化错误。
- [x] 添加 10 条来自告警手册的 smoke 样本；未伪造 `relevant_doc_ids`，待稳定 ID 重索引并人工核验后补充。
- [x] 运行 `go run ./cmd/eval -dataset eval/datasets/smoke.jsonl -output eval/results/latest.json`。

### 任务 7：加入本地监控栈和 Grafana 仪表盘

**文件：**

- 新建：`deploy/observability/docker-compose.yml`
- 新建：`deploy/observability/otel-collector.yaml`
- 新建：`deploy/observability/prometheus.yml`
- 新建：`deploy/observability/grafana/provisioning/*`
- 新建：`deploy/observability/grafana/dashboards/superbizagent.json`
- 新建：`deploy/observability/README.md`

**步骤：**

- [x] Collector 接收 OTLP gRPC/HTTP，批处理后将 trace 发往 Jaeger，并通过 Prometheus exporter 暴露 metrics。
- [x] 为 Jaeger、Collector、Prometheus、Grafana 固定镜像版本、健康检查、命名卷和 `restart: unless-stopped`。
- [x] Grafana 预置 Prometheus/Jaeger 数据源及请求、延迟、组件、工具、召回、token 面板。
- [x] 运行 `docker compose -f deploy/observability/docker-compose.yml config`，配置解析通过。
- [ ] 在安装 Docker 的环境运行 `up -d`，验证 Collector/Jaeger/Prometheus/Grafana 的实际连通性。

### 任务 8：最终验证和文档同步

**步骤：**

- [x] 运行 `gofmt`、`go vet ./...`、`go test ./...`。
- [x] 运行离线评测 smoke，10 条样本全部完成并生成 `eval/results/latest.json`。
- [x] 检查 Compose 配置和 Grafana JSON。
- [x] 记录 Windows 缺少 `gcc` 导致 `go test -race` 无法执行。
- [x] 在设计文档和实施指南中同步实际文件、验证证据、已接受偏差和后续前置条件。
- [ ] 轮换 `config.yaml` 中的明文 API Key，并迁移到环境变量或 Secret 管理系统。

## 当前验证结论

已通过：`go vet ./...`、`go test ./...`、离线评测 smoke、`docker compose config`。

尚待具备环境后执行：`go test -race`、Docker 监控栈运行时连通性、稳定 ID 重索引后的
`relevant_doc_ids` 人工核验，以及 API Key 轮换。
