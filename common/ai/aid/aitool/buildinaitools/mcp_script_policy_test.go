package buildinaitools

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestMCPScriptToolsRespectRuntimePolicy(t *testing.T) {
	for _, name := range []string{"manage_mcp_server", "query_mcp_tools", "call_mcp_tool", "mcp_server_echo"} {
		t.Run(name, func(t *testing.T) {
			tool := aitool.NewWithoutCallback(name)
			mgr := NewToolManagerByToolGetter(func() []*aitool.Tool { return []*aitool.Tool{tool} }, WithEnabledTools([]string{name}))
			_, err := mgr.GetToolByName(name)
			require.NoError(t, err)
			mgr.SetDisallowMCPServers(true)
			_, err = mgr.GetToolByName(name)
			require.ErrorContains(t, err, "disabled")
			enabled, err := mgr.GetEnableTools()
			require.NoError(t, err)
			for _, item := range enabled {
				require.NotEqual(t, name, item.Name)
			}
		})
	}
}
