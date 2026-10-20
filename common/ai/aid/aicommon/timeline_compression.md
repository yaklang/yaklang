# Timeline 压缩

## 主循环入口

主循环在 `reactloops/exec.go` 中正常构建整份 prompt，再检查 token 数。限制沿用 `WithTimelineContentLimit`，默认 `300 * 1024` tokens；检查对象是整份 prompt，不单独计算 Timeline 阈值。

超过限制时，`compressMainPrompt` 发出 `context.optimizing` UI 状态，执行一次无 AI 冗余折叠，然后重新构建整份主循环 prompt：

- 长度不超过限制的一半：不调用模型，返回主循环重新生成 prompt。
- 仍超过一半：代码触发 `compress_timeline` action，通过主模型完成摘要与保留/淘汰选择，再返回主循环重建 prompt。

Push、渲染、Save、恢复和 fork/merge 都不会自行调用 AI 压缩。模型报告上下文过长时，沿用主循环的动态限制调整与有界重试。旧 Timeline 调度器的冷却、失败指纹及单独预算测量已删除。

## 模型请求与输出

压缩请求直接复用折叠后构建的整份主循环 prompt，不改 PERSISTENT、SCHEMA 和输出示例，只在末尾追加压缩要求与动态候选表。指引资源在：

`promptloader/prompts/ai/aid/aireact/reactloops/prompts/timeline_compression.txt`

动态表提供整份 prompt 的当前长度、目标长度，以及普通历史 ID、可淘汰技能名和证据 id。正文片段只用于定位，不要求模型复制原文来标识条目。

```json
{
  "@action": "compress_timeline",
  "summary": "已完成主要修复，保留复现、验证与未完成回归的原始记录；其余探索已合并。",
  "retain_timeline": "3-8,12,20-26,37,41-45",
  "retire_skill_names": [],
  "retire_evidence_ids": [],
  "memory_entities": []
}
```

`retain_timeline` 是逗号分隔的单个 ID 与闭区间组合，支持多个散落段。空字符串表示普通历史全部由 summary 概括。范围起止必须属于本次快照列出的普通历史；只筛选这些候选，不展开区间中的每个整数，不影响夹在其中的受保护内容。重叠及重复 ID 自动去重，工具调用与结果按完整条目保留。

技能仍按名字整份淘汰（包含分页与 metadata），证据按观测 id 淘汰；没有点名的全部保留。工具 Schema、用户输入、TODO、任务指令和目录不允许删除。旧的 `timeline-summary` action、`ratain_timeline_item_range` 与 `retire_promoted_entries` 输出协议不再解析。

输入安全上限为 `512 * 1024` tokens；摘要与保留原文的输出安全上限默认 `16 * 1024` tokens。超限或非法选择会报错，不静默截断。输出解析、校验与提交放在同一个 AI transaction 内，由 transaction 提供错误引导及重试，不额外套压缩重试循环。

## 提交、并发与保存

`BeginCompressionDecision` 预占唯一压缩事务并获取独立快照，AI 请求期间不持有 Timeline 写锁。提交时再次校验取消状态、来源指纹、旧摘要和冻结版本：

- 成功：原子发布新摘要、保留条目和冻结状态，淘汰选中的普通历史及 promoted 项。
- 失败、取消或来源被修改：拒绝提交，保留原文；退出时释放事务占用。
- 快照之后追加的更大 ID：留在下一段 Open，不受旧选择影响。

无 AI 折叠仅移除冗余 promoted 操作，保留有效工具 Schema 和证据。摘要历史完整保留用于追溯，不叠加到当前 prompt；Save 不触发压缩或紧急裁剪。旧 reducer 存档的读取迁移仍保留。

记忆候选的既有通知与持久化通道不变；本次清理不新增独立记忆提取。

## 回归测试

- `timeline_compression_output_test.go`：散落 ID/范围组合、重叠去重、稀疏超大范围、非法选择、受保护材料、并发追加和保存恢复。
- `timeline_compression_summary_test.go`：动态候选 ID、整份 prompt 容量和校验失败后清除旧选择。
- `timeline_compression_promoted_test.go`：缺省保留及无 AI 冗余折叠。
- `reactloops/timeline_compression_before_prompt_test.go`：主循环 prompt 复用、压缩后重建及 UI 状态。
- 其余 `timeline_compression_*_test.go`：快照完整性、冻结、恢复、保存和分支合并。
