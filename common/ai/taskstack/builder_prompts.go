package taskstack

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed prompts/generate-tasklist.txt
var __prompt_GENERATE_TASKLIST string

//go:embed jsonschema/task.json
var taskJsonSchema string

//go:embed prompts/execute-task.txt
var executeTaskPromptTemplate string

//go:embed prompts/describe-tool.txt
var describeToolPromptTemplate string

//go:embed prompts/tool-result.txt
var toolResultPromptTemplate string

//go:embed prompts/summary.txt
var __prompt_SUMMARY string

//go:embed prompts/require-more-tool.txt
var __prompt_REQUIRE_MORE_TOOL string

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
