package aid

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
)

var planJsonSchema = promptloader.MustLoad("ai/aid/jsonschema/plan/plan.json")

var planWithUserInteractJsonSchema = promptloader.MustLoad("ai/aid/jsonschema/plan/plan-or-interact.json")

var rePlanSchema = promptloader.MustLoad("ai/aid/jsonschema/plan/re-plan.json")

var taskSummarySchema = promptloader.MustLoad("ai/aid/jsonschema/task/task-summary.json")

var toolDescRequireSchema = promptloader.MustLoad("ai/aid/jsonschema/tool/tool-desc-require.json")

var directAnswerSchema = promptloader.MustLoad("ai/aid/jsonschema/task/task-direct-answer.json")

var toolExecuteCheckSchema = promptloader.MustLoad("ai/aid/jsonschema/tool/tool-execute-check.json")

var toolExecuteCheckSchemaWithoutContinue = promptloader.MustLoad("ai/aid/jsonschema/tool/tool-execute-check-without-continue.json")

var planReviewCreateSubtasksSchema = promptloader.MustLoad("ai/aid/jsonschema/plan-review/create-subtask.json")

var keywordSearchSchema = promptloader.MustLoad("ai/aid/jsonschema/search/keyword_search.json")

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
	res["PlanWithUserInteractJsonSchema"] = planWithUserInteractJsonSchema
	res["RePlanJsonSchema"] = rePlanSchema
	res["TaskSummarySchema"] = taskSummarySchema
	res["ToolDescRequireSchema"] = toolDescRequireSchemaBuilder.String()
	res["ToolExecuteCheckSchema"] = toolExecuteCheckSchema
	res["ToolExecuteCheckSchemaWithoutContinue"] = toolExecuteCheckSchemaWithoutContinue
	res["PlanCreateSubtaskSchema"] = planReviewCreateSubtasksSchema
	res["KeywordSearchSchema"] = keywordSearchSchema
	res["DirectAnswerSchema"] = directAnswerSchema
	return res
}
