package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func buildStatusTools(loop *reactloops.ReActLoop, names []string, state aicommon.StatusState) []aicommon.StatusTool {
	tools := make([]aicommon.StatusTool, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		label := loop.StatusToolLabel(name)
		statusTool := aicommon.StatusTool{Name: name, DisplayName: label.Zh, DisplayNameI18n: &label, State: state}
		tools = append(tools, statusTool)
	}
	return tools
}

func statusToolNames(tools []aicommon.StatusTool, english bool) string {
	const visibleLimit = 3
	type toolLabelGroup struct {
		label string
		count int
	}
	groups := make([]toolLabelGroup, 0, len(tools))
	indexes := make(map[string]int, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		candidates := []string{tool.DisplayName}
		if tool.DisplayNameI18n != nil {
			zh, en := tool.DisplayNameI18n.Zh, tool.DisplayNameI18n.En
			if english {
				candidates = []string{en, tool.DisplayName, zh}
			} else {
				candidates = []string{zh, tool.DisplayName, en}
			}
		}
		var label string
		for _, candidate := range candidates {
			if candidate = strings.TrimSpace(candidate); candidate != "" && candidate != name {
				label = candidate
				break
			}
		}
		if label == "" {
			label = "工具"
			if english {
				label = "tool"
			}
		}
		if name == "" {
			name = label
		}

		if index, ok := indexes[name]; ok {
			groups[index].count++
			continue
		}
		indexes[name] = len(groups)
		groups = append(groups, toolLabelGroup{label: label, count: 1})
	}
	labels := make([]string, 0, len(groups))
	for _, group := range groups {
		label := group.label
		if group.count > 1 {
			label = fmt.Sprintf("%s × %d", label, group.count)
		}
		labels = append(labels, label)
	}
	visible := labels
	if len(visible) > visibleLimit {
		visible = visible[:visibleLimit]
	}
	joined := strings.Join(visible, "、")
	if english {
		joined = strings.Join(visible, ", ")
	}
	if remaining := len(labels) - len(visible); remaining > 0 {
		if english {
			joined += fmt.Sprintf(" and %d more", remaining)
		} else {
			joined += fmt.Sprintf("等 %d 个工具", remaining)
		}
	}
	return joined
}

func emitToolsPreparingStatus(loop *reactloops.ReActLoop, names []string) {
	tools := buildStatusTools(loop, names, aicommon.StatusStateRunning)
	if len(tools) == 0 {
		return
	}
	if len(tools) == 1 {
		reactloops.EmitStatusI18n(
			loop,
			fmt.Sprintf("正在准备使用「%s」", statusToolNames(tools, false)),
			fmt.Sprintf("Preparing to use %s", statusToolNames(tools, true)),
			aicommon.WithStatusCode("tool.preparing"),
			aicommon.WithStatusTools(tools...),
		)
		return
	}
	reactloops.EmitStatusI18n(
		loop,
		fmt.Sprintf("正在准备调用 %d 个工具：%s", len(tools), statusToolNames(tools, false)),
		fmt.Sprintf("Preparing %d tools: %s", len(tools), statusToolNames(tools, true)),
		aicommon.WithStatusCode("tool.batch.preparing"),
		aicommon.WithStatusProgress(0, int64(len(tools)), "tool"),
		aicommon.WithStatusTools(tools...),
	)
}

func emitToolBatchRunningStatus(loop *reactloops.ReActLoop, names []string) {
	tools := buildStatusTools(loop, names, aicommon.StatusStateRunning)
	if len(tools) == 0 {
		return
	}
	reactloops.EmitStatusI18n(
		loop,
		fmt.Sprintf("正在调用 %d 个工具：%s", len(tools), statusToolNames(tools, false)),
		fmt.Sprintf("Calling %d tools: %s", len(tools), statusToolNames(tools, true)),
		aicommon.WithStatusCode("tool.batch.running"),
		aicommon.WithStatusProgress(0, int64(len(tools)), "tool"),
		aicommon.WithStatusTools(tools...),
	)
}

func buildToolCallGroupResultTools(
	loop *reactloops.ReActLoop,
	request *aicommon.ToolCallGroupRequest,
	outcomes []aicommon.ToolCallOutcome,
) ([]aicommon.StatusTool, int) {
	if request == nil {
		return nil, 0
	}
	outcomeByIndex := make(map[int]aicommon.ToolCallOutcome, len(outcomes))
	for _, outcome := range outcomes {
		outcomeByIndex[outcome.Index] = outcome
	}

	tools := make([]aicommon.StatusTool, 0, len(request.Calls))
	successful := 0
	for _, call := range request.Calls {
		name := call.ToolName
		state := aicommon.StatusStateWarning
		if outcome, ok := outcomeByIndex[call.Index]; ok {
			if outcome.FinalTool != "" {
				name = outcome.FinalTool
			}
			switch {
			case outcome.Result != nil && outcome.Result.Success:
				state = aicommon.StatusStateSuccess
				successful++
			case outcome.Err != nil,
				outcome.Stage == aicommon.ToolCallStagePrepareFailed,
				outcome.Stage == aicommon.ToolCallStageValidationFailed,
				outcome.Stage == aicommon.ToolCallStageInvokeFailed,
				outcome.Result != nil && outcome.Result.Error != "":
				state = aicommon.StatusStateError
			}
		}
		statusTools := buildStatusTools(loop, []string{name}, state)
		if len(statusTools) > 0 {
			tools = append(tools, statusTools[0])
		}
	}
	return tools, successful
}

func emitToolCallGroupResultStatus(
	loop *reactloops.ReActLoop,
	request *aicommon.ToolCallGroupRequest,
	outcomes []aicommon.ToolCallOutcome,
) {
	tools, successful := buildToolCallGroupResultTools(loop, request, outcomes)
	total := len(tools)
	if total == 0 {
		return
	}

	state := aicommon.StatusStateWarning
	code := "tool.batch.partial"
	zh := fmt.Sprintf("%d/%d 次调用已完成：%s", successful, total, statusToolNames(tools, false))
	en := fmt.Sprintf("%d of %d calls completed: %s", successful, total, statusToolNames(tools, true))
	if successful == total {
		state = aicommon.StatusStateSuccess
		code = "tool.batch.completed"
		zh = fmt.Sprintf("%d 次调用已完成：%s", total, statusToolNames(tools, false))
		en = fmt.Sprintf("All %d calls completed: %s", total, statusToolNames(tools, true))
	} else if successful == 0 {
		state = aicommon.StatusStateError
		code = "tool.batch.failed"
		zh = "这轮调用未完成：" + statusToolNames(tools, false)
		en = "These calls did not complete: " + statusToolNames(tools, true)
	}
	reactloops.EmitStatusI18n(
		loop,
		zh,
		en,
		aicommon.WithStatusCode(code),
		aicommon.WithStatusState(state),
		aicommon.WithStatusProgress(int64(successful), int64(total), "tool"),
		aicommon.WithStatusTools(tools...),
	)
}

func emitToolResultStatus(loop *reactloops.ReActLoop, name string, success bool) {
	state := aicommon.StatusStateError
	code := "tool.failed"
	tools := buildStatusTools(loop, []string{name}, state)
	if len(tools) == 0 {
		return
	}
	zhName := statusToolNames(tools, false)
	enName := statusToolNames(tools, true)
	zh := fmt.Sprintf("「%s」暂时没能完成这一步", zhName)
	en := fmt.Sprintf("%s could not complete this step", enName)
	if success {
		state = aicommon.StatusStateSuccess
		code = "tool.completed"
		tools[0].State = state
		zh = fmt.Sprintf("%s 调用已完成", zhName)
		en = fmt.Sprintf("%s call completed", enName)
	}
	reactloops.EmitStatusI18n(
		loop,
		zh,
		en,
		aicommon.WithStatusCode(code),
		aicommon.WithStatusState(state),
		aicommon.WithStatusTools(tools...),
	)
}

func emitToolRetryStatus(loop *reactloops.ReActLoop, name string) {
	tools := buildStatusTools(loop, []string{name}, aicommon.StatusStateRecovering)
	if len(tools) == 0 {
		return
	}
	reactloops.EmitStatusI18n(loop,
		fmt.Sprintf("正在调整 %s 的调用", statusToolNames(tools, false)),
		fmt.Sprintf("Revising the call to %s", statusToolNames(tools, true)),
		aicommon.WithStatusCode("tool.retrying"),
		aicommon.WithStatusState(aicommon.StatusStateRecovering),
		aicommon.WithStatusTools(tools...),
	)
}
