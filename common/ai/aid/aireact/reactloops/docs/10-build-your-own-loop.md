# 10. 端到端：从零构建一个 loop_xxx

> 回到 [README](../README.md) | 上一章：[09-capabilities.md](09-capabilities.md) | 下一章：[11-case-studies.md](11-case-studies.md)

本章用 12 步从零构建一个完整的专注模式。我们要做的 loop 叫 `loop_log_analyze`：分析一段日志，找出异常事件并生成报告。

最后给一份完整 checklist，对照后即可上线。

---

## 步骤 1：创建目录结构

```text
common/ai/aid/aireact/reactloops/
├── loop_log_analyze/
│   ├── init.go                                  # 入口注册 + 主 factory
│   ├── action_extract_anomalies.go              # 自定义 action：抽异常
│   ├── action_directly_answer.go                # 覆盖默认 directly_answer
│   ├── finalize.go                              # OnPostIteraction hook
│   └── prompts/
│       ├── persistent_instruction.txt           # 主 prompt（人格设定）
│       ├── reactive_data.txt                    # 反应数据模板
│       └── reflection_output_example.txt        # 输出示例
└── reactinit/
    └── init.go                                  # 加 import
```

## 步骤 2：写 prompts/persistent_instruction.txt

这是 loop 的"人格"。每轮 prompt 都会渲染。

```text
你是一个安全日志分析专家。你的任务是从用户提供的日志中识别异常事件、攻击行为和系统健康问题。

# 工作流程

1. 用户给你一段日志
2. 你需要识别其中的异常事件、可疑行为
3. 必要时调用 `extract_anomalies` action 把发现写入上下文
4. 当分析完成时，调用 `directly_answer` 输出 markdown 报告

# 注意事项

- 关注 ERROR / WARN 级别的日志
- 关注异常的 IP、URL、payload 模式
- 不要凭空臆造数据，引用必须基于原始日志
- 优先识别 SQL 注入、XSS、命令注入等典型攻击模式

# 当前已收集的发现

{{ if .CollectedAnomalies }}{{ .CollectedAnomalies }}{{ else }}暂无{{ end }}
```

注意：`{{ .CollectedAnomalies }}` 是 [Go template 变量](https://pkg.go.dev/text/template)，在每轮渲染时由 `WithVar/WithVars` 注入。

## 步骤 3：写 prompts/reactive_data.txt

每轮迭代后，根据上一轮的反馈和状态生成"反应数据"，注入下一轮 prompt 的 `<|REFLECTION_<nonce>|>` 段。

```text
<|REACTIVE_DATA_{{.Nonce}}|>

# 上一轮反馈
{{ if .FeedbackMessages }}{{ .FeedbackMessages }}{{ else }}（无）{{ end }}

# 已识别的异常数量
{{ .AnomalyCount }}

# 最近的发现
{{ if .RecentFinding }}{{ .RecentFinding }}{{ else }}（无）{{ end }}

<|REACTIVE_DATA_END_{{.Nonce}}|>
```

## 步骤 4：写 prompts/reflection_output_example.txt

给 LLM 一个 JSON 输出示例，提高 schema 遵守率。

```text
* 当用户提供日志请求分析时，首先调用 extract_anomalies 收集发现：
  {"@action": "extract_anomalies", "anomaly_type": "sql_injection", "evidence": "GET /search?q='+OR+1=1--", "severity": "high", "human_readable_thought": "发现 SQL 注入尝试"}

* 当所有异常都已识别完成，调用 directly_answer 输出报告：
  {"@action": "directly_answer", "human_readable_thought": "已完成日志分析，输出报告"}
  
  <|FINAL_ANSWER_<nonce>|>
  ## 日志分析报告
  
  发现以下异常：
  
  1. SQL 注入尝试（高危）...
  <|FINAL_ANSWER_END_<nonce>|>
```

## 步骤 5：写 init.go 主 factory

```go
package loop_log_analyze

import (
    "bytes"
    _ "embed"

    "github.com/yaklang/yaklang/common/ai/aid/aicommon"
    "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
    "github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/persistent_instruction.txt
var instruction string

//go:embed prompts/reactive_data.txt
var reactiveDataTpl string

//go:embed prompts/reflection_output_example.txt
var outputExample string

const LoopLogAnalyzeName = "log_analyze"

func init() {
    err := reactloops.RegisterLoopFactory(
        LoopLogAnalyzeName,
        func(invoker aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
            preset := []reactloops.ReActLoopOption{
                reactloops.WithMaxIterations(15),
                reactloops.WithAllowToolCall(true),
                reactloops.WithAllowRAG(false),
                reactloops.WithAllowAIForge(false),

                reactloops.WithPersistentInstruction(instruction),
                reactloops.WithReflectionOutputExample(outputExample),
                reactloops.WithReactiveDataBuilder(buildReactiveData),

                reactloops.WithAITagFieldWithAINodeId("FINAL_ANSWER", "tag_final_answer", "re-act-loop-answer-payload", aicommon.TypeTextMarkdown),

                reactloops.WithOverrideLoopAction(loopActionDirectlyAnswerLogAnalyze),
                buildExtractAnomaliesAction(invoker),

                reactloops.WithInitTask(buildInitTask(invoker)),
                BuildOnPostIterationHook(invoker),

                reactloops.WithSameActionTypeSpinThreshold(3),
                reactloops.WithEnableSelfReflection(true),
                reactloops.WithPeriodicVerificationInterval(3),
            }
            preset = append(preset, opts...)
            return reactloops.NewReActLoop(LoopLogAnalyzeName, invoker, preset...)
        },
        reactloops.WithLoopDescription("Analyze logs to identify anomalies and security incidents"),
        reactloops.WithLoopDescriptionZh("日志分析模式：从日志中识别异常事件、攻击行为和系统问题"),
        reactloops.WithVerboseName("Log Analyze"),
        reactloops.WithVerboseNameZh("日志分析"),
        reactloops.WithLoopUsagePrompt("Use when user provides a log snippet and asks for anomaly analysis or security incident identification. Use 'extract_anomalies' to collect findings step by step, then 'directly_answer' to output the markdown report."),
        reactloops.WithLoopOutputExample(`
* When user requests to analyze logs:
  {"@action": "log_analyze", "human_readable_thought": "I will analyze the log for anomalies"}
`),
    )
    if err != nil {
        panic(err)
    }
}

func buildReactiveData(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
    return utils.RenderTemplate(reactiveDataTpl, map[string]any{
        "Nonce":            nonce,
        "FeedbackMessages": feedbacker.String(),
        "AnomalyCount":     loop.Get("anomaly_count"),
        "RecentFinding":    loop.Get("recent_finding"),
    })
}
```

## 步骤 6：写 action_extract_anomalies.go

```go
package loop_log_analyze

import (
    "encoding/json"
    "fmt"
    "strconv"

    "github.com/yaklang/yaklang/common/ai/aid/aicommon"
    "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
    "github.com/yaklang/yaklang/common/ai/aid/aitool"
    "github.com/yaklang/yaklang/common/utils"
)

func buildExtractAnomaliesAction(_ aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
    return reactloops.WithRegisterLoopActionWithStreamField(
        "extract_anomalies",
        "记录从日志中发现的异常或安全事件。每次调用记录一个发现。",
        []aitool.ToolOption{
            aitool.WithStringParam("anomaly_type",
                aitool.WithParam_Required(true),
                aitool.WithParam_Description("异常类型：sql_injection / xss / cmd_injection / brute_force / suspicious_ip / error_burst 等"),
            ),
            aitool.WithStringParam("evidence",
                aitool.WithParam_Required(true),
                aitool.WithParam_Description("原始日志证据（必须直接引用日志中的文本）"),
            ),
            aitool.WithStringParam("severity",
                aitool.WithParam_Required(true),
                aitool.WithParam_Description("严重程度 high / medium / low"),
            ),
            aitool.WithStringParam("explanation",
                aitool.WithParam_Description("简要说明为何这是异常"),
            ),
        },
        []*reactloops.LoopStreamField{
            {
                FieldName:   "explanation",
                AINodeId:    "extract-anomaly-explanation",
                ContentType: aicommon.TypeTextMarkdown,
            },
        },
        verifyExtractAnomalies,
        handleExtractAnomalies,
    )
}

func verifyExtractAnomalies(loop *reactloops.ReActLoop, action *aicommon.Action) error {
    if action.GetString("anomaly_type") == "" {
        return utils.Error("extract_anomalies requires anomaly_type")
    }
    if action.GetString("evidence") == "" {
        return utils.Error("extract_anomalies requires evidence")
    }
    severity := action.GetString("severity")
    if severity != "high" && severity != "medium" && severity != "low" {
        return fmt.Errorf("severity must be high/medium/low, got %q", severity)
    }
    return nil
}

func handleExtractAnomalies(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
    anomaly := map[string]any{
        "type":        action.GetString("anomaly_type"),
        "evidence":    action.GetString("evidence"),
        "severity":    action.GetString("severity"),
        "explanation": action.GetString("explanation"),
    }

    invoker := loop.GetInvoker()
    invoker.AddToTimeline("anomaly_found", anomaly)

    countStr := loop.Get("anomaly_count")
    count, _ := strconv.Atoi(countStr)
    count++
    loop.Set("anomaly_count", strconv.Itoa(count))

    bs, _ := json.Marshal(anomaly)
    loop.Set("recent_finding", string(bs))

    collected := loop.Get("collected_anomalies_json")
    var anomalies []map[string]any
    if collected != "" {
        _ = json.Unmarshal([]byte(collected), &anomalies)
    }
    anomalies = append(anomalies, anomaly)
    if newBs, err := json.Marshal(anomalies); err == nil {
        loop.Set("collected_anomalies_json", string(newBs))
    }

    op.Feedback(fmt.Sprintf("Recorded anomaly #%d: %s (%s severity). Continue analyzing or call directly_answer to finalize.",
        count, action.GetString("anomaly_type"), action.GetString("severity")))
    op.Continue()
}
```

要点：

1. **Verifier 严格校验**：返回 error 触发 LLM 重试
2. **Handler 状态落地**：`AddToTimeline` + `loop.Set` + `op.Feedback` 三处都写
3. **流式字段**：`explanation` 流到 `extract-anomaly-explanation` 节点，用户看得见
4. **不退出**：`op.Continue()`，让 LLM 继续找下一个

## 步骤 7：写 action_directly_answer.go（覆盖默认）

默认的 `directly_answer` 太通用。我们想要：必须输出 markdown 报告 + 落盘 + emit reference material（基于哪些异常）。

```go
package loop_log_analyze

import (
    "github.com/yaklang/yaklang/common/ai/aid/aicommon"
    "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
    "github.com/yaklang/yaklang/common/ai/aid/aitool"
    "github.com/yaklang/yaklang/common/utils"
)

var loopActionDirectlyAnswerLogAnalyze = &reactloops.LoopAction{
    ActionType:  "directly_answer",
    Description: "输出最终日志分析报告。报告必须以 markdown 形式放在 FINAL_ANSWER 标签内。",
    Options: []aitool.ToolOption{
        aitool.WithStringParam("human_readable_thought",
            aitool.WithParam_Description("简短描述为何已经可以输出报告"),
        ),
    },
    AITagStreamFields: []*reactloops.LoopAITagField{
        {
            TagName:      "FINAL_ANSWER",
            VariableName: "tag_final_answer",
            AINodeId:     "re-act-loop-answer-payload",
            ContentType:  aicommon.TypeTextMarkdown,
        },
    },
    ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
        finalAnswer := loop.Get("tag_final_answer")
        if finalAnswer == "" {
            return utils.Error("directly_answer requires content in <FINAL_ANSWER> tag")
        }
        return nil
    },
    ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
        report := loop.Get("tag_final_answer")
        invoker := loop.GetInvoker()
        invoker.AddToTimeline("log_analyze_report_delivered", "report length: "+utils.InterfaceToString(len(report)))
        invoker.EmitFileArtifactWithExt("log_analyze_report", ".md", report)
        invoker.EmitResultAfterStream(report)
        loop.Set("log_analyze_report_delivered", "true")
        op.Exit()
    },
}
```

要点：

1. `WithOverrideLoopAction` 注册时不要 `WithRegisterLoopAction`，**直接覆盖**
2. AITagField 的 `VariableName` = "tag_final_answer"，handler 里 `loop.Get("tag_final_answer")` 拿值
3. `EmitResultAfterStream` 在所有流结束后再发结果，避免错乱
4. `op.Exit()` 退出 loop

## 步骤 8：写 InitTask

InitTask 在 loop 第一轮前跑，给 LLM "环境搭建好"的状态。

```go
package loop_log_analyze

import (
    "context"
    "strings"

    "github.com/yaklang/yaklang/common/ai/aid/aicommon"
    "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func buildInitTask(invoker aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
    return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
        userInput := task.GetUserInput()

        if strings.TrimSpace(userInput) == "" {
            invoker.GetEmitter().EmitThoughtStream(task.GetIndex(), "No log content provided, cannot proceed.")
            op.Done()
            return
        }

        if !strings.Contains(userInput, "\n") && len(userInput) < 30 {
            invoker.GetEmitter().EmitThoughtStream(task.GetIndex(), 
                "Log content seems too short, please provide a longer log snippet.")
            op.Done()
            return
        }

        loop.Set("anomaly_count", "0")
        loop.Set("collected_anomalies_json", "[]")

        invoker.AddToTimeline("log_analyze_init", "Log analysis loop initialized")
        op.Continue()
    }
}
```

要点：

1. **早退**：用户输入不合法时 `op.Done()` 直接结束
2. **不调 LLM**：InitTask 是**确定性**步骤，避免开模型
3. **状态预置**：`loop.Set` 设置初始值
4. **emit 反馈**：用 `EmitThoughtStream` 告诉用户为什么早退

## 步骤 9：写 finalize.go (OnPostIteraction)

整个 loop 结束时如果还没生成报告，强制 fallback。

```go
package loop_log_analyze

import (
    "encoding/json"
    "strings"

    "github.com/yaklang/yaklang/common/ai/aid/aicommon"
    "github.com/yaklang/yaklang/common/ai/aid/aireact"
    "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
    "github.com/yaklang/yaklang/common/ai/aid/aitool"
    "github.com/yaklang/yaklang/common/log"
)

func BuildOnPostIterationHook(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
    return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, op *reactloops.OnPostIterationOperator) {
        if !isDone {
            return
        }
        if loop.Get("log_analyze_report_delivered") == "true" {
            return
        }
        deliverFallbackReport(loop, invoker)
        if reasonErr, ok := reason.(error); ok && strings.Contains(reasonErr.Error(), "max iterations") {
            op.IgnoreError()
        }
    })
}

func deliverFallbackReport(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime) {
    react, ok := invoker.(*aireact.ReAct)
    if !ok {
        log.Warn("invoker is not *ReAct, skip fallback report")
        return
    }

    collected := loop.Get("collected_anomalies_json")
    if collected == "" || collected == "[]" {
        invoker.EmitResultAfterStream("# 日志分析报告\n\n本次分析未发现明显异常。")
        return
    }

    var anomalies []map[string]any
    _ = json.Unmarshal([]byte(collected), &anomalies)

    bs, _ := json.MarshalIndent(anomalies, "", "  ")
    prompt := "请基于下面已收集的异常列表，生成一份 markdown 格式的安全分析报告。\n\n" +
        "<anomalies>\n" + string(bs) + "\n</anomalies>"

    taskID := loop.GetCurrentTask().GetIndex()
    result, err := react.InvokeQualityPriorityLiteForge(
        loop.GetCurrentTask().GetContext(),
        "log-analyze-fallback-summary",
        prompt,
        []aitool.ToolOption{
            aitool.WithStringParam("summary",
                aitool.WithParam_Required(true),
                aitool.WithParam_Description("markdown 总结报告"),
            ),
        },
        aicommon.WithGeneralConfigStreamableFieldWithNodeId("re-act-loop-answer-payload", "summary"),
    )
    _ = taskID
    if err != nil {
        log.Errorf("fallback report failed: %v", err)
        invoker.EmitResultAfterStream("# 日志分析报告\n\n（自动报告生成失败，但已识别 " + 
            len(anomalies) + " 个异常，请查看 timeline）")
        return
    }

    invoker.EmitFileArtifactWithExt("log_analyze_fallback_report", ".md", result.GetString("summary"))
    invoker.EmitResultAfterStream(result.GetString("summary"))
}
```

要点：

1. **`isDone == true` 才跑**：每轮 `false` 不要瞎写
2. **避免重复 deliver**：用 `loop.Get("log_analyze_report_delivered")` 状态守门
3. **LiteForge 兜底**：用 quality 模型生成最终总结
4. **`IgnoreError`**：max iterations 不算失败

## 步骤 10：在 reactinit/init.go 添加空白 import

```go
package reactinit

import (
    // ... 其他 imports ...
    _ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_log_analyze"
)
```

这样在程序启动时会自动触发 `init()` 注册到全局 LoopFactory 表。

## 步骤 11：写测试

新建 `loop_log_analyze/log_analyze_test.go`：

```go
package loop_log_analyze

import (
    "context"
    "encoding/json"
    "strings"
    "testing"
    "time"

    "github.com/yaklang/yaklang/common/ai/aid/aicommon"
    "github.com/yaklang/yaklang/common/ai/aid/aireact"
    _ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/reactinit"
)

func TestLogAnalyze_DetectSQLInjection(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()

    sampleLog := `2024-01-01 10:00:00 [INFO] GET /index.html 200
2024-01-01 10:00:01 [WARN] GET /search?q=' OR 1=1-- 200 (suspicious)
2024-01-01 10:00:02 [INFO] GET /about 200`

    callCount := 0
    aiCallback := func(_ aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
        callCount++
        var responseBody string
        if callCount == 1 {
            responseBody = `{"@action":"extract_anomalies","anomaly_type":"sql_injection",
"evidence":"GET /search?q=' OR 1=1--","severity":"high","explanation":"SQL injection attempt"}`
        } else {
            responseBody = `{"@action":"directly_answer","human_readable_thought":"finished"}

<|FINAL_ANSWER_xxx|>
## 报告

发现 SQL 注入尝试。
<|FINAL_ANSWER_END_xxx|>`
        }
        return aicommon.NewAIResponseFromMockedContent(responseBody), nil
    }

    react, err := aireact.NewReAct(
        aireact.WithAICallback(aiCallback),
        aireact.WithEnterFocusMode(LoopLogAnalyzeName),
    )
    if err != nil {
        t.Fatal(err)
    }

    err = react.Invoke(ctx, sampleLog)
    if err != nil {
        t.Fatalf("invoke failed: %v", err)
    }

    if !strings.Contains(react.GetTimelineSummary(), "anomaly_found") {
        t.Error("expected anomaly_found in timeline")
    }
}
```

参考实际项目的 [reactloop_test.go](../reactloop_test.go) 写法。

## 步骤 12：调试

打开 workspace debug 环境变量：

```bash
export YAKIT_AI_WORKSPACE_DEBUG=1
yak test ./common/ai/aid/aireact/reactloops/loop_log_analyze/...
```

会在 `~/yakit/aiworkspace_debug/` 下生成：

- `prompt/<task_id>/iter_<N>.md`：每轮完整 prompt
- `action_record/<task_id>/iter_<N>.md`：每轮 action 执行记录
- `perception/<task_id>/<epoch>.md`：感知层快照

详见 [12-debugging-and-observability.md](12-debugging-and-observability.md)。

---

## 上线 Checklist

按顺序对照检查，全部勾选才上线：

### 注册

- [ ] 在 `reactinit/init.go` 添加了空白 import
- [ ] `LoopMetadata` 设置完整：`WithLoopDescription` / `WithLoopDescriptionZh` / `WithVerboseName` / `WithVerboseNameZh` / `WithLoopUsagePrompt`
- [ ] `RegisterLoopFactory` 调用没漏 panic 处理

### Prompt

- [ ] `persistent_instruction.txt` 用 `//go:embed` 嵌入
- [ ] prompt 里**没有硬编码 schema 或 nonce**（系统会自动注入）
- [ ] 用户数据放在 `<|USER_QUERY_<nonce>|>` 标签内（系统自动包）
- [ ] `reactive_data.txt` 模板里有 `{{.Nonce}}` 包裹

### Actions

- [ ] 每个自定义 action 都有 Verifier 和 Handler
- [ ] Verifier 用 `utils.Error` 返回错误（触发 LLM 重试）
- [ ] Handler 末尾必须调 `op.Continue()` / `op.Exit()` / `op.Fail()` 之一
- [ ] 关键中间数据写到 `loop.Set` + timeline
- [ ] 关键消息用 `op.Feedback` 注入下一轮 prompt
- [ ] 错误处理：tool 失败用 `Continue + Critical reflection`，不要直接 Fail loop

### Hooks

- [ ] InitTask 早退条件用 `op.Done()`（不要 `op.Failed`）
- [ ] InitTask 不调用 LLM（除非确实必要）
- [ ] OnPostIteraction `isDone == true` 才执行 finalize
- [ ] OnPostIteraction 用状态 key 防止重复 deliver
- [ ] max iterations 错误用 `op.IgnoreError()` 吞掉

### 流式输出

- [ ] 长文本用 `LoopAITagField` 而不是 `LoopStreamField`
- [ ] 短文本字段用 `LoopStreamField`
- [ ] 关键节点用约定 NodeId（`re-act-loop-answer-payload` / `re-act-loop-thought`）
- [ ] 关键产物用 `EmitFileArtifactWithExt` 落盘 + emit pin
- [ ] 最终结果用 `EmitResultAfterStream`（不是 `EmitResult`）

### 确定性

- [ ] `WithSameActionTypeSpinThreshold` 设了合理阈值（默认 3）
- [ ] `WithEnableSelfReflection` 关键 loop 必须开（默认 true）
- [ ] `WithPeriodicVerificationInterval` 设了合理间隔
- [ ] `WithMaxIterations` 设置上限（默认 100，可以调小）

### 测试

- [ ] 单测覆盖核心 action（mock AI callback）
- [ ] 单测覆盖 InitTask 早退路径
- [ ] 单测覆盖 finalize fallback 路径
- [ ] `yak test ./...` 通过

### 调试

- [ ] 打开 `YAKIT_AI_WORKSPACE_DEBUG=1` 跑一次，看 prompt / action 记录是否正常
- [ ] timeline 关键事件齐全（init / actions / verify / finalize）
- [ ] 前端 UI 看得到 thought 流、关键节点流、最终结果

### 文档

- [ ] 在 [README.md](../README.md) 速查表里加一行
- [ ] 在 [11-case-studies.md](11-case-studies.md) 横向对比表加一行
- [ ] `WithLoopUsagePrompt` 写得足够清楚，让上层 ReAct 能正确路由到这个 loop

---

## 进一步阅读

- [02-options-reference.md](02-options-reference.md)：所有 With* 详解
- [04-actions.md](04-actions.md)：Action 4 种来源
- [11-case-studies.md](11-case-studies.md)：参考真实 loop 实现
- 参考项目：
  - 简单：[loop_default](../loop_default)
  - 中等：[loop_smart_qa](../loop_smart_qa)、[loop_knowledge_enhance](../loop_knowledge_enhance)
  - 复杂：[loop_http_fuzztest](../loop_http_fuzztest)、[loop_plan](../loop_plan)
