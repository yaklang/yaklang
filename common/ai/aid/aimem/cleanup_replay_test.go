package aimem

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

// Opt-in local investigation. Always use a disposable SQLite backup, never a
// live project database: this deliberately executes the real cleanup policy.
func TestCleanupLocalReplay(t *testing.T) {
	path := os.Getenv("YAK_AIMEM_REPLAY_DATABASE")
	if path == "" {
		t.Skip("set YAK_AIMEM_REPLAY_DATABASE to a disposable database backup")
	}
	require.Contains(t, filepath.Base(path), "cleanup-replay", "refuse to clean a database not explicitly named as a replay")
	require.False(t, strings.Contains(path, ".."))
	db, err := gorm.Open("sqlite3", path)
	require.NoError(t, err)
	db.DB().SetMaxOpenConns(1)
	defer db.Close()
	require.NoError(t, db.AutoMigrate(&schema.AIMemoryCollection{}, &schema.ProjectGeneralStorage{}).Error)
	for pass := 1; pass <= 2; pass++ {
		start := time.Now()
		ids, err := ScanAllCleanupMemories(db, "ai_memory_entities_v1", "default", DefaultCleanupConfig())
		require.NoError(t, err)
		if len(ids) > 100 {
			ids = ids[:100]
		}
		require.NoError(t, BatchCleanupMemories(context.Background(), db, "default", ids))
		var remaining int64
		require.NoError(t, db.Model(&schema.AIMemoryEntity{}).Where("session_id = ?", "default").Count(&remaining).Error)
		t.Logf("pass=%d cleaned=%d remaining=%d elapsed=%s", pass, len(ids), remaining, time.Since(start))
	}
}
