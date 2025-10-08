# ReActLoop 模块使用说明

## 模块概述

`reactloops` 是 Yak AI 框架中的核心模块，实现了 ReAct (Reasoning and Acting) 循环执行逻辑。该模块负责：

1. **循环执行**: 通过迭代调用 AI 模型，执行多步骤任务
2. **动作管理**: 注册和执行各种动作（Actions）
3. **状态控制**: 管理任务状态转换（Pending, Processing, Completed, Aborted）
4. **异步支持**: 支持同步和异步模式的动作执行
5. **流处理**: 处理 AI 输出流，支持标签提取和镜像

## 核心组件

### 1. ReActLoop

主要的循环执行器，负责协调整个执行流程。

```go
type ReActLoop struct {
    loopName    string
    config      AIInvokeRuntime
    actions     *utils.OrderedMap[string, *LoopAction]
    maxIterations int
    // ... 其他字段
}
```

### 2. LoopAction

定义可执行的动作。

```go
type LoopAction struct {
    ActionType      string
    Description     string
    AsyncMode       bool
    ActionVerifier  ActionVerifyHandler    // 验证动作参数
    ActionHandler   ActionHandler          // 执行动作逻辑
}
```

### 3. LoopActionHandlerOperator

动作处理器中的操作符，用于控制循环流程。

```go
type LoopActionHandlerOperator struct {
    // 提供以下方法：
    // - Continue()              // 继续下一次迭代
    // - Fail(reason)           // 失败并终止
    // - Feedback(message)      // 记录反馈
    // - DisallowNextLoopExit() // 禁止下一次迭代退出
}
```

## 使用方法

### 基本使用流程

```go
// 1. 创建 ReActLoop
loop, err := NewReActLoop("my-loop", runtime, 
    WithMaxIterations(10),
    WithOnTaskCreated(func(task AIStatefulTask) {
        // 任务创建回调
    }),
)

// 2. 注册动作
loop.actions.Set("custom_action", &LoopAction{
    ActionType:  "custom_action",
    Description: "执行自定义操作",
    AsyncMode:   false,
    ActionVerifier: func(loop *ReActLoop, action *Action) error {
        // 验证动作参数
        return nil
    },
    ActionHandler: func(loop *ReActLoop, action *Action, operator *LoopActionHandlerOperator) {
        // 执行动作逻辑
        operator.Continue() // 或 operator.Fail()
    },
})

// 3. 执行循环
err = loop.Execute("task-id", context.Background(), "用户输入")
```

### 内置动作

模块提供了两个内置动作：

1. **直接回答** (`directly_answer`): AI 直接回答用户问题
2. **结束** (`finish`): 完成任务并退出循环

### 核心文件说明

#### exec.go - 核心执行逻辑

主要函数：
- `Execute()`: 创建任务并执行循环
- `ExecuteWithExistedTask()`: 使用已有任务执行循环
- `createMirrors()`: 创建流镜像处理器

关键流程：
1. 状态管理：`taskStartProcessing()` → `complete()` / `abort()`
2. Prompt 生成：调用 `generateLoopPrompt()`
3. AI 调用：通过 `CallAITransaction()` 调用 AI
4. 动作提取：从流中提取 `next_action`
5. 动作验证：`ActionVerifier`
6. 动作执行：`ActionHandler`

#### prompt.go - Prompt 生成

主要函数：
- `generateLoopPrompt()`: 生成循环提示词
- `generateSchemaString()`: 生成动作 Schema

Prompt 组成：
- 背景信息（Background）
- 持久化指令（PersistentContext）
- 输出示例（OutputExample）
- 响应式数据（ReactiveData）- 包含反馈
- 用户查询（UserQuery）
- 动作 Schema（Schema）

## 状态转换

任务状态转换流程：

```
Created → Processing → Completed/Aborted
```

- **Created**: 任务创建
- **Processing**: 正在处理
- **Completed**: 成功完成
- **Aborted**: 失败中止

## 同步 vs 异步模式

### 同步模式 (AsyncMode = false)
- 动作在主循环中执行完成
- 状态由循环控制
- 执行完立即进入下一次迭代

### 异步模式 (AsyncMode = true)
- 动作触发后立即返回
- 状态由异步回调控制
- 通过 `WithOnAsyncTaskTrigger()` 注册异步回调
- 主循环不会自动进入下一次迭代

## 反馈机制

通过 `operator.Feedback()` 记录反馈，反馈会在下一次迭代时通过 `ReactiveData` 传递给 AI：

```go
ActionHandler: func(loop *ReActLoop, action *Action, operator *LoopActionHandlerOperator) {
    operator.Feedback("步骤1完成，发现3个问题")
    operator.Continue()
}
```

## Stream 处理和 Mirror

### AI Tag 字段

通过注册 AI Tag 字段，可以从 AI 输出中提取特定标签内容：

```go
loop.aiTagFields.Set("yaklang-code", &LoopAITagField{
    TagName:      "yaklang-code",
    VariableName: "generated_code",
})
```

当 AI 输出包含 `<yaklang-code>...</yaklang-code>` 时，内容会被提取并存储到 `generated_code` 变量中。

### Stream 字段

注册流字段可以实时处理 JSON 中的特定字段：

```go
loop.streamFields.Set("thought", &LoopStreamField{
    FieldName: "thought",
    Prefix:    "思考",
})
```

## 测试说明

### 测试策略

本模块的测试采用 mock AI response 驱动的方式：

1. **Mock Runtime**: 模拟 AIInvokeRuntime
2. **Mock Response**: 模拟 AI 返回的 JSON 格式动作
3. **验证状态**: 检查任务状态转换
4. **验证行为**: 检查动作处理器的调用

### 测试覆盖

主要测试场景：

1. ✅ 基本执行流程
2. ✅ 最大迭代次数限制
3. ✅ 异步/同步模式
4. ✅ ActionVerifier 和 ActionHandler
5. ✅ 状态转换（Processing, Completed, Aborted）
6. ✅ 错误处理和 Panic 恢复
7. ✅ 反馈机制
8. ✅ Prompt 生成
9. ✅ Stream 处理和 Mirror
10. ✅ 禁止退出循环

### 运行测试

```bash
# 运行所有测试
go test ./common/ai/aid/aireact/reactloops -v

# 运行特定测试
go test ./common/ai/aid/aireact/reactloops -v -run TestExecute_BasicFlow

# 查看覆盖率
go test ./common/ai/aid/aireact/reactloops -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Mock AI Response 示例

```go
runtime.callAIFunc = func(prompt string) (*AIResponse, error) {
    resp := aicommon.NewUnboundAIResponse()
    resp.SetTaskIndex("test-1")
    resp.EmitOutputStream(strings.NewReader(`{
        "thought": "我需要执行这个操作",
        "next_action": {
            "type": "custom_action",
            "params": {"key": "value"}
        }
    }`))
    resp.Close()
    return resp, nil
}
```

## 注意事项

1. **迭代限制**: 默认最大迭代次数为 100，可通过 `WithMaxIterations()` 自定义
2. **Emitter 必需**: 必须提供有效的 Emitter，否则执行会失败
3. **动作注册**: 所有使用的动作必须提前注册
4. **Panic 恢复**: 循环内部有 panic 恢复机制，会将任务标记为 Aborted
5. **异步模式**: 异步模式下，主循环不会自动完成任务，需要在异步回调中手动设置状态

## 最佳实践

1. **错误处理**: ActionVerifier 中进行参数验证，ActionHandler 中进行业务逻辑处理
2. **反馈信息**: 使用 `operator.Feedback()` 提供详细的执行信息，帮助 AI 做出更好的决策
3. **状态管理**: 正确使用 `Continue()` 和 `Fail()` 控制循环流程
4. **调试日志**: 使用 `common/log` 包输出调试信息
5. **测试覆盖**: 为自定义动作编写单元测试，确保逻辑正确

## 常见问题

### Q: 循环无限执行怎么办？
A: 设置合理的 `maxIterations`，并确保动作正确调用 `Continue()` 或 `Fail()`

### Q: 如何调试 Prompt？
A: 可以在 `generateLoopPrompt()` 中打印或记录生成的 prompt

### Q: 异步模式下如何完成任务？
A: 在 `WithOnAsyncTaskTrigger()` 回调中手动调用 `task.SetStatus(AITaskState_Completed)`

### Q: 如何处理超时？
A: 通过 context.WithTimeout() 创建带超时的 context 传递给 Execute()

## 相关文档

- AI Tag 解析：`common/ai/aid/aicommon/aitag/`
- 动作提取：`common/ai/aid/aicommon/action_extractor.go`
- Timeline 管理：`common/ai/aid/aicommon/timeline.go`

---

**维护者**: Yaklang Team  
**最后更新**: 2024年

