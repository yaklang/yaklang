package loopinfra

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_EnhanceKnowledgeAnswer = &reactloops.LoopAction{
	ActionType:  schema.AI_REACT_LOOP_ACTION_KNOWLEDGE_ENHANCE,
	Description: `Enhance the answer with additional knowledge`,
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"rewrite_user_query_for_knowledge_enhance",
			aitool.WithParam_Description(`The user query to enhance the knowledge answer. If not provided, the system will use the original user input as the query.`),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{FieldName: `rewrite_user_query_for_knowledge_enhance`},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		query := action.GetString("rewrite_user_query_for_knowledge_enhance")
		if query == "" {
			log.Warn("knowledge_enhance action must have 'rewrite_user_query_for_knowledge_enhance' field, use raw user-input")
			t := loop.GetCurrentTask()
			if utils.IsNil(t) {
				return utils.Errorf("knowledge_enhance action has no current task to get user query")
			}
			query = t.GetUserInput()
		}
		if query == "" {
			return utils.Errorf("knowledge_enhance action has empty user query to enhance")
		}
		loop.Set("enhance_user_query", query)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		rewriteQuery := loop.Get("enhance_user_query")
		if rewriteQuery == "" {
			op.Fail("knowledge_enhance action must have 'rewrite_user_query_for_knowledge_enhance' field")
			return
		}

		invoker := loop.GetInvoker()
		ctx := loop.GetConfig().GetContext()
		task := loop.GetCurrentTask()
		if task != nil && !utils.IsNil(task.GetContext()) {
			ctx = task.GetContext()
		}
		enhancedAnswer, err := invoker.EnhanceKnowledgeAnswer(ctx, rewriteQuery)
		if err != nil {
			op.Fail(err.Error())
			return
		}

		satisfied, reason, err := invoker.VerifyUserSatisfaction(
			ctx,
			rewriteQuery,
			false,
			enhancedAnswer,
		)
		if err != nil {
			op.Fail(utils.Wrap(err, "knowledge_enhance action enhanced knowledge answer"))
			return
		}
		loop.PushSatisfactionRecord(satisfied, reason)

		if satisfied {
			invoker.EmitResult(`** 知识增强结果已经初步满足用户需求(Knowledge enhancement results have initially met the user's needs) **`)
			invoker.EmitResultAfterStream(enhancedAnswer)
			op.Exit()
			return
		}
		op.Continue()
	},
}
