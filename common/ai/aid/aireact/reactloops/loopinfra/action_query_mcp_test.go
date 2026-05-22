package loopinfra

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestBuildMCPToolQueryFeedback_TruncationNotice(t *testing.T) {
	tools := make([]*schema.MCPServerToolConfig, 0, 23)
	for i := 0; i < 23; i++ {
		tools = append(tools, &schema.MCPServerToolConfig{
			ServerName:  "MCD",
			ToolName:    fmt.Sprintf("tool-%02d", i),
			Enable:      true,
			Description: "desc",
		})
	}

	feedback, pageCount, total, truncated, err := buildMCPToolQueryFeedback("MCD", tools, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, 20, pageCount)
	assert.Equal(t, 23, total)
	assert.True(t, truncated)
	assert.Contains(t, feedback, "truncated=true")
	assert.Contains(t, feedback, "complete=false")
	assert.Contains(t, feedback, "Showing tools 1-20 of 23")
	assert.Contains(t, feedback, "**TRUNCATED**: 3 more tool(s) not shown")
	assert.Contains(t, feedback, `"offset":20`)

	feedback, pageCount, total, truncated, err = buildMCPToolQueryFeedback("MCD", tools, 20, 20)
	require.NoError(t, err)
	assert.Equal(t, 3, pageCount)
	assert.Equal(t, 23, total)
	assert.False(t, truncated)
	assert.Contains(t, feedback, "truncated=false")
	assert.Contains(t, feedback, "complete=true")
	assert.Contains(t, feedback, "**COMPLETE**")
	assert.NotContains(t, feedback, "**TRUNCATED**")
}

func TestPaginateMCPToolConfigs(t *testing.T) {
	tools := []*schema.MCPServerToolConfig{
		{ServerName: "s", ToolName: "a"},
		{ServerName: "s", ToolName: "b"},
		{ServerName: "s", ToolName: "c"},
	}

	page, truncated := paginateMCPToolConfigs(tools, 0, 2)
	assert.Len(t, page, 2)
	assert.True(t, truncated)

	page, truncated = paginateMCPToolConfigs(tools, 2, 2)
	assert.Len(t, page, 1)
	assert.False(t, truncated)

	page, truncated = paginateMCPToolConfigs(tools, 3, 2)
	assert.Len(t, page, 0)
	assert.False(t, truncated)
}

func TestPaginateMCPServers(t *testing.T) {
	servers := []*schema.MCPServer{
		{Name: "a"}, {Name: "b"}, {Name: "c"},
	}
	page, truncated := paginateMCPServers(servers, 0, 2)
	assert.Len(t, page, 2)
	assert.True(t, truncated)
}

func TestBuildMCPServerQueryFeedback_TruncationNotice(t *testing.T) {
	servers := make([]*schema.MCPServer, 0, 25)
	for i := 0; i < 25; i++ {
		servers = append(servers, &schema.MCPServer{
			Name: fmt.Sprintf("srv-%02d", i),
			Type: "stdio",
		})
	}

	feedback, pageCount, total, truncated, err := buildMCPServerQueryFeedback("", servers, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, 20, pageCount)
	assert.Equal(t, 25, total)
	assert.True(t, truncated)
	assert.Contains(t, feedback, "Showing servers 1-20 of 25")
	assert.Contains(t, feedback, "truncated=true")
	assert.Contains(t, feedback, "**TRUNCATED**: 5 more server(s) not shown")
	assert.Contains(t, feedback, `"offset":20`)

	feedback, pageCount, total, truncated, err = buildMCPServerQueryFeedback("mcd", servers, 20, 20)
	require.NoError(t, err)
	assert.Equal(t, 5, pageCount)
	assert.False(t, truncated)
	assert.Contains(t, feedback, "Keyword filter: \"mcd\"")
	assert.Contains(t, feedback, "complete=true")
	assert.Contains(t, feedback, "**COMPLETE**")
}

func TestParseMCPQueryOffsetLimit_NumericJSON(t *testing.T) {
	ctx := context.Background()
	action := (&aicommon.ActionMaker{}).ReadFromReader(ctx, strings.NewReader(`{
		"@action":"query_mcp_tools",
		"server_name":"MCD",
		"offset":20,
		"limit":20
	}`))
	require.NotNil(t, action)
	action.WaitStream(ctx)

	offset, limit := parseMCPQueryOffsetLimit(action, defaultMCPToolQueryLimit)
	assert.Equal(t, 20, offset)
	assert.Equal(t, 20, limit)
}

func TestParseMCPQueryOffsetLimit_DefaultLimit(t *testing.T) {
	ctx := context.Background()
	action := (&aicommon.ActionMaker{}).ReadFromReader(ctx, strings.NewReader(`{
		"@action":"query_mcp_tools",
		"server_name":"MCD"
	}`))
	require.NotNil(t, action)
	action.WaitStream(ctx)

	offset, limit := parseMCPQueryOffsetLimit(action, defaultMCPToolQueryLimit)
	assert.Equal(t, 0, offset)
	assert.Equal(t, defaultMCPToolQueryLimit, limit)
}
