package taskstack

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractTaskFromRawResponse(t *testing.T) {
	t.Run("从task.json格式响应提取任务", func(t *testing.T) {
		rawResponse := `{
			"@action": "plan",
			"query": "用户的查询",
			"tasks": [
				{
					"subtask_name": "主任务名称",
					"subtask_goal": "主任务目标"
				},
				{
					"subtask_name": "子任务1",
					"subtask_goal": "子任务1目标"
				},
				{
					"subtask_name": "子任务2",
					"subtask_goal": "子任务2目标"
				}
			]
		}`

		task, err := ExtractTaskFromRawResponse(rawResponse)
		assert.NoError(t, err)
		assert.NotNil(t, task)
		assert.Equal(t, "主任务名称", task.Name)
		assert.Equal(t, "主任务目标", task.Goal)
		assert.Len(t, task.Subtasks, 2)
		assert.Equal(t, "子任务1", task.Subtasks[0].Name)
		assert.Equal(t, "子任务1目标", task.Subtasks[0].Goal)
		assert.Equal(t, "子任务2", task.Subtasks[1].Name)
		assert.Equal(t, "子任务2目标", task.Subtasks[1].Goal)
	})

	t.Run("从直接的Task对象提取任务", func(t *testing.T) {
		rawResponse := `{
			"Name": "直接任务名称",
			"Goal": "直接任务目标",
			"Subtasks": [
				{
					"Name": "直接子任务1",
					"Goal": "直接子任务1目标"
				}
			]
		}`

		task, err := ExtractTaskFromRawResponse(rawResponse)
		assert.NoError(t, err)
		assert.NotNil(t, task)
		assert.Equal(t, "直接任务名称", task.Name)
		assert.Equal(t, "直接任务目标", task.Goal)
		assert.Len(t, task.Subtasks, 1)
		assert.Equal(t, "直接子任务1", task.Subtasks[0].Name)
		assert.Equal(t, "直接子任务1目标", task.Subtasks[0].Goal)
	})

	t.Run("从简单JSON提取任务", func(t *testing.T) {
		rawResponse := `{
			"name": "简单任务名称",
			"goal": "简单任务目标",
			"subtasks": [
				{
					"name": "简单子任务1"
				},
				{
					"name": "简单子任务2",
					"goal": "简单子任务2目标"
				}
			]
		}`

		task, err := ExtractTaskFromRawResponse(rawResponse)
		assert.NoError(t, err)
		assert.NotNil(t, task)
		assert.Equal(t, "简单任务名称", task.Name)
		assert.Equal(t, "简单任务目标", task.Goal)
		assert.Len(t, task.Subtasks, 2)
		assert.Equal(t, "简单子任务1", task.Subtasks[0].Name)
		assert.Equal(t, "", task.Subtasks[0].Goal)
		assert.Equal(t, "简单子任务2", task.Subtasks[1].Name)
		assert.Equal(t, "简单子任务2目标", task.Subtasks[1].Goal)
	})

	t.Run("没有Goal字段的任务", func(t *testing.T) {
		rawResponse := `{
			"name": "只有名称的任务"
		}`

		task, err := ExtractTaskFromRawResponse(rawResponse)
		assert.NoError(t, err)
		assert.NotNil(t, task)
		assert.Equal(t, "只有名称的任务", task.Name)
		assert.Equal(t, "", task.Goal)
		assert.Empty(t, task.Subtasks)
	})

	t.Run("无效JSON格式", func(t *testing.T) {
		rawResponse := `无效的JSON格式`

		task, err := ExtractTaskFromRawResponse(rawResponse)
		assert.Error(t, err)
		assert.Nil(t, task)
		assert.Equal(t, "no task found", err.Error())
	})

	t.Run("缺少Name字段", func(t *testing.T) {
		rawResponse := `{
			"goal": "没有名称的任务目标"
		}`

		task, err := ExtractTaskFromRawResponse(rawResponse)
		assert.Error(t, err)
		assert.Nil(t, task)
		assert.Equal(t, "no task found", err.Error())
	})

	t.Run("多个JSON对象中提取任务", func(t *testing.T) {
		rawResponse := `
			其他内容
			{"不是任务": true}
			{"name": "正确的任务", "goal": "任务目标"}
			{"name": "另一个任务"}
		`

		task, err := ExtractTaskFromRawResponse(rawResponse)
		assert.NoError(t, err)
		assert.NotNil(t, task)
		assert.Equal(t, "正确的任务", task.Name)
		assert.Equal(t, "任务目标", task.Goal)
	})

	t.Run("从复杂的城市一日游行程规划JSON提取任务", func(t *testing.T) {
		rawResponse := `{
    "@action": "plan",
    "query": "为一个带着8岁孩子的三口之家，在北京规划一个春季周末的一日游行程。预算为1000元，偏好历史文化景点和当地美食体验。",
    "tasks": [
        {
            "subtask_name": "检查北京天气情况",
            "subtask_goal": "获取计划出游日期的天气预报，包括温度范围、降水概率和活动建议，确定是否适合户外游览"
        },
        {
            "subtask_name": "筛选适合家庭的历史文化景点",
            "subtask_goal": "从AttractionAPI获取2-3个适合8岁儿童的历史文化景点，包括开放时间、票价、推荐游览时长和位置信息"
        },
        {
            "subtask_name": "规划早餐安排",
            "subtask_goal": "选择一个靠近第一个景点或住宿地的适合家庭用餐的早餐地点，考虑儿童友好程度和价格"
        },
        {
            "subtask_name": "规划午餐安排",
            "subtask_goal": "选择一个位于上午景点附近、提供当地特色美食的餐厅，须适合儿童且价格符合预算"
        },
        {
            "subtask_name": "规划晚餐安排",
            "subtask_goal": "选择一个位于下午景点附近或返程路线上的有特色餐厅，须适合家庭用餐且符合预算"
        },
        {
            "subtask_name": "规划景点间交通方式",
            "subtask_goal": "确定各个景点之间最合适的交通方式，包括时间、成本和便捷性分析，考虑有8岁儿童的实际需求"
        },
        {
            "subtask_name": "创建详细行程时间表",
            "subtask_goal": "制定从早到晚的完整时间安排，包括各景点游览时长、用餐时间、交通时间和必要的休息时间，确保合理可行"
        },
        {
            "subtask_name": "计算总预算花费",
            "subtask_goal": "汇总所有景点门票、餐饮费用和交通费用，确认总花费不超过1000元预算，并留出应急资金"
        },
        {
            "subtask_name": "制定应急备选方案",
            "subtask_goal": "针对可能的天气变化、景点关闭或其他突发情况，准备至少一套备选景点和活动方案，确保旅程顺利进行"
        }
    ]
}`

		task, err := ExtractTaskFromRawResponse(rawResponse)
		assert.NoError(t, err)
		assert.NotNil(t, task)

		// 验证主任务
		assert.Equal(t, "检查北京天气情况", task.Name)
		assert.Equal(t, "获取计划出游日期的天气预报，包括温度范围、降水概率和活动建议，确定是否适合户外游览", task.Goal)

		// 验证子任务数量
		assert.Len(t, task.Subtasks, 8)

		// 验证第一个子任务
		assert.Equal(t, "筛选适合家庭的历史文化景点", task.Subtasks[0].Name)
		assert.Equal(t, "从AttractionAPI获取2-3个适合8岁儿童的历史文化景点，包括开放时间、票价、推荐游览时长和位置信息", task.Subtasks[0].Goal)

		// 验证最后一个子任务
		assert.Equal(t, "制定应急备选方案", task.Subtasks[7].Name)
		assert.Equal(t, "针对可能的天气变化、景点关闭或其他突发情况，准备至少一套备选景点和活动方案，确保旅程顺利进行", task.Subtasks[7].Goal)
	})
}
