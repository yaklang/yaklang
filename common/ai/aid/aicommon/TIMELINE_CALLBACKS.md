# Timeline 监听回调

本轮只提供监听基础设施。回调不负责记忆抽取、存储或检索；压缩模型、输出 schema、提示词和调度策略沿用当前实现。`MemoryEntities` 暂时为 `[]any{}`。

## 注册接口

在需要监听的 session Timeline 上直接注册：

```go
timeline.RegisterItemInputCallback("memory", func(event aicommon.TimelineItemInputEvent) {
    // 将不可变的输入快照交给后续消费者。
    // event.ID / event.Timestamp / event.ItemJSON
})

timeline.RegisterFreezeCallback("memory", func(event aicommon.TimelineFreezeResult) {
    // 本次新冻结的 ID，以及原样提升的用户输入、evidence、工具缓存变更。
    // event.Version / event.ThroughID / event.NewlyFrozenIDs / event.Promotions
})

timeline.RegisterCompressFreezeCallback("memory", func(event aicommon.TimelineCompressFreezeEvent) {
    // 一次原子提交的冻结收据和压缩结果。
    // event.Freeze / event.Compression
})

timeline.RegisterSummaryCallback("memory", func(event aicommon.TimelineSummaryEvent) {
    summary, err := io.ReadAll(event.Summary)
    if err != nil {
        return
    }
    // summary: 已提交的完整摘要。
    // event.Prompt: 本次实际使用的压缩正文，包含旧摘要、待总结历史、RetainedContext。
    // event.Instruction: 静态压缩指令。
    // event.MemoryEntities: 当前始终为空，后续才接入记忆候选内容。
    _, _, _, _ = summary, event.Prompt, event.Instruction, event.MemoryEntities
})
```

同一种事件里，相同 ID 的注册替换原回调；传入 `nil` 注销。不同事件的 ID 互不影响：

```go
timeline.RegisterSummaryCallback("memory", nil)
```

## 事件时机与内容

| 操作 | 通知 |
| --- | --- |
| 文本、工具结果、用户交互、当前任务输入、evidence / 工具缓存写入 | 完整写入提交后，逐 item 通知 input |
| 分支合并 | 仅通知本次真正写入父 Timeline 的 item，包含分支摘要条目 |
| `Freeze` / `FreezeAll` | 有新冻结内容时通知 freeze；空操作不通知 |
| `CompressOnce` / 超阈值的 `CompressBeforePrompt` 成功 | 按顺序通知 freeze → compress-freeze → summary |
| 只有原样保留内容的压缩提交 | 不调用 AI；通知 freeze 和 compress-freeze，无 summary |
| 压缩已冻结历史，没有新冻结 item | 通知 compress-freeze 和生成的 summary，无普通 freeze |
| 压缩失败、取消、快照过期，或未达到压缩阈值 | 不通知压缩成功事件 |
| 复制、恢复、创建分支、导入旧用户历史或同步已有 session evidence | 不重放历史 input，也不继承监听者 |

Input 的 `ItemJSON` 使用现有 `TimelineItem` 序列化，可以解码为独立的 `TimelineItem`，不暴露活跃 item 指针。它包含用户交互的回答和元信息、原始文本及 prompt projection、工具结果、原样提升的数据，避免普通展示省略 evidence 等内容。无法序列化的 item 会记录英文 warning，写入仍然成功。

Freeze 的 `Promotions` 是**本次新冻结**的原样变更，含删除记录，不是完整 evidence 库，也不是 AI 总结。Compression 的 `RetiredIDs` 则包含本次被摘要替换的旧 Frozen 和 Open 普通历史。压缩过程中追加的新 item 保持 Open，不进入这次收据或摘要。

Summary 的 `io.Reader` 来自已校验并提交的摘要。每个监听者都有独立 reader，回调返回后仍然可读；一个监听者读完不会消耗另一个监听者的数据。这里没有提前暴露模型参数流，失败输出也不会作为成功摘要通知。

`Prompt` 和 `Instruction` 是本次压缩请求的核心正文及指令，排除了传输协议外壳和重试纠正后缀。当前正文沿用普通历史压缩逻辑，**不会新加入原样 user input / evidence / 工具缓存 payload**；它们仍通过独立的提升机制保存。这次尚未实现未来的“历史 + evidence → memory”联合抽取。

## 执行与隔离

- 同步通知，释放 `Timeline.mu` 后执行；压缩标志与等待通道也先释放。可以在回调里读取 Timeline、注册回调或追加 item。
- 一次收据按注册顺序通知。回调注册变更不影响已捕获的通知名单；压缩的三个阶段使用同一提交时捕获的名单。
- 并发写入可能并发执行监听者，跨操作不保证通知的全局顺序。消费者应保护自身状态，并以事件 ID / freeze version 识别具体提交，不能把回调期间再次读取的当前状态当成原快照。
- 回调应只做轻量处理或向消费者入队。慢消费者、批处理、取消和重试由接入方负责；调用方自身持有的业务锁不由 Timeline 释放。
- 每个监听者获得独立的切片收据；回调 panic 被记录并隔离，不改变已提交状态，也不阻断后续监听者。
- 注册信息、压缩通知和 prompt 快照只存在于运行态，不写入持久化 Timeline 或主循环 prompt，不改变上下文前缀。没有监听者时，不生成 input JSON 快照或压缩回调收据，也不增加 AI 请求。

## 使用与回归测试

`timeline_callbacks_test.go` 包含四个接口的组合示例，覆盖实际 item 写入、evidence 原子批次、普通冻结、自动压缩、原样内容提交、失败与取消、压缩期间追加、摘要 reader 隔离、回调重入、复制恢复、分支合并、并发注册和 panic 隔离。

```shell
go test ./common/ai/aid/aicommon -run '^TestTimelineCallbacks' -count=1
go test -race ./common/ai/aid/aicommon -run '^TestTimelineCallbacks' -count=3
go test ./common/ai/aid/aicommon -count=1
```

测试位于 Timeline 所属包，复用现有压缩 mock，不调用真实模型，也不新增 `common/yak` 测试或 Yak 冒烟 CI 项。
