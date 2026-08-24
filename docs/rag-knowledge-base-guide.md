# RAG 知识库维护和重索引指南

## 1. 知识源目录

当前正式知识源位于：

```text
internal/ai/cmd/knowledge_cmd/docs/
├── 告警处理手册.md
└── runbooks/
    ├── 服务下线.md
    ├── 接口失败率过高.md
    ├── 与下游对账发现差异.md
    ├── 发现服务地域与资源地域不匹配.md
    └── 服务错误码与常见原因.md
```

根目录下的 `docs/告警处理手册.md` 是原始参考材料，不应和上述 runbook 同时索引，否则会形成重复知识块。

## 2. 文档结构约定

每份 runbook 使用一个一级标题表示主题，正文使用二级和三级标题组织：

- 适用范围
- 常见现象
- 初步判断
- 排查步骤
- 验证恢复
- 常见误判
- 升级条件

当前 Markdown splitter 只按一级标题切分。因此每份 runbook 会形成一个主题完整的主要 chunk，避免一个排查流程被拆散到多个互不完整的结果中。

## 3. 重索引命令

知识索引 CLI 使用相对路径 `./docs`，必须在命令目录运行：

```powershell
Push-Location internal/ai/cmd/knowledge_cmd
go run .
Pop-Location
```

不要直接在仓库根目录执行 `go run ./internal/ai/cmd/knowledge_cmd`。这种运行方式会把仓库根目录的 `docs/` 当成知识源，可能错误索引设计文档和实施说明。

执行前需要保证：

- Milvus 已启动并可通过 `localhost:19530` 访问。
- embedding 模型配置有效。
- 当前进程能够读取 `config/config.yaml`。

## 4. 重索引后的验证

至少使用以下五类问题各验证一次：

1. 服务没有发布但 Pod 反复重启，应该怎么排查？
2. 单个接口失败率突然升高，下一步看什么？
3. 对账任务成功但金额不一致，如何定位？
4. 资源地域和计费地域不一致，优先检查哪里？
5. 错误码 `12000000002` 表示什么？

验证内容：

- 返回的文档主题与问题一致。
- `_retrieval_rank` 从 1 开始且顺序正确。
- `_retrieval_distance` 存在，不是伪造的 0。
- 相同文档重复索引后 ID 保持稳定。
- 召回结果不同时出现旧总手册和对应 runbook 的重复正文。

## 5. 与评测集对齐

缓存测试集中的 `expected_document_topics` 已与 runbook 一级标题对齐。完成重索引后，应执行真实召回并人工核验 chunk，再把稳定 ID 写入：

- `eval/datasets/smoke.jsonl` 的 `relevant_doc_ids`
- `eval/datasets/cache_benchmark.jsonl` 每个 turn 的 `relevant_doc_ids`

在人工核验前，不要根据标题自行计算或猜测文档 ID。

## 6. 当前仍需业务补充的信息

- 服务负责人和升级联系人。
- 告警级别和升级时限。
- 不同服务实际使用的日志主题和地域映射。
- 可执行的止损、回滚和恢复命令。
- 经过脱敏的真实日志样例。
- 故障处理完成后的业务验证指标。

## 7. 本地真实验证记录

2026-08-20 已在本地 Milvus 2.5.10 上完成一次重索引和 smoke 召回验证：

- 索引文件：5 份 runbook 加 1 份目录文件。
- 评测样本：`eval/datasets/smoke.jsonl` 共 10 条。
- 结果：Recall@5 = 1.00，MRR = 0.825，nDCG@5 约为 0.869，执行失败数为 0。
- 召回结果包含 `_retrieval_rank` 和非零 `_retrieval_distance`。

Windows 下索引命令会把文件路径中的 `\\` 规范化为 `/` 后再生成 Milvus JSON 过滤表达式，避免 `\\r` 等字符被 Milvus 当成转义序列。索引命令遇到任意文件错误会以非零状态退出，不应只看单个 `[start]` 日志判断成功。

缓存基准集仍需单独的多轮 runner 执行；当前 `cmd/eval` 只支持 smoke 这类单轮 JSONL。
