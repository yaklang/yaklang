package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

// applyNextMovementsBottomLine 是主循环 next_movements 兜底拦截入口.
//
// 任何 action (除了 adjust_todolist 自己, 它的 ActionHandler 会单独调
// applyAdjustTodolistMovements) 如果在 JSON 中携带了 next_movements 字段,
// 都立即 apply 到全局 TODO store 并广播 todo_list_update + NEXT_MOVEMENTS
// timeline breadcrumb, 与 adjust_todolist / verification 路径字节级一致.
//
// 设计动机:
//
//	AI 在 directly_call_tool / require_tool / load_capability 等 action 的
//	JSON 中"自作主张"写 next_movements (例如顺手列出本轮待办), 但 schema 没
//	强制约束, prompt 也只能软约束. 旧设计只让 adjust_todolist 的 ActionHandler
//	处理 apply + emit, 其他 action 的 next_movements 只走了 stream handler
//	这一支 (因为 streamFields["next_movements"] 被全局共享注册, 见
//	callAITransaction), 导致前端"待办事项"显示已出现, 但 store 没更新,
//	EVENT_TYPE_TODO_LIST_UPDATE 永远缺席, 下一轮 prompt 的 TODO snapshot
//	也看不到这些 id — 用户看到的是孤儿待办.
//
// 实现:
//   - actionType == adjust_todolist 时跳过 (避免与 adjust_todolist 自己的
//     ActionHandler 重复 apply / emit).
//   - actionParams 为 nil 或解析不出 movements 时跳过.
//   - 其它一律调 aicommon.ApplyVerificationNextMovementsAndEmit 单源 helper.
//
// 关键词: applyNextMovementsBottomLine, 主 loop 兜底, 孤儿待办修复,
//
//	全 action 共用 store, ApplyVerificationNextMovementsAndEmit 单源汇聚
func applyNextMovementsBottomLine(
	r *ReActLoop,
	task aicommon.AIStatefulTask,
	iterationCount int,
	actionParams *aicommon.Action,
) {
	if r == nil || actionParams == nil {
		return
	}
	if actionParams.ActionType() == schema.AI_REACT_LOOP_ACTION_ADJUST_TODOLIST {
		// adjust_todolist 自己的 ActionHandler 会调 applyAdjustTodolistMovements,
		// 这里跳过避免双重 apply / emit.
		return
	}
	movements := aicommon.NormalizeVerifyNextMovements(actionParams)
	if len(movements) == 0 {
		return
	}

	cfg := r.config
	if cfg == nil {
		return
	}

	var timelineHook func(category, line string)
	if invoker := r.GetInvoker(); invoker != nil {
		timelineHook = func(category, line string) {
			invoker.AddToTimeline(category, line)
		}
	}

	scope := aicommon.BuildVerificationTodoScope(task)

	aicommon.ApplyVerificationNextMovementsAndEmit(
		cfg,
		cfg.GetEmitter(),
		task,
		scope,
		iterationCount,
		false,
		movements,
		timelineHook,
	)
}
