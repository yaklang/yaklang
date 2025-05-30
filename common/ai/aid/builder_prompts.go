package aid

import (
	"bytes"
	_ "embed"
	"text/template"

	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/plan/plan-to-task-list.txt
var __prompt_GenerateTaskListPrompt string

//go:embed prompts/plan/plan-to-task-list-with-user-interact.txt
var __prompt_GenerateTaskListPromptWithUserInteract string

//go:embed prompts/task/task-execute.txt
var __prompt_ExecuteTaskPromptTemplate string

//go:embed prompts/tool/tool-param-schema.txt
var __prompt_ToolParamSchemaPromptTemplate string

//go:embed prompts/tool/tool-result-to-decision.txt
var __prompt_ToolResultToDecisionPromptTemplate string

//go:embed prompts/tool/tool-result-history.txt
var __prompt_ToolResultHistoryPromptTemplate string

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

//go:embed prompts/plan-review/plan-create-subtask.txt
var planReviewCreateSubtaskPrompts string

//go:embed prompts/plan/plan-help.txt
var __prompt_PlanHelp string

//go:embed prompts/tool/tool-re-select.txt
var __prompt_toolReSelect string

//go:embed prompts/search/aitool-keyword-search.txt
var __prompt_KeywordSearchPrompt string

func (c *Config) quickBuildPrompt(tmp string, i map[string]any) (string, error) {
	tmpl, err := template.New("prompt").Parse(tmp)
	if err != nil {
		return "", err
	}

	if utils.IsNil(i) {
		i = make(map[string]any)
		i["Memory"] = c.memory
	}

	if _, ok := i["Memory"]; !ok {
		i["Memory"] = c.memory
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, i)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
