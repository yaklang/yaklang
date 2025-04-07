package aid

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed prompts/plan-to-task-list.txt
var __prompt_GenerateTaskListPrompt string

//go:embed jsonschema/task.json
var __prompt_TaskJsonSchema string

//go:embed prompts/task-execute.txt
var __prompt_ExecuteTaskPromptTemplate string

//go:embed prompts/tool-param-schema.txt
var __prompt_ToolParamSchemaPromptTemplate string

//go:embed prompts/tool-result-to-decision.txt
var __prompt_ToolResultToDecisionPromptTemplate string

//go:embed prompts/tool-result-history.txt
var __prompt_ToolResultHistoryPromptTemplate string

//go:embed prompts/task-summary.txt
var __prompt_TaskSummary string

//go:embed prompts/report-finished.txt
var __prompt_ReportFinished string

//go:embed prompts/dynamic-plan.txt
var __prompt_DynamicPlan string

//go:embed prompts/plan-review/plan-incomplete.txt
var planReviewPrompts string

var (
	__prompt_SUMMARY_TEMPLATE = template.Must(template.New("summary").Parse(__prompt_TaskSummary))
)

func GetAITaskJSONSchema() map[string]string {
	res := make(map[string]string)
	res["TaskJsonSchema"] = __prompt_TaskJsonSchema
	return res
}

func GenerateTaskSummaryPrompt(text string) (string, error) {
	var buf bytes.Buffer
	err := __prompt_SUMMARY_TEMPLATE.Execute(&buf, map[string]string{
		"Text": text,
	})
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
