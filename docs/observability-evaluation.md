# 可观测性与评测实施指南

## 当前状态

本指南随实现同步更新。详细设计见
`docs/superpowers/specs/2026-08-19-observability-evaluation-design.md`，逐任务计划见
`docs/superpowers/plans/2026-08-19-observability-evaluation.md`。

技术组件和核心代码说明见
`docs/observability-evaluation-components-and-code.md`。

缓存基准测试集见 `eval/datasets/cache_benchmark.jsonl`，字段和执行规则见
`eval/datasets/cache_benchmark.README.md`。当前数据集用于内容审阅，尚未接入 `cmd/eval` runner。

## 基线验证

```powershell
go test ./...
```

`temp` 下的 Milvus 探针默认不参与构建。需要单独运行时使用对应标签：

```powershell
go run -tags probe_milvus_retrieve ./temp/probe_milvus_retrieve.go
go run -tags inspect_milvus_schema ./temp/inspect_milvus_schema.go
```

## 数据安全规则

Telemetry 不得输出 API Key、Authorization、DSN 密码、完整 prompt、完整工具参数、
模型回答或上传文件内容。指标标签只使用有限集合，原始 query、请求 ID、对话 ID 和未来的
tenant ID 只能进入 trace 属性，并按配置脱敏。

## 稳定知识块 ID 与重新索引

评测集通过文档 ID 标注相关知识块。知识索引现在使用文件名和切分序号的 SHA-256
作为稳定 ID，替代每次索引生成的随机 UUID。Milvus 返回的 HAMMING 数值按距离保存为：

- `_retrieval_distance`
- `_retrieval_rank`
- `_retrieval_collection`

升级后必须重新构建 `biz` collection 中的知识数据，再创建或更新 golden dataset。
旧 UUID 与新 ID 不兼容。验证可复现性时，对同一个文件连续执行两次索引，并分别运行召回；
两次返回的文档 ID、排名和未变更索引下的距离应一致。

## Telemetry 数据流

应用通过 OTLP/gRPC 向 `localhost:4317` 发送 traces 和 metrics。可通过以下环境变量覆盖
`config.yaml`：

- `SUPERBIZAGENT_OTEL_ENABLED`
- `SUPERBIZAGENT_OTEL_SERVICE_NAME`
- `SUPERBIZAGENT_OTEL_ENDPOINT`
- `SUPERBIZAGENT_OTEL_INSECURE`
- `SUPERBIZAGENT_OTEL_SAMPLE_RATIO`
- `SUPERBIZAGENT_OTEL_DEBUG_CONTENT`

Eino 全局 callback 建立 `eino.<component>.<name>` span，覆盖 Graph Node、Retriever、Tool、
ChatModel、Loader、Transformer 和 Indexer。请求根 span 名称为 `HTTP <route>`。流式模型在
第一块 callback 输出到达时记录 `llm.first_token_seconds`。

主要指标：

- `superbizagent_requests_total`
- `superbizagent_request_duration_seconds`
- `superbizagent_component_duration_seconds`
- `superbizagent_component_errors_total`
- `superbizagent_tool_calls_total`
- `superbizagent_retrieval_documents`
- `superbizagent_retrieval_distance`
- `superbizagent_retrieval_empty_total`
- `superbizagent_llm_tokens_total`
- `superbizagent_llm_first_token_seconds`
- `superbizagent_llm_usage_missing_total`
- `superbizagent_sse_interrupts_total`

模型 token 必须来自 Eino model callback 的真实 `TokenUsage`。缺失时增加
`superbizagent_llm_usage_missing_total`，不根据字符数估算。

## 评测命令

离线 smoke（不连接 Milvus/模型）：

```powershell
go run ./cmd/eval -dataset eval/datasets/smoke.jsonl -output eval/results/latest.json
```

真实召回评测（需要本地 Milvus 和 embedding 配置）：

```powershell
go run ./cmd/eval `
  -dataset eval/datasets/smoke.jsonl `
  -output eval/results/retrieval.json `
  -top-k 5 `
  -run-retrieval=true
```

参数 `-run-agent=true` 才会创建 Eino Agent；`-judge=true` 还会调用配置的大模型作为
Judge。退出码为：`0` 全部选中样本完成，`1` 至少一个样本失败，`2` 参数、数据集或依赖
初始化错误。报告以临时文件写入后原子重命名，包含每条样本明细、聚合指标及每项指标的
有效样本分母 `count`。

`eval/datasets/smoke.jsonl` 的参考答案来自现有告警手册。由于稳定 chunk ID 切换前后不兼容，
该文件暂不填写 `relevant_doc_ids`；完成重新索引并人工核验对应 chunk 后，再补充文档 ID 才能
启用 Recall@K、MRR 和 nDCG。

## 已验证项

当前实现已通过：

```powershell
go vet ./...
go test ./...
go run ./cmd/eval -dataset eval/datasets/smoke.jsonl -output eval/results/latest.json
docker compose -f deploy/observability/docker-compose.yml config
```

`go test ./...` 包含 observability、retriever、evaluation、CLI 和 SSE 单元测试。`go test -race`
在本 Windows 环境因未安装 `gcc` 无法编译 `runtime/cgo`，需要 MinGW 或 LLVM C 编译器后再补跑。
本机尚未执行 `docker compose up -d`，因此 Collector/Jaeger/Prometheus/Grafana 的运行时连通性
仍需在安装 Docker 的环境中验证。

当前代码还保留仓库原有 `config.yaml` 中的明文模型/embedding key；这些 key 不会进入 telemetry，
但上线前必须轮换并迁移到环境变量或 Secret 管理系统。
