package yakgrpc

import (
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
	item := &ypb.MCPClientToolConfig{Description: "bridge desc"}
	ensureMCPToolDescriptionI18n(item)
	require.NotNil(t, item.GetDescriptionI18N())
	assert.Equal(t, "bridge desc", item.GetDescriptionI18N().GetZh())
	assert.Equal(t, "bridge desc", item.GetDescriptionI18N().GetEn())
}
