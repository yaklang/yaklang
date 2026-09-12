package buildinaitools

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestForkForSessionKeepsRegistryAndCacheIndependent(t *testing.T) {
	allowed := aitool.NewWithoutCallback("allowed", aitool.WithDescription("original description"))
	denied := aitool.NewWithoutCallback("denied")
	backing := make([]*aitool.Tool, 2, 8)
	backing[0], backing[1] = allowed, denied
	source := NewToolManagerByToolGetter(func() []*aitool.Tool { return backing },
		WithEnableAllTools(), WithNoToolsCache(), WithDisallowMCPServers(true),
		WithDisableTools([]string{denied.Name}),
		WithSearchToolEnabled(false), WithForgeSearchToolEnabled(false),
	)
	source.maxCacheTokens = 1024
	source.AddRecentlyUsedTool(allowed)
	fork := source.ForkForSession()
	require.NotSame(t, source, fork)
	require.True(t, fork.enableAllTools)
	require.True(t, fork.noCacheTools)
	require.True(t, fork.DisallowMCPServers())
	require.False(t, fork.enableSearchTool)
	require.False(t, fork.enableForgeSearchTool)
	require.Equal(t, source.GetRecentToolNames(), fork.GetRecentToolNames())
	require.Equal(t, source.GetRecentToolCacheMaxTokens(), fork.GetRecentToolCacheMaxTokens())
	tool, err := fork.GetToolByName(allowed.Name)
	require.NoError(t, err)
	require.Same(t, allowed, tool, "tool definitions remain shared inputs")

	sessionTool := aitool.NewWithoutCallback("mcp_session_only")
	fork.toolsGetter()[0] = sessionTool
	require.Same(t, allowed, backing[0], "fork getters must return independent slices")
	require.NoError(t, fork.AppendTools(sessionTool))
	fork.RestrictToTools(sessionTool.Name)
	fork.SetDisallowMCPServers(false)
	fork.EnableTool(denied.Name)
	fork.recentToolsCache[0].Description = "fork description"
	fork.AddRecentlyUsedTool(sessionTool)
	require.True(t, source.enableAllTools)
	require.True(t, source.DisallowMCPServers())
	require.Contains(t, source.disableTools, denied.Name)
	require.Equal(t, "original description", source.recentToolsCache[0].Description)
	require.Equal(t, []string{allowed.Name}, source.GetRecentToolNames())
	visible, err := source.GetEnableTools()
	require.NoError(t, err)
	require.Len(t, visible, 1)
	require.Same(t, allowed, visible[0])

	// Later caller-side updates must survive all changes to the session fork.
	callerAdded := aitool.NewWithoutCallback("caller-added")
	require.NoError(t, source.AppendTools(callerAdded))
	fork.RestrictToTools(sessionTool.Name)
	tool, err = source.GetToolByName(callerAdded.Name)
	require.NoError(t, err)
	require.Same(t, callerAdded, tool)
	_, err = fork.GetToolByName(callerAdded.Name)
	require.ErrorContains(t, err, "outside the restricted tool set")

	source.RestrictToTools(allowed.Name)
	restrictedFork := source.ForkForSession()
	require.True(t, restrictedFork.restrictToTools)
	require.False(t, restrictedFork.enableAllTools)
	source.RestrictToTools()
	tool, err = restrictedFork.GetToolByName(allowed.Name)
	require.NoError(t, err)
	require.Same(t, allowed, tool)
	_, err = restrictedFork.GetToolByName(denied.Name)
	require.ErrorContains(t, err, "outside the restricted tool set")
}

func TestForkForSessionRebindsCachedSearchTools(t *testing.T) {
	shared := aitool.NewWithoutCallback("shared")
	sourceOnly := aitool.NewWithoutCallback("source-only")
	source := NewToolManagerByToolGetter(func() []*aitool.Tool {
		return []*aitool.Tool{shared, sourceOnly}
	}, WithAIToolsSearcher(func(_ string, tools []*aitool.Tool) ([]*aitool.Tool, error) {
		return tools, nil
	}))
	sourceSearch, err := source.getSearchTools()
	require.NoError(t, err)
	require.Len(t, sourceSearch, 1)
	source.forgeSearchTool = []*aitool.Tool{aitool.NewWithoutCallback("cached-forge-search")}
	fork := source.ForkForSession()
	require.True(t, fork.enableSearchTool)
	require.True(t, fork.enableForgeSearchTool)
	require.Nil(t, fork.searchTool)
	require.Nil(t, fork.forgeSearchTool)
	fork.DisableTool(sourceOnly.Name)
	forkSearch, err := fork.getSearchTools()
	require.NoError(t, err)
	require.Len(t, forkSearch, 1)
	require.NotSame(t, sourceSearch[0], forkSearch[0])

	result, err := forkSearch[0].Callback(context.Background(), aitool.InvokeParams{"query": "tools"}, nil, io.Discard, io.Discard)
	require.NoError(t, err)
	require.Equal(t, []any{map[string]string{"Name": shared.Name, "Description": ""}}, result,
		"fork search callbacks must apply the fork's policy, not the original receiver")
	result, err = sourceSearch[0].Callback(context.Background(), aitool.InvokeParams{"query": "tools"}, nil, io.Discard, io.Discard)
	require.NoError(t, err)
	require.Len(t, result, 2)
}

func TestForkForSessionNilReceiver(t *testing.T) {
	var manager *AiToolManager
	require.Nil(t, manager.ForkForSession())
}
