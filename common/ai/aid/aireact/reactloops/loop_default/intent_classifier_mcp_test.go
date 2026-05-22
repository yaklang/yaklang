package loop_default

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type intentMCPTestInvoker struct {
	*mock.MockInvoker
	config aicommon.AICallerConfigIf
}

func (i *intentMCPTestInvoker) GetConfig() aicommon.AICallerConfigIf {
	return i.config
}

func setupIsolatedMCPToolConfig(t *testing.T) (serverName, toolName string, cleanup func()) {
	t.Helper()
	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Skip("profile database is not available")
	}
	serverName = "test_mcp_fast_" + utils.RandStringBytes(8)
	toolName = "tool_" + utils.RandStringBytes(6)
	cleanup = func() {
		_ = yakit.DeleteMCPServerToolConfigs(db, serverName)
	}
	return serverName, toolName, cleanup
}

func TestSearchMCPToolsForFastIntent_BM25ByDescription(t *testing.T) {
	serverName, toolName, cleanup := setupIsolatedMCPToolConfig(t)
	defer cleanup()

	db := consts.GetGormProfileDatabase()
	keyword := "zephyr_mcp_kw_" + utils.RandStringBytes(8)
	desc := keyword + " remote MCP echo capability"
	require.NoError(t, yakit.UpsertMCPServerToolMetadata(db, serverName, toolName, desc, "[]"))

	got := searchMCPToolsForFastIntent(db, keyword, 5)
	require.NotEmpty(t, got)
	found := false
	for _, cfg := range got {
		if cfg.ServerName == serverName && cfg.ToolName == toolName {
			found = true
			break
		}
	}
	assert.True(t, found, "expected isolated MCP tool in BM25 results")
}

func TestFastIntentMatch_MCPTools_WhenAllowed(t *testing.T) {
	serverName, toolName, cleanup := setupIsolatedMCPToolConfig(t)
	defer cleanup()

	db := consts.GetGormProfileDatabase()
	keyword := "zephyr_fast_intent_" + utils.RandStringBytes(8)
	require.NoError(t, yakit.UpsertMCPServerToolMetadata(db, serverName, toolName, keyword+" description", "[]"))

	ctx := context.Background()
	inv := &intentMCPTestInvoker{
		MockInvoker: mock.NewMockInvoker(ctx),
		config: aicommon.NewConfig(
			ctx,
			aicommon.WithDisallowMCPServers(false),
		),
	}

	result := FastIntentMatch(inv, "run "+keyword+" now")
	require.NotNil(t, result)
	assert.NotEmpty(t, result.MatchedMCPTools)
	assert.True(t, result.HasMatches())
}

func TestFastIntentMatch_SkipsMCP_WhenDisallowed(t *testing.T) {
	serverName, toolName, cleanup := setupIsolatedMCPToolConfig(t)
	defer cleanup()

	db := consts.GetGormProfileDatabase()
	keyword := "zephyr_fast_block_" + utils.RandStringBytes(8)
	require.NoError(t, yakit.UpsertMCPServerToolMetadata(db, serverName, toolName, keyword+" description", "[]"))

	ctx := context.Background()
	inv := &intentMCPTestInvoker{
		MockInvoker: mock.NewMockInvoker(ctx),
		config: aicommon.NewConfig(
			ctx,
			aicommon.WithDisallowMCPServers(true),
		),
	}

	result := FastIntentMatch(inv, "run "+keyword+" now")
	require.NotNil(t, result)
	assert.Empty(t, result.MatchedMCPTools)
}
