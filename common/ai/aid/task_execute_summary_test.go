package aid

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestSaveResultSummary_DeduplicatesEquivalentSummarySections(t *testing.T) {
	taskDir := t.TempDir()
	task := &AiTask{
		Coordinator:        &Coordinator{},
		AIStatefulTaskBase: aicommon.NewStatefulTaskBase("task-1-1", "", context.Background(), aicommon.NewDummyEmitter(), true),
		Index:              "1-1",
		Name:               "summary task",
		Goal:               "avoid duplicate summary sections",
	}

	err := task.saveResultSummary(taskDir, "", "", "same summary", "same summary", "same summary", "same summary")
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(taskDir, "task_1_1_result_summary.txt"))
	require.NoError(t, err)
	text := string(content)

	require.Contains(t, text, "### Task Summary")
	require.Equal(t, 1, strings.Count(text, "same summary"))
	require.NotContains(t, text, "### Status Summary")
	require.NotContains(t, text, "### Short Summary")
	require.NotContains(t, text, "### Long Summary")
}
