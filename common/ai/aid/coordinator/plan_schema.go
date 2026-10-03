package coordinator

import "github.com/yaklang/yaklang/common/ai/aid/aitool"

// planParameter retains the historical plan tree vocabulary in native function
// arguments. Groups are structural; only leaves become execution tasks.
func planParameter() aitool.ToolOption {
	return aitool.WithRawParam("plan", planSchema("#/properties/plan/definitions/task"), aitool.WithParam_Required())
}

func planSchema(taskRef string) map[string]any {
	text := func(description string) map[string]any {
		return map[string]any{"type": "string", "minLength": 1, "description": description}
	}
	deps := map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "前置任务的语义标识；[] 表示独立任务，数组顺序不产生依赖；引用任务组表示等待该组全部叶任务验收。"}
	children := map[string]any{"type": "array", "description": "子任务列表；含子任务的节点只组织任务，不参与执行。", "items": map[string]any{"$ref": taskRef}}
	task := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"subtask_name": text("任务或结构任务组名称。"), "subtask_goal": text("执行任务书或任务组目标。"),
			"subtask_identifier": text("唯一且稳定的语义标识；未变更任务保留原标识。"),
			"name":               text("subtask_name 的兼容别名。"), "goal": text("subtask_goal 的兼容别名。"), "identifier": text("subtask_identifier 的兼容别名。"),
			"semantic_identifier": text("唯一语义标识的兼容别名；修改时同步修正依赖引用。"),
			"depends_on":          deps, "sub_subtasks": children, "subtasks": children,
		},
		"anyOf": []any{map[string]any{"required": []string{"subtask_name", "subtask_goal", "subtask_identifier"}}, map[string]any{"required": []string{"name", "goal", "identifier"}}, map[string]any{"required": []string{"name", "goal", "semantic_identifier"}}},
	}
	return map[string]any{
		"type": "object", "description": "完整的嵌套 PLAN DAG；沿用 main_task/main_task_goal/tasks 和递归的 subtask_name/subtask_goal/subtask_identifier/sub_subtasks。父任务组仅组织任务，不参与执行。",
		"properties": map[string]any{
			"main_task": text("计划名称。"), "main_task_goal": text("总体目标。"), "main_task_identifier": text("稳定的根节点语义标识。"),
			"name": text("main_task 的兼容别名。"), "goal": text("main_task_goal 的兼容别名。"), "identifier": text("main_task_identifier 的兼容别名。"),
			"tasks": map[string]any{"type": "array", "description": "计划的顶层任务列表，至少包含一个任务。", "minItems": 1, "items": map[string]any{"$ref": taskRef}},
		},
		"definitions": map[string]any{"task": task},
		"anyOf":       []any{map[string]any{"required": []string{"main_task", "main_task_goal", "tasks"}}, map[string]any{"required": []string{"name", "goal", "tasks"}}},
	}
}

func modifyPlanParameters() []aitool.ToolOption {
	ref := "#/properties/tasks_patch/items/definitions/task"
	definition := planSchema(ref)["definitions"].(map[string]any)["task"].(map[string]any)
	changes := map[string]any{"type": "object", "minProperties": 1, "additionalProperties": false, "description": "仅修改定义字段；禁止 task_id、状态、attempt、结果和统计字段。", "properties": definition["properties"]}
	return []aitool.ToolOption{
		aitool.WithStringParam("document", aitool.WithParam_Description("完整覆盖当前 Markdown 文档；与 document_patch 互斥，未提供则保留。")),
		aitool.WithStringParam("document_patch", aitool.WithParam_Description("严格匹配当前正文、仅针对 plan_document.md 的标准 unified diff；保存 patch artifact 后 apply，无 fuzz。与 document 互斥。")),
		aitool.WithRawParam("tasks", planSchema("#/properties/tasks/definitions/task")),
		aitool.WithRawParam("tasks_patch", map[string]any{"type": "array", "minItems": 1, "description": "依次在副本上执行 add/delete/update，最后校验完整 DAG，整批原子生效。目标 ID 读取 PLAN DEFINITION。", "items": map[string]any{
			"type": "object", "additionalProperties": false, "definitions": map[string]any{"task": definition},
			"properties": map[string]any{
				"operator":       map[string]any{"type": "string", "enum": []string{"add", "delete", "update"}, "description": "add 添加至任务组；delete 删除节点及子树；update 修改定义。"},
				"parent_task_id": map[string]any{"type": "string", "minLength": 1, "description": "add 的父任务组稳定 ID，省略时使用根任务组。"},
				"task_id":        map[string]any{"type": "string", "minLength": 1, "description": "delete/update 的目标稳定 ID，不能使用显示 index。"},
				"task":           definition, "changes": changes,
			},
			"oneOf": []any{
				map[string]any{"properties": map[string]any{"operator": map[string]any{"enum": []string{"add"}}}, "required": []string{"operator", "task"}},
				map[string]any{"properties": map[string]any{"operator": map[string]any{"enum": []string{"delete"}}}, "required": []string{"operator", "task_id"}},
				map[string]any{"properties": map[string]any{"operator": map[string]any{"enum": []string{"update"}}}, "required": []string{"operator", "task_id", "changes"}},
			},
		}}),
	}
}
