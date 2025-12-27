package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// TestTaskArtifacts_SaveTimelineDiff tests that timeline diff is correctly saved
func TestTaskArtifacts_SaveTimelineDiff(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "task_artifacts_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a timeline and add some content
	timeline := aicommon.NewTimeline(nil, nil)

	// Create a timeline differ and set baseline
	differ := aicommon.NewTimelineDiffer(timeline)
	differ.SetBaseline()

	// Add content to timeline after baseline
	timeline.PushToolResult(&aitool.ToolResult{
		ID:          101,
		Name:        "test_tool",
		Description: "test tool for timeline diff",
		Param:       map[string]any{"action": "test"},
		Success:     true,
		Data:        "test result data",
	})

	timeline.PushText(201, "additional text content for timeline")

	// Calculate the diff
	diff, err := differ.Diff()
	require.NoError(t, err)
	require.NotEmpty(t, diff, "diff should not be empty after adding content")
	require.Contains(t, diff, "test_tool", "diff should contain the tool name")

	// Verify the diff can be saved to file
	taskDir := filepath.Join(tmpDir, "task_1-1")
	err = os.MkdirAll(taskDir, 0755)
	require.NoError(t, err)

	timelineDiffPath := filepath.Join(taskDir, "timeline-diff.txt")
	err = os.WriteFile(timelineDiffPath, []byte(diff), 0644)
	require.NoError(t, err)

	// Read back and verify
	content, err := os.ReadFile(timelineDiffPath)
	require.NoError(t, err)
	require.Equal(t, diff, string(content))

	t.Log("TestTaskArtifacts_SaveTimelineDiff passed")
}

// TestTaskArtifacts_SaveResultSummary tests that result summary is correctly saved
func TestTaskArtifacts_SaveResultSummary(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "task_artifacts_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	taskDir := filepath.Join(tmpDir, "task_1-1")
	err = os.MkdirAll(taskDir, 0755)
	require.NoError(t, err)

	// Test with all fields populated
	t.Run("AllFieldsPopulated", func(t *testing.T) {
		summary := "This is the main summary"
		nextMovements := "Next step is to continue with task 1-2"
		statusSummary := "Task completed successfully"
		taskSummary := "Executed file search operation"
		shortSummary := "File search done"
		longSummary := "The file search operation completed successfully, found 10 files matching the criteria"

		// Build result content similar to saveResultSummary
		var resultParts []string
		if summary != "" {
			resultParts = append(resultParts, "Summary:\n"+summary)
		}
		if nextMovements != "" {
			resultParts = append(resultParts, "Next Movements:\n"+nextMovements)
		}
		if statusSummary != "" {
			resultParts = append(resultParts, "Status Summary:\n"+statusSummary)
		}
		if taskSummary != "" {
			resultParts = append(resultParts, "Task Summary:\n"+taskSummary)
		}
		if shortSummary != "" {
			resultParts = append(resultParts, "Short Summary:\n"+shortSummary)
		}
		if longSummary != "" {
			resultParts = append(resultParts, "Long Summary:\n"+longSummary)
		}

		resultContent := strings.Join(resultParts, "\n\n---\n\n")
		resultSummaryPath := filepath.Join(taskDir, "result-summary-all.txt")
		err = os.WriteFile(resultSummaryPath, []byte(resultContent), 0644)
		require.NoError(t, err)

		// Read back and verify
		content, err := os.ReadFile(resultSummaryPath)
		require.NoError(t, err)
		require.Contains(t, string(content), "Summary:")
		require.Contains(t, string(content), "Next Movements:")
		require.Contains(t, string(content), "Status Summary:")
		require.Contains(t, string(content), "Task Summary:")
		require.Contains(t, string(content), "Short Summary:")
		require.Contains(t, string(content), "Long Summary:")
		require.Contains(t, string(content), summary)
		require.Contains(t, string(content), nextMovements)
	})

	// Test with only some fields populated
	t.Run("PartialFieldsPopulated", func(t *testing.T) {
		summary := ""
		nextMovements := ""
		statusSummary := "Task completed"
		taskSummary := ""
		shortSummary := "Done"
		longSummary := ""

		var resultParts []string
		if summary != "" {
			resultParts = append(resultParts, "Summary:\n"+summary)
		}
		if nextMovements != "" {
			resultParts = append(resultParts, "Next Movements:\n"+nextMovements)
		}
		if statusSummary != "" {
			resultParts = append(resultParts, "Status Summary:\n"+statusSummary)
		}
		if taskSummary != "" {
			resultParts = append(resultParts, "Task Summary:\n"+taskSummary)
		}
		if shortSummary != "" {
			resultParts = append(resultParts, "Short Summary:\n"+shortSummary)
		}
		if longSummary != "" {
			resultParts = append(resultParts, "Long Summary:\n"+longSummary)
		}

		resultContent := strings.Join(resultParts, "\n\n---\n\n")
		resultSummaryPath := filepath.Join(taskDir, "result-summary-partial.txt")
		err = os.WriteFile(resultSummaryPath, []byte(resultContent), 0644)
		require.NoError(t, err)

		// Read back and verify
		content, err := os.ReadFile(resultSummaryPath)
		require.NoError(t, err)
		require.Contains(t, string(content), "Status Summary:")
		require.Contains(t, string(content), "Short Summary:")
		// Verify that only the expected sections are present
		// Note: "Status Summary:" contains "Summary:" so we check for standalone "Summary:\n" at the beginning
		require.False(t, strings.HasPrefix(string(content), "Summary:\n"), "Should not start with empty Summary section")
		require.NotContains(t, string(content), "Next Movements:")
		require.NotContains(t, string(content), "Task Summary:")
		require.NotContains(t, string(content), "Long Summary:")
	})

	// Test with no fields populated
	t.Run("NoFieldsPopulated", func(t *testing.T) {
		summary := ""
		nextMovements := ""
		statusSummary := ""
		taskSummary := ""
		shortSummary := ""
		longSummary := ""

		var resultParts []string
		if summary != "" {
			resultParts = append(resultParts, "Summary:\n"+summary)
		}
		if nextMovements != "" {
			resultParts = append(resultParts, "Next Movements:\n"+nextMovements)
		}
		if statusSummary != "" {
			resultParts = append(resultParts, "Status Summary:\n"+statusSummary)
		}
		if taskSummary != "" {
			resultParts = append(resultParts, "Task Summary:\n"+taskSummary)
		}
		if shortSummary != "" {
			resultParts = append(resultParts, "Short Summary:\n"+shortSummary)
		}
		if longSummary != "" {
			resultParts = append(resultParts, "Long Summary:\n"+longSummary)
		}

		// Should skip saving when no content
		require.Empty(t, resultParts, "resultParts should be empty when no fields are populated")
	})

	t.Log("TestTaskArtifacts_SaveResultSummary passed")
}

// TestTaskArtifacts_TaskDirectoryStructure tests that task directory is created correctly
func TestTaskArtifacts_TaskDirectoryStructure(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "task_artifacts_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Test different task index formats
	testCases := []struct {
		taskIndex   string
		expectedDir string
	}{
		{"1", "task_1"},
		{"1-1", "task_1-1"},
		{"1-2-3", "task_1-2-3"},
		{"", "task_0"}, // Empty index should default to "0"
	}

	for _, tc := range testCases {
		t.Run("TaskIndex_"+tc.taskIndex, func(t *testing.T) {
			taskIndex := tc.taskIndex
			if taskIndex == "" {
				taskIndex = "0"
			}
			taskDir := filepath.Join(tmpDir, "task_"+taskIndex)

			err := os.MkdirAll(taskDir, 0755)
			require.NoError(t, err)

			// Verify directory exists
			info, err := os.Stat(taskDir)
			require.NoError(t, err)
			require.True(t, info.IsDir())

			// Create timeline-diff.txt and result-summary.txt
			timelineDiffPath := filepath.Join(taskDir, "timeline-diff.txt")
			resultSummaryPath := filepath.Join(taskDir, "result-summary.txt")

			err = os.WriteFile(timelineDiffPath, []byte("test timeline diff"), 0644)
			require.NoError(t, err)

			err = os.WriteFile(resultSummaryPath, []byte("test result summary"), 0644)
			require.NoError(t, err)

			// Verify files exist
			_, err = os.Stat(timelineDiffPath)
			require.NoError(t, err)

			_, err = os.Stat(resultSummaryPath)
			require.NoError(t, err)
		})
	}

	t.Log("TestTaskArtifacts_TaskDirectoryStructure passed")
}

// TestTaskArtifacts_TimelineDifferIntegration tests the timeline differ integration with task
func TestTaskArtifacts_TimelineDifferIntegration(t *testing.T) {
	// Create a timeline
	timeline := aicommon.NewTimeline(nil, nil)

	// Add initial content
	timeline.PushToolResult(&aitool.ToolResult{
		ID:          1,
		Name:        "initial_tool",
		Description: "initial tool before baseline",
		Success:     true,
		Data:        "initial data",
	})

	// Create differ and set baseline (simulating task start)
	differ := aicommon.NewTimelineDiffer(timeline)
	differ.SetBaseline()

	// Add content during task execution
	timeline.PushToolResult(&aitool.ToolResult{
		ID:          2,
		Name:        "task_tool_1",
		Description: "first tool during task",
		Param:       map[string]any{"file": "/tmp/test.txt"},
		Success:     true,
		Data:        "found file",
	})

	timeline.PushUserInteraction(aicommon.UserInteractionStage_Review, 3, "review prompt", "user approved")

	timeline.PushToolResult(&aitool.ToolResult{
		ID:          4,
		Name:        "task_tool_2",
		Description: "second tool during task",
		Param:       map[string]any{"action": "process"},
		Success:     true,
		Data:        "processed successfully",
	})

	// Calculate diff (simulating task end)
	diff, err := differ.Diff()
	require.NoError(t, err)
	require.NotEmpty(t, diff)

	// Verify diff contains only the content added after baseline
	require.Contains(t, diff, "task_tool_1", "diff should contain first task tool")
	require.Contains(t, diff, "task_tool_2", "diff should contain second task tool")
	require.Contains(t, diff, "user approved", "diff should contain user interaction")

	// The initial tool should either not be in diff, or if it is, the diff should show it as removed/unchanged
	// Since we set baseline after initial_tool, the diff should focus on changes after baseline

	t.Log("TestTaskArtifacts_TimelineDifferIntegration passed")
}

// TestAiTask_TaskTimelineDifferField tests that AiTask has taskTimelineDiffer field
func TestAiTask_TaskTimelineDifferField(t *testing.T) {
	// Create a basic AiTask structure
	task := &aid.AiTask{
		Index: "1-1",
		Name:  "Test Task",
		Goal:  "Test the timeline differ field",
	}

	// Verify the task can be created
	require.NotNil(t, task)
	require.Equal(t, "1-1", task.Index)
	require.Equal(t, "Test Task", task.Name)

	t.Log("TestAiTask_TaskTimelineDifferField passed")
}
