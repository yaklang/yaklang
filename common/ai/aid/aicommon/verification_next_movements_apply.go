package aicommon

// ApplyVerificationNextMovementsAndEmit is the single-source-of-truth helper
// that wires a `next_movements` delta through to the shared
// VerificationTodoStore and fires the canonical events consumed by the
// frontend TODO panel:
//
//  1. ApplyVerificationTodoOps mutates the global TODO store.
//  2. EVENT_TYPE_TODO_LIST_UPDATE and EVENT_TYPE_CURRENT_TASK_TODO_LIST_UPDATE
//     structured events carrying the post-apply snapshot (session-wide and
//     current-task scoped) plus applied_ops and minimal context.
//  3. Optional NEXT_MOVEMENTS timeline breadcrumb (via timelineHook) — written
//     in the same chronological position as the verification path's own
//     breadcrumb so log/test consumers see one unified TODO timeline
//     regardless of which channel produced the delta.
//
// Note: next_movements_snapshot stream ("待办" chat card) is intentionally not
// emitted here; the frontend uses todo_list_update only to avoid duplicate UI.
//
// 设计动机:
//
//	旧设计把 apply + 三连 emit 重复实现在两处 (verification path 内嵌, 主循环
//	adjust_todolist action 内嵌). 当主循环出现 第三 / 第 N 条通道 (例如 AI 在
//	directly_call_tool 等 action 的 JSON 中"自作主张"携带 next_movements 字段)
//	时, 没有统一的 apply 入口可以复用, 导致 stream display 已经 emit 出"待办
//	事项"显示, 但 store 不更新、todo_list_update 永远缺席, 用户看到的是孤儿待办.
//	本函数让任意调用方都能用同一个函数把 next_movements 推到 store 并广播
//	todo_list_update, 字节级与 verification / adjust_todolist 对齐.
//
// 调用方:
//
//   - verification path (after AI verify-satisfaction completes, 当前仍内嵌
//     在 aireact/verification.go, 后续可迁移到此函数)
//   - adjust_todolist 主循环 action handler
//   - ReActLoop 主循环 next_movements 兜底拦截 (任意 action JSON 携带
//     next_movements 字段都会被兜底 apply)
//
// 关键词: ApplyVerificationNextMovementsAndEmit, apply+update+timeline
//
//	单源汇聚, verification / adjust_todolist / main-loop 三路汇聚, 孤儿待办修复
func ApplyVerificationNextMovementsAndEmit(
	cfg AICallerConfigIf,
	emitter *Emitter,
	task AIStatefulTask,
	scope VerificationTodoScope,
	iterationIndex int,
	satisfied bool,
	movements []VerifyNextMovement,
	timelineHook func(category, line string),
) {
	if cfg == nil || len(movements) == 0 {
		return
	}

	scope = scope.normalize()

	// 1. apply: 把 delta 写入共享 store, 后续任何 prompt 渲染都能看到.
	//    无论 emitter 是否存在, store 都必须更新 — 这是契约的关键, 让
	//    "apply 必有效果"的语义不依赖前端事件通道的可用性.
	cfg.ApplyVerificationTodoOps(scope, satisfied, movements)

	// 2. emit 结构化 todo_list_update + current_task_todo_list_update.
	//    emitter 为 nil 时跳过 (例如部分单元测试).
	if emitter != nil {
		payload := TodoListUpdatePayload{
			Items:          cfg.SnapshotVerificationTodoItems(),
			Stats:          cfg.GetVerificationTodoStats(),
			AppliedOps:     append([]VerifyNextMovement(nil), movements...),
			Satisfied:      satisfied,
			IterationIndex: iterationIndex,
			TaskID:         scope.TaskID,
			TaskIndex:      scope.TaskIndex,
		}
		emitter.EmitTodoListUpdates(cfg, task, payload)
	}

	// 3. timeline breadcrumb: delta-only 一行一个 op, 与 verification 路径
	//    共用同一个 NEXT_MOVEMENTS 类别, 消费者无需区分来源即可还原 TODO
	//    时间线. timelineHook 为 nil 时跳过 (例如脱离 invoker 的纯单元测试).
	if timelineHook != nil {
		if line := FormatNextMovementsBreadcrumb(movements); line != "" {
			timelineHook("NEXT_MOVEMENTS", line)
		}
	}
}
