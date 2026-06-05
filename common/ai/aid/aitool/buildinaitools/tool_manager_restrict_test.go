package buildinaitools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// RestrictToTools must confine the manager to exactly the named tools and turn
// off the "enable all" shortcut plus both searchers, so a session-scoped MCP
// mount cannot leak builtin/profile tools (e.g. the local "ssa-risk" yak tool).
func TestRestrictToTools(t *testing.T) {
	builtinA := aitool.NewWithoutCallback("builtin_a")
	builtinB := aitool.NewWithoutCallback("builtin_b")
	sessionTool := aitool.NewWithoutCallback("mcp_session_only")

	mgr := NewToolManagerByToolGetter(func() []*aitool.Tool {
		return []*aitool.Tool{builtinA, builtinB, sessionTool}
	}, WithExtendTools([]*aitool.Tool{builtinA, builtinB, sessionTool}, true))

	mgr.RestrictToTools("mcp_session_only")

	tools, err := mgr.GetEnableTools()
	require.NoError(t, err)
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}

	assert.Equal(t, []string{"mcp_session_only"}, names, "only the restricted tool may remain")
	assert.NotContains(t, names, "builtin_a")
	assert.NotContains(t, names, "builtin_b")
	assert.False(t, mgr.enableAllTools)
	assert.False(t, mgr.enableSearchTool, "search tool must be disabled under restriction")
	assert.False(t, mgr.enableForgeSearchTool, "forge search must be disabled under restriction")
}

// A nil receiver must be a safe no-op (defensive guard for loadExtraMCPServers).
func TestRestrictToTools_NilReceiver(t *testing.T) {
	var mgr *AiToolManager
	assert.NotPanics(t, func() { mgr.RestrictToTools("anything") })
}
