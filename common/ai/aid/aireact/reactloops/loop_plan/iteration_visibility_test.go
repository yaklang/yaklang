package loop_plan

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlanningPromptsDoNotExposeIterationDeadline(t *testing.T) {
	text := strings.ToLower(reactiveData + "\n" + persistentInstruction)
	for _, forbidden := range []string{"islastiteration", "last iteration", "final iteration", "最后一次迭代", "最后一轮迭代"} {
		require.NotContains(t, text, strings.ToLower(forbidden))
	}
}
