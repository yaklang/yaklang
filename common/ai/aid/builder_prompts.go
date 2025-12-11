package aid

import (
	"bytes"
	_ "embed"
	"text/template"

	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/plan/deepthink-plan.txt
var __prompt_DeepthinkTaskListPrompt string

//go:embed prompts/tool/tool-result-to-decision.txt
var __prompt_ToolResultToDecisionPromptTemplate string

//go:embed prompts/task/task-summary.txt
var __prompt_TaskSummary string

//go:embed prompts/report-finished.txt
var __prompt_ReportFinished string

//go:embed prompts/plan/dynamic-plan.txt
var __prompt_DynamicPlan string

//go:embed prompts/task/current_task_info.txt
var __prompt_currentTaskInfo string

//go:embed prompts/tool/tools-list.txt
var __prompt_ToolsList string

//go:embed prompts/plan-review/plan-incomplete.txt
var planReviewPrompts string

//go:embed prompts/plan-review/plan-freedom-review.txt
var planFreedomReviewPrompts string

//go:embed prompts/plan-review/plan-create-subtask.txt
var planReviewCreateSubtaskPrompts string

//go:embed prompts/plan/plan-help.txt
var __prompt_PlanHelp string

//go:embed prompts/tool/tool-re-select.txt
var __prompt_toolReSelect string

//go:embed prompts/tool/tool-param-regenerate.txt
var __prompt_ParamsReGenerate string

//go:embed prompts/search/aitool-keyword-search.txt
var __prompt_KeywordSearchPrompt string

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
