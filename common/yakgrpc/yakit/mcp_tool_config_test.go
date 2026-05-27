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
	require.NoError(t, db.AutoMigrate(&schema.MCPToolConfig{}).Error)
	return db
}

func TestGetOrCreateMCPToolConfig(t *testing.T) {
	db := newToolConfigDB(t)

	t.Run("creates row on first call", func(t *testing.T) {
		cfg, err := GetOrCreateMCPToolConfig(db, "tool_a", schema.MCPToolSourceBuiltin, "", "desc_a")
		require.NoError(t, err)
		assert.Equal(t, "tool_a", cfg.ToolName)
		assert.Equal(t, schema.MCPToolSourceBuiltin, cfg.Source)
		assert.Equal(t, "desc_a", cfg.Description)
		assert.True(t, cfg.Enable)
	})

	t.Run("returns existing row without modifying it", func(t *testing.T) {
		// first call — creates the row
		_, err := GetOrCreateMCPToolConfig(db, "tool_b", schema.MCPToolSourceBuiltin, "", "initial")
		require.NoError(t, err)

		// second call with different description — should NOT update
		cfg2, err := GetOrCreateMCPToolConfig(db, "tool_b", schema.MCPToolSourceBuiltin, "", "changed")
		require.NoError(t, err)
		assert.Equal(t, "initial", cfg2.Description, "existing row should not be modified")
	})

	t.Run("bridge tool stores server name", func(t *testing.T) {
		cfg, err := GetOrCreateMCPToolConfig(db, "mcp_srv1_foo", schema.MCPToolSourceBridge, "srv1", "foo desc")
		require.NoError(t, err)
		assert.Equal(t, "srv1", cfg.ServerName)
		assert.Equal(t, schema.MCPToolSourceBridge, cfg.Source)
	})
}

func TestUpsertMCPToolConfigDescription(t *testing.T) {
	db := newToolConfigDB(t)

	t.Run("creates row when not exists", func(t *testing.T) {
		err := UpsertMCPToolConfigDescription(db, "mcp_s_new", schema.MCPToolSourceBridge, "s", "new desc")
		require.NoError(t, err)

		cfg, err := GetMCPToolConfigByName(db, "mcp_s_new")
		require.NoError(t, err)
		assert.Equal(t, "new desc", cfg.Description)
	})

	t.Run("updates description on existing row", func(t *testing.T) {
		// pre-create with empty description (simulates old row)
		_, err := GetOrCreateMCPToolConfig(db, "mcp_s_old", schema.MCPToolSourceBridge, "s", "")
		require.NoError(t, err)

		err = UpsertMCPToolConfigDescription(db, "mcp_s_old", schema.MCPToolSourceBridge, "s", "updated desc")
		require.NoError(t, err)

		cfg, err := GetMCPToolConfigByName(db, "mcp_s_old")
		require.NoError(t, err)
		assert.Equal(t, "updated desc", cfg.Description)
	})

	t.Run("enable flag is preserved across description update", func(t *testing.T) {
		_, err := GetOrCreateMCPToolConfig(db, "mcp_s_flag", schema.MCPToolSourceBridge, "s", "")
		require.NoError(t, err)

		// disable the tool
		require.NoError(t, SetMCPToolEnabled(db, "mcp_s_flag", false))

		// update description
		require.NoError(t, UpsertMCPToolConfigDescription(db, "mcp_s_flag", schema.MCPToolSourceBridge, "s", "refreshed"))

		cfg, err := GetMCPToolConfigByName(db, "mcp_s_flag")
		require.NoError(t, err)
		assert.False(t, cfg.Enable, "enable flag must survive a description update")
		assert.Equal(t, "refreshed", cfg.Description)
	})
}

func TestSetMCPToolEnabled(t *testing.T) {
	db := newToolConfigDB(t)

	t.Run("disable existing tool", func(t *testing.T) {
		_, err := GetOrCreateMCPToolConfig(db, "en_tool", schema.MCPToolSourceBuiltin, "", "")
		require.NoError(t, err)

		require.NoError(t, SetMCPToolEnabled(db, "en_tool", false))

		cfg, err := GetMCPToolConfigByName(db, "en_tool")
		require.NoError(t, err)
		assert.False(t, cfg.Enable)
	})

	t.Run("re-enable disabled tool", func(t *testing.T) {
		_, err := GetOrCreateMCPToolConfig(db, "dis_tool", schema.MCPToolSourceBuiltin, "", "")
		require.NoError(t, err)
		require.NoError(t, SetMCPToolEnabled(db, "dis_tool", false))
		require.NoError(t, SetMCPToolEnabled(db, "dis_tool", true))

		cfg, err := GetMCPToolConfigByName(db, "dis_tool")
		require.NoError(t, err)
		assert.True(t, cfg.Enable)
	})

	t.Run("creates row when tool not found yet", func(t *testing.T) {
		require.NoError(t, SetMCPToolEnabled(db, "brand_new_tool", false))

		cfg, err := GetMCPToolConfigByName(db, "brand_new_tool")
		require.NoError(t, err)
		assert.False(t, cfg.Enable)
	})
}

func TestGetDisabledMCPToolNames(t *testing.T) {
	db := newToolConfigDB(t)

	// seed: two enabled, two disabled
	for _, name := range []string{"t1", "t2", "t3", "t4"} {
		_, err := GetOrCreateMCPToolConfig(db, name, schema.MCPToolSourceBuiltin, "", "")
		require.NoError(t, err)
	}
	require.NoError(t, SetMCPToolEnabled(db, "t2", false))
	require.NoError(t, SetMCPToolEnabled(db, "t4", false))

	disabled, err := GetDisabledMCPToolNames(db)
	require.NoError(t, err)

	assert.Len(t, disabled, 2)
	assert.Contains(t, disabled, "t2")
	assert.Contains(t, disabled, "t4")
	assert.NotContains(t, disabled, "t1")
	assert.NotContains(t, disabled, "t3")
}

func TestDeleteMCPToolConfigsByServerAndNames(t *testing.T) {
	db := newToolConfigDB(t)

	const srvName = "test-srv"
	tools := []string{"mcp_test-srv_alpha", "mcp_test-srv_beta", "mcp_test-srv_gamma"}
	for _, name := range tools {
		_, err := GetOrCreateMCPToolConfig(db, name, schema.MCPToolSourceBridge, srvName, "desc")
		require.NoError(t, err)
	}
	// unrelated tool that must survive deletion
	_, err := GetOrCreateMCPToolConfig(db, "mcp_other-srv_delta", schema.MCPToolSourceBridge, "other-srv", "desc")
	require.NoError(t, err)

	t.Run("removes stale tools not in keepNames", func(t *testing.T) {
		keep := map[string]struct{}{
			"mcp_test-srv_alpha": {},
			// beta and gamma are gone from remote
		}
		require.NoError(t, DeleteMCPToolConfigsByServerAndNames(db, srvName, keep))

		// alpha survives
		_, err := GetMCPToolConfigByName(db, "mcp_test-srv_alpha")
		assert.NoError(t, err)

		// beta and gamma are deleted
		_, err = GetMCPToolConfigByName(db, "mcp_test-srv_beta")
		assert.Error(t, err)

		_, err = GetMCPToolConfigByName(db, "mcp_test-srv_gamma")
		assert.Error(t, err)

		// unrelated server's tool is untouched
		_, err = GetMCPToolConfigByName(db, "mcp_other-srv_delta")
		assert.NoError(t, err)
	})

	t.Run("empty keepNames removes all tools for that server", func(t *testing.T) {
		const srvEmpty = "empty-keep-srv"
		_, err := GetOrCreateMCPToolConfig(db, "mcp_empty-keep-srv_x", schema.MCPToolSourceBridge, srvEmpty, "")
		require.NoError(t, err)

		require.NoError(t, DeleteMCPToolConfigsByServerAndNames(db, srvEmpty, map[string]struct{}{}))

		_, err = GetMCPToolConfigByName(db, "mcp_empty-keep-srv_x")
		assert.Error(t, err)
	})
}

func TestQueryMCPToolConfigs(t *testing.T) {
	db := newToolConfigDB(t)

	// seed data
	rows := []struct {
		name   string
		source string
		srv    string
		desc   string
	}{
		{"port_scan_start", schema.MCPToolSourceBuiltin, "", "Start port scan"},
		{"codec_base64", schema.MCPToolSourceBuiltin, "", "Base64 codec"},
		{"mcp_MyServer_foo", schema.MCPToolSourceBridge, "MyServer", "Foo tool from MyServer"},
		{"mcp_MyServer_bar", schema.MCPToolSourceBridge, "MyServer", "Bar tool from MyServer"},
		{"mcp_OtherSrv_baz", schema.MCPToolSourceBridge, "OtherSrv", "Baz tool"},
	}
	for _, r := range rows {
		err := UpsertMCPToolConfigDescription(db, r.name, r.source, r.srv, r.desc)
		require.NoError(t, err)
	}
	// disable one
	require.NoError(t, SetMCPToolEnabled(db, "codec_base64", false))

	t.Run("returns all without filter", func(t *testing.T) {
		p, cfgs, err := QueryMCPToolConfigs(db, &ypb.GetMCPToolListRequest{
			Pagination: &ypb.Paging{Page: 1, Limit: 20},
		})
		require.NoError(t, err)
		assert.Equal(t, 5, p.TotalRecord)
		assert.Len(t, cfgs, 5)
	})

	t.Run("filters by source=builtin", func(t *testing.T) {
		_, cfgs, err := QueryMCPToolConfigs(db, &ypb.GetMCPToolListRequest{
			Source:     schema.MCPToolSourceBuiltin,
			Pagination: &ypb.Paging{Page: 1, Limit: 20},
		})
		require.NoError(t, err)
		assert.Len(t, cfgs, 2)
	})

	t.Run("filters by source=bridge + server_name", func(t *testing.T) {
		_, cfgs, err := QueryMCPToolConfigs(db, &ypb.GetMCPToolListRequest{
			Source:     schema.MCPToolSourceBridge,
			ServerName: "MyServer",
			Pagination: &ypb.Paging{Page: 1, Limit: 20},
		})
		require.NoError(t, err)
		assert.Len(t, cfgs, 2)
	})

	t.Run("filters only_enabled", func(t *testing.T) {
		_, cfgs, err := QueryMCPToolConfigs(db, &ypb.GetMCPToolListRequest{
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
		_, cfgs, err := QueryMCPToolConfigs(db, &ypb.GetMCPToolListRequest{
			Keyword:    "port_scan",
			Pagination: &ypb.Paging{Page: 1, Limit: 20},
		})
		require.NoError(t, err)
		assert.Len(t, cfgs, 1)
		assert.Equal(t, "port_scan_start", cfgs[0].ToolName)
	})

	t.Run("keyword matches description", func(t *testing.T) {
		_, cfgs, err := QueryMCPToolConfigs(db, &ypb.GetMCPToolListRequest{
			Keyword:    "MyServer",
			Pagination: &ypb.Paging{Page: 1, Limit: 20},
		})
		require.NoError(t, err)
		assert.Len(t, cfgs, 2, "description contains 'MyServer' for 2 bridge tools")
	})

	t.Run("pagination works", func(t *testing.T) {
		p1, cfgs1, err := QueryMCPToolConfigs(db, &ypb.GetMCPToolListRequest{
			Pagination: &ypb.Paging{Page: 1, Limit: 2},
		})
		require.NoError(t, err)
		assert.Len(t, cfgs1, 2)
		assert.Equal(t, 5, p1.TotalRecord)

		_, cfgs2, err := QueryMCPToolConfigs(db, &ypb.GetMCPToolListRequest{
			Pagination: &ypb.Paging{Page: 2, Limit: 2},
		})
		require.NoError(t, err)
		assert.Len(t, cfgs2, 2)

		// page 1 and page 2 must be disjoint
		names1 := map[string]struct{}{}
		for _, c := range cfgs1 {
			names1[c.ToolName] = struct{}{}
		}
		for _, c := range cfgs2 {
			assert.NotContains(t, names1, c.ToolName)
		}
	})
}
