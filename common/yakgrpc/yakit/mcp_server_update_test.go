package yakit

import (
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func newMCPUpdateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newMCPSyncTestDB(t)
	require.NoError(t, db.AutoMigrate(&schema.MCPServer{}).Error)
	require.NoError(t, db.AutoMigrate(&schema.MCPClientToolConfig{}).Error)
	return db
}

func seedBridgeToolCache(t *testing.T, db *gorm.DB, serverName, remoteTool, desc string, enable bool) {
	t.Helper()
	canonical := MCPBridgeToolCanonicalName(serverName, remoteTool)
	_, err := GetOrCreateMCPClientToolConfig(db, canonical, schema.MCPClientToolSourceBridge, serverName, desc)
	require.NoError(t, err)
	if !enable {
		require.NoError(t, SetMCPClientToolEnabled(db, canonical, false))
	}
}

func allBridgeRows(t *testing.T, db *gorm.DB, serverName string) []*schema.MCPClientToolConfig {
	t.Helper()
	var rows []*schema.MCPClientToolConfig
	require.NoError(t, db.Where("source = ? AND server_name = ?", schema.MCPClientToolSourceBridge, serverName).
		Find(&rows).Error)
	return rows
}

func createTestMCPServer(t *testing.T, db *gorm.DB, srv *schema.MCPServer) *schema.MCPServer {
	t.Helper()
	require.NoError(t, CreateMCPServer(db, srv))
	got, err := GetMCPServer(db, int64(srv.ID))
	require.NoError(t, err)
	return got
}

func seedToolCache(t *testing.T, db *gorm.DB, serverName string, entries ...MCPToolEntry) {
	t.Helper()
	require.NoError(t, SyncAndCacheMCPServerTools(db, serverName, entries))
}

// --- endpoint change detection (unit) ---

func TestMUSTPASS_McpServerEndpointChanged(t *testing.T) {
	base := &schema.MCPServer{Type: "sse", URL: "http://a/sse", Command: ""}
	same := &schema.MCPServer{Type: "sse", URL: "http://a/sse", Command: ""}
	assert.False(t, mcpServerEndpointChanged(base, same))

	assert.True(t, mcpServerEndpointChanged(base, &schema.MCPServer{Type: "stdio", URL: "http://a/sse", Command: ""}))
	assert.True(t, mcpServerEndpointChanged(base, &schema.MCPServer{Type: "sse", URL: "http://b/sse", Command: ""}))
	assert.True(t, mcpServerEndpointChanged(base, &schema.MCPServer{Type: "sse", URL: "http://a/sse", Command: "npx mcp"}))
	assert.True(t, mcpServerEndpointChanged(nil, same))
}

func TestMUSTPASS_MCPServerToolFullName_MatchesSyncEntry(t *testing.T) {
	srv := "my_srv"
	tool := "my_tool"
	assert.Equal(t, mkEntry(srv, tool, "", "").FullName, MCPServerToolFullName(srv, tool))
}

// --- UpdateMCPServer: rename ---

func TestMUSTPASS_UpdateMCPServer_RenameMigratesToolConfigs(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	oldName := newMCPSyncServerName()
	newName := newMCPSyncServerName()

	srv := createTestMCPServer(t, db, &schema.MCPServer{
		Name: oldName, Type: "sse", URL: "http://127.0.0.1:19999/sse", Enable: true,
	})
	seedToolCache(t, db, oldName, mkEntry(oldName, "echo", "echo tool", `[]`))
	seedBridgeToolCache(t, db, oldName, "echo", "bridge echo", true)

	require.NoError(t, UpdateMCPServer(db, int64(srv.ID), &schema.MCPServer{
		Name: newName, Type: "sse", URL: "http://127.0.0.1:19999/sse", Enable: true,
	}))

	rows := allToolRows(t, db, newName)
	require.Len(t, rows, 1)
	assert.Equal(t, "echo", rows[0].ToolName)
	assert.Equal(t, MCPServerToolFullName(newName, "echo"), rows[0].FullName)
	assert.Empty(t, allToolRows(t, db, oldName))

	got, err := GetMCPServerToolConfigByFullName(db, MCPServerToolFullName(newName, "echo"))
	require.NoError(t, err)
	assert.Equal(t, newName, got.ServerName)

	_, err = GetMCPServerToolConfigByFullName(db, MCPServerToolFullName(oldName, "echo"))
	require.Error(t, err)

	bridgeGot, err := GetMCPClientToolConfigByName(db, MCPBridgeToolCanonicalName(newName, "echo"))
	require.NoError(t, err)
	assert.Equal(t, newName, bridgeGot.ServerName)
	assert.Equal(t, "bridge echo", bridgeGot.Description)
	assert.Empty(t, allBridgeRows(t, db, oldName))
}

func TestMUSTPASS_UpdateMCPServer_RenamePreservesEnableAndMetadata(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	oldName := newMCPSyncServerName()
	newName := newMCPSyncServerName()

	createTestMCPServer(t, db, &schema.MCPServer{Name: oldName, Type: "stdio", Command: "npx -y mcp", Enable: true})
	seedToolCache(t, db, oldName, mkEntry(oldName, "t1", "desc-one", `[{"name":"x"}]`))
	require.NoError(t, UpsertMCPServerToolConfig(db, oldName, "t1", false))
	seedBridgeToolCache(t, db, oldName, "t1", "bridge desc", false)

	srv, _ := GetMCPServerByName(db, oldName)
	require.NoError(t, UpdateMCPServer(db, int64(srv.ID), &schema.MCPServer{
		Name: newName, Type: "stdio", Command: "npx -y mcp", Enable: true,
	}))

	rows := allToolRows(t, db, newName)
	require.Len(t, rows, 1)
	assert.False(t, rows[0].Enable)
	assert.Equal(t, "desc-one", rows[0].Description)
	assert.Equal(t, `[{"name":"x"}]`, rows[0].ParamsJSON)

	bridge, err := GetMCPClientToolConfigByName(db, MCPBridgeToolCanonicalName(newName, "t1"))
	require.NoError(t, err)
	assert.False(t, bridge.Enable)
	assert.Equal(t, "bridge desc", bridge.Description)
}

func TestMUSTPASS_UpdateMCPServer_RenameMultipleTools(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	oldName := newMCPSyncServerName()
	newName := newMCPSyncServerName()

	srv := createTestMCPServer(t, db, &schema.MCPServer{Name: oldName, Type: "sse", URL: "http://h/sse"})
	seedToolCache(t, db, oldName,
		mkEntry(oldName, "alpha", "a", `[]`),
		mkEntry(oldName, "beta", "b", `[]`),
	)

	require.NoError(t, UpdateMCPServer(db, int64(srv.ID), &schema.MCPServer{
		Name: newName, Type: "sse", URL: "http://h/sse",
	}))

	assert.Len(t, allToolRows(t, db, newName), 2)
	assert.Empty(t, allToolRows(t, db, oldName))
}

// --- UpdateMCPServer: endpoint change clears cache ---

func TestMUSTPASS_UpdateMCPServer_EndpointChangeClearsToolCache_URL(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	name := newMCPSyncServerName()

	srv := createTestMCPServer(t, db, &schema.MCPServer{Name: name, Type: "sse", URL: "http://127.0.0.1:18888/sse"})
	seedToolCache(t, db, name, mkEntry(name, "tool_a", "a", `[]`))
	seedBridgeToolCache(t, db, name, "tool_a", "bridge a", true)

	require.NoError(t, UpdateMCPServer(db, int64(srv.ID), &schema.MCPServer{
		Name: name, Type: "sse", URL: "http://127.0.0.1:17777/sse",
	}))
	assert.Empty(t, allToolRows(t, db, name))
	assert.Empty(t, allBridgeRows(t, db, name))
}

func TestMUSTPASS_UpdateMCPServer_EndpointChangeClearsToolCache_Type(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	name := newMCPSyncServerName()

	srv := createTestMCPServer(t, db, &schema.MCPServer{Name: name, Type: "sse", URL: "http://h/sse"})
	seedToolCache(t, db, name, mkEntry(name, "t", "d", `[]`))

	require.NoError(t, UpdateMCPServer(db, int64(srv.ID), &schema.MCPServer{
		Name: name, Type: "stdio", Command: "echo mcp",
	}))
	assert.Empty(t, allToolRows(t, db, name))
}

func TestMUSTPASS_UpdateMCPServer_EndpointChangeClearsToolCache_Command(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	name := newMCPSyncServerName()

	srv := createTestMCPServer(t, db, &schema.MCPServer{Name: name, Type: "stdio", Command: "cmd-a"})
	seedToolCache(t, db, name, mkEntry(name, "t", "d", `[]`))

	require.NoError(t, UpdateMCPServer(db, int64(srv.ID), &schema.MCPServer{
		Name: name, Type: "stdio", Command: "cmd-b",
	}))
	assert.Empty(t, allToolRows(t, db, name))
}

func TestMUSTPASS_UpdateMCPServer_RenameAndEndpointChangeClearsDoesNotLeaveOrphans(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	oldName := newMCPSyncServerName()
	newName := newMCPSyncServerName()

	srv := createTestMCPServer(t, db, &schema.MCPServer{Name: oldName, Type: "sse", URL: "http://old/sse"})
	seedToolCache(t, db, oldName, mkEntry(oldName, "only", "x", `[]`))
	seedBridgeToolCache(t, db, oldName, "only", "bridge only", true)

	require.NoError(t, UpdateMCPServer(db, int64(srv.ID), &schema.MCPServer{
		Name: newName, Type: "sse", URL: "http://new/sse",
	}))

	assert.Empty(t, allToolRows(t, db, oldName))
	assert.Empty(t, allToolRows(t, db, newName), "endpoint change must clear cache even when name changes")
	assert.Empty(t, allBridgeRows(t, db, oldName))
	assert.Empty(t, allBridgeRows(t, db, newName))
}

func TestMUSTPASS_UpdateMCPServer_RenameMigratesBridgeToolConfigs(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	oldName := newMCPSyncServerName()
	newName := newMCPSyncServerName()

	srv := createTestMCPServer(t, db, &schema.MCPServer{Name: oldName, Type: "stdio", Command: "cmd"})
	seedBridgeToolCache(t, db, oldName, "alpha", "desc-a", true)
	seedBridgeToolCache(t, db, oldName, "beta", "desc-b", false)

	require.NoError(t, UpdateMCPServer(db, int64(srv.ID), &schema.MCPServer{
		Name: newName, Type: "stdio", Command: "cmd",
	}))

	assert.Len(t, allBridgeRows(t, db, newName), 2)
	assert.Empty(t, allBridgeRows(t, db, oldName))

	alpha, err := GetMCPClientToolConfigByName(db, MCPBridgeToolCanonicalName(newName, "alpha"))
	require.NoError(t, err)
	assert.True(t, alpha.Enable)
	beta, err := GetMCPClientToolConfigByName(db, MCPBridgeToolCanonicalName(newName, "beta"))
	require.NoError(t, err)
	assert.False(t, beta.Enable)
}

func TestMUSTPASS_UpdateMCPServer_AfterEndpointClearResyncIsConsistent(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	name := newMCPSyncServerName()

	srv := createTestMCPServer(t, db, &schema.MCPServer{Name: name, Type: "sse", URL: "http://v1/sse"})
	seedToolCache(t, db, name, mkEntry(name, "old_tool", "old", `[]`))

	require.NoError(t, UpdateMCPServer(db, int64(srv.ID), &schema.MCPServer{Name: name, Type: "sse", URL: "http://v2/sse"}))
	require.NoError(t, SyncAndCacheMCPServerTools(db, name, []MCPToolEntry{
		mkEntry(name, "new_tool", "new", `[]`),
	}))

	rows := allToolRows(t, db, name)
	require.Len(t, rows, 1)
	assert.Equal(t, "new_tool", rows[0].ToolName)
	_, err := GetMCPServerToolConfigByFullName(db, MCPServerToolFullName(name, "old_tool"))
	require.Error(t, err)
}

// --- UpdateMCPServer: non-endpoint changes keep cache ---

func TestMUSTPASS_UpdateMCPServer_NoOpKeepsToolCache(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	name := newMCPSyncServerName()

	srv := createTestMCPServer(t, db, &schema.MCPServer{Name: name, Type: "sse", URL: "http://same/sse", Enable: true})
	seedToolCache(t, db, name, mkEntry(name, "stable", "s", `[]`))

	require.NoError(t, UpdateMCPServer(db, int64(srv.ID), &schema.MCPServer{
		Name: name, Type: "sse", URL: "http://same/sse", Enable: true,
	}))
	assert.Len(t, allToolRows(t, db, name), 1)
}

func TestMUSTPASS_UpdateMCPServer_HeadersChangeKeepsToolCache(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	name := newMCPSyncServerName()

	srv := createTestMCPServer(t, db, &schema.MCPServer{
		Name: name, Type: "sse", URL: "http://same/sse",
		Headers: schema.MapStringAny{"Authorization": "token-a"},
	})
	seedToolCache(t, db, name, mkEntry(name, "t", "d", `[]`))

	require.NoError(t, UpdateMCPServer(db, int64(srv.ID), &schema.MCPServer{
		Name: name, Type: "sse", URL: "http://same/sse",
		Headers: schema.MapStringAny{"Authorization": "token-b"},
	}))
	assert.Len(t, allToolRows(t, db, name), 1)
}

func TestMUSTPASS_UpdateMCPServer_DisableServerKeepsToolCache(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	name := newMCPSyncServerName()

	srv := createTestMCPServer(t, db, &schema.MCPServer{Name: name, Type: "stdio", Command: "cmd", Enable: true})
	seedToolCache(t, db, name, mkEntry(name, "t", "d", `[]`))

	require.NoError(t, UpdateMCPServer(db, int64(srv.ID), &schema.MCPServer{
		Name: name, Type: "stdio", Command: "cmd", Enable: false,
	}))
	assert.Len(t, allToolRows(t, db, name), 1)
	updated, err := GetMCPServer(db, int64(srv.ID))
	require.NoError(t, err)
	assert.False(t, updated.Enable)
}

// --- validation & errors ---

func TestMUSTPASS_UpdateMCPServer_RejectDuplicateName(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	nameA := newMCPSyncServerName()
	nameB := newMCPSyncServerName()

	createTestMCPServer(t, db, &schema.MCPServer{Name: nameA, Type: "stdio", Command: "echo a"})
	srvB := createTestMCPServer(t, db, &schema.MCPServer{Name: nameB, Type: "stdio", Command: "echo b"})

	err := UpdateMCPServer(db, int64(srvB.ID), &schema.MCPServer{Name: nameA, Type: "stdio", Command: "echo b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestMUSTPASS_UpdateMCPServer_NotFound(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	err := UpdateMCPServer(db, 999999, &schema.MCPServer{Name: "x", Type: "stdio", Command: "c"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- MigrateMCPServerToolConfigsServerName ---

func TestMUSTPASS_MigrateMCPServerToolConfigsServerName_NoRowsOK(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	require.NoError(t, MigrateMCPServerToolConfigsServerName(db, "no-such-old", "no-such-new"))
}

func TestMUSTPASS_MigrateMCPServerToolConfigsServerName_RejectsEmptyName(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	require.Error(t, MigrateMCPServerToolConfigsServerName(db, "", "new"))
	require.Error(t, MigrateMCPServerToolConfigsServerName(db, "old", ""))
}

// --- DeleteMCPServer cascade ---

func TestMUSTPASS_DeleteMCPServer_ClearsToolConfigs(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	name := newMCPSyncServerName()

	srv := createTestMCPServer(t, db, &schema.MCPServer{Name: name, Type: "sse", URL: "http://x/sse"})
	seedToolCache(t, db, name, mkEntry(name, "t", "d", `[]`))
	seedBridgeToolCache(t, db, name, "t", "bridge", true)

	require.NoError(t, DeleteMCPServer(db, int64(srv.ID)))
	assert.Empty(t, allToolRows(t, db, name))
	assert.Empty(t, allBridgeRows(t, db, name))

	_, err := GetMCPServer(db, int64(srv.ID))
	require.Error(t, err)
}

// --- user journey: create → sync → rename → still consistent ---

func TestMUSTPASS_MCPServerUserJourney_CreateSyncRenameStillConsistent(t *testing.T) {
	db := newMCPUpdateTestDB(t)
	displayOld := newMCPSyncServerName()
	displayNew := newMCPSyncServerName()

	srv := createTestMCPServer(t, db, &schema.MCPServer{
		Name: displayOld, Type: "sse", URL: "http://prod/sse", Enable: true,
	})
	seedToolCache(t, db, displayOld,
		mkEntry(displayOld, "search", "search files", `[]`),
		mkEntry(displayOld, "read", "read file", `[]`),
	)
	require.NoError(t, UpsertMCPServerToolConfig(db, displayOld, "read", false))

	require.NoError(t, UpdateMCPServer(db, int64(srv.ID), &schema.MCPServer{
		Name: displayNew, Type: "sse", URL: "http://prod/sse", Enable: true,
	}))

	rows := allToolRows(t, db, displayNew)
	require.Len(t, rows, 2)
	readRow := findRow(rows, "read")
	require.NotNil(t, readRow)
	assert.False(t, readRow.Enable)
	assert.Empty(t, allToolRows(t, db, displayOld))

	// Simulate UI re-fetch after rename: sync same remote tool set under new name.
	require.NoError(t, SyncAndCacheMCPServerTools(db, displayNew, []MCPToolEntry{
		mkEntry(displayNew, "search", "search files updated", `[]`),
		mkEntry(displayNew, "read", "read file updated", `[]`),
	}))
	rows = allToolRows(t, db, displayNew)
	require.Len(t, rows, 2)
	readRow = findRow(rows, "read")
	require.NotNil(t, readRow)
	assert.False(t, readRow.Enable, "re-sync must preserve user enable flag")
	searchRow := findRow(rows, "search")
	require.NotNil(t, searchRow)
	assert.Equal(t, "search files updated", searchRow.Description)
}
