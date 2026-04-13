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
			aitool.WithParam_Description(`MUST be set if 'knowledge_enhance_answer' @action is set. The user query to enhance the knowledge answer. If not provided, the system will use the original user input as the query.`),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{FieldName: `rewrite_user_query_for_knowledge_enhance`, AINodeId: `rewrite_user_query_for_knowledge_enhance`},
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
		originalInput := loop.GetCurrentTask().GetUserInput()
		rewriteQuery := loop.Get("enhance_user_query")
		if rewriteQuery == "" {
			op.Fail("knowledge_enhance action must have 'rewrite_user_query_for_knowledge_enhance' field")
			return
		}

		invoker := loop.GetInvoker()
		ctx := loop.GetConfig().GetContext()

		selectKBResults, err := invoker.SelectKnowledgeBase(ctx, utils.MustRenderTemplate(`
<|ORIGINAL_USER_INPUT_{{.nonce}}|>
{{ .original_input }}
<|ORIGINAL_USER_INPUT_END_{{.nonce}}|>

<|REWRITE_QUERY_{{.nonce}}|>
{{ .rewrite_query }}
<|REWRITE_QUERY_END_{{.nonce}}|>
`, map[string]interface{}{
			"original_input": originalInput,
			"rewrite_query":  rewriteQuery,
			"nonce":          utils.RandBytes(4),
		}))
		if err != nil {
			log.Errorf("knowledge_enhance action error: %v", err)
			invoker.AddToTimeline("knowledge_enhance_error", "Cannot select existed kb base: "+err.Error())
			op.Exit()
			return
		}

		task := loop.GetCurrentTask()
		if task != nil && !utils.IsNil(task.GetContext()) {
			ctx = task.GetContext()
		}

		enhancedAnswer, err := invoker.EnhanceKnowledgeGetterEx(ctx, rewriteQuery, nil, selectKBResults.KnowledgeBases...)
		if err != nil {
			invoker.AddToTimeline("knowledge_enhance_insufficient",
				"Knowledge enhancement FAILED for query '"+rewriteQuery+"': "+err.Error()+". "+
					"The knowledge base could not provide results. "+
					"You MUST use web_search or internet_research to find the answer from the internet. "+
					"Do NOT retry knowledge_enhance_answer for the same query.")
			op.Exit()
			return
		}

		if enhancedAnswer == "" {
			invoker.AddToTimeline("knowledge_enhance_insufficient",
				"Knowledge enhancement for '"+rewriteQuery+"' returned EMPTY results. "+
					"The knowledge base does not contain relevant information for this topic. "+
					"You MUST use web_search or internet_research to find the answer from the internet. "+
					"Do NOT retry knowledge_enhance_answer for the same query.")
			op.Exit()
			return
		}

		result, err := invoker.CompressLongTextWithDestination(ctx, enhancedAnswer, rewriteQuery, 10*1024)
		if err != nil {
			invoker.AddToTimeline("knowledge_enhance_insufficient",
				"Knowledge enhancement compression FAILED for query '"+rewriteQuery+"': "+err.Error()+". "+
					"Consider using web_search or internet_research as alternative information sources.")
			op.Exit()
			return
		}
		enhancedAnswer = result

		invoker.AddToTimeline("enhanced_knowledge_content", enhancedAnswer)

		directlyAnswerResult, err := invoker.DirectlyAnswer(ctx, rewriteQuery, nil)
		if err != nil {
			log.Warnf("DirectlyAnswer failed after knowledge enhancement: %v", err)
			invoker.EmitFileArtifactWithExt("directly_answer", ".md", enhancedAnswer)
			invoker.EmitResultAfterStream(enhancedAnswer)
			directlyAnswerResult = enhancedAnswer
		}

		verifyResult, err := invoker.VerifyUserSatisfaction(
			ctx,
			rewriteQuery,
			false,
			directlyAnswerResult,
		)
		if err != nil {
			op.Fail(utils.Wrap(err, "knowledge_enhance action enhanced knowledge answer"))
			return
		}
		loop.PushSatisfactionRecordFromVerifyResult(verifyResult)

		if verifyResult.Satisfied {
			invoker.AddToTimeline("knowledge_enhance_satisfied", `** 知识增强结果已经初步满足用户需求(Knowledge enhancement results have initially met the user's needs) **`)
			op.Exit()
			return
		}

		nextStepsSummary := aicommon.FormatVerifyNextMovementsSummary(verifyResult.NextMovements)

		invoker.AddToTimeline("knowledge_enhance_not_satisfied",
			"Knowledge enhancement did NOT satisfy the query '"+rewriteQuery+"'. "+
				"Reasoning: "+verifyResult.Reasoning+". "+
				"Suggested next steps: "+nextStepsSummary+". "+
				"If the knowledge base lacks relevant information, "+
				"you MUST try web_search or internet_research to search the internet. "+
				"Do NOT retry knowledge_enhance_answer with the same approach.")
		op.Continue()
	},
}
