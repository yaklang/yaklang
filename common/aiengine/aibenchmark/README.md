# AI Benchmark 脚本使用说明

## 文件位置

- 脚本: `common/aiengine/aibenchmark/generic-event-benchmark.yak`
- 示例配置: `common/aiengine/aibenchmark/generic-event-benchmark.example.json`
- 快速启动脚本: `common/aiengine/aibenchmark/run-generic-event-benchmark.sh`
- 安全发现 Runner: `common/aiengine/aibenchmark/generic-event-runner.yak`
- Runner 示例配置: `common/aiengine/aibenchmark/generic-event-runner.example.json`
- Runner 运行说明: `common/aiengine/aibenchmark/generic-event-runner.md`
- Runner 快速启动脚本: `common/aiengine/aibenchmark/run-generic-event-runner.sh`

## 安全发现 Runner

`generic-event-runner.yak` 是新的单运行单元脚本。它不同于旧的
`generic-event-benchmark.yak`：

- 输入只包含 `target`、`quality_priority_ai_config`、`speed_priority_ai_config`、`limits`、`run_config`
- 不接收 `unit_id`、`module_id`、`task_id`、`vuln_id`、漏洞公开状态
- 不知道哪些漏洞是公开/未公开/预期/非预期
- 输出只描述模型在每个 sample 中发现到的漏洞信息、证据引用和常规运行指标
- 公开 / 未公开 / 非预期 / 误报归类必须由后续 analyzer 或 registry 完成

Runner 输入格式：

```json
{
  "target": {
    "base_url": "https://target.local",
    "entry_path": "/account",
    "task_prompt": "检测账号功能的安全问题"
  },
  "quality_priority_ai_config": {
    "provider": "aibalance",
    "model": "memfit-qwen3.7-max",
    "base_url": "https://aibalance.yaklang.com/v1",
    "api_key": "{{AIBALANCE_PREMIUM_API_KEY}}",
    "temperature": 0.2,
    "max_tokens": 8192
  },
  "speed_priority_ai_config": {
    "provider": "aibalance",
    "model": "memfit-light-free",
    "base_url": "https://aibalance.yaklang.com/v1",
    "api_key": "free-user",
    "temperature": 0.2,
    "max_tokens": 8192
  },
  "limits": {
    "step_budget": 25,
    "time_budget_sec": 1800,
    "samples": 20
  },
  "run_config": {
    "mode": "blackbox",
    "agent_profile": "realworld-web"
  }
}
```

运行：

```bash
common/aiengine/aibenchmark/run-generic-event-runner.sh \
  common/aiengine/aibenchmark/generic-event-runner.example.json
```

如需传 API Key，可以用命令行覆盖；runner 会同时覆盖 quality / speed 两个配置中的
`api_key`，避免把真实密钥写入配置文件：

```bash
common/aiengine/aibenchmark/run-generic-event-runner.sh \
  common/aiengine/aibenchmark/generic-event-runner.example.json \
  --api-key "$AIBALANCE_API_KEY"
```

每个 sample 会生成一条 trial output，完整报告写到 `--output-dir` 下，
原始事件/流/consumption 证据写到 `evidence/*.jsonl`。token 用量来自
[common/schema/ai_event.go](../../../common/schema/ai_event.go) 中的
`EVENT_TYPE_CONSUMPTION = "consumption"` 事件，该事件内容包含
`input_consumption`、`output_consumption`、`cache_hit_token`、`consumption_uuid`；
脚本只读取外层累计值，不读取 `tier_consumption`。`cost_usd` 当前保留为 `null`，
因为仓库内没有统一的 provider/model 价格表；成本核算应由后处理按运行时价格表完成。

自定义模型配置通过 `quality_priority_ai_config` 与 `speed_priority_ai_config`
显式传入。脚本分别调用 `aim.qualityPriorityAIConfig(...)` 与
`aim.speedPriorityAIConfig(...)`，并配合 `ai.baseURL`、`ai.model`、
`ai.temperature`、`ai.maxTokens` 等 option 直接配置本次 run，不依赖全局默认 AI 配置。

上面的示例按你给的 tiered 配置做了展开：`quality_priority_ai_config` 取
`IntelligentModels` 中的 `memfit-qwen3.7-max`，`speed_priority_ai_config` 取
`LightweightModels` 中的 `memfit-light-free`。`VisionModels` 当前不参与这个 runner。

## 功能说明

这个 benchmark 脚本用于批量执行一组 case，并根据 AI 输出内容判断 case 是否通过。

当前判定逻辑非常简单：

- `pass.eventMatch`
  直接匹配 `aim.onEvent(...)` 回调里收到的 `event.Content`
- `pass.streamMatch`
  直接匹配 `aim.onStreamContent(...)` 回调里收到的 `data`

只要某个 case 在执行过程中命中任意一个配置的匹配条件，就会记为 `conditionMatched = true`。
最终 `passed = success && conditionMatched`。

## 配置格式

配置文件是一个 JSON list，每个元素就是一个 case。

最小格式：

```json
[
  {
    "name": "case-name",
    "input": "给 AI 的初始输入",
    "pass": {
      "eventMatch": "希望在 onEvent 的 content 里出现的字符串"
    }
  }
]
```

或者：

```json
[
  {
    "name": "case-name",
    "input": "给 AI 的初始输入",
    "pass": {
      "streamMatch": "希望在 onStream 的 data 里出现的字符串"
    }
  }
]
```

### case 字段

- `name`
  case 名称。可选，不填会自动生成 `case-1`、`case-2`
- `description`
  case 描述。可选
- `input`
  初始输入。推荐直接用这个字段
- `initialInput` / `target` / `goal` / `prompt` / `taskPrompt`
  `input` 的兼容别名，只有在 `input` 为空时才会尝试读取
- `pass`
  通过条件。必须至少包含一个：
  - `eventMatch` 系统事件需要包含的字符串
  - `streamMatch` 流输出需要包含的字符串（流输出主要有两个：ai 输出与工具输出，如果需要匹配flag等工作，大概率是从这里做匹配）

## 运行参数

脚本支持这些 CLI 参数：

- `--config`
  benchmark 配置文件路径，必填
- `--output-dir`
  报告输出目录，默认 `./benchmark-reports`
- `--report-prefix`
  报告文件名前缀，默认 `aim-benchmark`
- `--timeout`
  所有 case 的超时时间，单位秒，默认 `1200` (20分钟)
- `--max-iteration`
  所有 case 的原子任务最大迭代次数，默认 `12`
- `--language`
  所有 case 的语言，默认 `zh`
- `--var key=value`
  配置变量替换，可多次传入。脚本会把配置文件中的 `{{key}}` 替换成对应值

## 运行方式

在**仓库根目录**执行。

### 方式 1：直接用 go run 入口执行

```bash
go run common/yak/cmd/yak.go \
  common/aiengine/aibenchmark/generic-event-benchmark.yak \
  --config common/aiengine/aibenchmark/generic-event-benchmark.example.json
```

### 方式 2：使用包装脚本执行

```bash
common/aiengine/aibenchmark/run-generic-event-benchmark.sh
```

指定配置文件：

```bash
common/aiengine/aibenchmark/run-generic-event-benchmark.sh \
  common/aiengine/aibenchmark/generic-event-benchmark.example.json
```

传额外参数：

```bash
common/aiengine/aibenchmark/run-generic-event-benchmark.sh \
  common/aiengine/aibenchmark/generic-event-benchmark.example.json \
  --timeout 300 \
  --max-iteration 8 \
  --language zh
```

## 示例

### 1. 匹配事件内容

```json
[
  {
    "name": "report-file-created",
    "input": "请生成一份简短的测试 markdown，要求保存为 benchmark.md 文件。",
    "pass": {
      "eventMatch": "benchmark.md"
    }
  }
]
```

适合检测：

- 文件生成路径
- 结构化事件中的关键字
- 某些工具调用结果里的固定字符串

### 2. 匹配流输出内容

```json
[
  {
    "name": "simple-stream-answer",
    "input": "请只输出字符串 STREAM_MATCH_OK，不要输出其他任何内容。",
    "pass": {
      "streamMatch": "STREAM_MATCH_OK"
    }
  }
]
```

适合检测：

- 最终答案中是否出现预期文本
- 流式输出中是否包含某个标记串

## 报告输出

每次运行会在 `--output-dir` 下生成两份报告：

- `*.json`
  完整机器可读报告
- `*.md`
  简要可读报告

报告中会包含：

- 每个 case 的执行结果
- 是否执行成功
- 是否命中 pass 条件
- 命中的 event / stream 片段预览
- 总体统计摘要

## 注意事项

- 这个脚本不会处理 AI 配置，默认使用当前环境里的 AI 默认配置
- 这个脚本不会进行用户交互，内部固定 `allowUserInteract(false)`
- 这个脚本固定开启 `yoloMode()`
