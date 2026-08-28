package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/schema"
)

func TestBuiltinToolDescriptionI18nCoverage(t *testing.T) {
	tools := GlobalBuiltinTools()
	require.NotEmpty(t, tools)

	var missing []string
	for name, twh := range tools {
		require.NotNil(t, twh)
		tool := twh.Tool()
		require.NotNil(t, tool, "tool %q", name)
		entry := BuiltinToolDescriptionI18n(name)
		if entry == nil || entry.Zh == "" || entry.En == "" {
			missing = append(missing, name)
		}
	}
	assert.Empty(t, missing, "builtin tools missing schema.I18n Zh/En: %v", missing)
}

func TestToolDescriptionEnglishOnlyInMCPJSON(t *testing.T) {
	tool := mcp.NewTool("demo_tool",
		mcp.WithDescription("English description for AI"),
	)

	raw, err := json.Marshal(tool)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "English description for AI")
	assert.NotContains(t, string(raw), "DescriptionZh")
}

func TestResolveBuiltinToolDescriptionI18n(t *testing.T) {
	i18n := ResolveBuiltinToolDescriptionI18n("enable_global_hotpatch", "long AI english")
	require.NotNil(t, i18n)
	assert.Equal(t, builtinToolDescriptionI18n["enable_global_hotpatch"].Zh, i18n.Zh)
	assert.Equal(t, builtinToolDescriptionI18n["enable_global_hotpatch"].En, i18n.En)

	ypbI18n := i18n.I18nToYPB_I18n()
	require.NotNil(t, ypbI18n)
	assert.Equal(t, i18n.Zh, ypbI18n.Zh)
	assert.Equal(t, i18n.En, ypbI18n.En)

	// unknown tool: fall back to Description for En (Zh empty → NewI18n may mirror En)
	fallback := ResolveBuiltinToolDescriptionI18n("no_such_tool", "English only")
	require.NotNil(t, fallback)
	assert.Equal(t, schema.NewI18n("", "English only").Zh, fallback.Zh)
	assert.Equal(t, "English only", fallback.En)
}
