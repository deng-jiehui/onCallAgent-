# 缓存基准测试集说明

## 1. 文件用途

`cache_benchmark.jsonl` 用于比较不同上下文构造版本的模型 Prompt Cache 表现。样本采用口语化的运维事故场景，而不是直接询问知识点。它不是线上流量样本，也不替代 `smoke.jsonl`：

- `smoke.jsonl` 主要用于常规回答、工具和检索质量回归。
- `cache_benchmark.jsonl` 主要用于可重复制造稳定前缀、历史增长和召回变化，并同时提供质量保护标签。

当前版本包含 30 个场景、5 个类别，每类 6 个场景。

场景编写遵循以下约束：

- 先描述业务影响、时间、监控现象或已知线索，再询问下一步动作。
- 问题中尽量不直接泄露答案关键词。
- 多轮对话通过值班人员逐步补充证据推进，不连续考查孤立知识点。
- 语言采用同事协作时的口语表达，同时保留可复现的事实和质量标签。

## 2. 场景类别

| `category` | 数量 | 目的 |
| --- | ---: | --- |
| `single_turn_repeat` | 6 | 同一个单轮问题重复执行，测量最直接的热缓存命中 |
| `multi_turn_history` | 6 | 历史逐轮增长，观察缓存前缀能否随对话复用 |
| `shared_prefix_question_variation` | 6 | 固定系统/工具前缀，变化用户问题 |
| `retrieval_variation` | 6 | 使用改写或跨主题问题触发不同召回内容 |
| `long_history` | 6 | 使用五轮对话观察较长历史下的缓存率和上下文质量 |

## 3. 字段说明

| 字段 | 说明 |
| --- | --- |
| `schema_version` | 数据集结构版本，当前为 `cache-benchmark-v1` |
| `id` | 场景唯一 ID |
| `category` | 场景类别 |
| `group_id` | 可共享业务主题或上下文前缀的分组 |
| `conversation_id` | 固定的测试对话 ID |
| `warmup_runs` | 预热次数；预热结果不计入正式缓存率 |
| `measured_runs` | 正式计量的重复次数 |
| `concurrency` | 第一版固定为 1，先排除并发干扰 |
| `reset_between_turns` | 每轮是否清空历史；`false` 表示顺序构造多轮对话 |
| `turns` | 本场景需要执行的请求序列 |
| `reference_answer` | 回答质量保护标签 |
| `expected_tools` | 期望调用的工具集合 |
| `expected_document_topics` | 应命中的文档主题；稳定 ID 重索引后用于人工补充文档 ID |
| `tags` | 筛选场景所需标签 |

## 4. 建议执行协议

对 `baseline` 和每个候选上下文版本执行完全相同的协议：

1. 固定模型、模型版本、区域、temperature、topK、embedding、工具集合及工具顺序。
2. 每个场景先执行 `warmup_runs` 次，不计入正式结果。
3. 再执行 `measured_runs` 次，记录每次请求的 prompt token、cached token、首 token 延迟、总延迟和质量指标。
4. `reset_between_turns=false` 时，同一次场景运行中的 `turns` 顺序执行并保留历史。
5. `reset_between_turns=true` 时，每个 turn 使用空历史，但保持相同的系统指令和工具定义。
6. 每个上下文版本生成独立报告，不覆盖以前的结果。

推荐结果命名：

```text
eval/results/cache/
├── baseline.json
├── stable-prefix.json
├── history-summary-v1.json
└── retrieval-compression-v1.json
```

## 5. 核心统计口径

```text
Token 缓存率 = sum(cached_tokens) / sum(prompt_tokens)

请求命中率 = cached_tokens > 0 的请求数 / 返回真实 usage 的请求数

平均未缓存 Prompt Token =
sum(prompt_tokens - cached_tokens) / 返回真实 usage 的请求数
```

预热请求、usage 缺失请求和失败请求必须分别计数，不能混入有效分母。

## 6. 审阅重点

请重点确认：

1. 问题和参考答案是否符合真实运维表达。
2. 多轮问题是否具有自然的上下文承接关系。
3. `expected_tools` 是否符合你希望 Agent 采取的行为。
4. 是否需要增加其他业务主题，而不只是当前告警处理手册中的五类知识。
5. 每个场景正式执行 5 次是否符合可接受的模型调用成本。

## 7. 当前限制

- 当前 runner 尚未读取此 schema；本文件先用于内容审阅。
- `expected_document_topics` 不是最终检索标签。完成稳定 chunk ID 重索引后，应人工核验并补充 `relevant_doc_ids`。
- 数据集基于当前 `docs/告警处理手册.md`，业务覆盖面受现有文档限制。
- 实际是否产生 cached token 取决于模型供应商的缓存策略、最小前缀长度、有效时间和账号/区域配置。
