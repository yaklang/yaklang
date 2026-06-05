package yakit

import (
	"fmt"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

// newMCPSyncTestDB opens an in-memory SQLite database and migrates the
// MCPServerToolConfig table. Each test gets its own isolated DB instance.
func newMCPSyncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.MCPServerToolConfig{}).Error)
	return db
}

func newMCPSyncServerName() string { return "srv-" + ksuid.New().String() }

// mkEntry builds an MCPToolEntry with FullName pre-computed.
func mkEntry(serverName, toolName, desc, paramsJSON string) MCPToolEntry {
	return MCPToolEntry{
		ToolName:    toolName,
		FullName:    fmt.Sprintf("mcp_%s_%s", serverName, toolName),
		Description: desc,
		ParamsJSON:  paramsJSON,
	}
}

// allToolRows returns every non-deleted row for the given server, ordered by
// tool_name for deterministic assertions.
func allToolRows(t *testing.T, db *gorm.DB, serverName string) []*schema.MCPServerToolConfig {
	t.Helper()
	var rows []*schema.MCPServerToolConfig
	require.NoError(t, db.Model(&schema.MCPServerToolConfig{}).
		Where("server_name = ?", serverName).
		Order("tool_name ASC").
		Find(&rows).Error)
	return rows
}

// findRow is a helper that returns the row for (serverName, toolName) or nil.
func findRow(rows []*schema.MCPServerToolConfig, toolName string) *schema.MCPServerToolConfig {
	for _, r := range rows {
		if r.ToolName == toolName {
			return r
		}
	}
	return nil
}

// TestSyncAndCache_InitialLoad verifies that a first-time sync inserts all live
// tools with enable=true, correct metadata, and a pre-computed full_name.
func TestMUSTPASS_SyncAndCache_InitialLoad(t *testing.T) {
	db := newMCPSyncTestDB(t)
	srv := newMCPSyncServerName()

	live := []MCPToolEntry{
		mkEntry(srv, "tool_a", "desc_a", `[{"name":"x"}]`),
		mkEntry(srv, "tool_b", "desc_b", `[]`),
	}
	require.NoError(t, SyncAndCacheMCPServerTools(db, srv, live))

	rows := allToolRows(t, db, srv)
	require.Len(t, rows, 2)

	rowA := findRow(rows, "tool_a")
	require.NotNil(t, rowA)
	assert.True(t, rowA.Enable, "newly inserted tool should be enabled by default")
	assert.Equal(t, "desc_a", rowA.Description)
	assert.Equal(t, `[{"name":"x"}]`, rowA.ParamsJSON)
	assert.Equal(t, fmt.Sprintf("mcp_%s_tool_a", srv), rowA.FullName, "full_name must be stored verbatim")

	rowB := findRow(rows, "tool_b")
	require.NotNil(t, rowB)
	assert.True(t, rowB.Enable)
	assert.Equal(t, "desc_b", rowB.Description)
	assert.Equal(t, fmt.Sprintf("mcp_%s_tool_b", srv), rowB.FullName)
}

// TestSyncAndCache_RemovedToolIsHardDeleted verifies that a tool absent from the
// live list is hard-deleted and does not appear in subsequent queries.
func TestMUSTPASS_SyncAndCache_RemovedToolIsHardDeleted(t *testing.T) {
	db := newMCPSyncTestDB(t)
	srv := newMCPSyncServerName()

	// First sync: two tools.
	require.NoError(t, SyncAndCacheMCPServerTools(db, srv, []MCPToolEntry{
		mkEntry(srv, "keep", "keeper", `[]`),
		mkEntry(srv, "gone", "going away", `[]`),
	}))

	// Second sync: "gone" is no longer returned by the remote server.
	require.NoError(t, SyncAndCacheMCPServerTools(db, srv, []MCPToolEntry{
		mkEntry(srv, "keep", "keeper", `[]`),
	}))

	rows := allToolRows(t, db, srv)
	require.Len(t, rows, 1, "stale tool row must be hard-deleted")
	assert.Equal(t, "keep", rows[0].ToolName)

	// Confirm the row is also absent from an Unscoped query (no soft-delete remnant).
	var count int
	require.NoError(t, db.Unscoped().Model(&schema.MCPServerToolConfig{}).
		Where("server_name = ? AND tool_name = ?", srv, "gone").
		Count(&count).Error)
	assert.Equal(t, 0, count, "hard-delete must leave no trace in the table")
}

// TestSyncAndCache_MetadataUpdated verifies that description and params_json are
// refreshed when the remote server reports changed values.
func TestMUSTPASS_SyncAndCache_MetadataUpdated(t *testing.T) {
	db := newMCPSyncTestDB(t)
	srv := newMCPSyncServerName()

	require.NoError(t, SyncAndCacheMCPServerTools(db, srv, []MCPToolEntry{
		mkEntry(srv, "tool_x", "old desc", `[]`),
	}))

	// Remote updates both description and schema.
	require.NoError(t, SyncAndCacheMCPServerTools(db, srv, []MCPToolEntry{
		mkEntry(srv, "tool_x", "new desc", `[{"name":"p","type":"string"}]`),
	}))

	rows := allToolRows(t, db, srv)
	require.Len(t, rows, 1)
	assert.Equal(t, "new desc", rows[0].Description)
	assert.Equal(t, `[{"name":"p","type":"string"}]`, rows[0].ParamsJSON)
}

// TestSyncAndCache_EnableFlagPreservedOnMetadataUpdate verifies that the
// user-controlled enable flag is NOT touched when metadata changes.
func TestMUSTPASS_SyncAndCache_EnableFlagPreservedOnMetadataUpdate(t *testing.T) {
	db := newMCPSyncTestDB(t)
	srv := newMCPSyncServerName()

	// Initial sync creates the tool (enable=true by default).
	require.NoError(t, SyncAndCacheMCPServerTools(db, srv, []MCPToolEntry{
		mkEntry(srv, "tool_y", "original", `[]`),
	}))

	// User explicitly disables the tool.
	require.NoError(t, UpsertMCPServerToolConfig(db, srv, "tool_y", false))

	// Remote server reports a different description; sync should update metadata
	// but leave enable=false intact.
	require.NoError(t, SyncAndCacheMCPServerTools(db, srv, []MCPToolEntry{
		mkEntry(srv, "tool_y", "updated desc", `[]`),
	}))

	rows := allToolRows(t, db, srv)
	require.Len(t, rows, 1)
	assert.False(t, rows[0].Enable, "user-set enable=false must be preserved after metadata sync")
	assert.Equal(t, "updated desc", rows[0].Description)
}

// TestSyncAndCache_ToolRename simulates a rename: the old name disappears and a
// new name appears. The old row must be deleted and a new row inserted.
func TestMUSTPASS_SyncAndCache_ToolRename(t *testing.T) {
	db := newMCPSyncTestDB(t)
	srv := newMCPSyncServerName()

	require.NoError(t, SyncAndCacheMCPServerTools(db, srv, []MCPToolEntry{
		mkEntry(srv, "search_file", "search", `[]`),
	}))

	// Remote renames the tool.
	require.NoError(t, SyncAndCacheMCPServerTools(db, srv, []MCPToolEntry{
		mkEntry(srv, "find_file", "search", `[]`),
	}))

	rows := allToolRows(t, db, srv)
	require.Len(t, rows, 1)
	assert.Equal(t, "find_file", rows[0].ToolName)

	// Old row must be fully gone.
	var count int
	require.NoError(t, db.Unscoped().Model(&schema.MCPServerToolConfig{}).
		Where("server_name = ? AND tool_name = ?", srv, "search_file").
		Count(&count).Error)
	assert.Equal(t, 0, count)
}

// TestSyncAndCache_EmptyLiveList removes all tools when the server returns an
// empty tool list (e.g. all tools were unregistered).
func TestMUSTPASS_SyncAndCache_EmptyLiveList(t *testing.T) {
	db := newMCPSyncTestDB(t)
	srv := newMCPSyncServerName()

	require.NoError(t, SyncAndCacheMCPServerTools(db, srv, []MCPToolEntry{
		mkEntry(srv, "tool_a", "a", `[]`),
		mkEntry(srv, "tool_b", "b", `[]`),
	}))

	// Remote now returns nothing.
	require.NoError(t, SyncAndCacheMCPServerTools(db, srv, []MCPToolEntry{}))

	rows := allToolRows(t, db, srv)
	assert.Empty(t, rows, "all rows must be deleted when server reports no tools")
}

// TestSyncAndCache_NoChangeSkipsUpdate verifies that an identical sync does not
// alter the row (updated_at stays the same within the same second).
func TestMUSTPASS_SyncAndCache_NoChangeSkipsUpdate(t *testing.T) {
	db := newMCPSyncTestDB(t)
	srv := newMCPSyncServerName()

	entry := mkEntry(srv, "stable", "same", `[]`)
	require.NoError(t, SyncAndCacheMCPServerTools(db, srv, []MCPToolEntry{entry}))

	rows := allToolRows(t, db, srv)
	require.Len(t, rows, 1)
	updatedAt := rows[0].UpdatedAt

	// Sync again with identical data.
	require.NoError(t, SyncAndCacheMCPServerTools(db, srv, []MCPToolEntry{entry}))

	rows2 := allToolRows(t, db, srv)
	require.Len(t, rows2, 1)
	assert.Equal(t, updatedAt, rows2[0].UpdatedAt,
		"unchanged metadata must not trigger an UPDATE (updated_at must not change)")
}

// TestSyncAndCache_MultipleServersIsolated verifies that syncing one server does
// not affect rows belonging to another server.
func TestMUSTPASS_SyncAndCache_MultipleServersIsolated(t *testing.T) {
	db := newMCPSyncTestDB(t)
	srv1 := newMCPSyncServerName()
	srv2 := newMCPSyncServerName()

	require.NoError(t, SyncAndCacheMCPServerTools(db, srv1, []MCPToolEntry{
		mkEntry(srv1, "tool_1", "from srv1", `[]`),
	}))
	require.NoError(t, SyncAndCacheMCPServerTools(db, srv2, []MCPToolEntry{
		mkEntry(srv2, "tool_2", "from srv2", `[]`),
	}))

	// Wipe srv1's tools.
	require.NoError(t, SyncAndCacheMCPServerTools(db, srv1, []MCPToolEntry{}))

	// srv2 must be untouched.
	rows2 := allToolRows(t, db, srv2)
	require.Len(t, rows2, 1)
	assert.Equal(t, "tool_2", rows2[0].ToolName)
}

// TestSyncAndCache_NewToolDefaultEnable verifies that tools added in a subsequent
// sync (not initial) also get enable=true by default.
func TestMUSTPASS_SyncAndCache_NewToolDefaultEnable(t *testing.T) {
	db := newMCPSyncTestDB(t)
	srv := newMCPSyncServerName()

	require.NoError(t, SyncAndCacheMCPServerTools(db, srv, []MCPToolEntry{
		mkEntry(srv, "existing", "e", `[]`),
	}))

	// A second sync adds a brand-new tool.
	require.NoError(t, SyncAndCacheMCPServerTools(db, srv, []MCPToolEntry{
		mkEntry(srv, "existing", "e", `[]`),
		mkEntry(srv, "brand_new", "n", `[]`),
	}))

	rows := allToolRows(t, db, srv)
	require.Len(t, rows, 2)
	newRow := findRow(rows, "brand_new")
	require.NotNil(t, newRow)
	assert.True(t, newRow.Enable, "tool added in subsequent sync must default to enable=true")
}

// TestGetMCPServerToolConfigByFullName_ExactLookup verifies that the full_name
// column enables O(1) lookup without ambiguity, even when server or tool names
// contain underscores.
func TestMUSTPASS_GetMCPServerToolConfigByFullName_ExactLookup(t *testing.T) {
	db := newMCPSyncTestDB(t)

	// Use server and tool names that both contain underscores to exercise the
	// disambiguation that previously required a full-table scan.
	srv := "my_mcp_server"
	toolA := "my_tool"
	toolB := "server_my_tool" // would collide with a naive string-split approach

	require.NoError(t, SyncAndCacheMCPServerTools(db, srv, []MCPToolEntry{
		mkEntry(srv, toolA, "desc A", `[]`),
		mkEntry(srv, toolB, "desc B", `[]`),
	}))

	fullNameA := fmt.Sprintf("mcp_%s_%s", srv, toolA) // mcp_my_mcp_server_my_tool
	fullNameB := fmt.Sprintf("mcp_%s_%s", srv, toolB) // mcp_my_mcp_server_server_my_tool

	cfgA, err := GetMCPServerToolConfigByFullName(db, fullNameA)
	require.NoError(t, err)
	assert.Equal(t, toolA, cfgA.ToolName)
	assert.Equal(t, "desc A", cfgA.Description)

	cfgB, err := GetMCPServerToolConfigByFullName(db, fullNameB)
	require.NoError(t, err)
	assert.Equal(t, toolB, cfgB.ToolName)
	assert.Equal(t, "desc B", cfgB.Description)

	// Non-existent full name must return an error.
	_, err = GetMCPServerToolConfigByFullName(db, "mcp_ghost_tool")
	require.Error(t, err)
}
