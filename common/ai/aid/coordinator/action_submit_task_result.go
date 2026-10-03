package coordinator

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type workerAttemptRef struct {
	TaskID      string `json:"task_id"`
	AttemptID   uint64 `json:"attempt_id"`
	PlanVersion uint64 `json:"plan_version"`
}

func actionSubmitTaskResult() reactloops.ReActLoopOption {
	options := []aitool.ToolOption{
		requiredString("summary", "本次执行尝试的实际结果摘要。"),
		aitool.WithStringArrayParam("artifacts", aitool.WithParam_Description("实际产出的文件或其他 artifacts 引用。")),
		aitool.WithStringArrayParam("evidence_ids", aitool.WithParam_Description("已经保存到 session 的真实 Evidence ID 列表。")),
	}
	return registerAction("submit_task_result", "提交本次执行尝试的结果；不代表任务验收通过，也不结束循环。", options, handleSubmitTaskResult)
}

func handleSubmitTaskResult(loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
	result := Result{Summary: a.GetString("summary"), Artifacts: a.GetStringSlice("artifacts"), EvidenceIDs: a.GetStringSlice("evidence_ids")}
	var err error
	if strings.TrimSpace(result.Summary) == "" {
		err = fmt.Errorf("result summary is required")
	}
	if !recordActionOutcome(loop, a, op, "submit_task_result", result, err) {
		return
	}
	if err == nil {
		loop.Set("coordinator_task_result", result)
	}
	op.Continue()
}
