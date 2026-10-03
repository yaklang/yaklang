# Coordinator 第一阶段重构任务书

> 状态：待实现的设计与验收要求，不是当前功能说明。
>
> 本文只定义第一阶段 PLAN。EXEC 仅作为用户批准后的移交状态，不在本次任务中重新设计执行阶段。实施前先检查工作区已有修改，保留他人的工作；实现完成后留在本地供 review，不擅自提交、push 或重启服务。

> 后续补充：[第二阶段 EXEC 任务书](README.exec-phase.md) 允许协调员在 EXEC 内根据执行事实调整当前任务和计划，无需再次计划审批；这不等于回退 PLAN。第一阶段的修改和审核规则仍按本文执行。

## 1. 目标与范围

第一阶段的协调员负责：调查用户目标、收集会话 Evidence、编写 Document、构建 Tasks 与 DAG、反复修改计划，最后提交用户审核。

重构后的计划动作只有三个：

1. `create_plan`：首次创建完整计划。
2. `modify_plan`：一个动作原子修改文档、任务定义，支持覆盖与局部 patch。
3. `submit_plan`：提交当前计划供用户审核。

复用现有 coordinator、主循环协议、Timeline、session Evidence、计划解析器、子 Agent 管理器和 Yakit 审核事件。不要新增规划 ReAct 循环、观察请求/响应体系、另一套 DAG 调度器或计划版本系统。

## 2. 内部状态与第一阶段边界

内部保存明确的阶段字段：`Phase = PLAN | EXEC`。

- 新协调实例进入 `PLAN`。
- PLAN 中可以调查、创建及修改同一份计划。
- `submit_plan` 后等待审核，仍属于 PLAN；提交不等于批准。
- 等待审核期间锁定提交内容，禁止创建、修改或重复发起一个新的审核。
- 用户要求调整时，结束本次审核等待并恢复 PLAN 内的编辑能力。
- 用户批准最终内容后，原子保存该内容并切换为 EXEC，触发自动 DAG 调度；具体执行职责见第二阶段任务书。
- 本文不增加 EXEC 回到 PLAN 的入口。全部中断并确认子任务退出后，新的协调实例可以重新进入 PLAN，并继承同一 session Timeline 记忆。

```mermaid
flowchart LR
    A[PLAN：调查与探索] --> B[创建 Document 和 Tasks DAG]
    B --> C[modify_plan：反复修改]
    C --> D[submit_plan：锁定并等待用户审核]
    D -->|要求调整| C
    D -->|批准最终内容| E[采用计划并移交 EXEC]
```

等待审核是第一阶段的交互状态，不再增加另一套 Draft/Published/Submitted 业务状态机。可以复用现有审核端点及其等待状态；界面里的“草案”“等待审核”“已批准”等描述由阶段和审核情况派生。

第一阶段不得展示或执行第二阶段专属的业务任务查询、验收、重试、取消和报告动作，例如 `inspect_task`、`review_task`、`retry_task`、`cancel_tasks`、`create_report/modify_report/submit_report`。旧的模型 `start_tasks/wait_tasks/write_report` 流程由第二阶段自动调度、消息等待和报告动作替代，不在第一阶段开放。探索子 Agent 使用其自己的 job 管理，不借用这些计划任务动作。

权限既要作用于文本流 schema 和 function tools 的声明，也要在实际 handler/Controller 入口校验，不能只写提示词。

## 3. 一份 Plan，移除计划版本系统

只维护一份当前 Plan：

- `Document`：当前计划正文。
- 任务定义树：嵌套任务、任务组、稳定标识和依赖。
- 从定义树解析得到的叶任务 DAG：派生执行视图，不允许独立编辑。

删除 `DraftVersion`、`ApprovedVersion`、`SubmittedVersion`、`Attempt.PlanVersion` 和模型参数 `plan_version`。删除新旧获批版本并行和重新审批驱动的重规划通道。原有 Draft/Approved 双份计划合并为当前 Plan；EXEC 内不依赖版本的任务调整由第二阶段任务书定义。

保留真正必要的身份与控制信息：协调实例 ID、任务 ID、attempt ID、用户输入变更控制、任务状态、审核端点和持久化格式标识。不要把这些与计划版本混为一谈，也不要新造 edit revision/etag 来替代已删除的版本系统。

去掉版本校验后的并发规则：

- 只有协调员可以修改计划；探索子 Agent 只交付调查结果及 Evidence。
- 计划编辑串行处理，对当前 Plan 的拷贝计算完整候选结果，验证后原子提交。
- 审核期间禁止编辑。
- 审核回复使用现有 `interactive_id`/checkpoint 关联本次审核。旧卡回复不能批准另一份计划，重复确认不能重复移交。
- 文档 patch 必须严格匹配当前原文；失配直接报错，不猜测修补。

由于删除字段涉及已有调用和快照，必须同步清理引用、明确处理持久化兼容。必要的旧格式读取集中在适配边界；新运行态和新模型 schema 不保留无意义的版本字段。不允许为了通过编译给旧判断填一个固定假版本。

## 4. 三个计划动作

### 4.1 create_plan

首次创建完整计划，包含 Document 和任务定义。沿用现有入口参数 `plan`、`plan_document` 及现有任务树解析契约，不要求本次顺手更名所有入口。

- 仅允许 PLAN 阶段、没有当前 Plan、没有正在进行的审核。
- 文档必须非空，任务树必须能通过现有 DAG 校验。
- 创建不代表批准，也不派发任何业务任务。
- 已有预设/mocker 输入时，直接初始化同一份 Plan，不再次调用模型生成。
- 创建后立即发布当前 Document 和任务定义，供下一轮读取。

### 4.2 modify_plan

本文规定 PLAN 阶段已有 Plan、当前没有等待中的审核时允许修改。第二阶段复用同一动作及事务，增加运行任务保护，不回退第一阶段审核。修改动作只保留以下四个可选业务参数：

| 参数 | 类型 | 含义 |
| --- | --- | --- |
| `document` | string | 完整覆盖当前 Document |
| `document_patch` | string | 对当前 Document 应用 unified diff |
| `tasks` | object | 完整覆盖任务定义；对象沿用 create_plan 的完整嵌套计划树格式 |
| `tasks_patch` | array | 按稳定节点 ID，对任务定义批量执行增删改 |

这里的 `tasks` 是完整定义对象，可以包含 `main_task/main_task_goal/tasks`；不是另造一套扁平任务格式，也不是直接覆盖派生执行 DAG。

参数规则：

1. 至少提供一个参数；空对象调用拒绝。
2. `document` 与 `document_patch` 互斥。
3. `tasks` 与 `tasks_patch` 互斥。
4. 可同时修改文档和任务树，例如 `document_patch + tasks_patch`。
5. 未提供的部分完全保留。
6. 明确区分缺省与 null；null、错误类型、空 patch/空操作批次拒绝，不自动强制转换为有效输入。
7. 文档和任务修改属于一个事务；任一部分解析、apply、DAG 校验或提交失败，整次修改不生效。
8. 有效但没有造成内容变化的请求返回 `unchanged`，不重复记录大正文或触发无意义更新。

不再提供 `modify_type`，不创建 `modify_plan_document` 或 `modify_plan_definitions` 动作。参数本身已经清楚表达覆盖还是 patch。

#### 文档 patch

AI 生成标准 unified diff，作为 `document_patch` 参数；handler 将其保存为可 review 的 patch artifact，然后直接通过代码对当前文档尝试 apply。

- patch 仅针对计划文档，不能修改其他文件；多文件 patch、错误目标路径拒绝。
- 使用现有可复用的 patch 实现（如已有）；没有时实现边界清楚的严格 apply，不引入 shell 执行。
- 不启用 fuzz，不猜测上下文，不失配后自动改成覆盖。
- 候选新正文仍需满足非空文档要求。
- 失败时可以保留诊断 patch artifact，但当前 Plan、当前视图和成功记录不能改变。
- 成功记录实际生成的新正文，不让模型凭 patch 自行推断最终内容。

示例：

```json
{
  "document_patch": "--- plan_document.md\n+++ plan_document.md\n@@ -1,2 +1,3 @@\n # 检查计划\n 先核对来源。\n+记录来源路径与验证依据。\n"
}
```

#### 任务树 patch

`tasks_patch` 是有顺序的操作批次，仅支持 `add/delete/update`。读取当前定义直接使用 PLAN DEFINITION，不复活 inspect action。

| operator | 参数 | 行为 |
| --- | --- | --- |
| `add` | 可选 `parent_task_id`、必需 `task` | 向任务组添加节点；省略父 ID 时添加到根任务组 |
| `delete` | `task_id` | 删除节点；删除任务组时同时删除其子树 |
| `update` | `task_id`、非空 `changes` | 修改节点允许编辑的定义字段，未提供字段保留 |

要求：

- 修改目标使用当前 PLAN DEFINITION 中的稳定 task ID，不使用显示顺序 index 作为身份。
- 新节点沿用现有解析器分配 ID，未变化的节点保留原 ID。
- `task` 和 `changes` 的任务名、任务书、语义标识、子任务及依赖字段沿用现有树契约，中文 schema 明确列出可修改字段。
- 不允许通过 changes 覆盖 task ID、运行状态、attempt、结果、工具统计或其他执行字段。
- 依赖仍沿用现有语义标识解析规则；操作目标 ID 与依赖引用不要混淆。
- 批次按顺序作用于临时树，最后一次性校验最终结构；不得因中间状态短暂缺少某个引用就提交一半结果。
- 删除被依赖节点必须在同批中修正引用，否则整批拒绝；不得悄悄删除依赖边。
- 修改语义标识时也必须处理引用，禁止悬空依赖。
- 保留现有任务组语义：父节点用于组织，只有叶节点执行；依赖任务组展开为该组叶任务，组前置条件作用于内部入口叶任务。
- 校验非空任务树、唯一语义标识、存在的引用、自依赖、环和最终叶任务 DAG。

示例（其中 ID 从实际上下文获取）：

```json
{
  "document": "# 检查计划\n先检查环境，再核对来源并保存证据。",
  "tasks_patch": [
    {
      "operator": "update",
      "task_id": "已有来源任务的实际ID",
      "changes": {"depends_on": ["environment_check"]}
    }
  ]
}
```

成功回执只包含 `updated/unchanged`、修改的组件、必要节点 ID 和 patch artifact 引用；不复制完整正文、任务树或虚构版本号。失败回执指出具体错误，允许协调员修正后重试。

### 4.3 submit_plan

提交当前 Plan，不携带 `plan_version`。

提交条件：

- 当前处于 PLAN，已有非空 Document 和有效任务定义/DAG。
- 没有正在执行的计划修改或另一份待处理审核。
- 所有探索子 Agent 已完成，或者已取消且确认实际退出；不能只发送取消就视为探索结束。
- 探索结果及需要保留的 Evidence 已完成交接，不能把未完成的探索拖过审核边界。

提交时保存审核所需的当前完整计划，通过现有 Yakit/Memfit 事件展示。用户编辑后确认，以最终审核内容重新校验并保存；用户提出修改要求，留在 PLAN 继续完善。批准才产生阶段移交，不在 submit handler 中提前执行任务。

保持 `plan_review_require`、`detached_plan_require`、相应确认入口、选择器和计划树显示格式兼容，不要求修改前端。

## 5. 探索能力及配置

新增：

```go
// PLAN 阶段是否允许协调员派发探索子 Agent，默认关闭。
EnableSubagentsInPlan bool
```

配套 `aicommon.WithEnableSubagentsInPlan(bool)`。开关必须从实际入口正确传到 coordinator，包括配置复制及 detached/预设入口；不能只在一个构造函数生效。

- 默认 false 时，不声明探索派发/管理动作，实际入口也拒绝绕过调用。
- 开启时，由 coordinator 复用既有 dispatch、取消、结果管理及生命周期设施；显式等待可以保留，但不是取得进展的必需动作。
- 协调员无独立工作时，沿用自动挂起、发现或状态变化通知、下一轮检查机制，不轮询主模型。
- 不把普通流式输出、工具计数或重复 Evidence 当作新发现反复唤醒。
- 调查子 Agent 不能修改当前 Plan，也不能递归开启新的协调员/探索派发链。
- 对探索角色施加调查工具权限；不能因为增加开关就自动获得业务写入、任意命令或任意专注模式权限。
- 当前通用 `EnableDispatchSubReactAgents` 在 `ConvertConfigToOptions` 中被刻意排除继承。实现时保留这个通用边界，按角色连接新开关，不能为了让 PLAN 工作而让所有子配置自动继承派发权限。

适合交给子 Agent 的探索问题应满足：范围明确、能独立调查、输入与边界清楚、有明确证据来源和交付标准。避免多个探索任务重复调查同一问题。探索输出是发现、证据引用和结论，不是提前执行正式计划。

复用 session Evidence，不新增 plan facts 或另一份观察存储。父协调员获取可用结果及明确共享的发现，不把 generic 子 Agent 的全部私有 Timeline 强行 MergeBack。

## 6. Prompt 与上下文

High Static 保持纯静态，不写阶段条件、版本或过度解释提升机制。中文角色指令通过现有 promptloader 加载，放在 SemiDynamic2。

PLAN 指令明确：

- 调查用户目标和约束；注意 Timeline 中用户输入及补充。
- 保存已确认且可复用的 Evidence。
- 创建完整 Document 和 Tasks DAG。
- 修改使用唯一 modify_plan 的四个参数，遵守事务与审批锁定。
- 只在探索开关打开时派发调查子 Agent。
- 资料不足时澄清，准备完成后 submit_plan，不提前实施业务任务。

上下文职责：

| 区域 | 内容 |
| --- | --- |
| SemiDynamic1 / PLAN DOCUMENT | 当前完整文档，独立展示 |
| SemiDynamic1 / PLAN DEFINITION | 当前嵌套任务定义与 DAG，不包含运行状态 |
| Session Evidence | 确认的调查发现、来源及后续决策依据 |
| Timeline Open | 本轮修改记录、patch 引用、反馈、用户信息及探索事件 |
| PLAN STATUS | 当前阶段、是否已有计划、审核情况和必要探索状态 |
| SemiDynamic2 | 稳定的中文阶段角色与调用协议指令 |

成功修改后，下一次请求必须看到实际生成的完整新文档和任务定义；不能仅提供 patch。失败时继续展示原计划。当前视图只展示一份有效内容，旧文档和旧树如需保留，明确作为历史，不叠加成多份“当前计划”。

Timeline 历史按现有冻结/压缩/提升路径保存。完整正文不同时复制到 action feedback、PLAN STATUS、动作回执和 Evidence 多处。事务成功记录与当前内容发布要一致，失败不得伪装成功。

普通工具调用、等待和状态通知不得重新追加未变化的计划正文，不强制 Freeze。阶段和开关固定时，工具定义保持字节稳定，不能因每轮反馈变化重新生成 schema。

## 7. 双协议与前端契约

- 沿用主循环 `EnableFunctionCallMode`，不强制某一种协议，不回退到旧循环。
- 文本流 JSON action 与 native function call 各自生成声明，共用中文参数契约、验证器和 handler。
- 四个修改参数的互斥、存在性、类型与任务操作校验必须在真实执行入口再次验证，不能仅依赖模型遵守 schema。
- 保持原生 tool-call 的调用/回执配对；文本流按主循环 nonce/AITAG 约定处理，不混用输出协议。
- Yakit 的显示树中仍可由适配器输出前端必要字段，内部无状态定义不混入这些展示状态。
- 保留预设和 mocker 对原嵌套计划格式的支持，不要求调用方改用新任务格式。

## 8. 实施文件与清理范围

优先检查并修改：

| 位置 | 工作 |
| --- | --- |
| `controller.go` | 单一 Plan、阶段、编辑事务和审核门禁；移除计划版本 |
| `action_create_plan.go` / `action_modify_plan.go` / `action_submit_plan.go` | 三个动作各自职责；modify 四参数合并实现 |
| `actions.go` / `plan_schema.go` | 中文双协议声明、存在性与互斥校验、复用任务树定义 |
| `plan_wire.go` | 复用稳定标识、嵌套计划和 DAG 校验，不重写整套解析器 |
| `loop.go` / promptloader 中 coordinator 指令 | 阶段权限、探索开关、第一阶段指令 |
| `plan_context.go` / `session_events.go` | 独立 Document/Definition、移除版本展示、保留前端契约 |
| `session.go` / `preset_plan.go` | 配置传递、审核交接、预设/mocker 初始化 |
| `aicommon.Config` 及配置 options | 新增默认关闭的 EnableSubagentsInPlan，正确传递与角色隔离 |

文件清理要求：

- 删除分拆的 modify_plan_document/modify_plan_definitions 动作（如工作区存在），不能留下两套实现。
- 删除 modify_type、plan_version 及所有内部版本分支、重复 Draft/Approved 存储。
- 删除依赖计划版本、重新批准和切换获批备份的专属重规划逻辑及测试；保留无版本 DAG 调整所需的受影响关系与运行保护，具体按第二阶段任务书实现。
- 不新增 inspect_plan/inspect_tasks、Seen、prompt 内容反查或另一套观察回调。
- 保留现有 task/attempt 身份、自动等待、取消退出保护、共享 Evidence、审核和持久化契约；与版本删除相关的执行引用只做必要适配。
- 旧入口确需兼容时集中在边界，不把冗余字段重新带回模型动作或内部状态。
- 每个 action 的实现和测试保持独立文件；patch helper 只负责本动作需要的 apply/验证，不建立过度通用的框架。
- 更新 README 和样本，明确新规则；保留有效已有测试，修改/删除确实被新规则替代的旧测试。

## 9. Yak/aim 第一阶段冒烟

新增一个 `smoke/planning_phase.yak`，由 Go harness 注入确定性模型、事件 recorder、临时工作区及配置。实际执行 Yak + aim + coordinator + 工具 + 探索 Agent + 审核链路，仅替换模型决策，避免外部模型随机性掩盖实现问题。

主要矩阵：function call/text stream × EnableSubagentsInPlan 开/关。另覆盖 preset 和 mocker 来源。

### A. 调查生成路径

1. 从本地 source fixture 调用真实读取/搜索工具。
2. 开关打开时派发一个范围明确的探索任务；通过屏障验证新发现和完成通知、协调员自动挂起，以及挂起期间没有额外主模型请求。
3. 开关关闭时验证无探索动作声明，绕过调用也被拒绝。
4. 保存真实共享 Evidence，创建包含任务组、两个有依赖叶任务的计划。
5. 验证 document 覆盖及 document_patch，检查 patch artifact 与实际结果。
6. 验证 tasks 覆盖和任务 patch 的 add/update/delete，以及稳定 ID 和最终 DAG。
7. 验证同一次 modify_plan 同时修改文档与任务树。
8. 验证其中一项失败时整次回滚，原文档、原树、当前视图和成功记录均不变。
9. 收齐探索任务后提交审核，检查前端看到的是最后修改后的内容。

### B. 预设/mocker 路径

1. 加载原格式计划与文档，保留既有 session Evidence。
2. 不调用 create_plan，不重新生成完整计划。
3. 使用 modify_plan 局部修改文档及依赖，再提交审核。
4. 验证 mocker 每个新实例只初始化一次；恢复不重复构建。

### C. 拒绝与审批边界

- 空参数、null、错误类型及同组件覆盖/patch 冲突。
- 文档 patch 错误目标、多文件、上下文失配、应用后空正文。
- 未知节点、重复标识、自依赖、环、删除后悬空依赖。
- 批次内部顺序与最终验证、同事务失败回滚。
- 第一阶段越权调用执行动作时拒绝，实际业务任务启动数保持 0。
- 有未退出探索任务时 submit_plan 拒绝，不弹出审核。
- 审核等待期间 modify/create 拒绝。
- 用户要求调整后可继续修改，再提交新的审核端点。
- 旧卡回复、重复确认、用户编辑后批准和正常批准均走既有确认契约。

### D. Prompt 与缓存采样

采样创建后、修改后、提交前的 prompt，分别输出文本流 schema 和 function-call tools，供 review。

- 当前文档和定义只出现一份有效内容，修改后的内容真实可见。
- Evidence、用户补充和环境观测不丢失。
- 没有 plan_version、重复 Draft/Approved 文档或执行动作泄漏。
- 开关关闭时，提示词及工具中没有可用探索派发能力。
- 无计划修改的连续轮次不重写 Document/Definition，工具声明稳定。
- 真实修改只更新必要上下文，不把完整计划复制到 feedback 等动态区域。
- 使用消息及 tool 定义的稳定前缀比较来报告缓存友好程度；没有真实 provider usage 时，不把这个结果称为供应商缓存命中率。

第一阶段冒烟在用户批准产生移交后结束，不在此脚本中启动 pe_task 来扩展第二阶段测试。既有通知执行脚本继续负责其原有覆盖，不改造成这个规划脚本。

## 10. 实施顺序与完成标准

按以下顺序实现，逐步保持可 review：

1. 单一 Plan 与 PLAN 阶段权限，移除版本引用并适配已有入口。
2. 唯一 modify_plan 四参数及原子事务，完成文档/任务 patch。
3. 当前上下文发布和 Timeline 记录，验证不重复大正文。
4. 默认关闭的探索开关及既有子 Agent 管理/通知接入。
5. 审核锁定与原有前端确认契约，完成 preset/mocker 路径。
6. 新 Yak/aim 冒烟、必要回归、过时内容清理及文档更新。

完成后给用户：本地 diff、实际执行的测试命令和结果、冒烟时序、两种协议的提交前 prompt 样本，以及尚未解决的限制。未运行的检查不声称通过。未接入真实模型时明确说明模型回调是确定性脚本。

验收核心：第一阶段可以调查、可选探索子 Agent、反复修改一份完整计划；modify_plan 可以覆盖或 patch 两种内容且整次原子生效；最终提交真实当前内容；审核前绝不执行业务计划；没有计划版本、重复计划存储或新增观察体系。
