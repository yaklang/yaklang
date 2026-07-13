package yakit

import (
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcpcatalog"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestMCPGlobalConfig_CatalogDefaults(t *testing.T) {
	db := newToolConfigDB(t)
	cfg, err := GetMCPGlobalConfig(db)
	require.NoError(t, err)
	require.True(t, cfg.GetUsesCatalogDefaults())
	assert.Equal(t, mcpcatalog.DefaultToolSetNames(), cfg.GetDefaultToolSets())
}

func TestMCPGlobalConfig_SetAndGet(t *testing.T) {
	db := newToolConfigDB(t)

	custom := &ypb.MCPGlobalConfig{
		DefaultToolSets:       []string{"codec", "risk", "reverse_platform"},
		DefaultResourceSets:   []string{"codec"},
		EnableAIToolFramework: true,
	}
	saved, err := SetMCPGlobalConfig(db, custom)
	require.NoError(t, err)
	assert.False(t, saved.GetUsesCatalogDefaults())
	assert.Equal(t, custom.GetDefaultToolSets(), saved.GetDefaultToolSets())
	assert.True(t, saved.GetEnableAIToolFramework())

	loaded, err := GetMCPGlobalConfig(db)
	require.NoError(t, err)
	assert.False(t, loaded.GetUsesCatalogDefaults())
	assert.Equal(t, saved.GetDefaultToolSets(), loaded.GetDefaultToolSets())
}

func TestMCPGlobalConfig_Reset(t *testing.T) {
	db := newToolConfigDB(t)
	_, err := SetMCPGlobalConfig(db, &ypb.MCPGlobalConfig{
		DefaultToolSets: []string{"codec"},
	})
	require.NoError(t, err)

	reset, err := ResetMCPGlobalConfig(db)
	require.NoError(t, err)
	assert.True(t, reset.GetUsesCatalogDefaults())
	assert.Equal(t, mcpcatalog.DefaultToolSetNames(), reset.GetDefaultToolSets())
}

func TestEffectiveDefaultMCPToolSets_UsesCacheAfterSet(t *testing.T) {
	db := newToolConfigDB(t)
	SetCachedMCPGlobalConfigForTest(nil)

	RegisterMCPBuiltinToolDefaultEnableResolver(func(db *gorm.DB, toolName string) (bool, error) {
		toolToSet := map[string]string{
			"exec_codec":   "codec",
			"save_payload": "payload",
		}
		setName, ok := toolToSet[toolName]
		if !ok {
			return false, nil
		}
		defaultSets, err := EffectiveDefaultMCPToolSetMap(db)
		if err != nil {
			return false, err
		}
		_, enabled := defaultSets[setName]
		return enabled, nil
	})

	_, err := SetMCPGlobalConfig(db, &ypb.MCPGlobalConfig{
		DefaultToolSets: []string{"codec", "cve"},
	})
	require.NoError(t, err)

	sets, err := EffectiveDefaultMCPToolSets(db)
	require.NoError(t, err)
	assert.Equal(t, []string{"codec", "cve"}, sets)

	enabled, err := IsBuiltinToolInEffectiveDefaultSets(db, "exec_codec")
	require.NoError(t, err)
	assert.True(t, enabled)

	enabled, err = IsBuiltinToolInEffectiveDefaultSets(db, "save_payload")
	require.NoError(t, err)
	assert.False(t, enabled)
}
