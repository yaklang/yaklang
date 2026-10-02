# 待解决：两个 ReAct 循环之外的辅助模型请求

目标：逐项评估辅助请求的必要性、触发时机、输入范围和预算，最后形成统一机制。主循环只有 `coordinator` 与 `pe_task`；单次辅助调用虽然不是第三种 ReAct 循环，仍会消耗模型请求、token、时间和后台资源。

## 实测基线：7 类辅助请求，165 次响应

2026-10-02，Yak + aim、`planEngine("coordinator")`，运行 source → consistency → decision 三个依赖任务，逐项审核并写报告。Quality/Original 使用 DeepSeek V4.1 Flash，Speed 使用 Memfit Light。三个任务完成；同时出现辅助协议失败和旧验证越过关闭开关的问题。

以下是**修复前**该轮 provider usage 的统计，包含重试和冷请求；输入包含缓存命中部分，不是实际计费金额。按请求关联 ID 连接 usage 与 prompt dump，summary 缺失时根据原始请求用途补分类，不能只数 UI 事件。

| 辅助用途 / caller | 响应次数 | 输入 token | 缓存命中 token | 输出 token | 本轮实际模型 |
| --- | ---: | ---: | ---: | ---: | --- |
| 长期记忆筛选 `memory-triage` | 86 | 447,989 | 162,775 | 2,541 | Memfit Light |
| 价值反馈评分 `ai_value_feedback` | 46 | 344,172 | 127,872 | 4,565 | Memfit Light |
| 工具调用说明 `tool-call-reason` | 16 | 67,274 | 0 | 580 | Memfit Light |
| 历史压缩 `timeline-compress`，输出函数 `timeline-summary` | 10 | 526,241 | 414,720 | 2,941 | Memfit Light |
| 记忆检索标签 `tag-selection` | 4 | 17,727 | 0 | 199 | Memfit Light |
| 会话标题 `session-title-generator` | 1 | 3,020 | 0 | 28 | Memfit Light |
| 遗留自动验证 `verification` | 2 | 100,915 | 48,256 | 815 | DeepSeek |
| **辅助合计** | **165** | **1,507,338** | **753,623** | **11,669** | |

同轮 coordinator 20 次、pe_task 19 次，共 39 次主循环响应；全部模型共 204 次有 usage 的响应。辅助请求占响应次数约 80.9%，输入 token 约 45.0%。主循环 39 次不能与辅助 165 次混为“任务执行了 204 步”。取消后没有 usage 的请求不计入表内。

更早一次冒烟把 Original、Quality、Speed 都覆盖成 DeepSeek，使正常轻量任务也使用主模型。那是脚本配置错误，不是 coordinator → worker 的分层配置丢失。上述基线已经分开模型。

## 每类请求做什么、从哪里来

| 用途 | 入口及触发 | 结果去向 / 当前问题 |
| --- | --- | --- |
| `memory-triage` | [主循环迭代后](../aireact/re-act_mainloop.go)、[worker 迭代后](../coordinator_invoker.go) → [MemoryFlushBuffer](../aicommon/memory_flush_buffer.go) → [HandleMemory](../aimem/aimemory_handle_memory.go) → [AddRawText](../aimem/aimemory_build_memory.go) | 提取可跨后续交互复用的记忆，生成 content、tags、potential_questions 和七维 C.O.R.E. P.A.C.T. 评分，再过滤、去重、入库；不是 session `save_evidence` 的必要前置步骤。 |
| `ai_value_feedback` | 迭代结束、审批、风险反馈、verification 等 → [ValueFeedback](../aicommon/value_feedback.go) → [aive](../aive/value_feedback.go) | 对过程记录生成弱标签/评分并发事件；不负责推进任务。独立队列固定选择轻量回调，未统一继承 session Speed 配置；仍直接构造旧 JSON LiteForge。调用次数不能简单理解为业务事件数。 |
| `tool-call-reason` | [工具调用卡片](../aicommon/toolcall.go) `generateReasonIfNeeded`，经 ScheduleAuxiliaryTask 调用 | 生成短说明供界面展示。已有立即显示的 fallback；应评估复用主 function call 的 intent/description，避免再请求模型。 |
| `timeline-compress` | [组装 prompt 前检查](../aicommon/timeline_compression_before_prompt.go) → [压缩摘要](../aicommon/timeline_compression_summary.go) | 读取待压缩历史，提交新摘要；成功后才替换历史，失败保留原文。不同于低优先级装饰请求，必须保证来源完整、状态原子更新。 |
| `tag-selection` | [SelectTags](../aimem/aimemory_tag_selection.go)，记忆搜索时分析输入和已有标签 | 筛选记忆检索领域标签；需评估复用 triage 标签、缓存、非模型检索。 |
| `session-title-generator` | [ReAct 会话标题生成](../aireact/re-act_mainloop.go)；无标题时后台调用 | 会话列表标题与持久化；不影响计划正确性，应评估延后、复用首轮输出或规则生成。 |
| `verification` | [工具后自动检查及 watchdog](../aireact/reactloops/verification_gate.go) → [VerifyUserSatisfaction](../aireact/verification.go) | 旧 JSON 任务满足度判断，调用主模型。新引擎应由 coordinator 的 `review_task` 验收，不应在后台重复判定。 |

`save_evidence`、`review_task`、`write_report` 等属于两个主循环已经选出的 actions，其本地执行不应再被统计成额外模型请求。模型同一次响应的 reasoning 流也不是新请求。

## memory triage 为什么会放大开销

1. 它面向长期记忆，会从 Timeline 增量判断“什么值得以后记住”，并不是直接保存已经明确选出的 evidence。每轮筛选即使返回空数组，也消耗一次请求。
2. coordinator 和 worker 都有 flush 入口。默认阈值是 **6 个有增量的迭代或 4096 字节**，另外任务完成、结束迭代和 async milestone 也能触发。因此并不保证“每 6 步才调用一次”；工具结果稍大即可提前触发。此轮 21 次首轮输入中，**17 次为 `batch_byte_limit`，4 次为 `task_done`**，主要触发源确实是字节门槛。
3. 此轮 86 次响应中，原始请求 dump 可识别 **21 次首轮、65 次重试**。记录到 80 次 native 协议校验失败，14 个筛选流程耗尽五次尝试；其余失败可能属于尚在重试或关闭时被取消的流程，不能都当成成功保存记忆。
4. 平均每次约 5,209 个输入 token。待筛选输入已限制为 4096 token，但还有规则、评分说明、标签上下文和输出定义；这是输入截断上限，不是整个请求上限。重复发送相同规则和材料仍占请求、时间及未缓存 token。
5. 筛选后的去重、embedding、检索是另一层成本。本轮没有观察到 `batch-memory-deduplication` 模型请求，不能据此认为所有环境都不会产生。

## 本轮先修复的明确问题

- [x] 自动验证的关闭开关覆盖工具后检查、token 门、watchdog 创建/重置/触发；退出时始终清理 timer。保留旧模式显式调用 verification 的能力。
- [x] 去掉 memory triage 提示词中的旧 `@action` 输出示例。用同一份真实输入验证：原请求返回普通 JSON + `stop`，去掉旧示例后返回原生 function call + `tool_calls`。
- [x] 压缩提示词移除“禁止调用工具 / 只输出 JSON”与新协议的冲突，业务提示只说明摘要字段含义，由调用层选择输出协议。
- [x] native 校验错误记录是否收到调用、是否存在多个调用、函数名及 finish reason，避免所有异常都只有同一条笼统错误；不记录参数正文、不接受 JSON 降级。
- [x] 单次辅助输出只有一个函数，`tool_choice` 明确指定该函数，避免 provider 对泛化 `required` 的执行差异。严格校验单次调用、函数名、终止原因和参数，不做普通 JSON 回退。
- [x] 原生输出协议放入实际投影后的 system 消息，避免被 helper 自身的 high-static 指令排到普通 user 文本中；回归测试检查 provider 消息角色。
- [x] 工具说明的提示词、参数描述和示例统一到 Schema 的 30 字符限制。旧提示要求 15 个词，实跑因 33/37 字符的说明产生无效重试。

### 验证记录与边界

同日两轮重新编译 Yak，通过 aim 使用相同三个依赖任务与分层模型配置。三个任务均 completed，报告正确给出 DO NOT SHIP 及三处样本差异。自动验证回归、现有 coordinator/Yak 冒烟、记忆和 Timeline 压缩相关测试通过。

| 辅助请求 | 修复前基线 | 修正业务提示词 | 再指定输出函数 |
| --- | ---: | ---: | ---: |
| `memory-triage` | 86 | 21 | 27 |
| `ai_value_feedback` | 46 | 34 | 44 |
| `tool-call-reason` | 16 | 0 | 21 |
| `timeline-compress` | 10 | 1 | 2 |
| `tag-selection` | 4 | 4 | 1 |
| `session-title-generator` | 1 | 1 | 1 |
| `verification` | 2 | 0 | 0 |
| **辅助合计** | **165** | **61** | **96** |
| 两个主循环 | 39 | 32 | 40 |
| 全部有 usage 的响应 | 204 | 93 | 136 |

第一轮仅修正业务提示词，memory triage 仍有 4 次 native 校验失败，21 次响应包含 18 次首轮和 3 次重试，另一次失败在关闭时未完成重试。第二轮明确指定函数后，仍记录到 memory triage 3 次、工具说明 2 次普通文本响应，以及工具说明 4 次长度校验失败。不能把任一整轮标为零失败。两轮自动 verification 均为 0，Timeline 压缩分别 1、2 次成功，后续主循环均读到了 compressed head。

据此补上最后两处修改：system 消息中的原生输出约束，以及工具说明的 30 字符一致性。**最终补丁完成后做了三次真实轻量模型定向回放**：两份曾失败的 memory triage 输入、一份工具说明输入，均单次返回原生调用、通过参数校验、finish reason 为 `tool_calls`。这三次是通过真实 provider 和辅助调用层的独立回放，不是又一轮完整 Yak 任务，也不是零失败率保证；memory 回放采用最小 memory_entities 参数定义，完整评分字段仍由既有测试和整轮路径覆盖。

这不是严格性能 A/B：模型选择的动作、批次不同，观测脚本也改为结束时保存 events，避免每个事件写文件阻塞消费。`tool-call-reason=0` 不能解释为删除了该能力，下一轮又出现了 21 次。两轮分别约 129、171 秒（基线约 288 秒）；两个主循环缓存命中率约 87.57%、87.02%，仅作为观测，不把全部差异归因于修复。第一轮主循环的工具参数校验重试、关闭时取消且无 usage 的请求，均不混入辅助响应统计。

最后一轮完整任务仍有 96 次辅助响应，多于两个循环的 40 次；其中价值反馈 44 次、记忆筛选 27 次。修复明确的协议/约束错误不等于解决了辅助请求是否值得执行、如何统一安排的问题。

## 后续逐项决定，暂不预设统一实现

- [ ] 明确每类请求是否必要：删除、复用当前主循环输出、程序规则、按需调用、后台批处理，分别选最合适方式。
- [ ] 区分 session evidence 与长期记忆：哪些内容需要自动 triage、谁拥有持久化职责，避免协调员与 worker 对共享内容重复筛选。
- [ ] 统一统计口径：logical operation、provider attempt、重试、协议错误、成功应用、取消、缓存和实际模型，关联 session/task/loop；不能只靠可能丢失的消费事件数。
- [ ] 统一模型与输出协议边界：辅助任务默认使用配置的 Speed；梳理 AIVE 的独立 callback 与旧 JSON 入口。新 coordinator 路径保持 native function call，不靠升级主模型或接受普通 JSON 掩盖问题。
- [ ] 按任务价值分别规定频率、输入上限、批处理、去重/缓存、并发、截止时间和重试预算。不能让标题与 Timeline 压缩共享同一失败策略。
- [ ] 对确定性的协议错误避免反复原样请求；可选后台任务失败能否停止本次操作、需要如何告知与观测，单独讨论。
- [ ] 收紧关闭语义：任务完成后仍在排队/执行的辅助请求是否取消、是否需要等待持久化完成，区分关键状态与可丢弃反馈。
- [ ] 将以下**本轮未触发**的入口补入后续场景审计：`batch-memory-deduplication`、附件提取/观察、AI 风险审批 `review_risk`、知识检索/压缩等。其中 [NativeRiskReview](native_review.go) 当前显式使用 Quality；此轮采用 YOLO，不能用上述表格证明风险审批也走 Speed。注册表中的能力不等于本轮实际发生，另按场景记录。
- [ ] 固定同一输入、模型档位、provider 和观测方式复测；分别衡量任务正确性、请求数量、未缓存 token、延迟、辅助有效产出。单独的缓存命中率升高不能证明系统成本下降。

统一调度方案待后续 review；本轮不批量删除辅助能力，不改变 Yakit 事件契约。
