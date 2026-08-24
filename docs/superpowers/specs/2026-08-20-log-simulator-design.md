# 场景驱动的 CLS 日志模拟器设计

## 1. 背景与目标

当前 `eval/logs/cls/*.jsonl` 只有少量孤立日志，能够验证 CLS 写入和 MCP 基础查询，但不足以支撑 `eval/datasets/cache_benchmark.jsonl` 中的场景化评测。主要问题包括：

- 测试集包含 30 个场景，现有日志没有可验证的完整覆盖关系。
- 日志缺少 `incident_id`、`trace_id`、`request_id` 等关联字段，无法还原一次事故的处理链路。
- 错误码仅覆盖 `12000000001` 和 `12000000002`，缺少 `12000000003` 和 `12000000004`。
- 同一事故的不同阶段没有连续事件，Agent 即使查到单条日志，也难以判断原因和影响。
- 手工修改时间、ID 和日志数量后，测试结果难以复现。

本次目标是增加一个场景驱动、可复现的日志模拟器，使生成的 CLS 日志与缓存评测集明确对应，同时继续保留本地 JSONL 作为固定测试输入。

本次不修改 `cache_benchmark-v1` runner，不实现缓存率统计，也不让日志模拟器根据自然语言动态推断事故事实。这些内容属于后续独立工作。

## 2. 方案选择

### 2.1 方案 A：手工扩写 JSONL

直接扩写现有 5 个 JSONL 文件。优点是实现成本低，缺点是场景映射容易遗漏，动态时间和关联 ID 维护困难，测试集变化后无法自动发现不一致。

### 2.2 方案 B：结构化场景清单加确定性生成器

在代码中维护事故场景模板，由 `cmd/logsim` 使用固定 seed 生成 JSONL，并可选择上传至 CLS。测试集 ID 与事故模板显式映射，生成前执行覆盖校验。

这是本次采用的方案。它能同时满足场景对应、日志真实性、可复现和后续扩展要求。

### 2.3 方案 C：运行时解析测试问题生成日志

直接读取测试问题，让模型或规则从自然语言推导日志。该方案看似自动化，但问题措辞不等于完整事故事实，生成结果也难以稳定复现，因此不采用。

## 3. 场景映射原则

`cache_benchmark.jsonl` 中的 30 个场景必须全部被映射，但不是每个问题都复制一套日志。

- 语义相同的单轮、多轮、改写和长历史问题复用同一类事故事件链。
- 每条事件链使用唯一的 `incident_id`。
- `benchmark_ids` 记录该事件链能够支持的全部评测场景 ID。
- `scenario` 使用稳定枚举，不使用测试问题中的自然语言作为类型。
- 跨主题场景通过多个事故链共同覆盖，不额外制造内容重复的综合日志。
- 生成器启动时读取 `cache_benchmark.jsonl`，校验每个 benchmark ID 至少映射一次；存在缺失或未知 ID 时直接失败。

五类核心场景如下：

| `scenario` | 测试集主题 | 日志必须提供的关键证据 |
| --- | --- | --- |
| `service_offline` | 服务下线、Pod 反复重启 | 无发布、资源正常、进程 panic、Pod 重启次数、堆栈或错误位置 |
| `api_failure` | 单接口失败率升高 | 接口名、`response`、HTTP 状态、下游超时或本地错误、耗时 |
| `reconciliation` | 与下游对账出现差异 | `error`、`reconciliation`、本地/下游笔数或金额、同步或计算异常 |
| `region_mismatch` | 服务地域与资源地域不匹配 | `region mismatch`、调用方、资源地域、目标地域、错误 MQ 队列 |
| `error_code` | 服务错误码定位 | `12000000001` 至 `12000000004` 及各自原因和上下文 |

## 4. 组件与职责

### 4.1 `cmd/logsim`

提供统一命令入口，负责参数解析、调用生成器、写入 JSONL，并在显式启用时上传 CLS。

建议参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-dataset` | `eval/datasets/cache_benchmark.jsonl` | 用于映射完整性校验的数据集 |
| `-output-dir` | `eval/logs/cls` | JSONL 输出目录 |
| `-seed` | `20260820` | 随机种子，相同输入产生相同业务字段和事件顺序 |
| `-start-time` | 固定测试时间 | 事件链起始时间；显式传入便于完全复现 |
| `-scenario` | `all` | 生成全部或指定场景 |
| `-upload-cls` | `false` | 是否上传到腾讯云 CLS |
| `-topic-id` | 环境变量 `CLS_TOPIC_ID` | CLS Topic ID |

当 `-upload-cls=false` 时，命令不得读取云密钥，也不得产生外部写入。

### 4.2 场景生成包

场景生成逻辑放在可单元测试的内部包中，负责：

- 定义 benchmark ID 到事故场景的显式映射。
- 生成连续事件链和结构化字段。
- 使用 seed 生成稳定的实例名、请求 ID 和 trace ID。
- 校验映射完整性、事件时间顺序和必要证据字段。
- 按场景输出独立 JSONL 文件。

每个场景函数只负责一种事故类型，避免把所有条件集中在一个大型随机生成函数中。

### 4.3 CLS 上传组件

复用 `github.com/tencentcloud/tencentcloud-cls-sdk-go`。现有 `temp/upload_cls_logs.go` 是一次性探针，本次将上传能力收敛到日志模拟器可复用的组件中。

上传所需配置只从命令参数和环境变量读取：

```text
TENCENTCLOUD_SECRET_ID
TENCENTCLOUD_SECRET_KEY
TENCENTCLOUD_REGION
CLS_TOPIC_ID
```

任何密钥都不得写入代码、JSONL、配置示例或测试文档。

## 5. 日志数据契约

每行仍是一个合法 JSON 对象。通用字段如下：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `timestamp` | string | RFC3339 时间，便于本地检查 |
| `level` | string | `DEBUG`、`INFO`、`WARN` 或 `ERROR` |
| `scenario` | string | 五类稳定场景枚举之一 |
| `incident_id` | string | 一次事故事件链的稳定 ID |
| `benchmark_ids` | string | 逗号分隔的评测场景 ID，适配 CLS 字符串字段 |
| `service` | string | 产生日志的服务名 |
| `instance` | string | Pod 或实例名 |
| `region` | string | 当前服务地域 |
| `trace_id` | string | 跨服务调用关联 ID |
| `request_id` | string | 单次请求关联 ID |
| `event` | string | 具体事件，例如 `panic`、`downstream_timeout` |
| `message` | string | 接近真实程序输出的简洁消息 |

按场景可增加以下字段：

```text
method, path, status_code, latency_ms, downstream,
error_code, retry_count, resource_region, target_region,
mq_topic, local_count, downstream_count, amount_delta,
deployment_id, restart_count, stack
```

为兼容 CLS SDK 和动态键值索引，写入 CLS 前所有值转换为字符串。本地生成器内部可以使用强类型结构，但 JSONL 输出也保持字段类型稳定。

## 6. 事件链设计

每个事故不是若干互不相关的随机错误，而是按时间递增的连续链路。每类至少包含以下阶段：

1. 正常基线或任务开始。
2. 异常首次出现。
3. 重试、下游调用或内部处理过程。
4. 对用户或业务造成的可观察影响。
5. 恢复、持续失败或等待人工处理的最终状态。

不同事件可以共享 `incident_id`，一次跨服务调用共享 `trace_id`，单个请求使用不同 `request_id`。噪声日志应与真实业务相关，但不能包含会错误指向另一事故原因的强关键词。

## 7. 生成流程

```text
读取 CLI 参数
    -> 读取 cache_benchmark.jsonl 中全部场景 ID
    -> 加载静态映射并校验完整性
    -> 使用 seed 和 start-time 生成事故链
    -> 校验字段、事件顺序和场景证据
    -> 按场景原子写入 JSONL
    -> 可选上传 CLS
    -> 输出生成数、场景数、映射数和上传结果摘要
```

写文件时先生成到临时文件，全部校验成功后再替换目标文件，防止失败时留下半份数据。只生成单个 `-scenario` 时仍执行该场景相关的映射校验，但不要求其他场景出现在输出中。

## 8. 错误处理

以下情况命令必须以非零状态退出：

- 数据集 JSONL 解析失败。
- benchmark ID 缺少映射，或映射引用了不存在的 ID。
- 场景缺少必要证据字段。
- 输出目录不可写或文件替换失败。
- 启用 CLS 上传但缺少地域、Topic 或密钥。
- CLS SDK 返回上传失败。

上传失败不得删除已经成功生成的本地 JSONL，便于排查和重试。错误信息不得打印 SecretId 或 SecretKey。

## 9. 测试与验收标准

### 9.1 自动化测试

- 相同 seed、起始时间和场景参数生成的内容完全一致。
- 不同 seed 只改变允许变化的 ID、实例和数值，不改变场景证据和映射。
- 所有 JSONL 行均可解析，时间严格按事件链递增。
- 30 个 benchmark ID 全部至少映射一次，不存在未知 ID。
- 五类场景全部生成，每类至少一条完整事故链，总日志数不少于 50 条。
- 四个错误码全部存在，并具有正确原因字段。
- 每个事故链均可通过 `incident_id` 关联，每个跨服务链路可通过 `trace_id` 关联。
- `go test ./...` 通过。

### 9.2 CLS 与 MCP 集成测试

- 使用显式 `-upload-cls` 将生成日志上传到测试 Topic。
- 分别按 `scenario`、`event`、`service`、`request_id`、`incident_id` 和 `error_code` 查询。
- 通过本地 `cls-mcp-server` 的 `SearchLog` 找到预期事故链。
- 至少选择五类场景各一个 benchmark 问题，核对其预期证据能够从日志中查到。
- 在测试文档中记录命令、查询条件、返回数量和结论，但不记录密钥。

## 10. 文档更新

实现完成后更新以下中文文档：

- `docs/cls-mcp-local-deployment-test.md`：补充模拟日志生成、上传和 MCP 查询结果。
- 新增日志模拟器使用文档：说明参数、场景、字段、复现方式和常见错误。
- 如实现与本规格发生差异，在规格中记录最终决策，避免文档与代码脱节。

## 11. 安全与运维约束

- 腾讯云密钥只能来自环境变量，正式使用应采用最小权限 CAM 身份。
- 当前测试 Topic 会产生费用，批量生成和上传默认关闭。
- 日志内容全部为模拟数据，不得使用真实用户姓名、手机号、订单号或资源凭据。
- 已在截图或配置中暴露过的 SecretId、SecretKey 和其他 API Key 应尽快轮换。

## 12. 完成定义

满足以下条件才视为本功能完成：

1. `cmd/logsim` 能确定性生成不少于 50 条场景化 JSONL 日志。
2. 30 个缓存评测场景均有明确映射，五类核心场景和四个错误码全部覆盖。
3. 自动化测试与 `go test ./...` 通过。
4. 日志能够上传测试 CLS Topic，并通过本地 MCP 查询到预期证据。
5. 中文使用文档和 CLS 测试记录已更新。
