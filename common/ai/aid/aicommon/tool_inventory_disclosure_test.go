package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
)

func TestBuildToolInventoryDataWithExclusiveDisclosure(t *testing.T) {
	read := aitool.NewWithoutCallback("read_file")
	bash := aitool.NewWithoutCallback("bash")
	scan := aitool.NewWithoutCallback("scan_port")
	mcp := aitool.NewWithoutCallback("mcp_demo_query")
	data := BuildToolInventoryDataWithPolicy(
		[]*aitool.Tool{scan, bash, mcp, read},
		10,
		buildinaitools.ToolDisclosurePolicy{
			Names:     []string{"read_file", "bash"},
			Prefixes:  []string{"mcp_"},
			Exclusive: true,
		},
	)

	require.Equal(t, []string{"read_file", "bash", "mcp_demo_query"}, inventoryToolNames(data.TopTools))
	require.Equal(t, []string{"scan_port", "bash", "mcp_demo_query", "read_file"}, inventoryToolNames(data.VisibleTools))
	require.True(t, data.HasMoreTools)
}

func TestResolvePromptToolInventoryUsesManagerDisclosure(t *testing.T) {
	read := aitool.NewWithoutCallback("read_file")
	scan := aitool.NewWithoutCallback("scan_port")
	manager := buildinaitools.NewToolManagerByToolGetter(
		func() []*aitool.Tool { return []*aitool.Tool{scan, read} },
		buildinaitools.WithExtendTools([]*aitool.Tool{scan, read}, true),
		buildinaitools.WithDefaultToolDisclosure(buildinaitools.ToolDisclosurePolicy{
			Names:     []string{"read_file"},
			Exclusive: true,
		}),
	)
	cfg := NewConfig(context.Background(),
		WithDisableAutoSkills(true),
		WithAiToolManager(manager),
	)

	selection := ResolvePromptToolInventory(cfg, []*aitool.Tool{scan, read}, nil, true)
	require.Equal(t, []string{"read_file"}, inventoryToolNames(selection.DisplayTools))
	require.Equal(t, []string{"scan_port", "read_file"}, inventoryToolNames(selection.VisibleTools))
}

func inventoryToolNames(tools []*aitool.Tool) []string {
	result := make([]string, 0, len(tools))
	for _, tool := range tools {
		result = append(result, tool.Name)
	}
	return result
}
