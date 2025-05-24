package aid

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed jsonschema/plan-or-interact.json
var planJsonSchema string

//go:embed jsonschema/re-plan.json
var rePlanSchema string

//go:embed jsonschema/task-summary.json
var taskSummarySchema string

//go:embed jsonschema/tool-desc-require.json
var toolDescRequireSchema string

//go:embed jsonschema/tool-execute-check.json
var toolExecuteCheckSchema string

//go:embed jsonschema/plan-review/create-subtask.json
var planReviewCreateSubtasksSchema string

func planJSONSchema(toolNames []string) map[string]string {
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
	res["PlanJsonSchema"] = planJsonSchema
	res["RePlanJsonSchema"] = rePlanSchema
	res["TaskSummarySchema"] = taskSummarySchema
	res["ToolDescRequireSchema"] = toolDescRequireSchemaBuilder.String()
	res["ToolExecuteCheckSchema"] = toolExecuteCheckSchema
	res["PlanCreateSubtaskSchema"] = planReviewCreateSubtasksSchema
	return res
}
