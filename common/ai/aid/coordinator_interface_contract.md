# Coordinator 接口与 Yakit 兼容契约

本文描述本次实现的接口；[架构文档](coordinator_architecture.md)说明职责和状态，[模块 README](coordinator/README.md)提供使用及测试入口。Yakit 源码依据固定为 master 87904ea55a4af130b4fa8c22dc806405f62e3332；不修改 frontend 或 protobuf。

## 1. 三层接口

| 层次 | 调用者 | 约束 |
| --- | --- | --- |
| 模型动作 | coordinator / pe_task | 继承主循环协议：文本流 JSON @action 或 provider tool_calls；提示词按模式选择，两种协议不混用 |
| Controller / Host | action handlers、客户端控制适配 | 校验阶段、审核锁、任务归属、依赖和生命周期；产生真实状态 |
| AIInputEvent / AIOutputEvent | Yakit、RPC、aim | 保留原字段、事件名、路由、审批和恢复行为 |

专属 actions 分别提供中文 Options/Description 和 NativeOptions/NativeDescription：前者用于文本流 JSON Schema，后者用于原生函数定义；共用参数语义与执行校验。JSON 也继续作为 Content、旧 task_tree/task_progress 字符串及后台快照的数据格式。

## 2. 身份与持久化

| 标识 | 含义 |
| --- | --- |
| session_id / PersistentSessionId | 用户 Timeline、evidence、artifacts 和历史归属 |
| coordinator_id | 当前 PLAN 的路由和历史记录身份 |
| re-act_id / re-act_task | 外层 ReAct 与问题/恢复任务身份 |
| task_id | 稳定逻辑任务 ID；原任务树、输入控制及任务卡继续使用 |
| task_uuid | 当前运行体 UUID，不用于替代逻辑 ID |
| phase / review_pending | PLAN 或 EXEC，以及本次审核等待锁 |
| attempt_id | 当前尝试，重试增加；review/retry 必须匹配 |
| InteractiveId / id | 一次用户审批或交互端点 |
| SyncID | 同步请求关联，不能解释为 worker 已完成 |
| EventUUID / event_writer_id | stream 生命周期关联，沿用原 emitter |

AISessionPlanAndExec 不增加新表。task_tree、task_progress 保持 JSON 字符串；progress 增加可忽略的 plan_engine 和 coordinator_state。schema 2 快照记录一份当前 Plan、阶段、审核锁、当前/历史尝试、结果引用、验收状态、inbox 游标和当前报告，不序列化 goroutine、context 或 callback。Timeline 保存历史调用和用户/模型决策。

状态变更先经 Controller 校验，再向宿主发布独立快照。计划编辑和批准移交在 Host.CommitPlan 成功后才暴露候选内容；数据库失败不发布新当前视图或成功回执。执行阶段状态沿用原发布与错误上报通道。

## 3. Actions

| Action | 参数 | 返回语义 |
| --- | --- | --- |
| create_plan | plan, plan_document | PLAN 首次创建，返回 updated 与组件；不执行 |
| modify_plan | document / document_patch / tasks / tasks_patch | PLAN/EXEC 原子编辑；EXEC 保护受影响尝试，无再次审批 |
| submit_plan | 无业务参数 | PLAN 锁定、审核、保存最终编辑并移交 EXEC；detached 发布待审核卡 |
| wait_messages | timeout_seconds optional | inbox 等待，默认30秒；空超时不轮询主模型 |
| inspect_task | task_id, attempt_id optional, details optional | 单任务当前/历史状态及引用，详情按需 |
| review_task | task_id, attempt_id, decision, reason | YOLO 接受/拒绝/深入/取消；不能绕过人工策略 |
| retry_task | task_id, attempt_id, reason | 新尝试，受影响下游旧结果失效 |
| cancel_tasks | task_ids optional, reason | 请求取消；实际退出后结算 |
| create_report | title, document | 门禁通过后创建唯一 Markdown artifact |
| modify_report | document / document_patch | 更新报告正文或严格 diff，当前视图同步到 SemiDynamic1 |
| submit_report | summary | 交付最新正文、report_finish；宿主复查后结束 |
| directly_answer | 原有协议对应的答案参数 | 可见消息；继续协调 |
| finish | 原有参数 | 仅 PLAN-only 适配保留；完整 EXEC 不开放正常退出 |

plan 沿用嵌套的 main_task、main_task_goal、tasks，以及递归的 subtask_name、subtask_goal、subtask_identifier、sub_subtasks、depends_on；平面 name/goal/identifier 保留为兼容别名。语义标识在修改时保持稳定；模型不自填执行状态或 UUID。父节点仅组织任务，后端校验 DAG 并解析叶任务逻辑 IDs。

表中的返回语义是持久化动作观测的内容。模型 handler 将小型变更回执、结果、拒绝原因及验收理由写入 session Timeline Evidence；feedback 仅给出处理状态和记录 ID。Open 冻结后，沿已有 Evidence 通道提升到 SemiDynamic1，不按 action 强制冻结。

计划正文、DAG 和当前报告读取 SemiDynamic1，阶段与任务状态读取 PLAN STATUS。移除 inspect_plan/inspect_tasks，保留按需 inspect_task；默认只返回有界摘要与引用。任务结果由 Controller 在实际结算和验收时自动保存，每次尝试独立记录。review_task 校验当前已结算 attempt 与理由，不解析 AIRequest、不登记 Seen。schema 1 只在 snapshot_compat.go 读取转换，不恢复模型版本参数或双份替换草案。

协调员空闲时自动等待，无须模型 wait_messages。有效 Evidence 变化先保存再入队，实际 worker 退出及 Timeline 交接后才结算。消息携带 id/sequence/coordinator_id/task_id/attempt_id/type/summary/references；投递游标与处理游标分离，事实在 Timeline。回调不触发并发主模型；普通流式输出、重复证据及空超时不唤醒。正常人工通过直接推进 DAG，必要反馈通知模型。gRPC 审核、任务树与事件定义不变。

task_ids 缺省的含义由各函数明确声明。原生参数格式不正确由 schema/状态校验拒绝，不将格式错误转换为全选。未知任务、重复派发、过期审核或尝试、未验收依赖、活动任务冲突和配额不足返回可纠正反馈，不执行部分选中任务。

wait_messages 默认30秒、最大60秒，不提供 task_ids 或 any/all。已有消息立即返回；空超时给运行时一次检查机会，没有主动工作则继续休眠，不取消 worker。模型 start_tasks/wait_tasks/write_report 已删除；内部调度与状态读取方法不属于模型接口。

worker 只有执行职责，额外提供 submit_task_result(summary, artifacts?, evidence_ids?)。它与原有 TODO finish 门闩结合；不能接受自己的结果。共同基础 actions 包括 save_evidence、工具、澄清及技能/知识检索。没有任意设置状态的 update_plan_status，也不开放嵌套专注循环/蓝图/sub-agent。

PLAN 提供三个计划动作；EXEC 保留 modify_plan 并提供执行/报告动作，Controller/handler 同样检查阶段与门禁。四参数存在性、null、互斥、类型及有序 patch 再次校验。开启 WithEnableSubagentsInPlan 时复用 generic 调查 job，提交必须等待实际退出与交接。通知不改变工具声明，声明只随真实阶段或报告门禁变化。

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

worker 执行完成先保留 processing，释放执行槽位并由任务管理器发起人工 task_review_require；用户通过才发 completed/pop，并立即释放后继。YOLO 由协调员 review_task 判断实际结果。审核 context 属于 session，不随已退出 worker 取消。沿用 [审阅处理](https://github.com/yaklang/yakit/blob/87904ea55a4af130b4fa8c22dc806405f62e3332/app/renderer/src/main/src/pages/ai-re-act/hooks/grpcStreamHandler/aiReview.ts#L9) 的端点与 selectors。

## 6. 使用与验证边界

选择新引擎不改变 RPC。未指定 focus 的 PLAN 请求通过默认循环进入新版 coordinator；旧 plan / coordinator_legacy focus 名称也转入新版，公开列表不再展示旧模式。aim.planEngine(...) 仅为 focus 别名。Go 新运行体使用 coordinator.NewSession；新 Session 自己拥有任务树 DTO、审批、进度和恢复适配，不调用旧 Coordinator 的内部方法。plan_engine 仅保存在记录中，恢复入口校验原归属；旧记录在入队之前返回明确停用错误，要求重新生成新版计划，不隐式迁移旧状态。

自动化验证包含两种协议 × 人工/YOLO 的真实 Yak/aim DAG、普通/编辑/detached 确认、审核反馈重试、消息与等待、局部编辑、取消实际退出、恢复、报告与晚到消息，以及 PLAN-only/预设/mocker 回归。确定性 provider 只脚本化模型决定，不替代实际供应商模型质量、缓存命中率或完整 Electron UI 人工验收。详见 [本地验收记录](coordinator/execution_review.md)。
