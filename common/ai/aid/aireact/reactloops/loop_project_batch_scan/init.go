package loop_project_batch_scan

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfa "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_actions"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_PROJECT_BATCH_SCAN,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithPersistentInstruction("You coordinate batch SyntaxFlow scans across many SSA programs. Prefer list_ssa_projects / resolve_ssa_projects, then schedule syntaxflow_scan or server-side BulkScan jobs."),
				reactloops.WithInitTask(func(_ *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
					ui := utils.ShrinkTextBlock(task.GetUserInput(), 2000)
					msg := "[project_batch_scan] MVP entry: compose multiple syntaxflow_scan invocations via list_ssa_projects + yak orchestration server-side; input logged: " + ui
					r.AddToTimeline("project_batch_scan", msg)
					op.Continue()
				}),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				sfa.WithListSSAProjectsAction(r),
				sfa.WithResolveSSAProjectsAction(r),
				sfa.WithCompileSSAProjectAction(r),
				sfa.WithProjectBatchScanHintAction(r),
			}
			preset = append(preset, reactloops.WithAllowToolCall(true), reactloops.WithAllowRAG(false))
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_PROJECT_BATCH_SCAN, r, preset...)
		},
		reactloops.WithVerboseName("SyntaxFlow · Project batch scan"),
		reactloops.WithLoopDescription("Lists/projects handoff scaffold for concurrent scans across many SSA programs."),
	)
	if err != nil {
		log.Errorf("register reactloop %v failed: %v", schema.AI_REACT_LOOP_NAME_PROJECT_BATCH_SCAN, err)
	}
}
