package loopinfra

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_EnhanceKnowledgeAnswer = &reactloops.LoopAction{
	ActionType:  schema.AI_REACT_LOOP_ACTION_KNOWLEDGE_ENHANCE,
	Description: `Enhance the answer with additional knowledge`,
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"rewrite_user_query_for_knowledge_enhance",
		),
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		query := action.GetString("rewrite_user_query_for_knowledge_enhance")
		if query == "" {
			return utils.Error("knowledge_enhance action must have 'rewrite_user_query_for_knowledge_enhance' field")
		}
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		rewriteQuery := action.GetString("rewrite_user_query_for_knowledge_enhance")
		if rewriteQuery == "" {
			op.Fail("knowledge_enhance action must have 'rewrite_user_query_for_knowledge_enhance' field")
			return
		}

		invoker := loop.GetInvoker()
		enhancedAnswer, err := invoker.EnhanceKnowledgeAnswer(loop.GetConfig().GetContext(), rewriteQuery)
		if err != nil {
			op.Fail(err.Error())
			return
		}

		satisfied, err := invoker.VerifyUserSatisfaction(
			rewriteQuery,
			false,
			enhancedAnswer,
		)
		if err != nil {
			op.Fail(utils.Wrap(err, "knowledge_enhance action enhanced knowledge answer"))
			return
		}

		if satisfied {
			invoker.EmitResult(`** 知识增强结果已经初步满足用户需求(Knowledge enhancement results have initially met the user's needs) **`)
			invoker.EmitResultAfterStream(enhancedAnswer)
			op.Exit()
			return
		}
		op.Continue()
	},
}
