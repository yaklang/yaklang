package loop_yaklangcode

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildYaklangAnalyzeRequirementPromptWithAttachedPath(t *testing.T) {
	out := buildYaklangAnalyzeRequirementPrompt(yaklangAnalyzeRequirementOptions{
		userInput:       "fix scan timeout",
		hasAttachedPath: true,
		attachedPath:    "/tmp/project/scan.yak",
		workspacePath:   "/tmp/project",
		hasGrepSearcher: true,
	})
	require.Contains(t, out, "已知编辑器上下文")
	require.Contains(t, out, "/tmp/project/scan.yak")
	require.NotContains(t, out, "判断文件操作类型")
}

func TestBuildYaklangAnalyzeRequirementPromptCreateMode(t *testing.T) {
	out := buildYaklangAnalyzeRequirementPrompt(yaklangAnalyzeRequirementOptions{
		userInput:       "write port scan",
		createMode:      true,
		hasGrepSearcher: true,
	})
	require.Contains(t, out, "新建文件模式")
	require.NotContains(t, out, "判断文件操作类型")
}

func TestBuildYaklangAnalyzeRequirementPromptCreateModeWithWorkspace(t *testing.T) {
	out := buildYaklangAnalyzeRequirementPrompt(yaklangAnalyzeRequirementOptions{
		userInput:       "write port scan",
		createMode:      true,
		workspacePath:   "/tmp/project",
		hasGrepSearcher: true,
	})
	require.Contains(t, out, "新建文件模式")
	require.Contains(t, out, "/tmp/project")
	require.NotContains(t, out, "判断文件操作类型")
}

func TestBuildYaklangAnalyzeRequirementToolOptionsSkipsFileDetectWhenAttached(t *testing.T) {
	opts := yaklangAnalyzeRequirementOptions{
		hasAttachedPath: true,
		createMode:      false,
		hasGrepSearcher: true,
	}
	attachedOpts := buildYaklangAnalyzeRequirementToolOptions(opts, true)
	require.NotEmpty(t, attachedOpts)

	opts.hasAttachedPath = false
	opts.createMode = true
	createOpts := buildYaklangAnalyzeRequirementToolOptions(opts, true)
	require.Greater(t, len(attachedOpts), len(createOpts))
}
