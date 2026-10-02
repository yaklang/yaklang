# Coordinator 接口与 Yakit 兼容契约

本文描述本次实现的接口；[架构文档](coordinator_architecture.md)说明职责和状态，[模块 README](coordinator/README.md)提供使用及测试入口。Yakit 源码依据固定为 master 87904ea55a4af130b4fa8c22dc806405f62e3332；不修改 frontend 或 protobuf。

## 1. 三层接口

| 层次 | 调用者 | 约束 |
| --- | --- | --- |
| 原生模型函数 | coordinator / pe_task | 只接受 provider tool_calls；普通 JSON 响应无执行能力 |
| Controller / Host | action handlers、客户端控制适配 | 校验版本、任务归属、依赖和生命周期；产生真实状态 |
| AIInputEvent / AIOutputEvent | Yakit、RPC、aim | 保留原字段、事件名、路由、审批和恢复行为 |

函数参数的 JSON schema 仍用于参数校验；不使用 response-format JSON schema 驱动新循环。JSON 也继续作为 Content、旧 task_tree/task_progress 字符串及后台快照的数据格式。

## 2. 身份与持久化

| 标识 | 含义 |
| --- | --- |
| session_id / PersistentSessionId | 用户 Timeline、evidence、artifacts 和历史归属 |
| coordinator_id | 当前 PLAN 的路由和历史记录身份 |
| re-act_id / re-act_task | 外层 ReAct 与问题/恢复任务身份 |
| task_id | 稳定逻辑任务 ID；原任务树、输入控制及任务卡继续使用 |
| task_uuid | 当前运行体 UUID，不用于替代逻辑 ID |
| plan_version | 当前草稿版本，用于 modify/submit 的过期检查 |
| approved_version | 当前允许执行的批准版本 |
| submitted_version | detached 草稿已发布版本，不代表批准 |
| attempt_id | 当前尝试，重试增加；review/retry 必须匹配 |
| InteractiveId / id | 一次用户审批或交互端点 |
| SyncID | 同步请求关联，不能解释为 worker 已完成 |
| EventUUID / event_writer_id | stream 生命周期关联，沿用原 emitter |

AISessionPlanAndExec 不增加新表。task_tree、task_progress 保持 JSON 字符串；progress 增加可忽略的 plan_engine 和 coordinator_state。快照记录版本、当前尝试、结果引用和验收状态，不序列化 goroutine、context 或 callback。Timeline 保存历史调用和用户/模型决策。

状态变更先经 Controller 校验，再向宿主发布独立快照。状态快照保存沿用旧持久化设施；数据库错误会发送/记录错误，不能将它当成已验证的耐久写入保障。后续若需要数据库事务级操作回执，应单独扩展 Host 契约。

## 3. 原生 actions

| Action | 参数 | 返回语义 |
| --- | --- | --- |
| create_plan | plan, plan_document | 新草稿版本，不执行 |
| modify_plan | plan_version, plan, plan_document | 完整新草稿版本，批准内容尚未切换 |
| submit_plan | plan_version | 普通审批后采用合法编辑；detached 保存提交版本并等待执行请求 |
| start_tasks | task_ids optional | 指定 ready tasks 或当前 ready 集合的派发回执 |
| inspect_tasks | task_ids optional | 当前状态、尝试、结果和引用，登记结果已观察 |
| wait_tasks | task_ids optional, mode optional, timeout_seconds optional | any/all 等待的原因及快照 |
| review_task | task_id, attempt_id, decision, reason | accept/reject；只能审阅已观察的 awaiting_review |
| retry_task | task_id, attempt_id, reason | 新尝试，受影响下游旧结果失效 |
| cancel_tasks | task_ids optional, reason | 请求取消；实际退出后结算 |
| write_report | title, markdown, summary | Markdown artifact 路径及既有 report_finish |
| directly_answer | 原有原生答案参数 | 可见消息；继续协调 |
| finish | 原有参数 | 经过完成门闩结束 |

plan 使用 name、goal、tasks；每个 task 使用 name、goal、identifier、depends_on。identifier 在修改时保持稳定；模型不自填执行状态或 UUID。后端校验 DAG 并解析逻辑 IDs。原生首版采用平面 DAG；旧嵌套 root_task 在编辑/恢复通道继续使用。

task_ids 缺省的含义由各函数明确声明。原生参数格式不正确由 schema/状态校验拒绝，不将格式错误转换为全选。未知任务、重复派发、过期版本或尝试、未验收依赖、活动任务冲突和配额不足返回可纠正反馈，不执行部分选中任务。

wait 默认 any、30 秒，上限 60 秒；all 要求选中的任务已派发。返回原因包括 new_result、all_settled、changed、timeout；用户信息、批准版本或目标尝试变化会中断等待。超时不取消。new_result/结算不是业务验收。

worker 只有执行职责，额外提供 submit_task_result(summary, artifacts?, evidence_ids?)。它与原有 TODO finish 门闩结合；不能接受自己的结果。共同基础 actions 包括 save_evidence、工具、澄清及技能/知识检索。没有任意设置状态的 update_plan_status，也不开放嵌套专注循环/蓝图/sub-agent。

## 4. 客户端输入保持不变

| 输入 | 保留内容与行为 |
| --- | --- |
| IsStart / Params | 模型、会话、工具、审阅策略、资源及并发配置 |
| EnablePlan | 禁止从 coordinator focus 绕过关闭的 PLAN |
| EnableDetachedPlan | 发布待批准计划；执行必须来自用户执行请求 |
| FocusModeLoop | coordinator 通过现有 QueryAIFocus metadata 展示 |
| IsFreeInput / FreeInput | 按原外层队列交付下一任务；出队时写入用户 Timeline |
| AttachedResourceInfo / AttachedFilePath | 保留附件类型与引用、传递及观察流程 |
| IsInteractiveMessage | InteractiveId、InteractiveJSONInput；接收 suggestion、编辑树、回答及选项 |
| IsConfigHotpatch / TaskId | 延续现有传播；TaskId 保持逻辑任务身份 |
| plan | 通过原 SyncID 返回 root_task |
| skip_subtask_in_plan | subtask_id / subtask_index / reason；取消并等实际退出后回执 |
| redo_subtask_in_plan | subtask_id / subtask_index / user_message；UI 仅重做已完成任务，说明进入 Timeline，撤销旧验收并重试 |
| execute_detached_plan | coordinator_id、session_id、react_task_id、plan 参数及 plans.root_task 编辑 |
| recovery_plan_and_exec | coordinator_id、start_task_id、可选 session_id；恢复批准内容及引擎 |
| react_cancel_task / react_cancel_current_task | 外层取消传播到协调员与 worker |
| user_intervention | 宿主完成共享 Timeline 记录后唤醒协调等待，worker 不重复记录 |
| recovery_history / timeline | 原分页与 IsSync 回放，不重新派发或审批 |
| plan_exec_tasks / consumption / session_snapshot_sync / capability_inventory_sync | 原历史、消耗、快照和能力接口 |

execute_detached_plan 先校验记录归属、待批准阶段、编辑树和 DAG，再保存并入队；成功回执在入队后发送。编辑树优先于旧 plan_data；格式错误不会退回旧计划。恢复队列执行批准后的输入，不重新生成计划。

旧恢复请求是 recovery_plan_and_exec，回复 NodeId 是 recover_plan_and_exec，保留既有拼写差异。PLAN-only 仍停在 plan_ready，待批准 detached 仍使用 detached_pending_approval 及原历史隐藏规则。

## 5. 输出封套与路由

保留 CoordinatorId、Type、NodeId、TaskId、TaskIndex、TaskUUID、IsSync、SyncID、EventUUID、Timestamp、CallToolID、ContentType、NodeIdVerbose 及模型信息。Content 继续使用原 bytes/protobuf 字段。

Yakit 使用 currentChatStatus.coordinatorId 与事件 CoordinatorId 匹配区分 PLAN task 与外层 ReAct。普通审批/任务事件绑定 PLAN owner；外层完成、重做卡片状态事件绑定外层 ReAct。detached 面板在外层路由发布，执行时才建立 PLAN 路由。[路由源码](https://github.com/yaklang/yakit/blob/87904ea55a4af130b4fa8c22dc806405f62e3332/app/renderer/src/main/src/pages/ai-re-act/hooks/ChatMultiSessionController.ts#L1400)。

| Type / NodeId | 必需内容与语义 |
| --- | --- |
| start_plan_and_execution | coordinator_id、re-act_id、re-act_task，在普通审批/任务 live 事件前建立路由 |
| plan_review_require / review-require | id、plans.root_task、selectors、plans_id，原用户审批面板 |
| review_release | 同一 id 与实际 params，释放原端点 |
| detached_plan_require / detached-plan | id、coordinator_id、session_id、plans、selectors、plans_id 等，保持待批准 |
| plan / system | root_task 全量树，包含原 progress 展示 |
| structured / system | type=push_task/pop_task，task 的 index/name/goal/task_id/task_uuid；pop 包含 task_status |
| structured / react_task_status_changed | 外层路由，react_task_id 为逻辑 PLAN task ID，react_task_status=processing，重新显示已关闭的重做卡片 |
| structured / plan_exec_tasks | session_id、total、records；task_tree/task_progress 保持字符串 |
| end_plan_and_execution | 与启动关联字段一致，外层终态也须完成 |
| report_finish / report-finish | report_path、title、summary_markdown |
| stream / stream_start / stream-finished / status | 沿用原节点、writer 关联和加载状态键 |
| consumption / prompt_profile / capability_inventory / session_snapshot | 延续观测，指向实际运行任务 |

任务卡仍使用外层问题 ID 加逻辑 PLAN task ID 作为 key。相同逻辑任务不重复 push；task_uuid 变化不能替代重做状态事件。[任务卡处理](https://github.com/yaklang/yakit/blob/87904ea55a4af130b4fa8c22dc806405f62e3332/app/renderer/src/main/src/pages/ai-re-act/hooks/grpcStreamHandler/aiSingleItem.ts#L401)。

| 新状态 | 旧 progress |
| --- | --- |
| pending | 未开始 |
| running / awaiting_review / cancelling | processing |
| accepted | completed |
| failed / rejected | aborted |
| cancelled | skipped |

worker 执行完成先保留 processing，由可见 stream 说明待验收；验收通过才发 completed/pop。旧 task_review_require 自动 continue 不构成新验收；新的 review_task 是协调员原生动作。[旧审阅处理](https://github.com/yaklang/yakit/blob/87904ea55a4af130b4fa8c22dc806405f62e3332/app/renderer/src/main/src/pages/ai-re-act/hooks/grpcStreamHandler/aiReview.ts#L9)。

## 6. 使用与验证边界

选择新引擎不改变 RPC。未指定 focus 的 PLAN 请求通过默认循环进入新版 coordinator；旧 plan / coordinator_legacy focus 名称也转入新版，公开列表不再展示旧模式。aim.planEngine(...) 仅为 focus 别名。Go 新运行体使用 coordinator.NewSession；coordinator_legacy.NewCoordinatorContext 始终保留旧语义。新 Session 自己拥有任务树 DTO、审批、进度和恢复适配，不调用旧 Coordinator 的内部方法。plan_engine 仅保存在记录中，恢复入口校验原归属；旧记录在入队之前返回明确停用错误，要求重新生成新版计划，不隐式迁移旧状态。

本次自动化验证包括原生协议拒绝普通 JSON、审批与结果门闩、版本和尝试冲突、依赖调度、any/all、取消实际退出、恢复、PLAN-only、分离计划编辑字段和真实 Yak/aim 运行事件。确定性 provider 运行测试不替代实际模型质量、缓存指标或完整 Electron UI 人工验收。
