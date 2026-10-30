package buildinaitools

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestDeprecatedAIToolsCannotReturnThroughManager(t *testing.T) {
	active := aitool.NewWithoutCallback("query_pcapdb")
	tools := []*aitool.Tool{active}
	for _, name := range schema.DeprecatedAIToolNames() {
		tools = append(tools, aitool.NewWithoutCallback(name))
	}
	for _, only := range []bool{false, true} {
		options := []ToolManagerOption{WithEnableAllTools(), WithSearchToolEnabled(false), WithForgeSearchToolEnabled(false)}
		if only {
			options = append(options, WithOnlyTools(tools...))
		}
		manager := NewToolManagerByToolGetter(func() []*aitool.Tool { return tools }, options...)
		for _, name := range schema.DeprecatedAIToolNames() {
			manager.EnableTool(name)
			tool, err := manager.GetToolByName(name)
			require.Nil(t, tool)
			require.ErrorContains(t, err, "deprecated")
			require.ErrorContains(t, err, "load_pcapfile")
		}
		enabled, err := manager.GetEnableTools()
		require.NoError(t, err)
		require.Equal(t, []*aitool.Tool{active}, enabled)
		require.Equal(t, []*aitool.Tool{active}, manager.safeToolsGetter())
		restored := manager.RestoreRecentlyUsedTools(tools)
		require.Len(t, restored, 1)
		require.Equal(t, active.Name, restored[0].Name)
		for _, tool := range tools[1:] {
			manager.AddRecentlyUsedTool(tool)
			manager.PreloadRecentlyUsedToolForMode(tool, true)
		}
		require.Equal(t, []string{active.Name}, manager.GetRecentToolNames())
		fork := manager.ForkForSession()
		for _, name := range schema.DeprecatedAIToolNames() {
			_, err := fork.GetToolByName(name)
			require.ErrorContains(t, err, "pcapdb")
		}
	}
}

func TestDeprecatedAIToolsFilteredFromStaleSearch(t *testing.T) {
	active := aitool.NewWithoutCallback("query_pcapdb")
	retired := aitool.NewWithoutCallback("analyze_pcap")
	manager := NewToolManagerByToolGetter(func() []*aitool.Tool { return []*aitool.Tool{active, retired} },
		WithSearchToolEnabled(true),
		WithAIToolsSearcher(func(_ string, candidates []*aitool.Tool) ([]*aitool.Tool, error) {
			require.Equal(t, []*aitool.Tool{active}, candidates)
			// Simulate a search index that still contains the retired tool.
			return []*aitool.Tool{retired, active}, nil
		}))
	results, err := manager.SearchTools("", "pcap")
	require.NoError(t, err)
	require.Equal(t, []*aitool.Tool{active}, results)
}
