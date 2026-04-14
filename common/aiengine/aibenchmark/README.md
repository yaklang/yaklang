# AI Benchmark 脚本使用说明

## 文件位置

- 脚本: `common/aiengine/aibenchmark/generic-event-benchmark.yak`
- 示例配置: `common/aiengine/aibenchmark/generic-event-benchmark.example.json`
- 快速启动脚本: `common/aiengine/aibenchmark/run-generic-event-benchmark.sh`

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