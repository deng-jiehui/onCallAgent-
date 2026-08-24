# RAG 测试记录

## 1. 测试范围

本次测试覆盖知识文档重索引、Milvus 连接、真实召回、稳定 ID、重复索引幂等性和 Agent 端到端调用。

测试日期：2026-08-20。

## 2. 测试环境

- Milvus：Docker `milvusdb/milvus:v2.5.10`。
- Milvus 地址：`localhost:19530`。
- Embedding：DashScope `text-embedding-v4`。
- 知识库：`agent.biz`。
- 知识文件：5 份 runbook 加 1 份目录文件。

## 3. 执行命令

启动 Milvus：

```powershell
docker compose -f manifest/docker/docker-compose.yml up -d
```

重建知识索引时，从知识命令目录执行，并显式指定配置文件：

```powershell
Push-Location internal/ai/cmd/knowledge_cmd
$env:GF_GCFG_PATH = (Resolve-Path '../../../..').Path
$env:GF_GCFG_FILE = 'config.yaml'
go run .
Pop-Location
```

执行 smoke 召回评测：

```powershell
go run ./cmd/eval `
  -dataset eval/datasets/smoke.jsonl `
  -output eval/results/smoke-retrieval.json `
  -run-retrieval `
  -top-k 5
```

## 4. 测试结果

### 4.1 Milvus 和索引

- Milvus 健康检查返回 `200 OK`。
- `localhost:19530` TCP 连接成功。
- 索引命令成功处理 6 个 Markdown 文件，每个文件生成 1 个主题 chunk。
- 两次完整重索引后，collection 文档总数仍为 6，稳定 ID 未重复。
- 召回结果包含 `_retrieval_rank` 和非零 `_retrieval_distance`。

### 4.2 Smoke 召回

样本数为 10，执行失败数为 0。修正主题与稳定 ID 的映射后，当前基线结果为：

| 指标 | 结果 |
| --- | ---: |
| Recall@5 | 1.00 |
| MRR | 0.825 |
| nDCG@5 | 0.869 |

报告文件：`eval/results/smoke-retrieval-final.json`。

### 4.3 Agent 端到端

对 4 个错误码场景进行了少量真实 Agent 测试，暂未通过，原因属于环境配置：

- `mcp_url` 指向 `http://localhost:3000/sse`，当前端口未启动，因此 MCP 工具不可用。
- ChatModel 请求返回 HTTP 404，提示模型 `deepseek-v3-2-251201` 不存在或当前账号无权访问。

这部分不能作为回答质量结论，需要修正 MCP 服务和模型 endpoint 后重新执行。

## 5. 已修复问题

索引命令原先直接把 Windows 反斜杠路径拼入 Milvus JSON 过滤表达式，遇到 `\\r` 等字符时会解析失败，而且 WalkDir 错误没有向外传播。现在已统一构造过滤表达式，并在索引失败时返回非零状态。

## 6. 当前限制

- `cache_benchmark.jsonl` 是多轮缓存协议，现有 `cmd/eval` 仍只支持单轮 `question`，因此本次没有伪造缓存率结果。
- Prompt Cache 测试需要记录模型返回的 `prompt_tokens` 和 `cached_tokens`，并且必须固定模型、工具顺序、Embedding 和上下文构造版本。
- 当前配置文件包含敏感 API Key，正式环境应先轮换并迁移到环境变量或 Secret 管理系统。

## 7. 下一步

1. 配置可用的 ChatModel endpoint 和模型名称。
2. 启动或修正本地 MCP SSE 服务，并确认 `localhost:3000/sse` 可访问。
3. 实现 `cache_benchmark-v1` 多轮 runner，记录缓存 token、延迟和回答质量。
4. 在 baseline 和候选上下文版本之间生成独立缓存报告。
