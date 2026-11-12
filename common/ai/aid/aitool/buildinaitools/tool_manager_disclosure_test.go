package buildinaitools

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestWithDefaultToolDisclosureDoesNotRestrictTools(t *testing.T) {
	read := aitool.NewWithoutCallback("read_file")
	bash := aitool.NewWithoutCallback("bash")
	scan := aitool.NewWithoutCallback("scan_port")
	mcp := aitool.NewWithoutCallback("mcp_demo_query")

	manager := NewToolManagerByToolGetter(
		func() []*aitool.Tool { return []*aitool.Tool{scan, bash, mcp, read} },
		WithExtendTools([]*aitool.Tool{read, bash, scan, mcp}, true),
		WithDefaultToolDisclosure(ToolDisclosurePolicy{
			Names:     []string{"read_file", "bash"},
			Prefixes:  []string{"mcp_"},
			Exclusive: true,
		}),
	)

	enabled, err := manager.GetEnableTools()
	require.NoError(t, err)
	for _, name := range []string{"read_file", "bash", "scan_port", "mcp_demo_query"} {
		require.Contains(t, toolNameSet(enabled), name, "default disclosure must not remove capabilities")
	}
	_, err = manager.GetToolByName("scan_port")
	require.NoError(t, err, "non-default tools must remain callable")

	policy := manager.GetDefaultToolDisclosure()
	require.Equal(t, []string{"read_file", "bash"}, policy.Names)
	require.Equal(t, []string{"mcp_"}, policy.Prefixes)
	require.True(t, policy.Exclusive)

	policy.Names[0] = "mutated"
	require.Equal(t, "read_file", manager.GetDefaultToolDisclosure().Names[0])

	resolved, err := manager.GetToolByName("scan_port")
	require.NoError(t, err)
	require.Equal(t, scan, resolved, "lookup must activate the existing composed catalog entry")

	activated, err := manager.GetEnableTools()
	require.NoError(t, err)
	require.Contains(t, toolNameSet(activated), "scan_port")
}

func toolNameSet(tools []*aitool.Tool) map[string]bool {
	result := make(map[string]bool, len(tools))
	for _, tool := range tools {
		if tool != nil {
			result[tool.Name] = true
		}
	}
	return result
}
