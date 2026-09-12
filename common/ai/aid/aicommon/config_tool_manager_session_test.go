package aicommon

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/schema"
)

func toolManagerSessionTestOptions() []ConfigOption {
	return []ConfigOption{
		WithDisableAutoSkills(true),
		WithDisableCreateDBRuntime(true),
		WithAICallback(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
			return nil, errors.New("unexpected AI invocation in tool manager configuration test")
		}),
	}
}

func newToolManagerSessionTestConfig(t *testing.T, opts ...ConfigOption) *Config {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return NewConfig(ctx, append(toolManagerSessionTestOptions(), opts...)...)
}

func newToolManagerSessionTestManager(t *testing.T) *buildinaitools.AiToolManager {
	t.Helper()
	var tools []*aitool.Tool
	for _, name := range []string{"existing", "enable-then-disable", "enabled-earlier", "enabled-final", "removed-plugin"} {
		tool, err := aitool.New(name, aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			return name + "-ok", nil
		}))
		require.NoError(t, err)
		tools = append(tools, tool)
	}
	return buildinaitools.NewToolManagerByToolGetter(func() []*aitool.Tool { return tools },
		buildinaitools.WithEnabledTools([]string{"existing", "removed-plugin"}),
		buildinaitools.WithDisallowMCPServers(true),
		buildinaitools.WithSearchToolEnabled(false),
		buildinaitools.WithForgeSearchToolEnabled(false),
	)
}

func requireToolManagerSessionNames(t *testing.T, manager *buildinaitools.AiToolManager, names ...string) {
	t.Helper()
	tools, err := manager.GetEnableTools()
	require.NoError(t, err)
	require.ElementsMatch(t, names, toolNames(tools))
}

func toolManagerSessionExtraOption() ConfigOption {
	// NewConfig only stores this declaration; these tests never connect to it.
	return WithExtraMCPServers(&ExtraMCPServer{Server: &schema.MCPServer{Name: "session-config", Type: "unsupported"}})
}

func TestNewConfigToolManagerCapabilityOrder(t *testing.T) {
	for _, isolated := range []bool{false, true} {
		name := "shared_without_extra_mcp"
		if isolated {
			name = "isolated_with_extra_mcp"
		}
		t.Run(name, func(t *testing.T) {
			manager := newToolManagerSessionTestManager(t)
			opts := []ConfigOption{
				WithAiToolManager(manager),
				WithDisallowMCPServers(false),
				WithEnabledCapabilities(
					EnabledCapability{Name: "enable-then-disable", Type: EnabledCapabilityTypeTool},
					EnabledCapability{Name: "enabled-earlier", Type: EnabledCapabilityTypeTool},
				),
				WithDisabledCapabilities(
					EnabledCapability{Name: "enable-then-disable", Type: EnabledCapabilityTypeTool},
					EnabledCapability{Name: "removed-plugin", Type: EnabledCapabilityTypePlugin},
				),
				WithEnabledCapabilities(EnabledCapability{Name: "enabled-final", Type: EnabledCapabilityTypeTool}),
			}
			if isolated {
				opts = append(opts, toolManagerSessionExtraOption())
			}
			cfg := newToolManagerSessionTestConfig(t, opts...)
			requireToolManagerSessionNames(t, cfg.AiToolManager, "existing", "enabled-earlier", "enabled-final")
			tool, err := cfg.AiToolManager.GetToolByName("existing")
			require.NoError(t, err)
			result, err := tool.Callback(cfg.Ctx, aitool.InvokeParams{}, nil, io.Discard, io.Discard)
			require.NoError(t, err)
			require.Equal(t, "existing-ok", result, "inherited tools must remain callable in the session")
			require.False(t, cfg.AiToolManager.DisallowMCPServers())
			require.Equal(t, []EnabledCapability{{Name: "enabled-final", Type: EnabledCapabilityTypeTool}}, cfg.GetEnabledCapabilities())
			require.Empty(t, cfg.pendingToolManagerUpdates)
			require.False(t, cfg.collectingToolManagerOptions)
			if isolated {
				require.NotSame(t, manager, cfg.AiToolManager)
				requireToolManagerSessionNames(t, manager, "existing", "removed-plugin")
				require.True(t, manager.DisallowMCPServers())
			} else {
				require.Same(t, manager, cfg.AiToolManager)
			}
		})
	}
}

func TestNewConfigToolManagerRestrictionPrecedesDeferredCapabilities(t *testing.T) {
	manager := newToolManagerSessionTestManager(t)
	cfg := newToolManagerSessionTestConfig(t,
		WithAiToolManager(manager),
		WithEnabledCapabilities(EnabledCapability{Name: "enabled-earlier", Type: EnabledCapabilityTypeTool}),
		WithRestrictToolsToExtraMCPServers(true),
		toolManagerSessionExtraOption(),
	)
	// Required MCP mounting will subsequently install the exact allowlist.
	// Do not activate other capabilities while preparing that private manager.
	require.NotSame(t, manager, cfg.AiToolManager)
	requireToolManagerSessionNames(t, cfg.AiToolManager, "existing", "removed-plugin")
	requireToolManagerSessionNames(t, manager, "existing", "removed-plugin")
}

func TestNewConfigToolManagerCapabilityRestrictionOrderWithoutExtraMCP(t *testing.T) {
	for _, restrictedFirst := range []bool{false, true} {
		name := "restricted_final_option"
		wantEnabled := "enabled-earlier"
		if restrictedFirst {
			name = "restricted_first_option"
			wantEnabled = "enabled-final"
		}
		t.Run(name, func(t *testing.T) {
			manager := newToolManagerSessionTestManager(t)
			cfg := newToolManagerSessionTestConfig(t,
				WithAiToolManager(manager),
				WithRestrictToolsToExtraMCPServers(restrictedFirst),
				WithEnabledCapabilities(EnabledCapability{Name: "enabled-earlier", Type: EnabledCapabilityTypeTool}),
				WithRestrictToolsToExtraMCPServers(!restrictedFirst),
				WithEnabledCapabilities(EnabledCapability{Name: "enabled-final", Type: EnabledCapabilityTypeTool}),
			)
			require.Same(t, manager, cfg.AiToolManager)
			requireToolManagerSessionNames(t, manager, "existing", "removed-plugin", wantEnabled)
		})
	}
}

func TestNewConfigToolManagerUpdatesKeepOriginalTargets(t *testing.T) {
	for _, isolated := range []bool{false, true} {
		name := "shared_without_extra_mcp"
		if isolated {
			name = "isolated_with_extra_mcp"
		}
		t.Run(name, func(t *testing.T) {
			first := newToolManagerSessionTestManager(t)
			last := newToolManagerSessionTestManager(t)
			opts := []ConfigOption{
				WithAiToolManager(first),
				WithDisallowMCPServers(false),
				WithDisabledCapabilities(EnabledCapability{Name: "existing", Type: EnabledCapabilityTypeTool}),
				WithToolManager(last),
				WithDisallowMCPServers(true),
			}
			if isolated {
				opts = append(opts, toolManagerSessionExtraOption())
			}
			cfg := newToolManagerSessionTestConfig(t, opts...)
			requireToolManagerSessionNames(t, cfg.AiToolManager, "existing", "removed-plugin")
			require.True(t, cfg.AiToolManager.DisallowMCPServers())
			requireToolManagerSessionNames(t, last, "existing", "removed-plugin")
			if isolated {
				require.NotSame(t, last, cfg.AiToolManager)
				requireToolManagerSessionNames(t, first, "existing", "removed-plugin")
				require.True(t, first.DisallowMCPServers())
			} else {
				require.Same(t, last, cfg.AiToolManager)
				requireToolManagerSessionNames(t, first, "removed-plugin")
				require.False(t, first.DisallowMCPServers())
			}
		})
	}
}

func TestNewConfigToolManagerOptionsRemainImmediateAfterConstruction(t *testing.T) {
	manager := newToolManagerSessionTestManager(t)
	cfg := newToolManagerSessionTestConfig(t, WithAiToolManager(manager), WithDisallowMCPServers(false))
	require.Same(t, manager, cfg.AiToolManager)
	require.False(t, manager.DisallowMCPServers())
	require.NoError(t, WithDisallowMCPServers(true)(cfg))
	require.True(t, manager.DisallowMCPServers())
	require.NoError(t, WithDisabledCapabilities(EnabledCapability{Name: "existing", Type: EnabledCapabilityTypeTool})(cfg))
	requireToolManagerSessionNames(t, manager, "removed-plugin")
	require.NoError(t, WithEnabledCapabilities(EnabledCapability{Name: "existing", Type: EnabledCapabilityTypeTool})(cfg))
	requireToolManagerSessionNames(t, manager, "existing", "removed-plugin")
	tool, err := manager.GetToolByName("existing")
	require.NoError(t, err)
	result, err := tool.Callback(cfg.Ctx, aitool.InvokeParams{}, nil, io.Discard, io.Discard)
	require.NoError(t, err)
	require.Equal(t, "existing-ok", result)
	require.NoError(t, WithDisallowMCPServers(false)(cfg))
	require.False(t, manager.DisallowMCPServers())
}

func TestNewConfigToolManagerDefaultPolicy(t *testing.T) {
	for _, disallow := range []bool{false, true} {
		cfg := newToolManagerSessionTestConfig(t, WithDisallowMCPServers(!disallow), WithDisallowMCPServers(disallow))
		require.NotNil(t, cfg.AiToolManager)
		require.Equal(t, disallow, cfg.DisallowMCPServers)
		require.Equal(t, disallow, cfg.AiToolManager.DisallowMCPServers())
	}
}

func TestNewConfigToolManagerOriginOptionsDoNotAccumulate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	manager := newToolManagerSessionTestManager(t)
	optionCalls := 0
	opts := append(toolManagerSessionTestOptions(),
		WithAiToolManager(manager),
		WithDisallowMCPServers(false),
		toolManagerSessionExtraOption(),
		func(*Config) error { optionCalls++; return nil },
	)
	originalCount := len(opts)
	for attempt := 1; attempt <= 3; attempt++ {
		cfg := NewConfig(ctx, opts...)
		require.Equal(t, attempt, optionCalls, "user options must execute exactly once per construction")
		require.Len(t, cfg.OriginOptions(), originalCount)
		require.NotSame(t, manager, cfg.AiToolManager)
		require.True(t, manager.DisallowMCPServers())
		opts = cfg.OriginOptions()
	}
}
