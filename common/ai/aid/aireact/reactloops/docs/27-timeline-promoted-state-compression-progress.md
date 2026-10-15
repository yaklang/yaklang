# 27. Timeline 全量上下文压缩：实施进度

设计见 [26-timeline-promoted-state-compression-task-guide.md](./26-timeline-promoted-state-compression-task-guide.md)。本文件只记录当前实现与验证结果。

分支：`rookie/enhance/timeline-compress-improve`（基于 `rookie/enhance/skill-merge-to-timeline` 的 `6e54092e7`）

## 当前结构

调度和事务分开：

- [timeline_compression_before_prompt.go](../../../aicommon/timeline_compression_before_prompt.go)：`CompressBeforePrompt` 只做参数、执行权、触发判断，然后交给 `foldThenCompressTimelineToTarget`。
- [timeline_compression_measure.go](../../../aicommon/timeline_compression_measure.go)：用 `renderTimelinePromptLocked(...).PromptText()` 测量主模型占用，并计算失败退避。
- [timeline_compression_lifecycle.go](../../../aicommon/timeline_compression_lifecycle.go)：共享执行权、回执合并、锁外通知。
- [timeline_compression_compact.go](../../../aicommon/timeline_compression_compact.go)：`foldTimelineWithoutAI`，无 AI 的确定性折叠。
- [timeline_compression_transaction.go](../../../aicommon/timeline_compression_transaction.go)：`compressTimelineOnceWithAI`，一笔快照、模型、校验、提交。
- [timeline_compression_promoted.go](../../../aicommon/timeline_compression_promoted.go)：按原始 item 做黑名单退休，不读 `promotedState`。

触发条件是实际占用达到 `TimelineContentSizeLimit`。目标是 `max(1, limit/6)`，不是刚好贴着上限。先做一次无 AI 折叠并复测；折叠已达到目标就结束。否则只做一次 AI 压缩并返回，同一次检查不再循环。一次 AI 后仍高于目标留给下一次检查。用户输入等不可删材料过大时保留原文，不强删或截断。

无 AI 折叠只处理 recent-tool-cache 和 Evidence 的冗余操作。Skill 是追加语义，不自动折叠。

promoted 压缩项逐条来自 timeline item。`retire_promoted_entries` 只删点名 ID；缺失、空值和非法值默认保留。解析后直接执行合法 ID 名单，不按同 key 的操作扩大或取消删除，之后从剩余 item 重建 state。水位后的新 item 不在本次候选中，用户输入不可删除。已移除 `planCompressionPromotedRetirementLocked` 和辅助重放函数；事务来源校验继续保留。

辅助请求的默认输入安全上限是 512K，不截断输入。摘要与保留材料仍受 16K 输出安全检查。

## 测试隔离

`registerTimelineTestLiteForge` 必须和 `liteforge.Execute` 一样，在 `NewConfig` 之前丢掉或复制共享 Timeline。否则 helper 的默认上限会通过 `SoftBindConfig` 写回父 Timeline，压缩中途把运行时上限改成 `300 * 1024`。

## 验证

2026-10-15，直接按 ID 退休改动后的验证全部通过：

```sh
go test ./common/ai/aid/aicommon/... -count=1 -timeout=180s
go test ./common/ai/aid/aireact/... -run 'Test.*(TimelineCompression|TimelineMemory|TaskMemory)' -count=1 -timeout=180s
go test ./common/ai/aid/liteforge -run '^TestTimelineCompressionSummaryCompatibilityThroughLiteForge$' -count=1 -timeout=120s
```

`timeline_compression_promoted_test.go` 覆盖精确 ID 退休、同 key 的其它操作不取消删除、水位后新 ID 不可选，以及工具、Evidence、Skill 按剩余 item 重建和保存恢复。缺失/非法名单默认保留、用户输入保护和事务来源变化拒绝仍由原有回归覆盖。`aireact` 为上述正则筛选的相关回归，不是全量测试。

同日完成无 AI 折叠命名与顺序调整后，重新通过 `aicommon` 全量及上述 `aireact` 相关回归。新增 `TestTimelineCompressionFoldsWithoutAIOnceBeforeAIRounds` 连续运行 20 次通过：第一轮 AI 期间追加新的冗余操作，第二轮直接进入 AI，整次优化只产生一次无 AI 折叠提交。

## 已知边界

- 技能正文退休后保存会省略 journal，重启后旧名单导入可能重新展开。
- 分支 timeline 的 evidence 退休仍可能被 session 导入覆盖。
- `FinalizeMemory` 复用同一事务，也可以返回删除名单。
- 全量正文进入压缩请求后，体积随条目增长；512K 是本地安全上限，provider 仍可能更小。
