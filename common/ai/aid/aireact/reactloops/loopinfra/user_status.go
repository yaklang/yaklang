package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

func buildStatusTools(loop *reactloops.ReActLoop, names []string, state aicommon.StatusState) []aicommon.StatusTool {
	tools := make([]aicommon.StatusTool, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		statusTool := aicommon.StatusTool{Name: name, DisplayName: name, State: state}
		if loop != nil && loop.GetConfig() != nil && loop.GetConfig().GetAiToolManager() != nil {
			if tool, err := loop.GetConfig().GetAiToolManager().GetToolByName(name); err == nil && tool != nil {
				zh := strings.TrimSpace(tool.GetVerboseNameZh())
				en := strings.TrimSpace(tool.GetVerboseName())
				if zh != "" {
					statusTool.DisplayName = zh
				} else if en != "" {
					statusTool.DisplayName = en
				}
				if zh != "" || en != "" {
					statusTool.DisplayNameI18n = &schema.I18n{Zh: zh, En: en}
				}
			}
		}
		tools = append(tools, statusTool)
	}
	return tools
}

func statusToolNames(tools []aicommon.StatusTool, english bool) string {
	const visibleLimit = 3
	labels := make([]string, 0, len(tools))
	for _, tool := range tools {
		label := tool.DisplayName
		if english && tool.DisplayNameI18n != nil && strings.TrimSpace(tool.DisplayNameI18n.En) != "" {
			label = tool.DisplayNameI18n.En
		}
		if strings.TrimSpace(label) == "" {
			label = tool.Name
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

func buildToolBatchResultTools(
	loop *reactloops.ReActLoop,
	request *aicommon.ToolBatchRequest,
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

func emitToolBatchResultStatus(
	loop *reactloops.ReActLoop,
	request *aicommon.ToolBatchRequest,
	outcomes []aicommon.ToolCallOutcome,
) {
	tools, successful := buildToolBatchResultTools(loop, request, outcomes)
	total := len(tools)
	if total == 0 {
		return
	}

	state := aicommon.StatusStateWarning
	code := "tool.batch.partial"
	zh := fmt.Sprintf("%d 个工具中有 %d 个已完成，正在整理可用结果", total, successful)
	en := fmt.Sprintf("%d of %d tools completed; organizing the available results", successful, total)
	if successful == total {
		state = aicommon.StatusStateSuccess
		code = "tool.batch.completed"
		zh = fmt.Sprintf("%d 个工具已完成，正在整理结果", total)
		en = fmt.Sprintf("All %d tools completed; organizing the results", total)
	} else if successful == 0 {
		state = aicommon.StatusStateError
		code = "tool.batch.failed"
		zh = "这批工具暂时没能完成，正在调整"
		en = "This tool batch could not complete; adjusting the approach"
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
		zh = fmt.Sprintf("「%s」已经完成，正在整理结果", zhName)
		en = fmt.Sprintf("%s has finished; organizing the results", enName)
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
