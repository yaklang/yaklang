# SubAgent上下文与退出控制复核

本报告针对2026-09-16工作区中的非阻塞SubAgent实现。外部对照使用先前调研锁定的提交，重新阅读本地源码，不把不同版本的默认行为混在一起。

本文记录默认Fork与旧Clean的复核。后续已实现每job的fork/task_only选择，当前契约见[上下文策略实现](22-subagent-context-policy.md)。

## 1. 先用完整案例确定语义

用户：“分析支付和库存日志，你检查部署配置，最后给出综合结论。”

1. 自由输入经会话连接进入根task队列，根task获得执行机会后进入ReActLoop。task代表一次工作，loop代表这次工作的“模型选择→执行→反馈”循环，session是更长寿的会话。
2. 父模型读到任务、历史和工具描述，选择dispatch两个子任务。普通dispatch明确指定Fork模式：在派发线程上复制当时的Timeline和会话提示状态，再交后台排队。每个子任务的当前输入是自己的goal和result_contract。
3. 假设库存任务在队列中等了30秒。这期间父读到新部署配置：库存仍使用派发时的历史快照，不会因为晚启动就自动取得新配置。父应在任务书中提供必要信息，或待前置结果到达后再派发依赖它的新任务。当前实现没有给运行中子任务追加消息的动作。
4. 支付子任务只往自己的Timeline写工具观察和答案。父模型不会实时看到这些历史；用户可能先通过子任务UI事件看到输出。支付终态后，父下一轮输入收到结果摘要与完整产物引用，这才是父可据以决策的交付点。
5. 父已检查配置但库存未完成时，可先directly_answer作阶段答复，再继续或wait。普通任务的“答案已发送”与“工作已结束”分开。wait默认最多30秒，超时只结束本次观察。
6. 普通finish要求原有goal/TODO条件满足，并且没有活跃子任务、没有尚未进入本轮模型输入的终态结果。用户明确停止、执行错误或预算耗尽则走各自收尾路径，不要求无限等待子任务完成。

## 2. Yaklang到底继承什么

| 内容 | 实际语义 | 容易误解的地方 |
| --- | --- | --- |
| Timeline | 普通dispatch复制派发时快照；父子之后各写各的 | 不是共享可变历史，也不是只复制上一轮完整聊天 |
| 压缩历史 | 序列化包含当前保留条目、压缩头/历史、归档引用与提升状态 | 已经压缩的信息不会因fork恢复成完整原始对话 |
| 新目标 | 子task独立持有goal/result_contract，并成为子loop当前问题 | 继承父历史不表示接管父的全部任务 |
| 用户输入历史、会话证据 | 在派发时复制 | 父后来新增证据不自动传播给已有子任务 |
| 当前TODO | 不继承权威TODO状态；子从空清单开始 | 旧历史或摘要仍可能提到父待办，但不成为子任务的活动TODO |
| 证据/产物渲染状态 | Fork各自状态 | 文件引用对应的文件内容本身没有复制成只读快照 |
| 已报告漏洞清单 | 有意共享，用于父子去重 | 这是实时共享信息的明确例外，不是子Timeline合并 |
| 上下文提供器 | 固定任务级provider集合；普通provider继续共享 | 复制的是回调集合，不保证任意回调读取的数据永远不变 |
| 模型配置、提示词 | 通过ConvertConfigToOptions继承适用配置，子覆盖生命周期和策略 | 不是复制父完整的最终prompt，也不是所有配置都深拷贝 |
| 工具、技能、目录等 | 保留既有共享资源与权限边界 | 历史隔离不等于文件系统隔离 |
| 执行策略 | 关闭子模型的再次dispatch、plan、goal mode及追加迭代等顶层策略；默认子loop也关闭forge | 宿主代码内部仍可同步调用搜索子loop，不能把模型动作禁用理解为绝无嵌套 |
| 审批策略 | buildSubAgentInvoker沿用已有WithAgreeAuto覆盖 | 并非原样继承父审批策略；能力授权边界和是否请求审批是两个维度 |
| 生命周期context | 子取消信号派生父task | 这里的context是Go取消机制，与模型的“上下文文本”不同 |

**Clean仅表示空Timeline。** 内部fast_context等调用可选择Clean，但构造子配置时仍会Fork会话提示状态。因此它不等价于DeepSeek的全新对话，也不能称为“无父上下文”。

**不MergeBack。** 子运行轨迹不合并到父Timeline。后台manager保存结果，父线程在prompt组装边界投递并落历史；共享漏洞清单等既有旁路另按其明确语义工作。

主要代码：

- `loopinfra/dispatch_sub_react_agents.go`：普通派发固定Fork。
- `subagent_manager.go: SubmitSubAgents`：派发时准备，后台排队。
- `subagent_timeline.go: buildTimelineHandle / snapshotSubAgentConfig / buildSubAgentInvoker`：Timeline和配置边界。
- `aicommon/timeline_fork.go: ForkForTask`、`timeline_marshal.go`：序列化复制、压缩历史和引用。
- `aicommon/session_prompt_state.go: ForkForSubAgent`：复制、丢弃TODO、共享漏洞清单。
- `aicommon/contextprovider.go: snapshotForChild`：固定任务provider集合，保留普通provider共享。
- `prompt.go`、`subagent_manager.go: prepareSubAgentPrompt / commitSubAgentPrompt`：结果进入模型输入。

## 3. 四个Agent Harness如何处理

| 框架及锁定版本 | 子上下文 | 父子退出与结果交付 |
| --- | --- | --- |
| Codex `ffae979…` | V1的fork_context默认false；V2的fork_turns默认all，可none或最近N轮。fork会处理继承指令、通信消息和压缩历史，而非直接共享父历史 | 派发与等待分开。V2完成消息入父mailbox，在安全边界进入模型；完成通知trigger_turn=false，不能据此认为父final之后必然自动续跑。未见通用“所有孩子结束才准final”门禁 |
| DeepSeek Harness `0d1f500…` | spawn新对话；fork只复制到最近一次turn/end，当前尚未结束的整轮不包含。任务书必须明确本轮的新约束 | continuable有独立会话、收件箱及后续轮次；结束一轮与销毁会话不同。具体等待/后台语义取决于one-shot、continuable及Job路径 |
| PI示例 `60e7e76…` | 新CLI进程，传角色提示、task、工具/模型配置与cwd，不复制父聊天。chain显式传前一步输出 | 示例工具本身await子进程/整批，不是现成后台调度器；onUpdate进度不等价于父下一轮模型输入 |
| OpenCode `e03db9b…` | 新子session，传委派prompt及文件引用，不复制父messages；传已有task_id可以继续子自身历史 | background回执即时返回，结果作为synthetic消息交给父；支持父一轮结束后再处理通知。运行中追加任务串行排队，不是立刻打断子模型 |

固定源码：

- [Codex spawn与fork选项](https://github.com/openai/codex/blob/ffae979216bfbe94070bd21868d1695277105a63/codex-rs/core/src/tools/handlers/multi_agents_v2/spawn.rs#L273)、[fork历史处理](https://github.com/openai/codex/blob/ffae979216bfbe94070bd21868d1695277105a63/codex-rs/core/src/agent/control/spawn.rs#L884)、[完成通知](https://github.com/openai/codex/blob/ffae979216bfbe94070bd21868d1695277105a63/codex-rs/core/src/session/mod.rs#L2368)。
- [DeepSeek已完成轮次前缀](https://github.com/deepseek-ai/deepseek-harness/blob/0d1f50007f9bca3f52b06e1c3074fa14d5fb0720/packages/subagent/subagent-fork-in-process/src/index.ts#L48)、[子配置组合](https://github.com/deepseek-ai/deepseek-harness/blob/0d1f50007f9bca3f52b06e1c3074fa14d5fb0720/packages/subagent/subagent/src/child-agent.ts#L139)。
- [PI子进程参数和执行](https://github.com/earendil-works/pi/blob/60e7e76bd7ea25cad1dd6f3f1ce0d18814a42759/packages/coding-agent/examples/extensions/subagent/index.ts#L300)。
- [OpenCode子session及prompt](https://github.com/anomalyco/opencode/blob/e03db9bc6908f75c9334d8aa997deeaac81c0298/packages/opencode/src/tool/task.ts#L156)、[结果注入](https://github.com/anomalyco/opencode/blob/e03db9bc6908f75c9334d8aa997deeaac81c0298/packages/opencode/src/tool/task.ts#L233)。

这些实现的共同点是分别设计历史来源、执行生命周期和结果消息。没有“主流框架一律继承/一律不继承”的统一答案。

## 4. Yaklang退出不是一个布尔值

| 触发 | 原有行为 | 后台子任务必须满足的要求 |
| --- | --- | --- |
| 普通directly_answer | 发送答案，通常Continue | 可以阶段交付；不能把答案事件当作父task已结束 |
| classifier确认的simple_query | 无有效TODO变化/历史时可自动结束 | 仍要检查活跃子任务及未投递结果 |
| finish | 检查goal mode与当前task活动TODO，允许才Exit | 增加子任务结束及结果版本检查 |
| 普通成功Exit、post-iteration要求结束 | 结束当前loop | 中央门禁检查父子状态 |
| 初始化Done | 初始化/路由已经处理，提前返回 | 当前内置路径未派发后台任务；defer仍负责清理。将来init主动派发后Done不应当作可继续的等待态 |
| blueprint/plan/focus/loop转换 | 可能调用另一loop，部分路径复用同一个task | 必须在调用之前检查；等内层完成后再拦Exit已经来不及 |
| 用户明确要求直接回答并停止 | 用户控制优先 | 取消子工作、保留用户退出语义，不能被普通finish门禁改回Continue |
| 取消/skip | context取消、异步回调确认停止 | 回调可能把task设Skipped；通用收尾不能提前设不可改的Aborted |
| 达到迭代上限 | 软中断，待办deferred并生成阶段总结 | 清理子任务，说明未完成内容；Completed不等于业务验收成功 |
| error/panic | 应返回错误并Aborted | 清理子任务；不能把panic作为成功交付 |
| Release/外层结束 | 释放资源、关闭子任务owner | 有界清理；不响应取消的worker仍需标记cleanup_pending |

根task进入Completed/Aborted/Skipped会取消自身context，继而取消派生的子task。所以当前Yaklang采用“父task保持活跃直到正常收齐结果”的策略。若要像某些框架那样先结束父轮次、以后被通知唤醒，需要额外设计可恢复等待状态及调度入口，不能仅删除finish检查。

## 5. 本轮发现与修复

1. **空漏洞清单fork后失去共享。** 原来复制nil指针，子首次报告时创建私有store。已改为在fork时初始化共享store，并复现/补测子首次报告、父与兄弟可见及去重。
2. **批量工具审批的用户退出遗漏。** 批量分支普通Exit会被活跃子任务拦回Continue。改为与单工具一致的显式用户退出。
3. **同task内层loop交接遗漏。** LoopFactory转换动作可在内层结束时先取消共享父task。新增进入内层之前的检查。
4. **panic与完成defer顺序错误。** 旧顺序可能先Completed，再recover且返回nil。统一recover和最终状态判断，异常保持错误返回。
5. **取消确认状态被覆盖。** 后台worker先设Aborted会阻止取消回调设Skipped。改为先运行取消回调，再补默认终态，并保留回调异常保护。

## 6. 验证范围与剩余边界

新增/扩充受控测试验证：排队启动仍使用派发快照；子历史不进入父/兄弟；父TODO不继承且不被子修改；Clean仍继承会话上下文；首次漏洞报告共享；用户批量退出；内层loop交接；panic错误归类；取消回调终态。

上下文定向`-race`通过：Fork/排队快照、Clean上下文、任务provider快照、资源边界、子配置策略、共享漏洞清单。退出定向`-race`通过：批量用户退出、同task交接、panic归类、取消回调，以及后台派发/观察链路。

扩大到旧TimelineFork的MergeBack测试时，`-race`另发现`timeline_fork.go`修改条目与异步UI事件读取条目的竞态。后台SubAgent路径明确不调用MergeBack，该问题独立记录，不能把更大范围测试描述成全部通过。

复核阶段保留普通派发默认Fork。后续新增的`context_mode`已分别定义Timeline、用户历史、证据、附件、共享去重和资源权限的继承规则；它不等于旧Clean开关，也不增加持久唤醒协议。

结果进入模型输入仅证明送达，不证明模型理解、结论正确或任务书验收通过。本轮未运行外部仓库测试、真实模型或客户端渲染测试。
