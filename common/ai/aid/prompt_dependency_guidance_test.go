package aid

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPromptFilesIncludeDependsOnMaintenanceGuidance(t *testing.T) {
	testCases := []struct {
		path     string
		contains []string
	}{
		{
			path: "prompts/plan/dynamic-plan.txt",
			contains: []string{
				"depends_on",
				"remove",
				"skip",
				"append",
				"insert_after",
			},
		},
		{
			path: "prompts/plan/deepthink-plan.txt",
			contains: []string{
				"depends_on",
				"skip",
				"无意义串行",
			},
		},
		{
			path: "prompts/plan-review/plan-create-subtask.txt",
			contains: []string{
				"depends_on",
				"append",
				"replace_all",
				"skip",
			},
		},
		{
			path: "aicommon/prompts/review/ai-review-task.txt",
			contains: []string{
				"depends_on",
				"insert_after",
				"replace_all",
				"skippable",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			raw, err := os.ReadFile(tc.path)
			require.NoError(t, err)
			content := string(raw)
			for _, needle := range tc.contains {
				require.Truef(t, strings.Contains(content, needle), "expected %s to mention %q", tc.path, needle)
			}
		})
	}
}
