package loop_plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func TestSerializeTaskParams_WithSubSubtasks(t *testing.T) {
	tasks := []aitool.InvokeParams{
		{
			"subtask_name": "SQL注入漏洞深度验证",
			"subtask_goal": "对所有SQL注入点进行测试",
			"depends_on":   []string{},
			"sub_subtasks": []any{
				map[string]any{
					"subtask_name": "数字型参数SQL注入测试",
					"subtask_goal": "针对数字型参数发送Payload",
					"depends_on":   []string{},
				},
				map[string]any{
					"subtask_name": "字符型参数SQL注入测试",
					"subtask_goal": "针对字符型参数发送Payload",
					"depends_on":   []string{},
				},
			},
		},
		{
			"subtask_name": "XSS漏洞验证",
			"subtask_goal": "对XSS注入点进行测试",
			"depends_on":   []string{},
		},
	}

	result := serializeTaskParams(tasks)
	require.Len(t, result, 2)

	sqlTask := result[0]
	assert.Equal(t, "SQL注入漏洞深度验证", sqlTask["subtask_name"])
	nestedTasks, ok := sqlTask["sub_subtasks"].([]map[string]any)
	require.True(t, ok, "nested tasks should be []map[string]any, got %T", sqlTask["sub_subtasks"])
	assert.Len(t, nestedTasks, 2)
	assert.Equal(t, "数字型参数SQL注入测试", nestedTasks[0]["subtask_name"])
	assert.Equal(t, "字符型参数SQL注入测试", nestedTasks[1]["subtask_name"])

	xssTask := result[1]
	assert.Equal(t, "XSS漏洞验证", xssTask["subtask_name"])
	_, hasNested := xssTask["sub_subtasks"]
	assert.False(t, hasNested, "XSS task should not have nested tasks")
}

func TestSerializeAndExtractAction_RoundTrip(t *testing.T) {
	tasks := []aitool.InvokeParams{
		{
			"subtask_name":       "SQL注入漏洞深度验证",
			"subtask_identifier": "sql_injection",
			"subtask_goal":       "对所有SQL注入点进行测试",
			"depends_on":         []string{},
			"sub_subtasks": []any{
				map[string]any{
					"subtask_name":       "数字型参数SQL注入测试",
					"subtask_identifier": "numeric_sqli",
					"subtask_goal":       "针对数字型参数发送Payload",
					"depends_on":         []string{},
				},
				map[string]any{
					"subtask_name":       "字符型参数SQL注入测试",
					"subtask_identifier": "string_sqli",
					"subtask_goal":       "针对字符型参数发送Payload",
					"depends_on":         []string{"数字型参数SQL注入测试"},
				},
			},
		},
		{
			"subtask_name": "XSS漏洞验证",
			"subtask_goal": "对XSS注入点进行测试",
			"depends_on":   []string{"SQL注入漏洞深度验证"},
		},
	}

	taskPayload := serializeTaskParams(tasks)
	payload := map[string]any{
		"@action":        "plan",
		"main_task":      "Web安全测试",
		"main_task_goal": "系统性安全评估",
		"tasks":          taskPayload,
	}
	planJSON := string(utils.Jsonify(payload))
	t.Logf("Serialized plan JSON:\n%s", planJSON)

	action, err := aicommon.ExtractAction(planJSON, "plan", "plan")
	require.NoError(t, err)
	require.NotNil(t, action)

	assert.Equal(t, "Web安全测试", action.GetAnyToString("main_task"))
	assert.Equal(t, "系统性安全评估", action.GetAnyToString("main_task_goal"))

	parsedTasks := action.GetInvokeParamsArray("tasks")
	require.Len(t, parsedTasks, 2, "should have 2 top-level tasks")

	sqlTask := parsedTasks[0]
	assert.Equal(t, "SQL注入漏洞深度验证", sqlTask.GetAnyToString("subtask_name"))
	assert.Equal(t, "sql_injection", sqlTask.GetAnyToString("subtask_identifier"))

	nestedSubtasks := sqlTask.GetObjectArray("sub_subtasks")
	require.Len(t, nestedSubtasks, 2, "SQL task should have 2 nested subtasks via 'sub_subtasks' key")
	assert.Equal(t, "数字型参数SQL注入测试", nestedSubtasks[0].GetAnyToString("subtask_name"))
	assert.Equal(t, "字符型参数SQL注入测试", nestedSubtasks[1].GetAnyToString("subtask_name"))
	assert.Equal(t, "string_sqli", nestedSubtasks[1].GetAnyToString("subtask_identifier"))

	deps := nestedSubtasks[1].GetStringSlice("depends_on")
	assert.Contains(t, deps, "数字型参数SQL注入测试")

	xssTask := parsedTasks[1]
	assert.Equal(t, "XSS漏洞验证", xssTask.GetAnyToString("subtask_name"))
	xssNested := xssTask.GetObjectArray("sub_subtasks")
	assert.Len(t, xssNested, 0, "XSS task should have no nested subtasks")
}

func TestSerializeAndExtractAction_NestedTasksKeyCollision(t *testing.T) {
	tasks := []aitool.InvokeParams{
		{
			"subtask_name": "Parent1",
			"subtask_goal": "parent1 goal",
			"depends_on":   []string{},
			"sub_subtasks": []any{
				map[string]any{
					"subtask_name": "Child1",
					"subtask_goal": "child1 goal",
					"depends_on":   []string{},
				},
			},
		},
		{
			"subtask_name": "Parent2",
			"subtask_goal": "parent2 goal",
			"depends_on":   []string{},
			"sub_subtasks": []any{
				map[string]any{
					"subtask_name": "Child2A",
					"subtask_goal": "child2a goal",
					"depends_on":   []string{},
				},
				map[string]any{
					"subtask_name": "Child2B",
					"subtask_goal": "child2b goal",
					"depends_on":   []string{},
				},
			},
		},
		{
			"subtask_name": "Parent3_NoChildren",
			"subtask_goal": "parent3 goal",
			"depends_on":   []string{},
		},
	}

	taskPayload := serializeTaskParams(tasks)
	payload := map[string]any{
		"@action":        "plan",
		"main_task":      "Test",
		"main_task_goal": "Test Goal",
		"tasks":          taskPayload,
	}
	planJSON := string(utils.Jsonify(payload))
	t.Logf("Serialized plan JSON:\n%s", planJSON)

	action, err := aicommon.ExtractAction(planJSON, "plan", "plan")
	require.NoError(t, err)

	topTasks := action.GetInvokeParamsArray("tasks")
	require.Len(t, topTasks, 3, "should have 3 top-level tasks after re-parse")

	p1 := topTasks[0]
	assert.Equal(t, "Parent1", p1.GetAnyToString("subtask_name"))
	p1Children := p1.GetObjectArray("sub_subtasks")
	require.Len(t, p1Children, 1, "Parent1 should have 1 child")
	assert.Equal(t, "Child1", p1Children[0].GetAnyToString("subtask_name"))

	p2 := topTasks[1]
	assert.Equal(t, "Parent2", p2.GetAnyToString("subtask_name"))
	p2Children := p2.GetObjectArray("sub_subtasks")
	require.Len(t, p2Children, 2, "Parent2 should have 2 children")
	assert.Equal(t, "Child2A", p2Children[0].GetAnyToString("subtask_name"))
	assert.Equal(t, "Child2B", p2Children[1].GetAnyToString("subtask_name"))

	p3 := topTasks[2]
	assert.Equal(t, "Parent3_NoChildren", p3.GetAnyToString("subtask_name"))
	p3Children := p3.GetObjectArray("sub_subtasks")
	assert.Len(t, p3Children, 0, "Parent3 should have no children")
}
