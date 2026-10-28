# 27. Timeline 全量上下文压缩：实施进度

设计见 [26-timeline-promoted-state-compression-task-guide.md](./26-timeline-promoted-state-compression-task-guide.md)。本文件只记录当前实现与验证结果。

分支：`rookie/enhance/timeline-compress-strategy-2`

## 当前结构

调度和事务分开：

- [prompt.go](../prompt.go)：`generateLoopPromptFromInput` 组装完整 prompt、检查上限，超限后直接重建压缩 prompt。
- [exec.go](../exec.go)：按照模式解析响应，检查压缩无进展和单一 action，再交给统一 action 执行。
- [action_compress_context.go](../action_compress_context.go)：verifier 只检查 action 合规；handler 解析 action、生成最终 selection 并提交。
- [timeline_compression_active.go](../../../aicommon/timeline_compression_active.go)：维护 Timeline 压缩快照、暴露候选并原子应用最终 selection。
- [timeline_compression_lifecycle.go](../../../aicommon/timeline_compression_lifecycle.go)：压缩选项的上下文默认值。
- [timeline_compression_transaction.go](../../../aicommon/timeline_compression_transaction.go)：原子更新摘要、保留条目、promoted 状态与冻结信息。
- [timeline_compression_promoted.go](../../../aicommon/timeline_compression_promoted.go)：按原始 item 做黑名单退休，不读 `promotedState`。

当前构建函数先组装完整 prompt；超限后直接替换动态段返回压缩模式，目标为 `max(1, limit/6)`。主循环使用质量优先模型并强制单一 `compress_context`，交给下面的统一 action 流程执行，下一轮重新组装。用户输入等不可删材料不会强删或截断，多轮压缩无进展时明确退出。

普通历史使用 `retain_timeline`，支持散落 ID 与多个范围混合；技能按完整名字、证据按观测 id 淘汰。候选来自原始 timeline item；没有点名的 promoted 默认保留，工具 Schema 和用户输入不能删。水位后的新 item 不在本次候选中，事务来源校验继续保留。

正常与压缩模式共用静态前缀、PERSISTENT、SCHEMA 和输出示例，只替换动态段。压缩指引和 few-shot 集中在 promptloader，当前任务、计划与 TODO 保持可见。文本与原生调用都由主循环质量优先模型生成参数，输出不合法时由现有 AI transaction 纠正；没有 action 内部的第二次模型调用或额外重试层。

压缩响应不重复写回 Timeline。prompt_profile 仍只发布实际普通请求，压缩回合不产生额外观测。

压缩 prompt 的输入安全上限是 512K，不截断输入。摘要与保留材料仍受 16K 输出安全检查。

## 测试隔离

`registerTimelineTestLiteForge` 必须和 `liteforge.Execute` 一样，在 `NewConfig` 之前丢掉或复制共享 Timeline。否则 helper 的默认上限会通过 `SoftBindConfig` 写回父 Timeline，压缩中途把运行时上限改成 `300 * 1024`。

观测测试的 emitter 使用唯一运行 ID，避免不同用例读取相同 checkpoint。复用固定 emitter ID 会使原生调用偶尔读到旧文本响应；这不是压缩快照未释放。

## 验证入口

- `context_compression_test.go`：六分之一目标、两种协议的强制 action、纠错重试和 action 不调用 AI。
- `prompt_compression_mode_test.go`：仅改变动态段，任务、TODO 和缓存前缀不变。
- `prompt_observation_publish_test.go`：两种协议下的正常、AI 压缩、provider 超长和无进展退出；压缩不重复写回、不额外发布 profile。
- `aicommon/timeline_compression_*_test.go`：散落范围、选择校验、冻结、原子提交、保存恢复与分支行为。

2026-10-21：压缩与主循环观测相关用例连续 10 轮通过，动态段切换与前缀一致性用例连续 10 轮通过；`aicommon` 和 `aireact/...` 完整回归复跑通过，`aimem`、`liteforge` 编译检查通过。首次完整回归曾出现原生流式 Unicode 转义用例偶发失败，单独连续 50 轮未复现，没有改动该流式解码逻辑。

## 已知边界

- 技能正文退休后保存会省略 journal，重启后旧名单导入可能重新展开。
- 分支 timeline 的 evidence 退休仍可能被 session 导入覆盖。
- 已跳过的独立长期记忆提取不在本次范围内。
- 全量正文进入压缩请求后，体积随条目增长；512K 是本地安全上限，provider 仍可能更小。
