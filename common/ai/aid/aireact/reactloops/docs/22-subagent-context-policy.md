# 子Agent上下文策略：从调研到实现

## 决定及依据

派发动作支持每个job指定`context_mode: fork | task_only`，省略时为`fork`。同一批次可以混用。

- Codex锁定版本的V2把历史范围作为显式派发参数，默认all，可none或最近N轮。这支持“让模型选择上下文来源，由程序保证具体语义”的接口设计。
- DeepSeek区分spawn和fork；fork只继承已完成用户轮次。PI指定示例、OpenCode新建子任务均以显式任务书开始，不复制整段父聊天。它们证明自包含委派有独立价值。
- Yaklang原有普通派发依赖Timeline Fork、会话证据和任务附件，直接把默认值改成空白会改变既有任务行为。因此保留默认Fork，新增显式task_only。
- Yaklang的Timeline包含压缩摘要、工具观察及其他状态，并不等同于聊天轮次。本次不复制Codex的“最近N轮”选项，不制造不准确的对应关系。

外部固定提交、源码链接见[上下文复核中的框架对照](21-subagent-context-and-exit-audit.md)。

## 两种模式的精确定义

| 内容 | fork（默认） | task_only |
| --- | --- | --- |
| Timeline | 派发时快照 | 新建空Timeline |
| 用户输入历史、会话证据 | 派发时复制 | 不自动继承 |
| 证据/产物渲染快照 | 复制 | 新状态 |
| 父活动TODO | 不继承 | 不继承 |
| 当前输入附件providers | 派发时固定集合 | 不自动注入 |
| 父PlanPrompt、冻结计划/记忆分区 | 保留原规则 | 清空 |
| 父持久会话ID | 保留原规则 | 不继承，防止辅助调用从DB重新加载父历史 |
| MemoryTriage | 保留原规则 | 关闭并使用no-op，避免自动补回父记忆 |
| 目标与交付契约 | 显式传入 | 显式传入，必须自包含 |
| 宿主preset、普通providers | 保留 | 保留 |
| 工具/资源权限、工作目录 | 原有边界 | 同样保留 |
| 已报告漏洞去重清单 | 共享 | 共享 |
| task取消与退出门禁 | 受父生命周期约束 | 相同 |

task_only是“不自动注入父会话事实”，不是文件系统沙箱或绝对空上下文。宿主自定义提示和普通provider依然有权提供上下文；子任务也仍可使用获准工具读取任务书指定的材料。必要的用户约束、输入路径、验收条件必须写入goal/result_contract。框架不能推断自然语言任务书是否已经包含所有必要事实。

旧内部`SubAgentOptions.TimelineMode=Clean`且job未指定context_mode时，继续表示“只清Timeline”。审计与内部搜索的调用语义保持兼容，不把它悄悄改成task_only。

## 完整调用例子

用户：“继续根据刚才的故障分析检查支付模块，同时独立复核库存代码，你整理部署配置。”

模型可以选择：

```json
{
  "@action": "dispatch_sub_react_agents",
  "dispatches": [
    {
      "identifier": "payment",
      "context_mode": "fork",
      "goal": "沿用当前调查证据，检查支付模块的连接超时原因。只读分析。",
      "result_contract": "给出原因、代码或日志证据、未确认项。"
    },
    {
      "identifier": "inventory",
      "context_mode": "task_only",
      "goal": "独立只读检查项目inventory目录的连接池释放逻辑，不预设已有故障结论。",
      "result_contract": "给出问题位置、触发条件；没有发现时说明检查范围。"
    }
  ]
}
```

1. **解析和验证**：从完整动作对象取dispatches，保留每个job的字段边界；非法context_mode整批报错。支持原有JSON字符串数组表示，同样严格验证。
2. **派发时准备**：支付取得当前Timeline/会话快照；库存取得空Timeline和清除会话事实后的配置。两者获得相同的宿主权限边界，各自保留明确任务书。
3. **立即返回**：回执包含各job的实际context_mode。父下一轮可以读取部署配置；排队期间不会重新决定上下文策略。
4. **后台目标扩写**：使用已经准备好的子配置和Timeline。LiteForge额外自动注入Timeline被关闭，避免fork把派发后数据库历史混进快照，也避免task_only重新读入父历史。
5. **子模型运行**：支付可见先前调查；库存的模型请求不含父聊天、父证据或父附件。两者各写自己的Timeline。
6. **交付及退出**：两种模式沿用同一结果摘要/原文引用、等待、取消和finish门禁。父收到两份结果后综合判断，子中间轨迹不MergeBack。

## 实现入口

- `loopinfra/dispatch_sub_react_agents.go`：模型schema、两种模式使用指导。
- `subagent_pipeline.go`：ContextMode、完整对象解析、每job模式选择、扩写提示。
- `subagent_timeline.go: snapshotSubAgentConfig`：会话状态、provider、冻结分区、持久会话与memory策略。
- `aicommon/session_prompt_state.go: ForkForTaskOnlySubAgent`：只保留共享去重清单。
- `aicommon/contextprovider.go: WithoutTaskContext`：去除任务附件，保留普通宿主provider。
- `subagent_manager.go`：提交前校验，回执及观察报告实际context_mode。

## 验证

受控测试覆盖混合批次、默认Fork、非法值、原JSON字符串兼容、旧Clean行为，以及父配置不被修改。真实子运行体测试沿BuildReActInvoker→LiteForge→子ReActLoop捕获发往模型的请求：父持久会话、父历史、冻结计划和记忆哨兵在task_only中缺席，显式goal/result_contract和宿主preset仍在；fork保留准备时的Timeline，却不额外加载数据库历史。

测试模拟模型响应，不使用真实模型服务。它们验证上下文契约和执行路径，不证明模型每次都能选出最佳模式。
