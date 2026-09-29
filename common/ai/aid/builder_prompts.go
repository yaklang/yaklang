package aid

import (
	"bytes"
	"text/template"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/utils"
)

var __prompt_TaskSummaryInstruction = promptloader.MustLoad("ai/aid/prompts/task/task-summary_instruction.txt")

var __prompt_TaskSummaryOutputExample = promptloader.MustLoad("ai/aid/prompts/task/task-summary_output_example.txt")

var __prompt_TaskSummary = promptloader.MustLoad("ai/aid/prompts/task/task-summary.txt")

var __prompt_currentTaskInfoStable = promptloader.MustLoad("ai/aid/prompts/task/current_task_info/stable.txt")

var __prompt_currentTaskInfoDynamic = promptloader.MustLoad("ai/aid/prompts/task/current_task_info/dynamic.txt")

var __prompt_ToolsList = promptloader.MustLoad("ai/aid/prompts/tool/tools-list.txt")

var __prompt_PlanHelp = promptloader.MustLoad("ai/aid/prompts/plan/plan-help.txt")

var __prompt_KeywordSearchPrompt = promptloader.MustLoad("ai/aid/prompts/search/aitool-keyword-search.txt")

var __prompt_dynamicPlanInstruction = promptloader.MustLoad("ai/aid/prompts/plan/dynamic-plan/instruction.txt")

var __prompt_dynamicPlanDynamic = promptloader.MustLoad("ai/aid/prompts/plan/dynamic-plan/dynamic.txt")

var __prompt_planIncompleteInstruction = promptloader.MustLoad("ai/aid/prompts/plan-review/plan-incomplete/instruction.txt")

var __prompt_planIncompleteDynamic = promptloader.MustLoad("ai/aid/prompts/plan-review/plan-incomplete/dynamic.txt")

var __prompt_planFreedomReviewInstruction = promptloader.MustLoad("ai/aid/prompts/plan-review/plan-freedom-review/instruction.txt")

var __prompt_planFreedomReviewDynamic = promptloader.MustLoad("ai/aid/prompts/plan-review/plan-freedom-review/dynamic.txt")

var __prompt_planCreateSubtaskInstruction = promptloader.MustLoad("ai/aid/prompts/plan-review/plan-create-subtask/instruction.txt")

var __prompt_planCreateSubtaskDynamic = promptloader.MustLoad("ai/aid/prompts/plan-review/plan-create-subtask/dynamic.txt")

func (c *Coordinator) quickBuildPrompt(tmp string, i map[string]any) (string, error) {
	tmpl, err := template.New("prompt").Parse(tmp)
	if err != nil {
		return "", err
	}

	if utils.IsNil(i) {
		i = make(map[string]any)
		i["ContextProvider"] = c.ContextProvider
	}

	if _, ok := i["ContextProvider"]; !ok {
		i["ContextProvider"] = c.ContextProvider
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, i)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
