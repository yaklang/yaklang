package consts

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func requireAIEventRecoveryIndexes(t *testing.T, db *gorm.DB) {
	t.Helper()
	for name, want := range map[string][]string{
		"idx_ai_output_events_session_recovery_anchor": {"session_id", "is_recovery_block", "id", "deleted_at"},
		"idx_ai_output_events_session_recovery_block":  {"session_id", "recovery_index_id", "deleted_at", "id"},
	} {
		rows, err := db.Raw(`PRAGMA index_info("` + name + `")`).Rows()
		require.NoError(t, err)
		var columns []string
		for rows.Next() {
			var sequence, columnID int
			var column string
			require.NoError(t, rows.Scan(&sequence, &columnID, &column))
			columns = append(columns, column)
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		require.Equal(t, want, columns, name)
	}
}

func aiEventSchemaVersion(t *testing.T, db *gorm.DB) int {
	t.Helper()
	var version int
	require.NoError(t, db.Raw("PRAGMA schema_version").Row().Scan(&version))
	return version
}

func TestAIEventRecoveryIndexesUpgradeOnce(t *testing.T) {
	t.Setenv(YakitSQLiteProjectMaxOpenConnsEnv, "")
	t.Setenv(YakitSQLiteProjectReadPoolConnsEnv, "")
	path := filepath.Join(t.TempDir(), "released-project.db")
	db, err := createAndConfigDatabase(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if db != nil {
			_ = db.Close()
		}
	})
	// Model-only migration reproduces the released table and its single-column
	// indexes, without the new recovery patch.
	require.NoError(t, db.AutoMigrate(&schema.AiOutputEvent{}).Error)
	for _, event := range []*schema.AiOutputEvent{
		{SessionId: "old-session", IsRecoveryBlock: true, RecoveryIndexID: "old-block", Content: []byte("start"), CallToolID: "old-call"},
		{SessionId: "old-session", RecoveryIndexID: "old-block", Content: []byte("result"), CallToolID: "old-call"},
		{SessionId: "another-session", IsRecoveryBlock: true, RecoveryIndexID: "old-block", Content: []byte("separate")},
		{IsRecoveryBlock: true, Content: []byte("legacy event without a session")},
	} {
		require.NoError(t, db.Create(event).Error)
	}
	deleted := &schema.AiOutputEvent{SessionId: "old-session", IsRecoveryBlock: true, Content: []byte("deleted")}
	require.NoError(t, db.Create(deleted).Error)
	require.NoError(t, db.Delete(deleted).Error)
	var before []*schema.AiOutputEvent
	require.NoError(t, db.Unscoped().Order("id").Find(&before).Error)
	var tableSQL string
	require.NoError(t, db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'ai_output_events'").Row().Scan(&tableSQL))
	var oldIndexes []struct {
		Name string
		SQL  string `gorm:"column:sql"`
	}
	require.NoError(t, db.Raw("SELECT name, sql FROM sqlite_master WHERE type = 'index' AND tbl_name = 'ai_output_events' ORDER BY name").Scan(&oldIndexes).Error)
	require.NotEmpty(t, oldIndexes)
	require.NoError(t, db.Close())

	// Opening an old project through the real upgrade entry point is enough.
	db, err = CreateProjectDatabase(path)
	require.NoError(t, err)
	requireAIEventRecoveryIndexes(t, db)
	var after []*schema.AiOutputEvent
	require.NoError(t, db.Unscoped().Order("id").Find(&after).Error)
	require.Equal(t, before, after, "migration must preserve all event fields and soft deletions")
	var migratedTableSQL string
	require.NoError(t, db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'ai_output_events'").Row().Scan(&migratedTableSQL))
	require.Equal(t, tableSQL, migratedTableSQL, "the event table must not be rewritten")
	for _, index := range oldIndexes {
		var indexSQL string
		require.NoError(t, db.Raw("SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?", index.Name).Row().Scan(&indexSQL))
		require.Equal(t, index.SQL, indexSQL, "old binaries must retain index %s", index.Name)
	}
	version := aiEventSchemaVersion(t, db)
	for i := 0; i < 2; i++ {
		schema.ApplyPatches(db, schema.KEY_SCHEMA_YAKIT_DATABASE)
		require.Equal(t, version, aiEventSchemaVersion(t, db), "repeated patches must not rebuild indexes")
	}
	require.NoError(t, db.Close())
	db, err = CreateProjectDatabase(path)
	require.NoError(t, err)
	requireAIEventRecoveryIndexes(t, db)
	require.Equal(t, version, aiEventSchemaVersion(t, db), "migration must remain complete across process/database reopens")

	// A partially upgraded database completes only the missing index on retry.
	require.NoError(t, db.Exec("DROP INDEX idx_ai_output_events_session_recovery_block").Error)
	version = aiEventSchemaVersion(t, db)
	schema.ApplyPatches(db, schema.KEY_SCHEMA_YAKIT_DATABASE)
	requireAIEventRecoveryIndexes(t, db)
	require.Equal(t, version+1, aiEventSchemaVersion(t, db))
}

func TestAIEventRecoveryIndexesFreshDatabaseQueryPlans(t *testing.T) {
	t.Setenv(YakitSQLiteProjectMaxOpenConnsEnv, "")
	t.Setenv(YakitSQLiteProjectReadPoolConnsEnv, "")
	db, err := CreateProjectDatabase(filepath.Join(t.TempDir(), "fresh-project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	requireAIEventRecoveryIndexes(t, db)
	require.NoError(t, db.Create(&schema.AiOutputEvent{SessionId: "existing-session", IsRecoveryBlock: true, RecoveryIndexID: "block"}).Error)
	require.NoError(t, db.Create(&schema.AiOutputEvent{SessionId: "existing-session", RecoveryIndexID: "block"}).Error)

	// The regression is a query-plan choice, so assert bounded session lookups
	// without relying on timing or on an ANALYZE of the user's database.
	for _, sessionID := range []string{"new-empty-session", "existing-session"} {
		for _, query := range []struct {
			name, sql, index string
			args             []any
		}{
			// YieldModel's pagination COUNT uses Table(), so unlike Find() it
			// does not implicitly filter deleted_at. Cover its SQL as well.
			{"pagination count", "SELECT count(*) FROM ai_output_events WHERE session_id = ? AND is_recovery_block = ?", "session_recovery_anchor", []any{sessionID, true}},
			{"next pagination count", "SELECT count(*) FROM ai_output_events WHERE session_id = ? AND is_recovery_block = ? AND id < ?", "session_recovery_anchor", []any{sessionID, true, 100}},
			{"anchor count", "SELECT count(*) FROM ai_output_events WHERE deleted_at IS NULL AND session_id = ? AND is_recovery_block = ?", "session_recovery_anchor", []any{sessionID, true}},
			{"first anchor", "SELECT * FROM ai_output_events WHERE deleted_at IS NULL AND session_id = ? AND is_recovery_block = ? ORDER BY id DESC LIMIT 1", "session_recovery_anchor", []any{sessionID, true}},
			{"anchor page", "SELECT * FROM ai_output_events WHERE deleted_at IS NULL AND session_id = ? AND is_recovery_block = ? AND id < ? ORDER BY id DESC LIMIT 1024", "session_recovery_anchor", []any{sessionID, true, 100}},
			{"has more", "SELECT count(*) FROM ai_output_events WHERE deleted_at IS NULL AND session_id = ? AND is_recovery_block = ? AND id < ?", "session_recovery_anchor", []any{sessionID, true, 100}},
			{"block count", "SELECT count(*) FROM ai_output_events WHERE deleted_at IS NULL AND session_id = ? AND recovery_index_id = ?", "session_recovery_block", []any{sessionID, "block"}},
			{"block pagination count", "SELECT count(*) FROM ai_output_events WHERE session_id = ? AND recovery_index_id = ?", "session_recovery_block", []any{sessionID, "block"}},
			{"block page", "SELECT * FROM ai_output_events WHERE deleted_at IS NULL AND session_id = ? AND recovery_index_id = ? ORDER BY id ASC LIMIT 1024", "session_recovery_block", []any{sessionID, "block"}},
		} {
			t.Run(sessionID+"/"+query.name, func(t *testing.T) {
				rows, err := db.Raw("EXPLAIN QUERY PLAN "+query.sql, query.args...).Rows()
				require.NoError(t, err)
				defer rows.Close()
				var details []string
				for rows.Next() {
					var id, parent, unused int
					var detail string
					require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
					details = append(details, detail)
				}
				require.NoError(t, rows.Err())
				plan := strings.Join(details, "\n")
				require.Contains(t, plan, "idx_ai_output_events_"+query.index)
				require.Contains(t, plan, "session_id=?")
				if len(query.args) == 3 {
					require.Contains(t, plan, "id<?", "the pagination cursor must narrow the index search")
				}
				if strings.Contains(query.sql, "count(*)") {
					require.Contains(t, plan, "COVERING INDEX", "COUNT must avoid loading large event rows")
				}
				require.NotContains(t, plan, "SCAN ")
				require.NotContains(t, plan, "TEMP B-TREE")
			})
		}
	}
}
