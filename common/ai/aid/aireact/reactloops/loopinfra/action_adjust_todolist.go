package loopinfra

import (
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// loopAction_AdjustTodolist is the main-loop sibling of the verification path's
// `next_movements`: it lets the AI proactively post a delta into the shared
// global TODO store from inside a normal ReAct iteration, without having to
// wait for a verification AI call. Operations mirror verification semantics
// exactly (add / doing / done / delete / skip) and are applied through the
// same SessionPromptState.VerificationTodoStore that verification writes,
// so both channels converge on a single TODO snapshot rendered into every
// loop prompt.
//
// 设计目标:
//   - 让 AI 在主循环里就能维护 TODO, 不必凑 verification 才能纠正方向;
//   - 与 verification.next_movements 共享同一份 store, 不再人为拆通道;
//   - 事件流与 verification 一致 (timeline NEXT_MOVEMENTS 对偶, EVENT_TYPE_TODO_LIST_UPDATE),
//     前端 TODO 面板无需区分来源即可消费.
//
// 关键词: adjust_todolist 主循环 TODO 通道, ApplyVerificationTodoOps 复用,
//
//	NEXT_MOVEMENTS timeline 对偶, EVENT_TYPE_TODO_LIST_UPDATE
var loopAction_AdjustTodolist = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_ADJUST_TODOLIST,
	Description: "Proactively adjust the global TODO list from the main ReAct loop using the same increment grammar as verification.next_movements " +
		"(ops: add / doing / done / delete / skip). Use this when the current iteration produced enough new information to enqueue, mark in-progress, " +
		"close, drop, or skip TODO items, but you don't want to wait for the next verification round. Always submit only the delta against the existing " +
		"TODO list; never repeat unchanged items. The applied delta is written into the shared TODO store, broadcast as a structured todo_list_update " +
		"event, and breadcrumbed into the timeline under NEXT_MOVEMENTS, exactly like verification. " +
		"LANGUAGE: every human-readable string (especially the `content` field) MUST be written in the same natural language as the user's query " +
		"and the surrounding session. If the user is speaking Chinese, write Chinese; if English, write English. Never auto-translate to English " +
		"just because the action description happens to be in English. The frontend renders these strings verbatim into the shared TODO panel " +
		"alongside verification.next_movements output, so any language drift will look jarring to the user.",
	Options: []aitool.ToolOption{
		aitool.WithStructArrayParam("next_movements",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("TODO increment array. Each item describes one delta op against the shared global TODO list. " +
					"Submit only the items that should change this round; never repeat existing TODOs unchanged. " +
					"Same shape as verification.next_movements: {op, id, content}. When you have nothing to change, do NOT emit this action. " +
					"Language rule: the `content` field MUST match the language of the user query / surrounding session " +
					"(用户用中文 → content 写中文; English user → English content). Do NOT translate to English just because this schema is English."),
				aitool.WithParam_Required(true),
			},
			nil,
			aitool.WithStringParam("op",
				aitool.WithParam_Description("TODO operation. add=create a new TODO; doing=mark an existing TODO as in-progress; done=close an existing TODO; delete=drop an existing TODO (no longer needed); skip=actively decide not to pursue an existing TODO within the current task scope."),
				aitool.WithParam_EnumString("add", "doing", "done", "delete", "skip"),
				aitool.WithParam_Required(true),
			),
			aitool.WithStringParam("id",
				aitool.WithParam_Description("Stable TODO identifier (snake_case ASCII, e.g. `recon_dns_whois`). The id is a machine handle, " +
					"NOT user-facing text, so keep it ASCII regardless of the session language. For non-add ops it must refer to an existing TODO."),
				aitool.WithParam_Required(true),
			),
			aitool.WithStringParam("content",
				aitool.WithParam_Description("TODO content shown directly in the user-facing TODO panel. Required when op=add; optional otherwise. "+
					"Keep it short, action-oriented, aligned with the current task goal. "+
					"LANGUAGE: write in the SAME natural language as the user's query (用户中文 → 用中文; English user → English). "+
					"Do NOT translate to English. This text sits next to verification.next_movements output in the same panel; "+
					"language drift between the two channels will look broken to the user."),
			),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{
			// AINodeId 保留 "adjust_todolist", 前端 i18n 把它翻译成 "待办事项"
			// 这个用户已经熟悉的标签 (见 schema/ai_node_id_i18n.go). 关键改动
			// 在 StreamHandler: 把原本会被框架原样转发的 next_movements JSON
			// 数组在流过程中实时翻译成 verification 路径完全一致的 display
			// 行 (`- [+]: [id: x]: y`), 前端不再看到裸 JSON, 与 verification.
			// next_movements 字节级对齐.
			//
			// markdown 形态的全量快照仍然走独立的 next_movements_snapshot
			// 节点 (apply 后由 emitAdjustTodolistMarkdownSnapshot 发出),
			// 与 verification 路径同位 — 前端 TODO 面板因此能在 adjust 和
			// verification 两条通道之间自动联动, 不需要区分来源.
			//
			// 关键词: adjust_todolist next_movements StreamHandler, 不裸 JSON,
			//   verification 字节对齐, 待办事项 i18n 不破坏
			FieldName:     "next_movements",
			AINodeId:      "adjust_todolist",
			StreamHandler: adjustTodolistNextMovementsStreamHandler,
		},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		movements := aicommon.NormalizeVerifyNextMovements(action)
		if len(movements) == 0 {
			return utils.Error("adjust_todolist requires a non-empty next_movements array; each item must carry both op and id")
		}
		// 缓存到 loop 变量, 让 handler 在不重新解析 action 的前提下复用,
		// 与 verifier 的归一化结果保持一致.
		// 关键词: adjust_todolist verifier 归一化产物缓存
		loop.Set("adjust_todolist_movements", movements)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		movements := getAdjustTodolistMovements(loop, action)
		if len(movements) == 0 {
			// 理论上 verifier 已经挡掉, 这里做防御性兜底.
			operator.Feedback("adjust_todolist skipped: empty next_movements after normalization")
			operator.Continue()
			return
		}

		cfg := loop.GetConfig()
		if cfg == nil {
			operator.Fail(utils.Error("adjust_todolist requires a non-nil config to access the global TODO store"))
			return
		}

		// markdown delta 必须在 apply 之前算, 因为预览语义需要"还未提交时的
		// (new) / (done) 标记". 与 verification 路径完全一致: 先 emit 伪流再
		// 真 apply, 让前端按"流先到, 结构化事件随后定稿"的顺序消费.
		// 关键词: adjust_todolist markdown 预览, apply 前计算 delta
		markdownSnapshot := cfg.GetVerificationTodoMarkdownDelta(false, movements)
		emitAdjustTodolistMarkdownSnapshot(loop, cfg, markdownSnapshot)

		// satisfied=false: 主循环路径仅做增量调整, 不主张"任务已完成";
		// 收口判定仍交给 verification 路径的 enforceTodoCompletionBeforeSatisfaction.
		// 关键词: adjust_todolist satisfied=false, 仅增量, 不抢 verification 收口
		cfg.ApplyVerificationTodoOps(false, movements)

		emitAdjustTodolistUpdate(loop, cfg, movements)

		invoker := loop.GetInvoker()
		if invoker != nil {
			if line := aicommon.FormatNextMovementsBreadcrumb(movements); line != "" {
				// 与 verification 的 NEXT_MOVEMENTS breadcrumb 共用同一个
				// timeline 类别, 消费者无需区分来源即可还原 TODO 时间线.
				// 关键词: NEXT_MOVEMENTS timeline 通道融合, 主循环来源对齐
				invoker.AddToTimeline("NEXT_MOVEMENTS", line)
			}
		}

		summary := aicommon.FormatVerifyNextMovementsSummary(movements)
		feedback := "TODO list adjusted"
		if summary != "" {
			feedback = fmt.Sprintf("TODO list adjusted: %s", summary)
		}
		operator.Feedback(feedback)
		operator.Continue()
	},
}

// getAdjustTodolistMovements 复用 verifier 写入 loop 变量的归一化结果;
// 若变量丢失 (例如 handler 被直接调用而绕过 verifier), 退回到原始 action 再
// 解析一次, 保证 handler 即使在脱离 verifier 流程下也能保持幂等语义.
//
// 关键词: adjust_todolist handler 兜底, verifier 缓存复用
func getAdjustTodolistMovements(loop *reactloops.ReActLoop, action *aicommon.Action) []aicommon.VerifyNextMovement {
	if loop == nil {
		return aicommon.NormalizeVerifyNextMovements(action)
	}
	if raw, ok := loop.GetVariable("adjust_todolist_movements").([]aicommon.VerifyNextMovement); ok && len(raw) > 0 {
		return raw
	}
	return aicommon.NormalizeVerifyNextMovements(action)
}

// adjustTodolistNextMovementsStreamHandler 把 AI 流式输出的 next_movements
// JSON 数组实时翻译成"- [+]: [id: x]: y / - [DOING]: ..."这种 display
// 行, 与 verification 路径的 next_movements stream 字节级一致. 这里只是把
// aicommon 的公开 helper 包裹一层 — LoopStreamFieldHandler 签名不返回
// error, 解析失败由 helper 内部 log/return 自然消化即可, 上层 framework
// 拷贝完后会自动关闭管道.
//
// 关键词: adjustTodolistNextMovementsStreamHandler, next_movements 实时翻译,
//
//	verification 字节一致, LoopStreamFieldHandler 适配
func adjustTodolistNextMovementsStreamHandler(fieldReader io.Reader, emitWriter io.Writer) {
	if err := aicommon.WriteNextMovementsDisplayStream(fieldReader, emitWriter); err != nil {
		// 解析失败往往是 AI 把 next_movements 写成了非数组形态 (例如裸字符串
		// 或者半截 JSON); 此时静默 drain 余下输入避免 producer 卡在 pipe 上.
		// 关键词: next_movements stream 兜底, JSON 非数组容错, drain pipe
		log.Debugf("adjust_todolist: next_movements display stream failed: %v", err)
		_, _ = io.Copy(io.Discard, fieldReader)
	}
}

// emitAdjustTodolistMarkdownSnapshot 把"应用本轮增量后"的全量 TODO markdown
// 快照以伪流形式发出, 节点 ID 与 verification 路径的 next_movements_snapshot
// 完全一致, 让前端 markdown 渲染器无需感知调用方 — 不管来源是 verification
// 还是 adjust_todolist, 走同一个面板渲染入口.
//
// 入参 markdownSnapshot 已经是 `cfg.GetVerificationTodoMarkdownDelta` 渲染好的
// 文本; 在 apply 之前算好传入, 这样 (new) / (done) / (deleted) / (skipped)
// 这些 delta 标记才是相对"上一轮 store"的真实增量.
//
// 关键词: emitAdjustTodolistMarkdownSnapshot, next_movements_snapshot 节点对齐,
//
//	EmitTextMarkdownStreamEvent 伪流, verification 同位事件
func emitAdjustTodolistMarkdownSnapshot(loop *reactloops.ReActLoop, cfg aicommon.AICallerConfigIf, markdownSnapshot string) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(markdownSnapshot) == "" {
		return
	}
	emitter := cfg.GetEmitter()
	if emitter == nil {
		return
	}
	taskID := ""
	if loop != nil {
		if task := loop.GetCurrentTask(); task != nil {
			taskID = task.GetId()
		}
	}
	if _, err := emitter.EmitTextMarkdownStreamEvent(
		"next_movements_snapshot",
		strings.NewReader(markdownSnapshot),
		taskID,
		func() {},
	); err != nil {
		log.Warnf("adjust_todolist: emit next_movements_snapshot markdown stream event failed: %v", err)
	}
}

// emitAdjustTodolistUpdate publishes the post-apply TODO snapshot as a
// structured EVENT_TYPE_TODO_LIST_UPDATE event, matching the verification
// path's emitTodoListUpdate so the frontend TODO panel sees both channels
// through one consistent contract.
//
// 关键词: emitAdjustTodolistUpdate, EVENT_TYPE_TODO_LIST_UPDATE 双通道一致,
//
//	IterationIndex / TaskID 上下文
func emitAdjustTodolistUpdate(loop *reactloops.ReActLoop, cfg aicommon.AICallerConfigIf, movements []aicommon.VerifyNextMovement) {
	if cfg == nil {
		return
	}
	emitter := cfg.GetEmitter()
	if emitter == nil {
		return
	}
	payload := aicommon.TodoListUpdatePayload{
		Items:      cfg.SnapshotVerificationTodoItems(),
		Stats:      cfg.GetVerificationTodoStats(),
		AppliedOps: append([]aicommon.VerifyNextMovement(nil), movements...),
		Satisfied:  false,
	}
	if loop != nil {
		payload.IterationIndex = loop.GetCurrentIterationIndex()
		if task := loop.GetCurrentTask(); task != nil {
			payload.TaskID = task.GetId()
		}
	}
	if _, err := emitter.EmitTodoListUpdate(payload); err != nil {
		log.Warnf("adjust_todolist: emit todo_list_update event failed: %v", err)
	}
}
