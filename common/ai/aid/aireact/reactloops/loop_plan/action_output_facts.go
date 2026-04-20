package loop_plan

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var outputFactsAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	_ = r
	return reactloops.WithRegisterLoopActionWithStreamField(
		"output_facts",
		"Append newly observed concrete facts into the shared FACTS document. Prefer the FACTS AITag format: output {\"@action\":\"output_facts\"} and then emit <|FACTS_nonce|>...<|FACTS_END_nonce|>. Facts must be Markdown and contain only precise, verifiable values.",
		[]aitool.ToolOption{
			aitool.WithStringParam("facts",
				aitool.WithParam_Description("本轮新增 facts 的 Markdown 文本。系统会自动与历史 FACTS 合并。也可以不在 JSON 中传递该字段，而是使用 FACTS AITag 输出。"),
			),
		},
		[]*reactloops.LoopStreamField{},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			facts := normalizeFactsDocument(action.GetString(PlanFactsFieldName))
			if facts == "" {
				return utils.Error("output_facts: facts content is required, either via JSON field 'facts' or FACTS AITag block")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			facts := normalizeFactsDocument(action.GetString(PlanFactsFieldName))
			merged, changed := appendPlanFacts(loop, facts)
			if changed {
				log.Infof("plan loop: output_facts merged, length=%d", len(merged))
			} else {
				log.Infof("plan loop: output_facts received no new facts")
			}
			op.Continue()
		},
	)
}
