package loop_scan_risk_analysis

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func makeScanRiskAnalysisAction(aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	desc := "Run the scan risk pipeline: load/merge by scan_id, false-positive triage, Markdown/JSON artifacts (no automatic PoC). Call after Init set scan_id; ends focused loop on success."

	toolOpts := []aitool.ToolOption{
		aitool.WithStringParam("cumulative_summary",
			aitool.WithParam_Description("Optional short summary for this step."),
			aitool.WithParam_Required(false)),
	}

	streamFields := []*reactloops.LoopStreamField{
		{FieldName: "human_readable_thought", AINodeId: "re-act-loop-thought"},
	}

	return reactloops.WithRegisterLoopActionWithStreamField(
		schema.AI_REACT_LOOP_NAME_SCAN_RISK_ANALYSIS,
		desc,
		toolOpts,
		streamFields,
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(loop.Get("scan_id")) == "" {
				return utils.Error("scan_id missing: init should have set loop variable scan_id after project resolution")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, _ *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			loop.LoadingStatus("running scan risk analysis pipeline")
			invoker := loop.GetInvoker()
			scanID := strings.TrimSpace(loop.Get("scan_id"))
			workDir := strings.TrimSpace(loop.Get("scan_risk_workdir"))
			thoughtKey := ""
			if t := loop.GetCurrentTask(); t != nil {
				thoughtKey = strings.TrimSpace(t.GetIndex())
			}
			if workDir == "" {
				if cfg, ok := invoker.GetConfig().(interface{ GetOrCreateWorkDir() string }); ok {
					workDir = cfg.GetOrCreateWorkDir()
				}
			}
			if workDir == "" {
				op.Fail("scan_risk_workdir is empty and config has no workdir")
				return
			}
			executeScanRiskAnalysisPipeline(invoker, op, workDir, scanID, thoughtKey)
		},
	)
}
