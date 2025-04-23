package aid

import (
	_ "embed"
	"fmt"
	"html/template"
	"strings"
)

//go:embed jsonschema/task.json
var taskJsonSchema string

//go:embed jsonschema/re-plan.json
var rePlanSchema string

//go:embed jsonschema/task-summary.json
var taskSummarySchema string

//go:embed jsonschema/tool-desc-require.json
var toolDescRequireSchema string

//go:embed jsonschema/tool-execute-check.json
var toolExecuteCheckSchema string

func taskJSONSchema(toolNames []string) map[string]string {
	var toolNamesStrs []string
	for _, toolName := range toolNames {
		toolNamesStrs = append(toolNamesStrs, fmt.Sprintf("\"%s\"", toolName))
	}
	toolDescRequireSchemaTmp := template.Must(template.New("tool-desc-require").Parse(toolDescRequireSchema))
	var toolDescRequireSchemaBuilder strings.Builder
	toolDescRequireSchemaTmp.Execute(&toolDescRequireSchemaBuilder, map[string]any{
		"ToolsList": strings.Join(toolNamesStrs, ", "),
	})
	res := make(map[string]string)
	res["TaskJsonSchema"] = taskJsonSchema
	res["RePlanJsonSchema"] = rePlanSchema
	res["TaskSummarySchema"] = taskSummarySchema
	res["ToolDescRequireSchema"] = toolDescRequireSchemaBuilder.String()
	res["ToolExecuteCheckSchema"] = toolExecuteCheckSchema
	return res
}
