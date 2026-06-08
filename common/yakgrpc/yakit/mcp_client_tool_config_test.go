package yakit

import (
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func newToolConfigDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.MCPClientToolConfig{}).Error)
	return db
}

func TestGetOrCreateMCPClientToolConfig(t *testing.T) {
	db := newToolConfigDB(t)

	t.Run("creates row on first call", func(t *testing.T) {
		cfg, err := GetOrCreateMCPClientToolConfig(db, "tool_a", schema.MCPClientToolSourceBuiltin, "", "desc_a")
		require.NoError(t, err)
		assert.Equal(t, "tool_a", cfg.ToolName)
		assert.Equal(t, schema.MCPClientToolSourceBuiltin, cfg.Source)
		assert.Equal(t, "desc_a", cfg.Description)
		assert.True(t, cfg.Enable)
	})

	t.Run("returns existing row without modifying it", func(t *testing.T) {
		_, err := GetOrCreateMCPClientToolConfig(db, "tool_b", schema.MCPClientToolSourceBuiltin, "", "initial")
		require.NoError(t, err)

		// second call with different description — should NOT update
		cfg2, err := GetOrCreateMCPClientToolConfig(db, "tool_b", schema.MCPClientToolSourceBuiltin, "", "changed")
		require.NoError(t, err)
		assert.Equal(t, "initial", cfg2.Description, "existing row should not be modified")
	})

	t.Run("bridge tool stores server name", func(t *testing.T) {
		cfg, err := GetOrCreateMCPClientToolConfig(db, "mcp_srv1_foo", schema.MCPClientToolSourceBridge, "srv1", "foo desc")
		require.NoError(t, err)
		assert.Equal(t, "srv1", cfg.ServerName)
		assert.Equal(t, schema.MCPClientToolSourceBridge, cfg.Source)
	})
}

func TestUpsertMCPClientToolConfigDescription(t *testing.T) {
	db := newToolConfigDB(t)

	t.Run("creates row when not exists", func(t *testing.T) {
		err := UpsertMCPClientToolConfigDescription(db, "mcp_s_new", schema.MCPClientToolSourceBridge, "s", "new desc")
		require.NoError(t, err)

		cfg, err := GetMCPClientToolConfigByName(db, "mcp_s_new")
		require.NoError(t, err)
		assert.Equal(t, "new desc", cfg.Description)
	})

	t.Run("updates description on existing row", func(t *testing.T) {
		_, err := GetOrCreateMCPClientToolConfig(db, "mcp_s_old", schema.MCPClientToolSourceBridge, "s", "")
		require.NoError(t, err)

		err = UpsertMCPClientToolConfigDescription(db, "mcp_s_old", schema.MCPClientToolSourceBridge, "s", "updated desc")
		require.NoError(t, err)

		cfg, err := GetMCPClientToolConfigByName(db, "mcp_s_old")
		require.NoError(t, err)
		assert.Equal(t, "updated desc", cfg.Description)
	})

	t.Run("enable flag is preserved across description update", func(t *testing.T) {
		_, err := GetOrCreateMCPClientToolConfig(db, "mcp_s_flag", schema.MCPClientToolSourceBridge, "s", "")
		require.NoError(t, err)

		require.NoError(t, SetMCPClientToolEnabled(db, "mcp_s_flag", false))
		require.NoError(t, UpsertMCPClientToolConfigDescription(db, "mcp_s_flag", schema.MCPClientToolSourceBridge, "s", "refreshed"))

		cfg, err := GetMCPClientToolConfigByName(db, "mcp_s_flag")
		require.NoError(t, err)
		assert.False(t, cfg.Enable, "enable flag must survive a description update")
		assert.Equal(t, "refreshed", cfg.Description)
	})
}

func TestSetMCPClientToolEnabled(t *testing.T) {
	db := newToolConfigDB(t)

	t.Run("disable existing tool", func(t *testing.T) {
		_, err := GetOrCreateMCPClientToolConfig(db, "en_tool", schema.MCPClientToolSourceBuiltin, "", "")
		require.NoError(t, err)

		require.NoError(t, SetMCPClientToolEnabled(db, "en_tool", false))

		cfg, err := GetMCPClientToolConfigByName(db, "en_tool")
		require.NoError(t, err)
		assert.False(t, cfg.Enable)
	})

	t.Run("re-enable disabled tool", func(t *testing.T) {
		_, err := GetOrCreateMCPClientToolConfig(db, "dis_tool", schema.MCPClientToolSourceBuiltin, "", "")
		require.NoError(t, err)
		require.NoError(t, SetMCPClientToolEnabled(db, "dis_tool", false))
		require.NoError(t, SetMCPClientToolEnabled(db, "dis_tool", true))

		cfg, err := GetMCPClientToolConfigByName(db, "dis_tool")
		require.NoError(t, err)
		assert.True(t, cfg.Enable)
	})

	t.Run("returns error when tool not found", func(t *testing.T) {
		err := SetMCPClientToolEnabled(db, "nonexistent_tool_xyz", false)
		assert.Error(t, err, "must reject enable/disable of an unknown tool")
	})
}

func TestGetDisabledMCPClientToolNames(t *testing.T) {
	db := newToolConfigDB(t)

	for _, name := range []string{"t1", "t2", "t3", "t4"} {
		_, err := GetOrCreateMCPClientToolConfig(db, name, schema.MCPClientToolSourceBuiltin, "", "")
		require.NoError(t, err)
	}
	require.NoError(t, SetMCPClientToolEnabled(db, "t2", false))
	require.NoError(t, SetMCPClientToolEnabled(db, "t4", false))

	disabled, err := GetDisabledMCPClientToolNames(db)
	require.NoError(t, err)

	assert.Len(t, disabled, 2)
	assert.Contains(t, disabled, "t2")
	assert.Contains(t, disabled, "t4")
	assert.NotContains(t, disabled, "t1")
	assert.NotContains(t, disabled, "t3")
}

func TestMUSTPASS_MigrateMCPClientBridgeToolConfigsServerName(t *testing.T) {
	db := newToolConfigDB(t)
	oldSrv := "srv-old-bridge-migrate"
	newSrv := "srv-new-bridge-migrate"

	_, err := GetOrCreateMCPClientToolConfig(db, MCPBridgeToolCanonicalName(oldSrv, "tool1"), schema.MCPClientToolSourceBridge, oldSrv, "d1")
	require.NoError(t, err)
	require.NoError(t, SetMCPClientToolEnabled(db, MCPBridgeToolCanonicalName(oldSrv, "tool1"), false))

	require.NoError(t, MigrateMCPClientBridgeToolConfigsServerName(db, oldSrv, newSrv))

	got, err := GetMCPClientToolConfigByName(db, MCPBridgeToolCanonicalName(newSrv, "tool1"))
	require.NoError(t, err)
	assert.Equal(t, newSrv, got.ServerName)
	assert.False(t, got.Enable)
	assert.Equal(t, "d1", got.Description)

	_, err = GetMCPClientToolConfigByName(db, MCPBridgeToolCanonicalName(oldSrv, "tool1"))
	require.Error(t, err)
}

func TestMUSTPASS_MigrateMCPClientBridgeToolConfigsServerName_Conflict(t *testing.T) {
	db := newToolConfigDB(t)
	oldSrv := "conflict-old"
	newSrv := "conflict-new"

	require.Error(t, MigrateMCPClientBridgeToolConfigsServerName(db, "", "x"))
	require.Error(t, MigrateMCPClientBridgeToolConfigsServerName(db, "a", ""))

	_, err := GetOrCreateMCPClientToolConfig(db, MCPBridgeToolCanonicalName(newSrv, "dup"), schema.MCPClientToolSourceBridge, newSrv, "")
	require.NoError(t, err)
	_, err = GetOrCreateMCPClientToolConfig(db, MCPBridgeToolCanonicalName(oldSrv, "dup"), schema.MCPClientToolSourceBridge, oldSrv, "")
	require.NoError(t, err)

	err = MigrateMCPClientBridgeToolConfigsServerName(db, oldSrv, newSrv)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestDeleteMCPClientToolConfigsByServerAndNames(t *testing.T) {
	db := newToolConfigDB(t)

	const srvName = "test-srv"
	tools := []string{"mcp_test-srv_alpha", "mcp_test-srv_beta", "mcp_test-srv_gamma"}
	for _, name := range tools {
		_, err := GetOrCreateMCPClientToolConfig(db, name, schema.MCPClientToolSourceBridge, srvName, "desc")
		require.NoError(t, err)
	}
	_, err := GetOrCreateMCPClientToolConfig(db, "mcp_other-srv_delta", schema.MCPClientToolSourceBridge, "other-srv", "desc")
	require.NoError(t, err)

	t.Run("removes stale tools not in keepNames", func(t *testing.T) {
		keep := map[string]struct{}{
			"mcp_test-srv_alpha": {},
		}
		require.NoError(t, DeleteMCPClientToolConfigsByServerAndNames(db, srvName, keep))

		_, err := GetMCPClientToolConfigByName(db, "mcp_test-srv_alpha")
		assert.NoError(t, err)

		_, err = GetMCPClientToolConfigByName(db, "mcp_test-srv_beta")
		assert.Error(t, err)

		_, err = GetMCPClientToolConfigByName(db, "mcp_test-srv_gamma")
		assert.Error(t, err)

		_, err = GetMCPClientToolConfigByName(db, "mcp_other-srv_delta")
		assert.NoError(t, err)
	})

	t.Run("empty keepNames removes all tools for that server", func(t *testing.T) {
		const srvEmpty = "empty-keep-srv"
		_, err := GetOrCreateMCPClientToolConfig(db, "mcp_empty-keep-srv_x", schema.MCPClientToolSourceBridge, srvEmpty, "")
		require.NoError(t, err)

		require.NoError(t, DeleteMCPClientToolConfigsByServerAndNames(db, srvEmpty, map[string]struct{}{}))

		_, err = GetMCPClientToolConfigByName(db, "mcp_empty-keep-srv_x")
		assert.Error(t, err)
	})

	t.Run("hard-deleted tool can be recreated with same name", func(t *testing.T) {
		const srvRecycle = "recycle-srv"
		const toolName = "mcp_recycle-srv_tool"
		_, err := GetOrCreateMCPClientToolConfig(db, toolName, schema.MCPClientToolSourceBridge, srvRecycle, "v1")
		require.NoError(t, err)

		require.NoError(t, DeleteMCPClientToolConfigsByServerAndNames(db, srvRecycle, map[string]struct{}{}))

		var ghostCount int
		require.NoError(t, db.Unscoped().Model(&schema.MCPClientToolConfig{}).
			Where("tool_name = ?", toolName).Count(&ghostCount).Error)
		assert.Equal(t, 0, ghostCount)

		cfg, err := GetOrCreateMCPClientToolConfig(db, toolName, schema.MCPClientToolSourceBridge, srvRecycle, "v2")
		require.NoError(t, err)
		assert.Equal(t, "v2", cfg.Description)
	})
}

func TestQueryMCPClientToolConfigs(t *testing.T) {
	db := newToolConfigDB(t)

	rows := []struct {
		name   string
		source string
		srv    string
		desc   string
	}{
		{"port_scan_start", schema.MCPClientToolSourceBuiltin, "", "Start port scan"},
		{"codec_base64", schema.MCPClientToolSourceBuiltin, "", "Base64 codec"},
		{"mcp_MyServer_foo", schema.MCPClientToolSourceBridge, "MyServer", "Foo tool from MyServer"},
		{"mcp_MyServer_bar", schema.MCPClientToolSourceBridge, "MyServer", "Bar tool from MyServer"},
		{"mcp_OtherSrv_baz", schema.MCPClientToolSourceBridge, "OtherSrv", "Baz tool"},
	}
	for _, r := range rows {
		err := UpsertMCPClientToolConfigDescription(db, r.name, r.source, r.srv, r.desc)
		require.NoError(t, err)
	}
	require.NoError(t, SetMCPClientToolEnabled(db, "codec_base64", false))

	t.Run("returns all without filter", func(t *testing.T) {
		p, cfgs, err := QueryMCPClientToolConfigs(db, &ypb.GetMCPToolListRequest{
			Pagination: &ypb.Paging{Page: 1, Limit: 20},
		})
		require.NoError(t, err)
		assert.Equal(t, 5, p.TotalRecord)
		assert.Len(t, cfgs, 5)
	})

	t.Run("filters by source=builtin", func(t *testing.T) {
		_, cfgs, err := QueryMCPClientToolConfigs(db, &ypb.GetMCPToolListRequest{
			Source:     schema.MCPClientToolSourceBuiltin,
			Pagination: &ypb.Paging{Page: 1, Limit: 20},
		})
		require.NoError(t, err)
		assert.Len(t, cfgs, 2)
	})

	t.Run("filters by source=bridge + server_name", func(t *testing.T) {
		_, cfgs, err := QueryMCPClientToolConfigs(db, &ypb.GetMCPToolListRequest{
			Source:     schema.MCPClientToolSourceBridge,
			ServerName: "MyServer",
			Pagination: &ypb.Paging{Page: 1, Limit: 20},
		})
		require.NoError(t, err)
		assert.Len(t, cfgs, 2)
	})

	t.Run("filters only_enabled", func(t *testing.T) {
		_, cfgs, err := QueryMCPClientToolConfigs(db, &ypb.GetMCPToolListRequest{
			OnlyEnabled: true,
			Pagination:  &ypb.Paging{Page: 1, Limit: 20},
		})
		require.NoError(t, err)
		for _, c := range cfgs {
			assert.True(t, c.Enable)
		}
		assert.Len(t, cfgs, 4, "disabled codec_base64 should be excluded")
	})

	t.Run("keyword matches tool_name", func(t *testing.T) {
		_, cfgs, err := QueryMCPClientToolConfigs(db, &ypb.GetMCPToolListRequest{
			Keyword:    "port_scan",
			Pagination: &ypb.Paging{Page: 1, Limit: 20},
		})
		require.NoError(t, err)
		assert.Len(t, cfgs, 1)
		assert.Equal(t, "port_scan_start", cfgs[0].ToolName)
	})

	t.Run("keyword matches description", func(t *testing.T) {
		_, cfgs, err := QueryMCPClientToolConfigs(db, &ypb.GetMCPToolListRequest{
			Keyword:    "MyServer",
			Pagination: &ypb.Paging{Page: 1, Limit: 20},
		})
		require.NoError(t, err)
		assert.Len(t, cfgs, 2, "description contains 'MyServer' for 2 bridge tools")
	})

	t.Run("pagination works", func(t *testing.T) {
		p1, cfgs1, err := QueryMCPClientToolConfigs(db, &ypb.GetMCPToolListRequest{
			Pagination: &ypb.Paging{Page: 1, Limit: 2},
		})
		require.NoError(t, err)
		assert.Len(t, cfgs1, 2)
		assert.Equal(t, 5, p1.TotalRecord)

		_, cfgs2, err := QueryMCPClientToolConfigs(db, &ypb.GetMCPToolListRequest{
			Pagination: &ypb.Paging{Page: 2, Limit: 2},
		})
		require.NoError(t, err)
		assert.Len(t, cfgs2, 2)

		names1 := map[string]struct{}{}
		for _, c := range cfgs1 {
			names1[c.ToolName] = struct{}{}
		}
		for _, c := range cfgs2 {
			assert.NotContains(t, names1, c.ToolName)
		}
	})
}
