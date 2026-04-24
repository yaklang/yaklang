package loop_syntaxflow_scan

import (
	"bytes"
	_ "embed"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/persistent_instruction.txt
var persistentInstruction string

//go:embed prompts/reactive_data.txt
var reactiveData string

//go:embed prompts/reflection_output_example.txt
var outputExample string

// interpretLoopName is a package-scoped sub-loop; no global schema constant required (same pattern as code_audit internal phases).
const interpretLoopName = "syntaxflow_scan_interpret"

// buildPhaseInterpretLoop builds the ReAct loop for risk interpretation, reload tools, and optional final report.
func buildPhaseInterpretLoop(r aicommon.AIInvokeRuntime, extra ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
	preset := []reactloops.ReActLoopOption{
		reactloops.WithInitTask(buildInterpretEngineInitTask(r)),
		buildInterpretPostIterationHook(r),
		reactloops.WithOverrideLoopAction(loopActionDirectlyAnswerSyntaxflowScan),
		reactloops.WithAllowRAG(true),
		reactloops.WithAllowToolCall(true),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
		reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
		reactloops.WithPersistentInstruction(persistentInstruction),
		reactloops.WithReflectionOutputExample(outputExample + sfu.ReflectionOutputSharedAppendix),
		WithReloadSyntaxFlowScanSessionAction(r),
		WithReloadSSARiskOverviewAction(r),
		WithSetSSARiskReviewTargetAction(r),
		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
			fb := strings.TrimSpace(feedbacker.String())
			return utils.RenderTemplate(reactiveData, map[string]any{
				"Preface":          loop.Get("sf_scan_review_preface"),
				"TaskID":           loop.Get("sf_scan_task_id"),
				"SessionMode":      loop.Get("sf_scan_session_mode"),
				"ConfigInferred":   loop.Get("sf_scan_config_inferred"),
				"FinalReportDue":   loop.Get(sfu.LoopVarSFFinalReportDue),
				"CompileMeta":      loop.Get(sfu.LoopVarSFCompileMeta),
				"PipelineSummary":  loop.Get(sfu.LoopVarSFPipelineSummary),
				"ScanEndSummary":   loop.Get(sfu.LoopVarSFScanEndSummary),
				"FindingsDoc":      loop.Get("sf_scan_findings_doc"),
				"InterpretLog":     loop.Get(LoopVarInterpretLog),
				"RiskListSummary":  loop.Get("ssa_risk_list_summary"),
				"RiskTotalHint":    loop.Get("ssa_risk_total_hint"),
				"FeedbackMessages": fb,
				"Nonce":            nonce,
			})
		}),
	}
	preset = append(preset, extra...)
	return reactloops.NewReActLoop(interpretLoopName, r, preset...)
}
