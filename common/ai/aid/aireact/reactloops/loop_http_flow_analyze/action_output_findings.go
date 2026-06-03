package loop_http_flow_analyze

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const outputFindingsActionDescription = "Persist reusable intermediate findings to the FINDINGS document. Use this SPARINGLY only when you have a durable observation worth carrying across later iterations, such as a confirmed suspicious pattern, cross-flow correlation, evidence gap, or rationale for dispatching fuzz tests. Provide the content either via the JSON field `findings` or the `FINDINGS` AITag block, but ONLY when `@action=\"output_findings\"`. NEVER attach `findings` to `directly_answer`, `finish`, `filter_and_match_http_flows`, `match_http_flows_with_matcher`, `get_http_flow_detail`, or `dispatch_fuzz_test`. Do not dump ordinary query results or duplicate final-answer prose here."

const outputFindingsParamDescription = "Reusable findings in Markdown format. USE THIS FIELD ONLY IF `@action` IS `output_findings`. Keep it concise and durable: record stable conclusions, correlations, evidence gaps, or fuzz rationale. Do not use it for raw hit dumps, routine search summaries, or final answer text. Use ## headings to categorize (for example: ## Suspicious Patterns, ## Security Issues). Content is merged with existing findings and duplicates are removed automatically."

var outputFindingsAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"output_findings",
		outputFindingsActionDescription,
		[]aitool.ToolOption{
			aitool.WithStringParam(findingsFieldName,
				aitool.WithParam_Description(outputFindingsParamDescription),
				aitool.WithParam_Required(true),
			),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName:   findingsFieldName,
				AINodeId:    findingsAINodeID,
				ContentType: aicommon.TypeTextMarkdown,
			},
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			content := normalizeFindings(action.GetString(findingsFieldName))
			if content == "" {
				return utils.Error("output_findings: findings content is required, either via JSON field 'findings' or FINDINGS AITag block")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			incoming := normalizeFindings(action.GetString(findingsFieldName))
			_, changed := appendFindings(loop, incoming)
			if changed {
				log.Infof("http_flow_analyze: output_findings merged, length=%d", len(incoming))
			} else {
				log.Infof("http_flow_analyze: output_findings received no new findings")
			}

			emitter := loop.GetEmitter()
			taskID := ""
			if task := loop.GetCurrentTask(); task != nil {
				taskID = task.GetId()
			}
			emitter.EmitThoughtStream(taskID, "Recorded analysis findings (%d chars)", len(incoming))

			recordMetaAction(loop, "output_findings",
				"recorded analysis findings",
				utils.ShrinkTextBlock(incoming, 200))
			op.Continue()
		},
	)
}
