package aid

import _ "embed"

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

func taskJSONSchema() map[string]string {
	res := make(map[string]string)
	res["TaskJsonSchema"] = taskJsonSchema
	res["RePlanJsonSchema"] = rePlanSchema
	res["TaskSummarySchema"] = taskSummarySchema
	res["ToolDescRequireSchema"] = toolDescRequireSchema
	res["ToolExecuteCheckSchema"] = toolExecuteCheckSchema
	return res
}
