# 显式工具调用与 Schema 加载

## 模型动作的边界

`require_tool` 与 `load_capability` 的工具分支只加载完整参数定义到 Timeline 的 `CACHE_TOOL_CALL`。加载不执行业务工具，不请求辅助模型构造参数，也不交付阶段成果。所属循环随后读取定义并用 `directly_call_tool` 继续当前任务。

`directly_call_tool` 接受完整显式参数，支持单个调用与独立调用批次。模型必须依据真实 Schema 构参。参数校验失败会补充可用 Schema，在 Timeline 中记录 reason/retry，交由同一循环修正；不转入自动参数生成。

后一调用依赖前一结果时，先执行前一调用，观察真实输出，再在下一轮构造后续参数。PLAN 的任务 DAG 继续负责任务依赖、派发及验收；普通工具调用不另建意图 DAG。

## 两种协议

原生 Function Call 通过函数名选择动作，以下对象直接写入 arguments，不携带 `@action`：

```json
{"tool_require_payload":"read_file"}
```

读取 Schema 后，调用 `directly_call_tool`：

```json
{"directly_call_tool_name":"read_file","directly_call_tool_params":{"file":"/workspace/README.md"},"directly_call_reason":"确认项目说明"}
```

文本流使用完整动作对象：

```json
{"@action":"require_tool","tool_require_payload":"read_file"}
```

```json
{"@action":"directly_call_tool","directly_call_tool_name":"read_file","directly_call_tool_params":{"file":"/workspace/README.md"},"directly_call_reason":"确认项目说明"}
```

多个 Schema 可以用 `tool_require_calls` 加载，每项指定 `tool_name`。这仍是定义加载，不是执行批次。`load_capability` 使用准确 `capability_identifier`；非工具能力保留各自生命周期。

## 独立执行批次

原生 `directly_call_tool` 的批次 arguments 示例：

```json
{
  "directly_call_tool_calls": [
    {"tool_name":"read_file","params":{"file":"/workspace/a.txt"},"identifier":"read_a","reason":"核对来源 A"},
    {"tool_name":"read_file","params":{"file":"/workspace/b.txt"},"identifier":"read_b","reason":"核对独立来源 B"}
  ]
}
```

文本流在同一对象顶层增加 `"@action":"directly_call_tool"`。批次数组与单次字段互斥；每项必须提供完整 `params` 对象，不在数组内放 `@action`。调用相互独立、风险和规模符合门槛时才使用批次；同名工具的不同参数按数组顺序保留。

动作验证使用 canonical 参数，不读会把数组成员混合的 flattened 兼容缓存。原生同一响应中的多个动作拥有各自验证状态，不共享目标或批次。

## 审核与返回所属循环

| 用户决定 | 行为 |
| --- | --- |
| `continue` | 同意当前提案，校验及最终边界后执行 |
| `continue` 携带修改的 `params` | 作为新提案验证，并重新审核 |
| `wrong_tool` | 关闭当前提案，不执行；加载相关 Schema，记录反馈，返回所属循环重新选择工具 |
| `wrong_params` 无 `params` | 关闭当前提案，不执行；记录反馈及原参数，返回所属循环重新构参 |
| `wrong_params` 携带明确 `params` | 使用人工修改值；验证并审核新提案，不请求模型修参 |
| `direct_answer` | 按明确用户意图停止工具流程并直接回答 |
| 任务取消或超时 | 终止待审核、待执行操作，保留已完成结果 |

人工修改值与原参数完全一致时，验证后视作同意，避免对同一提案反复产生卡片。缺少 `params` 与显式空对象不是同一种操作；空对象仍需通过业务 Schema。

`ToolReviewReconsiderError` 表示用户否决提案。动作处理器用 feedback/continue 返回循环，不把它当作插件错误或直接回答。循环归属在调用准入时捕获，审核等待期间当前任务指针变化不会把反馈交给另一任务。用户反馈及 Schema 加载事实进入所属 Timeline；旧审核子 AI 不再参与纠错。

## 批次并发与部分结果

批次为每项分配独立结果、事件和 checkpoint。人工审核按数组顺序展示，同一时刻只有一个 pending endpoint。通过审核的项在最终 barrier 等待所有兄弟项审核结束或提前失败；因此用户明确 `direct_answer` 时，尚未启动的回调不会越过屏障。

普通失败和提案否决按项结算，独立且通过审核的兄弟项可以继续。错误反馈包含已完成与未完成操作，主模型只修正未完成项；不能重发整个批次而重复已完成副作用。调用取消后，最终准入边界再次检查 context。

显式参数在准入阶段应用工具参数 mutator。人工修改产生新提案时重新应用当前工具的 mutator；不重复修改原提案。

settled 结果保留原顺序、请求工具、最终工具、stage、错误及实际执行结果。实际执行次数依据结果边界记录，不以工具名出现次数判断。Schema 加载和审核拒绝均不是执行事实。

## Checkpoint 与恢复

已完成的审核决定在恢复时按 checkpoint 应用，不重复展示审核卡。已结算的批次结果可重放，已完成回调不重新执行。被拒绝项重新尝试时，由所属循环给出新的显式调用；不能把旧拒绝变成批准。

恢复重放应保留取消、直接回答、错误及部分完成状态，不能仅按成功项重新推断整批状态。旧低层 name-only 调用、参数生成及 checkpoint 兼容 API 尚未整体删除；它们不属于当前模型动作入口，将在后续迁移中单独处理。

## 侦察模式与缓存

进入 `infosec_recon` 时，按稳定优先顺序考虑常用且已启用的工具。预加载零执行，复用现有 Timeline 工具缓存及协议渲染。预算不足的可选定义跳过，不淘汰已有工作集；缺失或尚未就绪的可选工具不触发数据库发现或等待。随后仍可用 `require_tool` 按需加载定义。

端口、爬虫、Banner、DNS、子域名、网络空间查询统一使用 `directly_call_tool` 携带完整参数，不再注册同名自动构参动作。既有带明确业务参数的转发动作和侦察资料管线保持原有功能。

缓存相同定义产生 REUSE，不重复 UPSERT；普通按需加载遵守现有淘汰规则。批量加载完成后检查最终成员，已被后续定义淘汰的工具不会被报告为可用。切换协议时 usage examples 随当前循环模式渲染。

## 验证位置

- `aireact/invoke_toolcall_review_reconsider_test.go`：主循环/Coordinator worker × 双协议，拒绝后重新决策，捕获正确归属，无辅助构参。
- `aireact/invoke_toolcall_batch_hardening_test.go`：逐项拒绝、兄弟项结算、人工改参、直接回答、取消及 checkpoint 重放。
- `aireact/recon_explicit_tools_test.go`：六类侦察工具预加载零执行、显式调用及缓存复用。
- `reactloops/loopinfra/tool_call_action_isolation_test.go`：原生多个动作的验证状态隔离。
- `aitool/buildinaitools/timeline_toolcache_manager_test.go`：预算、工作集保留、并发准入及协议渲染。
- `common/ai/aismoking`：独立 Yak 本地业务冒烟，不加入 CI。

公共 `ToolComposeConcurrency` 兼容选项仍被 fast-context grep 使用，因此保留；它不重新注册已删除的工具 DAG 动作。
