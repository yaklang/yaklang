# Timeline 监听回调

在 session Timeline 上注册监听。摘要提交与记忆候选生成是两个独立节点：模型完成 `summary` 和 `ratain_timeline_item_range` 后，Timeline 校验并原子提交，主循环即可继续；同一次请求随后完成 `memory_entities`，通过独立订阅交给消费者。这里不写入记忆库。

## 注册接口

```go
timeline.RegisterItemInputCallback("consumer", func(event aicommon.TimelineItemInputEvent) {
    // 已提交的输入快照：ID、Timestamp、ItemJSON。
})
timeline.RegisterFreezeCallback("consumer", func(event aicommon.TimelineFreezeResult) {
    // NewlyFrozenIDs、Promotions 是本次新冻结内容。
})
timeline.RegisterCompressFreezeCallback("consumer", func(event aicommon.TimelineCompressFreezeEvent) {
    // Freeze 冻结收据 + Compression 摘要/保留原文提交收据。
})
timeline.RegisterSummaryCallback("next-loop", func(event aicommon.TimelineSummaryEvent) {
    summary, err := io.ReadAll(event.Summary)
    if err != nil {
        return
    }
    // 摘要已提交。RetainedIDs / RetainedRange 对应原样保留的普通 item。
    // ThroughID 是本次处理边界；Prompt / Instruction 是请求资料与静态指令。
    _, _, _ = summary, event.RetainedIDs, event.RetainedRange
})
timeline.RegisterMemoryCallback("memory-storage", func(event aicommon.TimelineMemoryEvent) {
    if event.Err != nil {
        // 记录或交给上层处理；摘要和保留原文不回滚。
        return
    }
    // 向存储消费者入队 event.MemoryEntities（[]any）；本层不负责落库。
    // 使用 event.ThroughID 关联对应摘要，避免误用当前 Timeline 状态。
})
```

同一种事件里，相同 ID 替换原回调；传入 `nil` 注销。不同事件的 ID 互不影响：

```go
timeline.RegisterMemoryCallback("memory-storage", nil)
```

## 通知时机

| 操作 | 通知 |
| --- | --- |
| 输入、工具结果、用户交互、evidence / 工具缓存写入 | 完整写入提交后逐 item 通知 input |
| 分支合并 | 仅通知真正写入父 Timeline 的 item |
| `Freeze` / `FreezeAll` | 有新冻结内容时通知 freeze |
| 压缩的摘要与保留范围可用、快照校验通过 | 提交后按顺序通知 freeze → compress-freeze → summary |
| 同一次压缩请求的完整响应结束 | 在摘要成功提交后，异步通知 memory；可为空候选或带 Err |
| 只有原样提升内容的提交 | 无 AI 请求；通知 freeze、compress-freeze，无 summary / memory |
| 压缩已冻结历史、没有新冻结 item | 通知 compress-freeze、summary，随后独立通知 memory |
| 摘要无效、取消或快照过期，未能提交 | 不通知 summary / memory |
| 复制、恢复、分支、导入历史或同步已有 evidence | 不重放历史，不继承监听者 |

兼容旧的仅 `summary` 响应时，需要等完整对象解析后提交。常规响应先输出摘要和范围，因此不必等待记忆尾部。晚到的记忆解析错误、超预算、取消或与已提交摘要不一致，会通过 `TimelineMemoryEvent.Err` 报告，候选为空，不撤销摘要，也不会将失败尾部认作记忆。原始快照失效而未提交的请求不会产生记忆通知。

`ItemJSON` 使用已有 `TimelineItem` 序列化，包含交互回答、元信息、文本、工具结果和提升数据。`Promotions` 是本次新冻结的原样变更，包含删除记录；`RetiredIDs` 是被摘要替换的普通历史，`RetainedIDs` 是原样保留的普通历史。压缩期间新写入的 item 保持 Open。

摘要 reader 和 ID 切片按监听者隔离，记忆候选深拷贝；一个消费者不会耗尽或改坏另一个消费者的数据。`Prompt` 是 AITAG 分段的原文资料，包括用户数据、session evidence、旧摘要与冻结/即将冻结历史；`Instruction` 是静态指令。两者不含传输协议外壳或重试纠正后缀。

## 执行与隔离

- input、freeze、compress-freeze、summary 同步通知，释放 Timeline 锁和压缩等待标志后执行。回调可以读取 Timeline、注册回调或追加 item，应只做轻量处理。
- memory 在独立 goroutine 中等完整响应，不阻塞摘要提交或后续循环。其消费者也应入队处理，避免长时间占用通知 goroutine。请求取消后会报告完成错误，不保证取消的会话能继续生成记忆。
- 各阶段使用提交时捕获的名单；注册变更只作用于后续提交。同一提交内按注册顺序通知，跨提交和并发写入不保证全局顺序。消费者需保护自己的状态。
- 注册信息与请求快照只存在于运行态，不写入持久化 Timeline 或主循环 prompt，不增加 AI 请求。只有记忆订阅、没有摘要订阅也能收到记忆通知。
- 回调 panic 被记录并隔离，不改变已提交状态。

## 验证

`timeline_compression_stream_test.go` 使用可控管道，验证记忆未输出时摘要已返回、后续输入可写入；覆盖失败尾部、摘要后改写、慢记忆消费者及独立记忆订阅。`timeline_callbacks_test.go` 和 `timeline_compression_output_test.go` 覆盖原子提交、过期快照、隔离、重入和候选容错。

```shell
go test ./common/ai/aid/aicommon -count=1
go test -race ./common/ai/aid/aicommon -run 'TestTimeline(Compression|Callbacks)|TestAction' -count=1
```

本地独立 Yak 冒烟位于 `common/ai/aismoking`，不加入 CI。
