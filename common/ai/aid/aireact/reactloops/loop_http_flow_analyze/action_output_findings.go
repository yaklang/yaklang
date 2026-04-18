package loop_http_flow_analyze

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var outputFindingsAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"output_findings",
		"Record intermediate analysis findings to the FINDINGS document. Use this when you discover noteworthy patterns, suspicious traffic, security issues, or any valuable observations during the analysis process. FINDINGS accumulate across iterations and prevent redundant searches. You can output findings via JSON field 'findings' or via FINDINGS AITag.",
		[]aitool.ToolOption{
			aitool.WithStringParam(findingsFieldName,
				aitool.WithParam_Description("Analysis findings in Markdown format. Use ## headings to categorize (e.g. ## Suspicious Patterns, ## Security Issues). Content is merged with existing findings, duplicates are automatically removed."),
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
