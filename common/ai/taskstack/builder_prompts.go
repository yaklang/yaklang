package taskstack

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed prompts/plan-to-task-list.txt
var generateTaskListPrompt string

//go:embed jsonschema/task.json
var taskJsonSchema string

//go:embed prompts/execute-task.txt
var executeTaskPromptTemplate string

//go:embed prompts/tool-param-schema.txt
var toolParamSchemaPromptTemplate string

//go:embed prompts/tool-result-to-decision.txt
var toolResultToDecisionPromptTemplate string

//go:embed prompts/tool-result-history.txt
var toolResultHistoryPromptTemplate string

//go:embed prompts/summary.txt
var __prompt_SUMMARY string

var (
	__prompt_SUMMARY_TEMPLATE = template.Must(template.New("summary").Parse(__prompt_SUMMARY))
)

func GenerateSummaryPrompt(text string) (string, error) {
	var buf bytes.Buffer
	err := __prompt_SUMMARY_TEMPLATE.Execute(&buf, map[string]string{
		"Text": text,
	})
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
