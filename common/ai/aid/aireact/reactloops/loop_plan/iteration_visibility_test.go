package loop_plan

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestPlanningPromptsDoNotExposeIterationDeadline(t *testing.T) {
	text := strings.ToLower(reactiveData + "\n" + persistentInstruction)
	for _, forbidden := range []string{"islastiteration", "last iteration", "final iteration", "最后一次迭代", "最后一轮迭代"} {
		require.NotContains(t, text, strings.ToLower(forbidden))
	}
}

func TestPlanningHandoffDoesNotRequestWithdrawnExploration(t *testing.T) {
	for _, closed := range []bool{false, true} {
		prompt, err := utils.RenderTemplate(reactiveData, map[string]any{
			"PlanMode": "deep", "ExplorationClosed": closed,
			"Facts": "known fact", "Nonce": "test",
		})
		require.NoError(t, err)
		require.Contains(t, prompt, "known fact")
		if closed {
			require.Contains(t, prompt, "当前已关闭信息收集入口")
			require.Contains(t, prompt, "调用 `finish_exploration`")
			require.Contains(t, prompt, "不要把推断写成已确认事实")
			require.NotContains(t, prompt, "禁止**在此阶段直接调用")
			require.NotContains(t, prompt, "你必须先使用信息收集工具")
		} else {
			require.Contains(t, prompt, "你必须先使用信息收集工具")
			require.NotContains(t, prompt, "当前已关闭信息收集入口")
		}
	}
}
