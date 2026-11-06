# 28. Agent 循环错误恢复现状

> 分支：`rookie/refactor/ai-loop-recovery`
> 更新时间：2026-11-06
> 范围：ReActLoop 单轮 Prompt、AI transaction、模型输出校验、Action 执行、压缩和生命周期错误。

本文记录当前错误恢复策略和剩余的硬退出边界。源码使用仓库相对路径，GoLand 中可直接跳转文件。

## 1. 当前原则

- `operator.Fail()` 是 Action handler 唯一的业务失败出口，由循环写入 Timeline 后继续。
- `operator.Fatal()` 已删除；Action 不再声明“不可自愈”并中断任务。
- Prompt 成功构建后，除取消控制信号外，不因 transaction、模型输出、Action 或压缩错误直接退出。
- 网络和 API 错误继续自愈，同时通过普通用户可见的 `notify` 事件提示原因。
- 只有无法建立 Prompt/运行环境，或者调用方主动取消时，才保留硬退出。
- 不再限制连续压缩次数；压缩是否继续由下一轮 Prompt 构建自行判断。

## 2. 单轮执行顺序

```text
生成 Prompt
  -> 请求 AI / transaction
  -> 校验模型输出
  -> 准备 function-call / 压缩响应
  -> 执行 Action
  -> post-iteration
```

错误决策集中在 [exec.go](../exec.go)：

1. Prompt 组装失败：当前轮无法得到可发送输入，返回错误。
2. Prompt 已组装后的 transaction 错误：写 Timeline，继续下一轮。
3. 模型输出、function-call replay、零 Action 等错误：写 Timeline，继续下一轮。
4. Action handler、panic、无 handler、operator `Fail`：写 Timeline，继续下一轮。
5. `context.Canceled`、`context.DeadlineExceeded` 和用户取消是控制信号，不作为业务错误自愈。

## 3. 已实现的自愈路径

### 3.1 Prompt 与压缩

普通 Prompt 先缓存；压缩 guide、指令或压缩 Prompt 组装失败时直接返回普通 Prompt，不终止任务。相关逻辑位于 [prompt.go](../prompt.go)。

- `compress_context` 候选读取或 Timeline 提交失败按普通 Action 失败处理。
- 压缩 prompt 超过 advisory 限制只记录 warning，不再硬失败。
- 压缩进度低于 99% 时 emit warning 并记录日志，不再跳过或终止压缩模式。
- 连续压缩失败计数和 5 次熔断已删除。

统一恢复记录位于 [loop_recovery.go](../loop_recovery.go)。

### 3.2 模型输出和 Action

以下错误都会转换为 `REACT_RECOVERABLE_ERROR` Timeline 事件：

- JSON、Action schema、未知 Action、零 Action。
- 压缩模式返回错误数量或错误 Action。
- function-call arguments 或 replay 数据非法。
- ActionVerifier 失败。
- `operator.Fail()`、ActionHandler panic、nil Action/handler、ActionHandler 缺失。
- 子 Agent 控制状态记录失败。

同一 Action 和错误内容重复出现时，Timeline guidance 会要求模型更换参数或路径；失败提示节点使用普通流，前端可展示。

### 3.3 Init、Async Review 和展示层

- Init handler 失败会写入 Timeline，并尝试生成修复建议；建议生成失败只 warning。
- Async review 失败非取消时写入 Timeline，下一轮继续。
- general output、字段展示、reactive data、TODO 快照等可选阶段失败只 warning。
- post-iteration callback panic 被隔离，不终止主循环。

## 4. 网络和 API 通知

最终 transaction 失败时，[ai_failure_notify.go](../../../aicommon/ai_failure_notify.go) 使用现有 `EVENT_TYPE_NOTIFY` 发出用户可见通知：

- `network_error`：连接失败、超时、EOF、broken pipe 等。
- `authentication_failed`：401/403、API Key 或权限错误。
- `quota_exceeded`：402、余额不足、欠费、额度耗尽；与现有 429 额度通知复用。
- `api_error`：其他 provider HTTP/API 失败。
- `rate_limited`：429 继续沿用已有专门通知，不重复发送。

通知文案登记在 [ai_node_id_i18n.go](../../../../../schema/ai_node_id_i18n.go)。通知失败本身只记录日志，不影响恢复流程。

## 5. 剩余的硬退出边界

以下情况不属于 Prompt 成功后的业务错误，仍会直接返回或终止：

- Prompt 组装本身失败，无法得到可发送内容。
- task、loop、task mutex、invoker 或 emitter 等运行环境无效，无法安全写 Timeline。
- loop 没有任何可注册 Action，无法形成有效 Prompt。
- task 在开始前已经 skipped。
- `context.Canceled`、`context.DeadlineExceeded` 或用户主动取消。
- 进入最终 defer 的不可恢复 panic；panic 会转为任务 Aborted 并保留堆栈，避免进程崩溃。

达到最大迭代次数属于机制层的 soft interrupt，不是业务错误硬失败。

## 6. 验证

```bash
go test ./common/ai/aid/aicommon -count=1
go test ./common/ai/aid/aireact/reactloops -count=1
go test ./common/ai/aid/aireact/reactloops/reactloopstests -count=1 -timeout 5m
```
