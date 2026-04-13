package aid

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestSplitLargeInterfaceTasks(t *testing.T) {
	coordinator := &Coordinator{Config: &aicommon.Config{Ctx: context.Background()}}
	root := coordinator.generateAITaskWithName("根任务", "根任务目标")
	child := coordinator.generateAITaskWithName("验证用户接口", buildLargeInterfaceGoal(7))
	child.ParentTask = root
	root.Subtasks = []*AiTask{child}

	changed := splitLargeInterfaceTasks(root)
	require.True(t, changed)
	require.Len(t, root.Subtasks, 3)

	for index, subtask := range root.Subtasks {
		require.Contains(t, subtask.Name, fmt.Sprintf("第%d组", index+1))
		require.Contains(t, subtask.Goal, "待测列表")
		targets := extractInterfaceTargets(subtask.Goal)
		require.LessOrEqual(t, len(targets), planTaskPreferredTargetCount)
	}

	require.Equal(t, []string{root.Subtasks[0].Name}, root.Subtasks[1].DependsOn)
	require.Equal(t, []string{root.Subtasks[1].Name}, root.Subtasks[2].DependsOn)
}

func TestValidateSingleTaskQualityRequiresConcreteTargetListAndAcceptance(t *testing.T) {
	coordinator := &Coordinator{Config: &aicommon.Config{Ctx: context.Background()}}
	task := coordinator.generateAITaskWithName("泛化任务", "测试相关接口，检查是否有问题")

	issue, ok := validateSingleTaskQuality(task)
	require.True(t, ok)
	require.NotEmpty(t, issue.Reasons)
	require.Contains(t, strings.Join(issue.Reasons, "|"), "缺少具体待测列表")
	require.Contains(t, strings.Join(issue.Reasons, "|"), "缺少具体验收标准")
}

func TestShouldCompleteCurrentTaskBlockedByCoverageGap(t *testing.T) {
	record := &reactloops.SatisfactionRecord{
		CompletedTaskIndex:  "1-1",
		MissingTargets:      []string{"/user/name?name=admin"},
		MissingRequirements: []string{"时间盲注验证"},
	}
	require.False(t, shouldCompleteCurrentTask("1-1", true, "1-1", record))
	require.False(t, shouldCompleteCurrentTask("1-1", false, "1-1", record))

	record.MissingTargets = nil
	record.MissingRequirements = nil
	require.True(t, shouldCompleteCurrentTask("1-1", false, "1-1", record))
}

func buildLargeInterfaceGoal(targetCount int) string {
	goal := "待测列表：\n"
	for index := 0; index < targetCount; index++ {
		goal += fmt.Sprintf("- /api/user/%d?id=%d\n", index+1, index+1)
	}
	goal += "\n验收标准：\n"
	goal += "- 对本组全部接口执行布尔盲注验证\n"
	goal += "- 对本组全部接口执行时间盲注验证\n"
	goal += "- 输出 PoC 请求和结论证据\n"
	return goal
}
