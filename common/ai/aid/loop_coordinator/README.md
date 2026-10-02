# Coordinator：PLAN 的协调运行体

`coordinator` 是新的 PLAN 引擎和专注模式。它负责探索、计划文档、用户审批、任务调度、等待、验收、重试和报告；`pe_task` 执行批准的任务书。新引擎只有这两种 ReAct 循环，均强制使用原生 `tool_calls`，不能通过配置切换到 JSON 响应动作，也不会回退到旧 PLAN/replan/task-review/report 循环。

这个目录包括控制对象、模型 actions、worker 工厂、工具权限、审批辅助调用以及测试。现有 `aid.Coordinator` 提供运行宿主，通过 [coordinator_loop.go](../coordinator_loop.go) 适配 Yakit 事件、交互端点、输入控制、session Timeline 和持久化。它不只是一个提示词专注模式。

## 启用与替换

旧引擎仍是默认值；迁移调用方只需选择 `coordinator`，不修改 protobuf 或前端。选择新引擎后，普通 ReAct 输入直接进入协调员循环。执行任务、恢复和 PLAN-only 入口也使用同一协调循环。

```go
// 保留原来的 Coordinator.Run / RunPlanOnly / RunExecuteApprovedPlan 等入口。
c, err := aid.NewCoordinatorContext(ctx, query, aid.WithCoordinatorLoop(true), options...)
if err != nil { return err }
return c.Run()
```

```yak
err = aim.InvokeReAct(
    "读取目录，规划并执行两个有依赖的验证任务，逐项验收后写报告。",
    aim.planEngine("coordinator"),
    aim.maxIteration(30),
    aim.reviewPolicy("yolo"),
)
die(err)
```

也可以选 `aim.focus("coordinator")`；它同样强制 native function call。`EnablePlan=false` 禁止从该 focus 开启 PLAN。工具的原有人工/AI/YOLO 策略继续生效，AI 风险评估使用一次原生 `review_risk` 调用。Timeline 压缩、附件提取等继承新引擎的单步辅助输出也使用原生函数调用，不新增 ReAct 角色。

`aim.planEngine("legacy")` / `aid.WithCoordinatorLoop(false)` 可以显式选择旧路径。记录中的 `plan_engine` 与 `coordinator_state` 使旧客户端的恢复请求能够找回对应引擎。

## 模型 actions

参数 schema 是原生函数的参数定义。JSON 仍用于工具参数、事件 Content 和存储；模型的普通 JSON 响应不能选择或执行动作。

| Action | 参数 | 作用 |
| --- | --- | --- |
| `create_plan` | `plan`, `plan_document` | 校验并保存草稿，返回版本；不执行任务 |
| `modify_plan` | `plan_version`, `plan`, `plan_document` | 完整替换对应版本的草稿，批准版本继续有效 |
| `submit_plan` | `plan_version` | 进入现有用户审批；采用合法编辑树；detached 只发布待批准计划 |
| `start_tasks` | `task_ids` 可省略 | 原子检查并发、依赖、批准和当前状态，立即派发；省略时选择当前可执行任务 |
| `inspect_tasks` | `task_ids` 可省略 | 返回状态、尝试、结果和引用，并记录结果已被观察 |
| `wait_tasks` | `task_ids`, `mode`, `timeout_seconds` 均可省略 | `any` 默认等待更新；`all` 等选中的已派发尝试全部结算；默认 30、最多 60 秒，超时不取消 |
| `review_task` | `task_id`, `attempt_id`, `decision`, `reason` | 对已观察的 `awaiting_review` 尝试作出 `accept` 或 `reject` |
| `retry_task` | `task_id`, `attempt_id`, `reason` | 重试已结算的当前尝试，撤销下游旧结果；受影响 worker 必须先退出 |
| `cancel_tasks` | `task_ids` 可省略，`reason` | 请求停止；运行中的任务先进入 `cancelling`，退出后才是 `cancelled` |
| `write_report` | `title`, `markdown`, `summary` | 在当前协调循环写 Markdown artifact，发送既有 `report_finish` |
| `directly_answer` | 既有原生答案参数 | 发布进展/答案，继续协调循环 |
| `finish` | 既有参数 | 经过批准、验收、未读结果、worker、TODO、用户输入及报告门闩后结束 |

继续复用 `save_evidence`、`adjust_todolist`、`ask_for_clarification`、工具加载/调用、知识及技能检索 actions。不存在任意改状态的 `update_plan_status`：状态来自执行、观察、验收和取消的真实结果。

`plan` 的首版原生参数采用平面任务 DAG：

```json
{
  "name": "验证方案",
  "goal": "确认两个步骤的结果",
  "tasks": [
    {"name": "验证来源", "goal": "读取来源并保存 evidence", "identifier": "source", "depends_on": []},
    {"name": "验证结论", "goal": "依据来源检查结论", "identifier": "conclusion", "depends_on": ["source"]}
  ]
}
```

`identifier` 保持稳定且唯一，`depends_on` 使用这些语义标识。宿主分配稳定 `task_id`，解析依赖并拒绝重复标识、未知依赖和环。客户端编辑及恢复仍接受原有嵌套 `root_task`，不改变 Yakit 任务树协议。首版修改采用完整草稿替换，避免另造一套 delta 解释器。

## 任务与结果

```text
pending -> running -> awaiting_review -> accepted
                      └──────────────> rejected
           └────────> failed
running -> cancelling -> cancelled
settled -> retry -> running（新的 attempt_id）
```

worker 的 `submit_task_result(summary, artifacts?, evidence_ids?)` 提交结果，随后 `finish` 经过原有微观 TODO 门闩。worker 返回并不意味着验收通过，也不会自动放行依赖任务。协调员读取结果及引用，再调用 `review_task`；需要额外验证时派发批准范围内的验证 task。

任务书在派发时冻结。批准新版本前，改变任务书或上游输入的活动任务必须停止；改变输入后，下游已完成或待验收的旧结果也会失效。重试使用新尝试序号，保留 Timeline 中的调用、结果和审阅记录；快照保存当前尝试。恢复不自动重启曾在运行的 worker，先标记 interrupted/failed，交给协调员显式重试。

`wait_tasks` 使用通知等待。用户交互、计划修改或尝试替换会让等待返回，协调员重新评估。`all` 不能等待尚未派发的任务；不能把还未获验收的依赖当成已完成。

`user_intervention` 由会话宿主写入共享 Timeline 后再通知协调员，不重复转交给每个 worker 记录。普通 `FreeInput` 保持外层队列语义，供下一任务执行。要求报告时由 `write_report` 完成；已有 `ResultHandler` 仍优先负责收尾。

## 工具边界与上下文

协调员只开放显式内置读取/目录/搜索工具；`write_file` 限制在工作目录 `artifacts` 下的 `.md` 文件，检查路径穿越和已有父目录的符号链接。未知工具、MCP 命令包装器、脚本执行和业务修改交给 worker。worker 继承原有工具审阅策略，但不能开启其他专注循环、蓝图、PLAN 或通用 sub-agent。

用户输入、澄清回答、选项和用户重做说明进入 session Timeline，经过 Open 的冻结/压缩提升；纯动态区域不重新附加 USER QUERY。任务派发书也记录在 Timeline，worker 使用既有 fork/merge。`save_evidence` 复用 session journal，所有 tasks 与协调员共享、去重并沿现有机制提升。

```text
High static：现有纯静态系统指令
SemiDynamic1：提升后的用户信息 / session evidence / 批准的 PLAN DOCUMENT
SemiDynamic2：协调员或 worker 的稳定职责说明
Dynamic：Timeline Open -> PLAN STATUS -> 微观 TODO -> 运行状态
```

PLAN STATUS 包含草稿/批准版本、逻辑 task ID、状态、尝试和结果观察状态。它不复制用户输入或计划文档。无状态 PLAN TREE 的重新设计仍单独待定。

## Yakit 与旧接口

| 通道 | 适配规则 |
| --- | --- |
| 启动/结束 | 保留 `start_plan_and_execution` / `end_plan_and_execution` 和 `coordinator_id`, `re-act_id`, `re-act_task`；外层 ReAct 终态仍由原入口发送 |
| 普通审批 | `plan_review_require` / `review-require`，保持 `id`, `selectors`, `plans.root_task`, `plans_id`，由现有端点释放 |
| Detached | `detached_plan_require` / `detached-plan`；提交后不会启动 worker；`execute_detached_plan` 接收并校验用户编辑树，再入旧恢复队列 |
| 任务卡 | 相同逻辑 task ID 只 push 一次；已关闭任务重做通过外层路由的 `react_task_status_changed` 重新显示处理状态；验收或取消结束时 pop |
| 任务状态 | `running/awaiting_review/cancelling` 映射 processing；`accepted` 映射 completed；`failed/rejected` 映射 aborted；`cancelled` 映射 skipped |
| 控制输入 | 保留 `plan`, `skip_subtask_in_plan`, `redo_subtask_in_plan`、原 SyncID 和返回格式；skip 在实际退出后回执；UI 重做只接受已完成任务，说明进入 Timeline |
| 恢复/历史 | 保留 `recovery_plan_and_exec` 请求及 `recover_plan_and_exec` 回复拼写；`task_tree/task_progress` 仍是 JSON 字符串 |
| 展示/观测 | 沿用 stream、status、能力、消耗、prompt profile、session snapshot、artifact 和 `report_finish` 通道 |

旧 `task_review_require` 面板的自动 continue 不是新的业务验收；新协调员通过原生 `review_task` 作出验收，利用已有 stream 展示理由。整个新流程不要求修改 Yakit。

更多字段与依据见 [接口契约](../coordinator_interface_contract.md)，组件关系见 [架构文档](../coordinator_architecture.md)。

两个循环之外的辅助模型请求、实测开销和后续统一治理问题见 [TODO](TODO.md)。

## 测试与 Yak 冒烟

```powershell
go test ./common/ai/aid/loop_coordinator -count=1
go test ./common/ai/aid ./common/ai/aid/aireact -run 'TestCoordinatorRecovery|TestCoordinatorDetached|TestPublishDetachedPlan|TestHandleSyncTypeExecuteDetachedPlanEvent' -count=1
```

[native_coordinator.yak](smoke/native_coordinator.yak) 由测试通过真实 Yak 引擎执行，调用 `aim.InvokeReAct`。只有模型 provider 使用可重复的原生函数调用响应；协调员、worker、审批、Timeline、报告和 Yakit 事件适配均运行实际代码。测试核对真实文件读取、两个有依赖的任务、共享 evidence、逐项验收、push/pop、报告文件及两种 loop_marker。另有实际 AID detached 提交、编辑、引擎恢复及执行测试，以及干预入 Timeline 后通知的时序测试。

[live_coordinator.yak](smoke/live_coordinator.yak) 可使用本机配置的实际 provider/model 执行；凭据由外部配置，不写入脚本。确定性冒烟不衡量模型任务质量或真实 provider 的缓存命中率。

新增 actions 应放在此包，状态变化通过 Controller，外部渠道通过 Host。不在 worker 中暗中恢复旧 review/replan 循环，也不新增第三种 ReAct 角色。
