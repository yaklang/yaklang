# Timeline 压缩

## 主循环入口

`reactloops/prompt.go` 的 `generateLoopPromptFromInput` 返回正文、成分快照和 prompt 模式。它先正常组装，再检查整份 prompt 的 token 数；默认上限为 `300 * 1024` tokens，不单独测量 Timeline。

超过限制时，直接用同一份任务输入重建压缩 prompt：只替换动态段，加入压缩指引与候选表，返回上下文压缩模式。默认目标为 `50 * 1024` tokens（上限的六分之一），触发上限保持 `300 * 1024`。冗余的工具和证据操作由 promoted 状态转移自己折叠，不再在压缩前单独扫一遍。

`exec.go` 根据返回模式调用模型，并在压缩模式外发出上下文优化状态。压缩模式只把预期 action 改为 `compress_context`；该 action 走原有执行流程，只提交已验证的选择，不再内部调用 AI。压缩响应不重复写回 Timeline；完成后下一轮重新构建 prompt，连续压缩没有有效缩小时以明确错误退出。

Push、渲染、Save、恢复和 fork/merge 都不会自行调用 AI 压缩。模型报告上下文过长时，沿用主循环的动态限制调整与有界重试。旧 Timeline 调度器的冷却、失败指纹及单独预算测量已删除。

## 模型请求与输出

正常模式沿用原动态段；压缩模式使用独立的 `context_compression_dynamic_section.txt`，保留当前任务、TODO、计划、运行状态和记忆参考，加入压缩指引，不展示普通能力推荐。两者共用相同的静态前缀、PERSISTENT、SCHEMA 和输出示例。指引资源在：

`promptloader/prompts/ai/aid/aireact/reactloops/prompts/timeline_compression.txt`

动态表提供整份 prompt 的当前长度、目标长度，以及普通历史 ID、可淘汰技能名和证据 id。正文片段只用于定位，不要求模型复制原文来标识条目。

```json
{
  "@action": "compress_context",
  "summary": "已完成主要修复，保留复现、验证与未完成回归的原始记录；其余探索已合并。",
  "retain_timeline": "3-8,12,20-26,37,41-45",
  "retire_skill_names": [],
  "retire_evidence_ids": [],
  "memory_entities": []
}
```

`retain_timeline` 是逗号分隔的单个 ID 与闭区间组合，支持多个散落段。空字符串表示普通历史全部由 summary 概括。选择会与本次请求中的普通历史候选求交集，因此 ID 可以是用户输入、技能、证据或内部条目的 ID，也可以是已经不在本次范围内的旧 ID；这些 ID 不会报错，也不会让受保护内容被淘汰。格式损坏的段和未知 ID 单独忽略，其他有效段继续生效。提交前还会按实际 source 再收敛一次：越界、不存在、重复或未在请求中观测到的 ID 单独丢弃，promoted 淘汰也不会作用于未观测项。工具调用与结果按完整条目保留。

技能仍按名字整份淘汰（包含分页与 metadata），证据按观测 id 淘汰；没有点名、未知或格式无效的项全部保留。工具 Schema、用户输入、TODO、任务指令和目录不允许删除。旧的 `timeline-summary` action、`ratain_timeline_item_range` 与 `retire_promoted_entries` 输出协议不再解析。

压缩 prompt 的建议输入上限为配置的 Timeline 限制再加 `200 * 1024` tokens。超过时只记录 warning 并继续请求，不截断 prompt。Timeline 没有旧摘要、普通历史或可处理 promoted 项时，本轮直接退出压缩模式并继续普通执行；摘要与保留原文的合计告警阈值为 `64 * 1024` tokens，超过时复用 `warning` 普通流。压缩后 prompt 未下降至少 1% 时同时记录日志并发送普通 warning 流，不再终止任务。

`summary` 是唯一必需内容；可用的 summary 足以提交压缩，缺失或无法使用 summary 才要求 AI 重试。`retain_timeline`、技能/证据淘汰、memory 和附带 TODO 均为 best-effort，解析失败时降级为空，其中 TODO 永远不会由压缩 action 应用。正常模式沿用既有模型策略；压缩模式由主循环使用质量优先模型。文本和原生 tool-call 仍只接受单一 `compress_context`。

压缩 guide 构建、候选读取或结果提交遇到 Timeline 边界、并发状态、快照或模板错误时，不修改 Timeline，也不把基础设施错误交给 AI 纠正；系统发送普通 warning，并回退普通模式或下一轮重新构建压缩 prompt。Task context 的取消状态仍由核心循环统一验证。原生 tool-call 如果一次返回多个 `compress_context`，在 AI transaction 内视为结构错误并重试，不会直接终止核心循环。

## 提交、并发与保存

`RenderCompressionGuide` 只读取当前 timeline 并返回动态段，不保留快照。`compress_context` 是普通 action，模型可以主动调用；超限时主循环只把动态段换成压缩指引，并要求这一轮响应是单个 `compress_context`。verifier 只检查 action 合规；handler 再读一次当前 timeline，把 action 翻译成 ID，然后调用 `ApplyCompressionSelection`。应用看当时的 timeline，不再对照引导生成时的快照：

- 成功：原子发布新摘要、保留条目和冻结状态，淘汰未保留的普通历史及点名的 promoted 项。
- 失败或取消：拒绝提交，保留原文。
- 应用时已经存在、且没有被保留的普通历史会一起淘汰。应用之后才追加的 ID 留在下一段 Open。

摘要历史完整保留用于追溯，不叠加到当前 prompt；Save 不触发压缩或紧急裁剪。旧 reducer 存档的读取迁移仍保留。

记忆候选的既有通知与持久化通道不变；本次清理不新增独立记忆提取。

## 回归测试

- `timeline_compression_output_test.go`：散落 ID/范围组合、重叠去重、稀疏超大范围、非法选择、受保护材料、应用时未保留的新条目和保存恢复。
- `timeline_compression_summary_test.go`：动态候选 ID、整份 prompt 容量和校验失败后清除旧选择。
- `timeline_compression_promoted_test.go`：未点名的 promoted 项默认保留。
- `reactloops/context_compression_test.go`：构建模式选择、六分之一目标、两种协议的强制 action 与校验重试、action 不调用 AI。
- `aireact/prompt_compression_mode_test.go`：仅替换动态段，静态前缀与当前任务/TODO 保持不变。
- `reactloops/prompt_observation_publish_test.go`：主循环统一执行压缩 action、无进展退出及正常 prompt 观测数量。
- 其余 `timeline_compression_*_test.go`：快照完整性、冻结、恢复、保存和分支合并。

## 主模型可配置阈值

配置质量优先模型的 ReAct 使用该具体服务/模型保存的阈值，初始为 500,000 tokens（1K = 1,000）。轻量、视觉及未绑定的 SDK 路径保留原默认值。压缩算法、时机、历史边界和调用路由不变，新增功能只保存原有自动收缩成功后的阈值。具体身份、边界、静默更新和测试见 [主模型上下文阈值行为契约](model_context_contract.md)。
