package loop_fast_context

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

const (
	loopVarUserQuery         = "fastcontext_user_query"
	loopVarWorkDir           = "fastcontext_work_dir"
	loopVarReferenceMaterial = "fastcontext_reference_material"
	loopVarEnvSnapshot       = "fastcontext_env_snapshot"
	loopVarSearchRounds      = "fastcontext_search_rounds"
	loopVarReport            = "fastcontext_report_json"
	defaultMaxIterations     = 12
)

func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
		query := strings.TrimSpace(loop.Get(loopVarUserQuery))
		if query == "" {
			query = strings.TrimSpace(task.GetUserInput())
		}
		loop.Set(loopVarUserQuery, query)
		loop.Set(loopVarSearchRounds, "0")
		loop.Set(loopVarFileIndex, "")
		loop.Set(loopVarReport, "")

		workDir := resolveWorkDir(r, loop.Get(loopVarWorkDir))
		loop.Set(loopVarWorkDir, workDir)
		snap := buildWorkEnvSnapshot(r, workDir)
		loop.Set(loopVarEnvSnapshot, snap)

		log.Infof("[FastContext] init query_len=%d workdir=%s", len(query), workDir)
		r.AddToTimeline("[FASTCONTEXT_START]", "query="+query+"\nworkdir="+workDir)
		op.Continue()
	}
}
