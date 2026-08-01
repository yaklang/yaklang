# 08. 长链路稳定机制：感知 / 验证门 / TODO 软检查点

ReAct 主循环依靠确定性的运行时信号维持长链路稳定，不再为每轮行动额外调用 AI 做自我评价，也不再使用通用重复行动检测来强制退出。

| 机制 | 解决的问题 | 是否新增 AI 调用 |
|------|------------|------------------|
| Perception | 按需刷新环境和任务态势 | 仅在既有感知流程需要时 |
| Verification Gate | 控制用户满意度验证频率 | 仅在验证门放行时 |
| Finished TODO Checkpoint | 结束前检查遗漏和无痕关闭 | 否，复用下一轮主循环 |
| CURRENT TODO Checkpoint | 长时间聚焦同一事项时克制地校正路径 | 否，复用下一轮主循环 |

## 8.1 Perception

Perception 负责形成当前任务的结构化态势。普通反馈仍通过 timeline、ReactiveData、operator feedback 和下一轮主循环交给模型处理。工具失败不会启动额外的评价调用。

即时刷新只保留明确的运行时触发，例如强制刷新和 loop 切换；普通轮次继续受原有频率控制。

## 8.2 Verification Gate

验证门由最小迭代、周期节流和 watchdog 共同控制。它的目标是减少频繁的满意度询问，同时在长任务中保留确定性的完成检查。相关实现位于 verification runtime 与 watchdog 文件中。

## 8.3 Finished TODO 软检查点

模型第一次选择 `finish` 时，主循环不退出，而是为下一轮排队：

```text
[SOFT TODO CHECKPOINT]
```

模型可继续工作，也可在处理开放 TODO 后再次选择 `finish`。第二次结束请求仍有开放 TODO 时会被拒绝，但不会重复注入同一个 checkpoint。若模型改选非 `finish` action，本次结束流程重置。

Goal mode 的最小迭代门禁优先于 Finished checkpoint。

## 8.4 CURRENT TODO 25 轮软检查点

每个 ReAct task scope 独立记录当前 TODO 和连续有效迭代数。有效迭代必须满足：

1. action 已成功解析和校验；
2. `todo_delta` 已应用；
3. ActionHandler 已实际返回；
4. action 不是 `finish`。

handler 返回业务失败仍表示进行了一次真实尝试，因此计数。AI transaction 重试、流式字段、解析或校验失败、以及没有进入 handler 的失败不计数。

同一 CURRENT TODO 达到 25 次后，只为下一次正常主循环请求排队：

```text
[CURRENT TODO CHECKPOINT]
```

注入后立即从零重新计数；同一 TODO 再执行 25 次可再次触发。current 切换、关闭或清除会重置；同轮切换 current 时，该 action 计为新 current 的第 1 轮。checkpoint 注入前 current 已变化时，过期 checkpoint 被丢弃。

Finished 和 CURRENT checkpoint 同时存在时，只注入 Finished checkpoint，并从零开启后续 CURRENT 计数窗口。

## 8.5 Prompt 边界

两种 checkpoint 共用 `LoopPromptAssemblyInput.TodoCheckpoint`。动态区固定顺序为：

```text
UserQuery
AutoContext
ExtraCapabilities
ReactiveData
InjectedMemory
TodoCheckpoint
PROMPT_SECTION_dynamic_END
```

`TodoCheckpoint` 是纯动态区域最后一个内容块，不进入 high-static、frozen、semi-dynamic 或 timeline-open，也不会被 lightweight prompt 裁剪。Prompt observation 将它记录为独立的 `section.dynamic.todo_checkpoint` 子项。

## 8.6 错误处理原则

- 工具或能力失败：写入普通 feedback 和 timeline，由下一轮主决策选择替代路径。
- 可确定识别的重复读取、重复查询或路径失败：保留局部、无 AI 调用的保护。
- 不因 action 类型相同就判定任务无进展；长链路校正由 CURRENT TODO 的 25 轮软检查点承担。
- checkpoint 只提示模型检查，不制造 TODO 修改，也不伪造完成。

## 相关源码

- [soft_todo_checkpoint.go](../soft_todo_checkpoint.go)
- [prompt.go](../prompt.go)
- [perception.go](../perception.go)
- [verification_runtime.go](../verification_runtime.go)
- [verification_watchdog.go](../verification_watchdog.go)
