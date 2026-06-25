package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func setupTestSQLiteSession(t *testing.T, workdir string) (*gorm.DB, string) {
	t.Helper()
	db, err := store.OpenSessionDB(workdir)
	require.NoError(t, err)
	sessUUID := uuid.NewString()
	sess := &store.DiscoverySession{
		UUID:         sessUUID,
		CodeRootPath: filepath.Join(workdir, "code"),
		Phase:        "ssa_done",
		CodePathOK:   true,
		Language:     "java",
	}
	repo := store.NewRepository(db)
	require.NoError(t, repo.CreateSession(sess))
	return db, sessUUID
}

func countHttpEndpoints(t *testing.T, db *gorm.DB, sessionUUID string) int {
	t.Helper()
	var count int
	row := db.Raw(`SELECT COUNT(*) FROM http_endpoints WHERE session_id = (SELECT id FROM discovery_sessions WHERE uuid = ?)`, sessionUUID).Row()
	require.NoError(t, row.Scan(&count))
	return count
}

func getSessionMeta(t *testing.T, db *gorm.DB, sessionUUID, column string) string {
	t.Helper()
	var val string
	row := db.Raw(`SELECT `+column+` FROM discovery_sessions WHERE uuid = ?`, sessionUUID).Row()
	err := row.Scan(&val)
	if err != nil {
		return ""
	}
	return val
}

func ensureDir(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
}
