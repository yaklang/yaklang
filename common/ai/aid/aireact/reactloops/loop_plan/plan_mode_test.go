package loop_plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestBuildPlanDataFromLiteForgeAction(t *testing.T) {
	action, err := aicommon.ExtractAction(`{
		"@action":"plan",
		"main_task":"测试任务",
		"main_task_goal":"完成两个请求",
		"tasks":[{"subtask_name":"请求目标","subtask_goal":"GET https://app.example.com","depends_on":[]}]
	}`, "plan")
	require.NoError(t, err)
	planData := buildPlanDataFromLiteForgeAction(action, "test")
	require.NotEmpty(t, planData)
	assert.Contains(t, planData, `"@action":"plan"`)
	assert.Contains(t, planData, "请求目标")
}

func TestShouldEnterDeepPlanModeFromAction(t *testing.T) {
	assert.True(t, shouldEnterDeepPlanModeFromAction("read_file"))
	assert.True(t, shouldEnterDeepPlanModeFromAction("finish_exploration"))
	assert.False(t, shouldEnterDeepPlanModeFromAction("generate_direct_plan"))
}
