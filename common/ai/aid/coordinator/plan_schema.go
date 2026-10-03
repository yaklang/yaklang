package coordinator

import "github.com/yaklang/yaklang/common/ai/aid/aitool"

// planParameter retains the historical plan tree vocabulary in native function
// arguments. Groups are structural; only leaves become execution tasks.
func planParameter() aitool.ToolOption {
	text := func(description string) map[string]any {
		return map[string]any{"type": "string", "minLength": 1, "description": description}
	}
	deps := map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "前置任务的语义标识；[] 表示独立任务，数组顺序不产生依赖；引用任务组表示等待该组全部叶任务验收。"}
	children := map[string]any{"type": "array", "description": "子任务列表；含子任务的节点只组织任务，不参与执行。", "items": map[string]any{"$ref": "#/properties/plan/definitions/task"}}
	task := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"subtask_name": text("任务或结构任务组名称。"), "subtask_goal": text("执行任务书或任务组目标。"),
			"subtask_identifier": text("唯一且稳定的语义标识；未变更任务保留原标识。"),
			"name":               text("subtask_name 的兼容别名。"), "goal": text("subtask_goal 的兼容别名。"), "identifier": text("subtask_identifier 的兼容别名。"),
			"depends_on": deps, "sub_subtasks": children,
		},
		"anyOf": []any{map[string]any{"required": []string{"subtask_name", "subtask_goal", "subtask_identifier"}}, map[string]any{"required": []string{"name", "goal", "identifier"}}},
	}
	return aitool.WithRawParam("plan", map[string]any{
		"type": "object", "description": "完整的嵌套 PLAN DAG；沿用 main_task/main_task_goal/tasks 和递归的 subtask_name/subtask_goal/subtask_identifier/sub_subtasks。父任务组仅组织任务，不参与执行。",
		"properties": map[string]any{
			"main_task": text("计划名称。"), "main_task_goal": text("总体目标。"), "main_task_identifier": text("稳定的根节点语义标识。"),
			"name": text("main_task 的兼容别名。"), "goal": text("main_task_goal 的兼容别名。"), "identifier": text("main_task_identifier 的兼容别名。"),
			"tasks": map[string]any{"type": "array", "description": "计划的顶层任务列表，至少包含一个任务。", "minItems": 1, "items": map[string]any{"$ref": "#/properties/plan/definitions/task"}},
		},
		"definitions": map[string]any{"task": task},
		"anyOf":       []any{map[string]any{"required": []string{"main_task", "main_task_goal", "tasks"}}, map[string]any{"required": []string{"name", "goal", "tasks"}}},
	}, aitool.WithParam_Required())
}
