package loopinfra

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func subAgentControlAction(name, description string) *reactloops.LoopAction {
	options := []aitool.ToolOption{
		aitool.WithStringArrayParam("job_ids", aitool.WithParam_Description("IDs from the dispatch receipt. Omit to select all jobs owned by this parent. Unknown IDs are errors.")),
	}
	if name == reactloops.SubAgentWaitAction {
		options = append(options, aitool.WithIntegerParam("timeout_ms", aitool.WithParam_Description(fmt.Sprintf("Observation deadline in milliseconds, 0..%d (10 minutes); default 30000. Does not cancel the child task.", reactloops.SubAgentMaxWaitTimeout.Milliseconds()))))
	}
	return &reactloops.LoopAction{
		ActionType: name, Description: description, Options: options,
		ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if getSubAgentDepth(loop) > 0 {
				return fmt.Errorf("sub-agent controls are parent-only")
			}
			// A malformed selector must never silently become "all jobs",
			// especially for cancellation.
			if raw, exists := action.GetParams()["job_ids"]; exists {
				encoded, err := json.Marshal(raw)
				var ids []string
				if err != nil || json.Unmarshal(encoded, &ids) != nil || ids == nil {
					return fmt.Errorf("job_ids must be an array of strings")
				}
				for _, id := range ids {
					if strings.TrimSpace(id) == "" {
						return fmt.Errorf("job_ids cannot contain an empty ID")
					}
				}
			}
			if name == reactloops.SubAgentWaitAction {
				if raw, exists := action.GetParams()["timeout_ms"]; exists {
					encoded, err := json.Marshal(raw)
					var timeout *int
					if err != nil || json.Unmarshal(encoded, &timeout) != nil || timeout == nil || *timeout < 0 || int64(*timeout) > reactloops.SubAgentMaxWaitTimeout.Milliseconds() {
						return fmt.Errorf("timeout_ms must be an integer between 0 and %d", reactloops.SubAgentMaxWaitTimeout.Milliseconds())
					}
				}
			}
			return nil
		},
		ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			manager := loop.GetSubAgentManager()
			if manager == nil {
				op.Feedback("No sub-agents have been dispatched by this parent.")
				op.Continue()
				return
			}
			ids := action.GetStringSlice("job_ids")
			var result any
			var err error
			var waitLog io.WriteCloser
			var observed []reactloops.SubAgentSnapshot
			switch name {
			case reactloops.SubAgentInspectAction:
				observed, err = manager.Inspect(ids)
				result = observed
			case reactloops.SubAgentCancelAction:
				observed, err = manager.Cancel(ids)
				result = observed
			case reactloops.SubAgentWaitAction:
				timeout := 30000
				if _, exists := action.GetParams()["timeout_ms"]; exists {
					timeout = action.GetInt("timeout_ms")
				}
				if timeout < 0 || int64(timeout) > reactloops.SubAgentMaxWaitTimeout.Milliseconds() {
					err = fmt.Errorf("timeout_ms must be between 0 and %d", reactloops.SubAgentMaxWaitTimeout.Milliseconds())
					break
				}
				// Validate before announcing a wait. A dedicated visible stream keeps
				// adjacent reasoning streams from grouping across a blocking action.
				observed, err = manager.Inspect(ids)
				if err != nil {
					break
				}
				zh := fmt.Sprintf("正在等待 %d 个子任务的结果（最多 %g 秒，有结果将提前返回）。", len(observed), float64(timeout)/1000)
				reactloops.EmitStatusI18n(loop, zh, "Waiting for sub-task results; new results return early",
					aicommon.WithStatusCode("subagent.waiting"), aicommon.WithStatusState(aicommon.StatusStateWaiting))
				waitLog = startSubAgentWaitLog(loop, zh)
				defer waitLog.Close()
				observation, waitErr := manager.Wait(op.GetTask().GetContext(), ids, time.Duration(timeout)*time.Millisecond, loop.SubAgentModelSeenRevision())
				result, err = observation, waitErr
				if observation != nil {
					observed = observation.Jobs
				}
			}
			if err != nil {
				op.Feedback("Sub-agent observation: " + err.Error())
				zh := "子任务状态操作未完成，请查看任务当前状态。"
				if waitLog != nil {
					_, _ = io.WriteString(waitLog, "\n"+zh)
				}
				reactloops.EmitStatusI18n(loop, zh, "The sub-task operation did not complete; check the current task state",
					aicommon.WithStatusCode("subagent.observation_failed"), aicommon.WithStatusState(aicommon.StatusStateWarning))
			} else {
				raw, _ := json.Marshal(result)
				op.Feedback(string(raw))
				zh, en, settled := subAgentObservationSummary(observed)
				if observation, ok := result.(*reactloops.SubAgentObservation); ok && observation.TimedOut {
					zh = "本次等待已到时，子任务继续执行。" + zh
					en = "The observation deadline elapsed; child tasks continue. " + en
				}
				if name == reactloops.SubAgentCancelAction {
					zh = "已请求取消所选子任务。" + zh
					en = "Cancellation requested for the selected sub-tasks. " + en
				}
				if waitLog != nil {
					_, _ = io.WriteString(waitLog, "\n"+zh)
				} else {
					loopInfraActionFinish(loop, loopInfraNodeSubReactReport, zh)
				}
				reactloops.EmitStatusI18n(loop, zh, en, aicommon.WithStatusCode("subagent.observed"),
					aicommon.WithStatusProgress(int64(settled), int64(len(observed)), "task"))
			}
			op.Continue()
		},
	}
}

// Only display factual lifecycle counts; a delivered answer is not a terminal
// child result, and a finished child is not necessarily a successful one.
func subAgentObservationSummary(jobs []reactloops.SubAgentSnapshot) (string, string, int) {
	counts := make(map[string]int)
	for _, job := range jobs {
		counts[job.State]++
	}
	var zh, en []string
	settled := 0
	for _, state := range []struct{ key, zh, en string }{
		{"queued", "排队中", "queued"}, {"preparing", "准备中", "preparing"},
		{"running", "执行中", "running"}, {"cancelling", "取消中", "cancelling"},
		{"completed", "已完成", "completed"}, {"failed", "失败", "failed"},
		{"cancelled", "已取消", "cancelled"}, {"timed_out", "执行超时", "execution timed out"},
	} {
		count := counts[state.key]
		if count == 0 {
			continue
		}
		zh = append(zh, fmt.Sprintf("%d 个%s", count, state.zh))
		en = append(en, fmt.Sprintf("%d %s", count, state.en))
		switch state.key {
		case "completed", "failed", "cancelled", "timed_out":
			settled += count
		}
	}
	if len(jobs) == 0 {
		return "没有选中的子任务。", "No sub-tasks selected.", 0
	}
	return "子任务状态：" + strings.Join(zh, "，") + "。", "Sub-tasks: " + strings.Join(en, ", ") + ".", settled
}

func startSubAgentWaitLog(loop *reactloops.ReActLoop, message string) io.WriteCloser {
	reader, writer := io.Pipe()
	if loop.GetEmitter() == nil {
		_ = reader.Close()
		return writer
	}
	taskID := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskID = task.GetId()
	}
	if _, err := loop.GetEmitter().EmitDefaultStreamEvent("sub_react_agents_wait", reader, taskID, func() { _ = reader.Close() }); err != nil {
		_ = reader.CloseWithError(err)
	}
	_, _ = io.WriteString(writer, message)
	return writer
}

var loopAction_InspectSubAgents = subAgentControlAction(reactloops.SubAgentInspectAction,
	"Inspect queued, running and terminal jobs without waiting. Reports actual recent event activity and saved result references, not a guessed completion percentage. Prefer independent work over repeated unchanged polling.")
var loopAction_WaitSubAgents = subAgentControlAction(reactloops.SubAgentWaitAction,
	"Wait for new terminal sub-agent results, at most 30 seconds by default. Returns early on completion or parent cancellation. A timeout returns current progress and leaves child execution running. Use when no useful independent work remains, then decide again.")
var loopAction_CancelSubAgents = subAgentControlAction(reactloops.SubAgentCancelAction,
	"Request cancellation of selected child jobs. Running jobs remain cancelling until their workers stop; queued jobs will not execute. Inspect or wait for the terminal result before finishing.")
