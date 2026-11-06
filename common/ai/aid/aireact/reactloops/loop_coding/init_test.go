package loop_coding_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_coding"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
)

func TestCodingLoopRegistered(t *testing.T) {
	_, ok := reactloops.GetLoopFactory(loop_coding.LoopName)
	require.True(t, ok)

	meta, ok := reactloops.GetLoopMetadata(loop_coding.LoopName)
	require.True(t, ok)
	require.Equal(t, "Coding Agent", meta.VerboseName)
	require.Contains(t, meta.Description, "Coding-only")
}

func TestCodingDefaultDisclosureIsCopiedAndCodingFocused(t *testing.T) {
	first := loop_coding.DefaultToolNames()
	second := loop_coding.DefaultToolNames()
	require.NotEmpty(t, first)
	require.NotSame(t, &first[0], &second[0])

	require.Contains(t, first, "read_file")
	require.Contains(t, first, "modify_file")
	require.Contains(t, first, "use_browser")
	require.Contains(t, first, "bash")
	require.NotContains(t, first, "scan_port")

	skills := loop_coding.DefaultSkillNames()
	require.Contains(t, skills, loop_coding.CodingWorkflowSkillName)
	require.Contains(t, skills, "yaklang-system")
}

func TestToolManagerOptionsComposeExistingTools(t *testing.T) {
	all := []*aitool.Tool{
		aitool.NewWithoutCallback("read_file"),
		aitool.NewWithoutCallback("bash"),
		aitool.NewWithoutCallback("use_browser"),
		aitool.NewWithoutCallback("scan_port"),
		aitool.NewWithoutCallback("mcp_demo_query"),
	}
	manager := buildinaitools.NewToolManagerByToolGetter(
		func() []*aitool.Tool { return all },
		buildinaitools.WithExtendTools(all, true),
	)
	for _, option := range loop_coding.ToolManagerOptions() {
		option(manager)
	}

	enabled, err := manager.GetEnableTools()
	require.NoError(t, err)
	names := make(map[string]bool, len(enabled))
	for _, tool := range enabled {
		names[tool.Name] = true
	}
	require.True(t, names["read_file"])
	require.True(t, names["bash"])
	require.True(t, names["use_browser"])
	require.False(t, names["scan_port"])
	require.Contains(t, manager.GetDefaultToolDisclosure().Names, "use_browser")
}

func TestCodingToolManagerRetainsMCPDiscovery(t *testing.T) {
	read := aitool.NewWithoutCallback("read_file")
	bash := aitool.NewWithoutCallback("bash")
	mcpTool := aitool.NewWithoutCallback("mcp_code_repository_search")
	manager := buildinaitools.NewToolManagerByToolGetter(
		func() []*aitool.Tool { return []*aitool.Tool{bash, mcpTool, read} },
		buildinaitools.WithExtendTools([]*aitool.Tool{bash, mcpTool, read}, true),
	)
	for _, option := range loop_coding.ToolManagerOptions() {
		option(manager)
	}
	require.NoError(t, manager.AppendTools(mcpTool))
	manager.EnableTool(mcpTool.Name)

	tools, err := manager.GetEnableTools()
	require.NoError(t, err)
	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		names[tool.Name] = true
	}
	require.True(t, names["read_file"])
	require.True(t, names["mcp_code_repository_search"])
	require.True(t, names["bash"])
	require.True(t, manager.GetDefaultToolDisclosure().Exclusive)
}

func TestCodingReActWithComposedManagerStillSupportsDynamicTools(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	read := aitool.NewWithoutCallback("read_file")
	dynamic := aitool.NewWithoutCallback("dynamic_coding_tool")
	manager := buildinaitools.NewToolManagerByToolGetter(
		func() []*aitool.Tool { return []*aitool.Tool{dynamic, read} },
		buildinaitools.WithExtendTools([]*aitool.Tool{dynamic, read}, true),
	)
	for _, option := range loop_coding.ToolManagerOptions() {
		option(manager)
	}
	runtime, err := aireact.NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithAiToolManager(manager),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, errors.New("unexpected AI invocation")
		}),
	)
	require.NoError(t, err)

	tool, err := runtime.GetConfig().GetAiToolManager().GetToolByName("dynamic_coding_tool")
	require.NoError(t, err, "non-default existing tools must remain discoverable through the shared catalog")
	require.NotNil(t, tool)
	require.Equal(t, "dynamic_coding_tool", tool.GetName())
}
