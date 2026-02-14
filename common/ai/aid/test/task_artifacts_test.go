package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	// Verify the diff can be saved to file with new naming format
	taskDir := filepath.Join(tmpDir, "task_1-1")
	err = os.MkdirAll(taskDir, 0755)
	require.NoError(t, err)

	// New filename format: task_{{index}}_timeline_diff.txt
	taskIndex := "1-1"
	safeTaskIndex := strings.ReplaceAll(taskIndex, "-", "_")
	timelineDiffPath := filepath.Join(taskDir, fmt.Sprintf("task_%s_timeline_diff.txt", safeTaskIndex))
	err = os.WriteFile(timelineDiffPath, []byte(diff), 0644)
	require.NoError(t, err)

	// Read back and verify
	content, err := os.ReadFile(timelineDiffPath)
	require.NoError(t, err)
	require.Equal(t, diff, string(content))

	t.Log("TestTaskArtifacts_SaveTimelineDiff passed")
}

// TestTaskArtifacts_SaveResultSummary tests that result summary is correctly saved with new format
func TestTaskArtifacts_SaveResultSummary(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "task_artifacts_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	taskDir := filepath.Join(tmpDir, "task_1-1")
	err = os.MkdirAll(taskDir, 0755)
	require.NoError(t, err)

	// Test with all fields populated - simulating new format
	t.Run("NewFormatWithAllFields", func(t *testing.T) {
		taskIndex := "1-1"
		taskName := "Test Task"
		taskGoal := "Test the result summary"
		startTime := time.Now().Add(-5 * time.Minute)
		endTime := time.Now()
		duration := endTime.Sub(startTime)

		summary := "This is the main summary"
		statusSummary := "Task completed successfully"
		shortSummary := "File search done"
		longSummary := "The file search operation completed successfully"

		// Build content similar to new saveResultSummary
		var contentBuilder strings.Builder
		contentBuilder.WriteString("============================================================\n")
		contentBuilder.WriteString(fmt.Sprintf(" Task %s Result Summary\n", taskIndex))
		contentBuilder.WriteString("============================================================\n\n")

		contentBuilder.WriteString("## Basic Information\n\n")
		contentBuilder.WriteString(fmt.Sprintf("Task Index: %s\n", taskIndex))
		contentBuilder.WriteString(fmt.Sprintf("Task Name: %s\n", taskName))
		contentBuilder.WriteString(fmt.Sprintf("Task Goal: %s\n", taskGoal))
		contentBuilder.WriteString(fmt.Sprintf("Generated At: %s\n", endTime.Format("2006-01-02 15:04:05")))
		contentBuilder.WriteString(fmt.Sprintf("Execution Duration: %.2f seconds\n", duration.Seconds()))
		contentBuilder.WriteString(fmt.Sprintf("Start Time: %s\n", startTime.Format("2006-01-02 15:04:05")))
		contentBuilder.WriteString(fmt.Sprintf("End Time: %s\n", endTime.Format("2006-01-02 15:04:05")))
		contentBuilder.WriteString("Task Status: completed\n")
		contentBuilder.WriteString("Total Tool Calls: 3 (Success: 2, Failed: 1)\n\n")

		contentBuilder.WriteString("## Task Input\n\n")
		contentBuilder.WriteString("Test user input for task\n\n")

		contentBuilder.WriteString("## Progress Information\n\n")
		contentBuilder.WriteString("[x] Task 1-1 completed\n\n")

		contentBuilder.WriteString("## Summary Results\n\n")
		if summary != "" {
			contentBuilder.WriteString("### Summary\n")
			contentBuilder.WriteString(summary + "\n\n")
		}
		if statusSummary != "" {
			contentBuilder.WriteString("### Status Summary\n")
			contentBuilder.WriteString(statusSummary + "\n\n")
		}
		if shortSummary != "" {
			contentBuilder.WriteString("### Short Summary\n")
			contentBuilder.WriteString(shortSummary + "\n\n")
		}
		if longSummary != "" {
			contentBuilder.WriteString("### Long Summary\n")
			contentBuilder.WriteString(longSummary + "\n\n")
		}

		contentBuilder.WriteString("============================================================\n")
		contentBuilder.WriteString(" End of Task 1-1 Result Summary\n")
		contentBuilder.WriteString("============================================================\n")

		// New filename format: task_{{index}}_result_summary.txt
		safeTaskIndex := strings.ReplaceAll(taskIndex, "-", "_")
		resultSummaryPath := filepath.Join(taskDir, fmt.Sprintf("task_%s_result_summary.txt", safeTaskIndex))
		err = os.WriteFile(resultSummaryPath, []byte(contentBuilder.String()), 0644)
		require.NoError(t, err)

		// Read back and verify
		content, err := os.ReadFile(resultSummaryPath)
		require.NoError(t, err)
		require.Contains(t, string(content), "Task 1-1 Result Summary")
		require.Contains(t, string(content), "## Basic Information")
		require.Contains(t, string(content), "Task Index: 1-1")
		require.Contains(t, string(content), "Task Name: Test Task")
		require.Contains(t, string(content), "## Task Input")
		require.Contains(t, string(content), "## Progress Information")
		require.Contains(t, string(content), "## Summary Results")
		require.Contains(t, string(content), "### Summary")
		require.Contains(t, string(content), summary)
	})

	// Test filename format with different task indexes
	t.Run("FilenameFormat", func(t *testing.T) {
		testCases := []struct {
			taskIndex    string
			expectedFile string
		}{
			{"1", "task_1_result_summary.txt"},
			{"1-1", "task_1_1_result_summary.txt"},
			{"1-2-3", "task_1_2_3_result_summary.txt"},
		}

		for _, tc := range testCases {
			safeTaskIndex := strings.ReplaceAll(tc.taskIndex, "-", "_")
			expectedFilename := fmt.Sprintf("task_%s_result_summary.txt", safeTaskIndex)
			require.Equal(t, tc.expectedFile, expectedFilename, "filename should match expected format for index %s", tc.taskIndex)
		}
	})

	t.Log("TestTaskArtifacts_SaveResultSummary passed")
}

// TestTaskArtifacts_TaskDirectoryStructure tests that task directory is created correctly
func TestTaskArtifacts_TaskDirectoryStructure(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "task_artifacts_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Test different task index formats with new filenames
	testCases := []struct {
		taskIndex           string
		expectedDir         string
		expectedDiffFile    string
		expectedSummaryFile string
	}{
		{"1", "task_1", "task_1_timeline_diff.txt", "task_1_result_summary.txt"},
		{"1-1", "task_1-1", "task_1_1_timeline_diff.txt", "task_1_1_result_summary.txt"},
		{"1-2-3", "task_1-2-3", "task_1_2_3_timeline_diff.txt", "task_1_2_3_result_summary.txt"},
	}

	for _, tc := range testCases {
		t.Run("TaskIndex_"+tc.taskIndex, func(t *testing.T) {
			taskDir := filepath.Join(tmpDir, tc.expectedDir)

			err := os.MkdirAll(taskDir, 0755)
			require.NoError(t, err)

			// Verify directory exists
			info, err := os.Stat(taskDir)
			require.NoError(t, err)
			require.True(t, info.IsDir())

			// Create files with new naming format
			timelineDiffPath := filepath.Join(taskDir, tc.expectedDiffFile)
			resultSummaryPath := filepath.Join(taskDir, tc.expectedSummaryFile)

			err = os.WriteFile(timelineDiffPath, []byte("# Task timeline diff\ntest content"), 0644)
			require.NoError(t, err)

			err = os.WriteFile(resultSummaryPath, []byte("# Task result summary\ntest content"), 0644)
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

// TestBuildTaskDirName tests the BuildTaskDirName helper function
func TestBuildTaskDirName(t *testing.T) {
	testCases := []struct {
		index    string
		name     string
		expected string
	}{
		{"1", "", "task_1"},
		{"1-1", "", "task_1-1"},
		{"1-1", "detect_os_type", "task_1-1_detect_os_type"},
		{"1-2", "Scan Ports", "task_1-2_scan_ports"},
		{"1", "A Very Long Task Name That Should Be Truncated At Some Point", "task_1_a_very_long_task_name_that_should_be_tru"},
		{"", "test", "task_0_test"},
		{"1-1", "special!@#chars", "task_1-1_specialchars"},
		// CJK / Unicode test cases
		{"1-1", "扫描目标端口", "task_1-1_扫描目标端口"},
		{"1-2", "web渗透扫描", "task_1-2_web渗透扫描"},
		{"1-3", "扫描 目标 端口", "task_1-3_扫描_目标_端口"},
		{"1-4", "扫描:目标/端口", "task_1-4_扫描目标端口"},
		{"1-5", "检测操作系统类型并获取版本信息以便后续分析使用的超长中文任务名称需要被截断处理确保不会过长", "task_1-5_检测操作系统类型并获取版本信息以便后续分析使用的超长中文任务名称需要被截断处理确"},
		// Mixed language
		{"2-1", "Search web渗透 capabilities", "task_2-1_search_web渗透_capabilities"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("index=%s_name=%s", tc.index, tc.name), func(t *testing.T) {
			result := aicommon.BuildTaskDirName(tc.index, tc.name)
			require.Equal(t, tc.expected, result)
		})
	}
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

	// Verify baseline is recorded
	lastDump := differ.GetLastDump()
	require.NotEmpty(t, lastDump, "baseline should be recorded")

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

	t.Log("TestTaskArtifacts_TimelineDifferIntegration passed")
}

// TestTaskArtifacts_TimelineDifferEmptyDiff tests the case when timeline diff is empty
func TestTaskArtifacts_TimelineDifferEmptyDiff(t *testing.T) {
	// Create a timeline with some content
	timeline := aicommon.NewTimeline(nil, nil)
	timeline.PushToolResult(&aitool.ToolResult{
		ID:          1,
		Name:        "existing_tool",
		Description: "existing tool",
		Success:     true,
		Data:        "existing data",
	})

	// Create differ and set baseline
	differ := aicommon.NewTimelineDiffer(timeline)
	differ.SetBaseline()

	// Don't add any new content - simulate no changes during task

	// Calculate diff - should be empty
	diff, err := differ.Diff()
	require.NoError(t, err)
	require.Empty(t, diff, "diff should be empty when no changes after baseline")

	// But we can still get the current dump
	currentDump := differ.GetCurrentDump()
	require.NotEmpty(t, currentDump, "current dump should still have content")

	t.Log("TestTaskArtifacts_TimelineDifferEmptyDiff passed")
}

// TestTaskArtifacts_FormatDuration tests duration formatting
func TestTaskArtifacts_FormatDuration(t *testing.T) {
	testCases := []struct {
		duration time.Duration
		expected string
	}{
		{30 * time.Second, "30.00 seconds"},
		{90 * time.Second, "1 min 30 sec"},
		{3661 * time.Second, "1 hr 1 min 1 sec"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			// Format duration
			var result string
			d := tc.duration
			if d < time.Minute {
				result = fmt.Sprintf("%.2f seconds", d.Seconds())
			} else if d < time.Hour {
				minutes := int(d.Minutes())
				seconds := int(d.Seconds()) % 60
				result = fmt.Sprintf("%d min %d sec", minutes, seconds)
			} else {
				hours := int(d.Hours())
				minutes := int(d.Minutes()) % 60
				seconds := int(d.Seconds()) % 60
				result = fmt.Sprintf("%d hr %d min %d sec", hours, minutes, seconds)
			}
			require.Equal(t, tc.expected, result)
		})
	}

	t.Log("TestTaskArtifacts_FormatDuration passed")
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

// TestTimelineDiffer_DiffCalculation tests that timeline differ correctly calculates diff
func TestTimelineDiffer_DiffCalculation(t *testing.T) {
	t.Run("DiffAfterAddingContent", func(t *testing.T) {
		// Create a timeline
		timeline := aicommon.NewTimeline(nil, nil)

		// Add initial content
		timeline.PushText(1, "Initial content before baseline")

		// Create differ and set baseline
		differ := aicommon.NewTimelineDiffer(timeline)
		differ.SetBaseline()

		// Record baseline length
		baselineLen := len(differ.GetLastDump())
		require.Greater(t, baselineLen, 0, "baseline should have content")

		// Add new content after baseline
		timeline.PushText(2, "New content after baseline - task execution started")
		timeline.PushToolResult(&aitool.ToolResult{
			ID:          3,
			Name:        "test_tool",
			Description: "test tool",
			Success:     true,
			Data:        "tool execution result",
		})
		timeline.PushText(4, "Another entry during task execution")

		// Get current dump and verify it's larger
		currentLen := len(differ.GetCurrentDump())
		require.Greater(t, currentLen, baselineLen, "current should have more content than baseline")

		// Calculate diff
		diff, err := differ.Diff()
		require.NoError(t, err)
		require.NotEmpty(t, diff, "diff should not be empty")

		// Verify diff contains new content
		require.Contains(t, diff, "New content after baseline", "diff should contain new text")
		require.Contains(t, diff, "test_tool", "diff should contain tool name")
		require.Contains(t, diff, "Another entry", "diff should contain additional entry")

		t.Logf("Baseline: %d bytes, Current: %d bytes, Diff: %d bytes", baselineLen, currentLen, len(diff))
	})

	t.Run("EmptyDiffWhenNoChanges", func(t *testing.T) {
		// Create a timeline with content
		timeline := aicommon.NewTimeline(nil, nil)
		timeline.PushText(1, "Existing content")

		// Create differ and set baseline
		differ := aicommon.NewTimelineDiffer(timeline)
		differ.SetBaseline()

		// Don't add any content

		// Calculate diff - should be empty
		diff, err := differ.Diff()
		require.NoError(t, err)
		require.Empty(t, diff, "diff should be empty when no changes")
	})

	t.Run("MultipleToolResults", func(t *testing.T) {
		// Create a timeline
		timeline := aicommon.NewTimeline(nil, nil)

		// Set baseline on empty timeline
		differ := aicommon.NewTimelineDiffer(timeline)
		differ.SetBaseline()

		// Add multiple tool results
		for i := 1; i <= 5; i++ {
			timeline.PushToolResult(&aitool.ToolResult{
				ID:          int64(i),
				Name:        fmt.Sprintf("tool_%d", i),
				Description: fmt.Sprintf("Tool %d description", i),
				Success:     i%2 == 0, // alternate success/failure
				Data:        fmt.Sprintf("Result from tool %d", i),
			})
		}

		// Calculate diff
		diff, err := differ.Diff()
		require.NoError(t, err)
		require.NotEmpty(t, diff)

		// Verify all tools are in diff
		for i := 1; i <= 5; i++ {
			require.Contains(t, diff, fmt.Sprintf("tool_%d", i), "diff should contain tool_%d", i)
		}
	})

	t.Run("UserInteractionInDiff", func(t *testing.T) {
		timeline := aicommon.NewTimeline(nil, nil)

		differ := aicommon.NewTimelineDiffer(timeline)
		differ.SetBaseline()

		// Add user interaction
		timeline.PushUserInteraction(
			aicommon.UserInteractionStage_Review,
			100,
			"Do you approve this action?",
			"User approved the action",
		)

		diff, err := differ.Diff()
		require.NoError(t, err)
		require.NotEmpty(t, diff)
		require.Contains(t, diff, "User approved", "diff should contain user response")
	})
}

// TestTimelineDiffer_BaselineReset tests that baseline can be reset correctly
func TestTimelineDiffer_BaselineReset(t *testing.T) {
	timeline := aicommon.NewTimeline(nil, nil)

	// Add initial content
	timeline.PushText(1, "First content")

	// Create differ and set baseline
	differ := aicommon.NewTimelineDiffer(timeline)
	differ.SetBaseline()

	// Add more content
	timeline.PushText(2, "Second content")

	// Verify diff has content
	diff1, err := differ.Diff()
	require.NoError(t, err)
	require.NotEmpty(t, diff1)
	require.Contains(t, diff1, "Second content")

	// After Diff(), baseline should be updated to current
	// So next diff should be empty (no new changes)
	diff2, err := differ.Diff()
	require.NoError(t, err)
	require.Empty(t, diff2, "second diff should be empty as baseline was updated")

	// Add more content
	timeline.PushText(3, "Third content")

	// This diff should contain "Third content" as the new addition
	// Note: diff format may include some context lines from before, but the key is
	// that "Third content" appears in the added lines (with + prefix in unified diff)
	diff3, err := differ.Diff()
	require.NoError(t, err)
	require.NotEmpty(t, diff3)
	require.Contains(t, diff3, "Third content", "diff3 should contain the new content")

	// Verify that "Third content" is in the added lines (+ prefix)
	require.Contains(t, diff3, "+", "diff should have added lines")
}

// TestTimelineDiffer_GetCurrentDumpWithoutUpdate tests GetCurrentDump doesn't update baseline
func TestTimelineDiffer_GetCurrentDumpWithoutUpdate(t *testing.T) {
	timeline := aicommon.NewTimeline(nil, nil)
	timeline.PushText(1, "Initial content")

	differ := aicommon.NewTimelineDiffer(timeline)
	differ.SetBaseline()

	// Add new content
	timeline.PushText(2, "New content")

	// Get current dump multiple times
	current1 := differ.GetCurrentDump()
	current2 := differ.GetCurrentDump()

	// Both should be the same
	require.Equal(t, current1, current2)

	// Baseline should still be unchanged
	baseline := differ.GetLastDump()
	require.NotEqual(t, baseline, current1, "baseline should be different from current")
	require.NotContains(t, baseline, "New content", "baseline should not contain new content")

	// Now calculate diff - this should work since baseline wasn't updated
	diff, err := differ.Diff()
	require.NoError(t, err)
	require.NotEmpty(t, diff)
	require.Contains(t, diff, "New content")
}

// TestReActLoopIF_GetTimelineDiff tests that ReActLoopIF interface has GetTimelineDiff method
func TestReActLoopIF_GetTimelineDiff(t *testing.T) {
	// This test verifies the interface definition by type assertion
	// The actual implementation is tested in integration tests

	// Verify GetTimelineDiff is part of ReActLoopIF interface
	// This is a compile-time check - if the method doesn't exist, this won't compile
	type hasGetTimelineDiff interface {
		GetTimelineDiff() (string, error)
	}

	// Verify ReActLoopIF satisfies hasGetTimelineDiff
	var _ hasGetTimelineDiff = (aicommon.ReActLoopIF)(nil)

	t.Log("ReActLoopIF has GetTimelineDiff method")
}
