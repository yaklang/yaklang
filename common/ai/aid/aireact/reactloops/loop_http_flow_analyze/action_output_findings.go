package loop_http_flow_analyze

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const recordHTTPFlowEvidenceActionDescription = "Persist reusable HTTP flow analysis evidence to the HTTP_FLOW_EVIDENCE document. Use this SPARINGLY only when you have durable evidence worth carrying across later iterations, such as a confirmed suspicious pattern, cross-flow correlation, evidence gap, or rationale for dispatching fuzz tests. Provide the content either via the JSON field `http_flow_evidence` or the `HTTP_FLOW_EVIDENCE` AITag block, but ONLY when `@action=\"record_http_flow_evidence\"`. NEVER attach `http_flow_evidence` to `directly_answer`, `finish`, `filter_and_match_http_flows`, `match_http_flows_with_matcher`, `get_http_flow_detail`, or `dispatch_fuzz_test`. Do not dump ordinary query results or duplicate final-answer prose here."

const httpFlowEvidenceParamDescription = "Reusable HTTP flow analysis evidence in Markdown format. USE THIS FIELD ONLY IF `@action` IS `record_http_flow_evidence`. Keep it concise and durable: record stable conclusions, correlations, evidence gaps, or fuzz rationale. Do not use it for raw hit dumps, routine search summaries, or final answer text. Use ## headings to categorize evidence (for example: ## Suspicious Patterns, ## Authentication Chain). Content is merged with existing evidence and duplicates are removed automatically."

var recordHTTPFlowEvidenceAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		httpFlowEvidenceActionName,
		recordHTTPFlowEvidenceActionDescription,
		[]aitool.ToolOption{
			aitool.WithStringParam(httpFlowEvidenceFieldName,
				aitool.WithParam_Description(httpFlowEvidenceParamDescription),
				aitool.WithParam_Required(true),
			),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName:   httpFlowEvidenceFieldName,
				AINodeId:    httpFlowEvidenceAINodeID,
				ContentType: aicommon.TypeTextMarkdown,
			},
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			content := normalizeHTTPFlowEvidence(action.GetString(httpFlowEvidenceFieldName))
			if content == "" {
				return utils.Error("record_http_flow_evidence: http_flow_evidence content is required, either via JSON field 'http_flow_evidence' or HTTP_FLOW_EVIDENCE AITag block")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			incoming := normalizeHTTPFlowEvidence(action.GetString(httpFlowEvidenceFieldName))
			_, changed := appendHTTPFlowEvidence(loop, incoming)
			if changed {
				log.Infof("http_flow_analyze: record_http_flow_evidence merged, length=%d", len(incoming))
			} else {
				log.Infof("http_flow_analyze: record_http_flow_evidence received no new evidence")
			}

			emitter := loop.GetEmitter()
			taskID := ""
			if task := loop.GetCurrentTask(); task != nil {
				taskID = task.GetId()
			}
			emitter.EmitThoughtStream(taskID, "Recorded HTTP flow evidence (%d chars)", len(incoming))

			recordMetaAction(loop, httpFlowEvidenceActionName,
				"recorded HTTP flow evidence",
				utils.ShrinkTextBlock(incoming, 200))
			op.Continue()
		},
	)
}
