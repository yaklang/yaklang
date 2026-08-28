package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSetMCPToolDescriptionI18n(t *testing.T) {
	item := &ypb.MCPClientToolConfig{Description: "English only"}
	setMCPToolDescriptionI18n(item, "仅中文说明", "English only")
	require.NotNil(t, item.GetDescriptionI18N())
	assert.Equal(t, "English only", item.GetDescriptionI18N().GetEn())
	assert.Equal(t, "仅中文说明", item.GetDescriptionI18N().GetZh())

	item2 := &ypb.MCPClientToolConfig{}
	setMCPToolDescriptionI18n(item2, "", "English fallback")
	require.NotNil(t, item2.GetDescriptionI18N())
	assert.Equal(t, "English fallback", item2.GetDescriptionI18N().GetEn())
	assert.Equal(t, "English fallback", item2.GetDescriptionI18N().GetZh())
}

func TestEnsureMCPToolDescriptionI18nFromDescription(t *testing.T) {
	item := &ypb.MCPClientToolConfig{
		Source:      "bridge",
		Description: "bridge desc",
	}
	ensureMCPToolDescriptionI18n(item)
	require.NotNil(t, item.GetDescriptionI18N())
	assert.Equal(t, "bridge desc", item.GetDescriptionI18N().GetZh())
	assert.Equal(t, "bridge desc", item.GetDescriptionI18N().GetEn())
}

func TestEnsureMCPToolDescriptionI18nSkipsWhenAlreadySet(t *testing.T) {
	item := &ypb.MCPClientToolConfig{
		Source: "builtin",
		DescriptionI18N: &ypb.I18N{
			Zh: "已有中文",
			En: "existing english",
		},
	}
	ensureMCPToolDescriptionI18n(item)
	assert.Equal(t, "已有中文", item.GetDescriptionI18N().GetZh())
	assert.Equal(t, "existing english", item.GetDescriptionI18N().GetEn())
}

func TestGRPC_GetMCPToolList_BuiltinDescriptionI18n(t *testing.T) {
	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	// Sync builtin tool rows (pagination does not affect sync).
	_, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:     "builtin",
		Pagination: &ypb.Paging{Page: 1, Limit: 1},
	})
	require.NoError(t, err)

	const toolName = "enable_global_hotpatch"
	detail, err := grpcSrv.GetMCPToolDetail(ctx, &ypb.GetMCPToolDetailRequest{ToolName: toolName})
	require.NoError(t, err)
	require.Equal(t, toolName, detail.GetToolName())

	require.NotNil(t, detail.GetDescriptionI18N(), "builtin tool should export DescriptionI18n")
	assert.Contains(t, detail.GetDescriptionI18N().GetZh(), "热加载")
	assert.NotEmpty(t, detail.GetDescriptionI18N().GetEn())
	assert.NotEmpty(t, detail.GetDescription(), "AI/MCP English Description must remain")
}
