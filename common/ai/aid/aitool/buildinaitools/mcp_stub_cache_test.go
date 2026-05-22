package buildinaitools

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestBuildStubToolFromMCPCache_MarksPendingAndReturnsInitializing(t *testing.T) {
	cfg := &schema.MCPServerToolConfig{
		ServerName:  "srv",
		ToolName:    "echo",
		Enable:      true,
		Description: "echo remote data",
		ParamsJSON:  "[]",
	}
	fullName := "mcp_srv_echo"
	stub := BuildStubToolFromMCPCachePublic(fullName, cfg)
	require.NotNil(t, stub)
	assert.True(t, IsMCPPendingStub(stub))
	assert.Equal(t, fullName, stub.Name)

	_, err := stub.InvokeWithParams(map[string]any{})
	require.Error(t, err)
	assert.True(t, IsMCPInitializingError(err))
	assert.Contains(t, err.Error(), "not yet available")
}

func TestGetToolByName_MCPCacheStub_WhenEnabled(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Skip("profile database is not available")
	}
	serverName := "stub_srv_" + utils.RandStringBytes(8)
	toolName := "echo_" + utils.RandStringBytes(6)
	defer func() { _ = yakit.DeleteMCPServerToolConfigs(db, serverName) }()

	desc := "unique mcp stub kw " + utils.RandStringBytes(8)
	require.NoError(t, yakit.UpsertMCPServerToolMetadata(db, serverName, toolName, desc, "[]"))

	fullName := fmt.Sprintf("mcp_%s_%s", serverName, toolName)
	mgr := NewToolManagerByToolGetter(func() []*aitool.Tool { return nil })
	got, err := mgr.GetToolByName(fullName)
	require.NoError(t, err)
	assert.True(t, IsMCPPendingStub(got))
}

func TestGetToolByName_MCPCacheStub_SkipsWhenDisabled(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Skip("profile database is not available")
	}
	serverName := "stub_srv_" + utils.RandStringBytes(8)
	toolName := "echo_" + utils.RandStringBytes(6)
	defer func() { _ = yakit.DeleteMCPServerToolConfigs(db, serverName) }()

	desc := "disabled mcp stub kw " + utils.RandStringBytes(8)
	require.NoError(t, yakit.UpsertMCPServerToolMetadata(db, serverName, toolName, desc, "[]"))
	require.NoError(t, yakit.UpsertMCPServerToolConfig(db, serverName, toolName, false))

	fullName := fmt.Sprintf("mcp_%s_%s", serverName, toolName)
	mgr := NewToolManagerByToolGetter(func() []*aitool.Tool { return nil })
	_, err := mgr.GetToolByName(fullName)
	require.Error(t, err)
}

func TestGetToolByName_MCPCacheStub_SkipsWhenRuntimeDisallowsMCP(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Skip("profile database is not available")
	}
	serverName := "stub_srv_" + utils.RandStringBytes(8)
	toolName := "echo_" + utils.RandStringBytes(6)
	defer func() { _ = yakit.DeleteMCPServerToolConfigs(db, serverName) }()

	desc := "runtime disallow mcp stub kw " + utils.RandStringBytes(8)
	require.NoError(t, yakit.UpsertMCPServerToolMetadata(db, serverName, toolName, desc, "[]"))

	fullName := fmt.Sprintf("mcp_%s_%s", serverName, toolName)
	mgr := NewToolManagerByToolGetter(func() []*aitool.Tool { return nil }, WithDisallowMCPServers(true))
	_, err := mgr.GetToolByName(fullName)
	require.Error(t, err)
}
