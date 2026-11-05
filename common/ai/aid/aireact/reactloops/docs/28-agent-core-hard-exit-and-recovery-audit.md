# 28. Agent 循环错误恢复现状

> 分支：`rookie/refactor/ai-loop-recovery`
> 更新时间：2026-11-05
> 范围：ReActLoop 单轮 Prompt、AI transaction、模型输出校验、Action 执行、压缩和生命周期错误。

本文记录当前错误恢复策略和仍未覆盖的自愈缺口。源码链接使用仓库相对路径，行号显示在链接文字中；GoLand 点击文件后可用 `Cmd+G` 定位。

## 1. 当前原则

- `operator.Fail()` 表示当前 Action 失败，由循环统一写入 Timeline 并继续下一轮。
- `operator.Fatal()` 表示循环无法安全继续，直接终止。
- `RecoverableError` 表示当前错误可以交给循环通过纠正 Prompt 或下一轮 Timeline 自愈。
- 展示、字段流等观测失败只记录 warning，不影响 canonical AI 响应。
- 压缩失败属于维护失败，不终止业务任务。

## 2. 单轮执行顺序

```text
生成 Prompt
  -> 请求 AI / transaction
  -> 校验模型输出
  -> 准备 function-call / 压缩响应
  -> 执行 Action
  -> post-iteration
```

模型输出后的校验集中在 [exec.go:784](../exec.go) 附近：

1. 无条件派生本轮响应形态。
2. 将可能失败的模式校验、零 Action、function-call 回放和压缩凭证写入同一个 `turnErr`。
3. 在唯一错误决策点处理：
   - `RecoverableError`：写 Timeline 后继续。
   - 其他错误：终止当前任务。

## 3. 已实现的自愈路径

### 3.1 模型输出错误

以下错误会在 transaction 内纠正重试；耗尽后进入 Timeline recovery：

- JSON 或 Action schema 错误
- 未知 Action
- 零 Action
- 压缩模式返回错误的 Action 数量
- function-call arguments 非法
- ActionVerifier 失败
- 压缩响应 replay 准备失败

统一记录逻辑位于 [loop_recovery.go:55](../loop_recovery.go)。

### 3.2 普通 Action 失败

[exec_actions.go:444](../exec_actions.go) 之后，以下情况会转换为 `loopActionsRecover`：

- `operator.Fail()`
- ActionHandler panic
- 子 Agent 控制状态记录失败

恢复记录包含：

```text
phase=execute_ai_output
action=<action>
iteration=<n>
error=<message>
side_effect_started=unknown
retry_count=<n>
next_step=<guidance>
```

同一 Action 和错误内容连续出现 3 次后，会禁用当前 Action，避免相同错误形成热循环。

### 3.3 压缩失败

- 压缩 guide 失败：返回已缓存的普通 Prompt。
- 压缩指令模板失败：返回普通 Prompt。
- 压缩 Prompt 组装失败：返回普通 Prompt。
- `compress_context` 候选读取或 Timeline 提交失败：普通 Action 方式进入 Timeline recovery。
- 连续进入压缩模式超过 5 次：硬退出，防止无限压缩。

普通 Prompt 缓存和直接回退位于 [prompt.go:535](../prompt.go)。
连续压缩计数位于 [context_compression.go:39](../context_compression.go)。

### 3.4 Prompt 可选能力

- Output Example 失败：日志 warning，置空继续。
- Reactive Data 失败：日志 warning，置空继续。
- TODO 快照渲染失败：日志 warning，置空继续。

### 3.5 展示与生命周期

- general output callback、field display、field emission 失败：warning 后继续。
- post-iteration callback panic：warning 后跳过当前 callback。
- deferred post-iteration callback panic：warning 后跳过。

## 4. 仍未处理的自愈缺口

### 4.1 Async Review 的直接回答失败

- 入口：[async_execution_control.go:40](../async_execution_control.go)
- 调用点：[exec_actions.go:203](../exec_actions.go)、[exec.go:759](../exec.go)

`answerAsyncReview` 调用 `DirectlyAnswer` 后直接返回 error。当前没有区分：

- `RecoverableError`：应当自愈。
- 网络或 API 错误：应按请求错误处理。
- context 取消：应当终止。

这是当前最明确的未收口路径。

### 4.2 Init Handler 没有 Fatal / BestEffort 区分

- 入口：[exec.go:457](../exec.go)

所有 `InitTaskOperator.Failed()` 当前都会终止任务，包括：

- 必需目录或持久化资源初始化失败
- 知识增强失败
- 意图分析失败
- 可选能力发现失败

后续需要为 InitTaskOperator 增加：

```text
Failed(err)       -> 有必要时终止
BestEffort(err)   -> warning 后继续
```

## 5. 需要保持 fatal 的边界

以下错误不应进入 AI Timeline 自愈：

- 网络、stream、429、5xx 等请求错误在 transaction 重试后仍失败。
- API 直接返回不可重试错误，例如鉴权失败、余额不足。
- API prompt overflow 在压缩和边界收缩后仍无法生成可发送 Prompt。
- context canceled/deadline。
- 显式 `operator.Fatal()`。
- nil Action、nil Handler、ActionHandler 缺失。
- Invoker、Emitter、核心锁或 Timeline 原子状态损坏。
- 连续压缩超过 5 次。

## 6. 验证

```bash
go test ./common/ai/aid/aicommon -count=1
go test ./common/ai/aid/aireact/reactloops -count=1
go test ./common/ai/aid/aireact/reactloops/reactloopstests -count=1
```
