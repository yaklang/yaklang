package aicommon

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"path/filepath"
	"testing"
)

func TestTimelineHistorySurvivesCompressionAndRestoredIDs(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.AutoMigrate(&schema.AIAgentRuntime{}).Error)
	require.NoError(t, db.Create(&schema.AIAgentRuntime{Uuid: "history-runtime", PersistentSession: "session-a"}).Error)
	tl := NewTimeline(nil, nil)
	tl.PushText(100, "original HTTP sentinel before compression")
	require.NoError(t, tl.saveChecked(db, "session-a"))
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) { return compressionMockSummary("short summary without sentinel"), nil })
	cfg := tl.config.(*Config)
	cfg.DisableCreateDBRuntime = false
	cfg.PersistentSessionId = "session-a"
	cfg.BaseCheckpointableStorage = NewCheckpointableStorageWithDB("history-runtime", db)
	tl.PushText(101, "unsaved 历史 sentinel before compression")
	_, err = tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	require.NoError(t, tl.saveChecked(db, "session-a"))
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	var seq int64
	restored.ReassignIDs(func() int64 { seq++; return seq })
	restored.PushText(100, "new entry reusing original timeline ID")
	require.NoError(t, restored.saveChecked(db, "session-a"))
	for _, mode := range []string{"bm25", "regexp", "literal"} {
		rows, err := yakit.GrepAITimelineHistory(context.Background(), db, "sentinel", yakit.AITimelineHistoryQuery{SessionID: "session-a", Mode: mode, Limit: 10})
		require.NoError(t, err)
		require.Len(t, rows, 2)
	}
	rows, err := yakit.GrepAITimelineHistory(context.Background(), db, "", yakit.AITimelineHistoryQuery{SessionID: "session-a", Mode: "literal", Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 3)
	rows, err = yakit.GrepAITimelineHistory(context.Background(), db, "sentinel", yakit.AITimelineHistoryQuery{SessionID: "session-b", Mode: "bm25", Limit: 10})
	require.NoError(t, err)
	require.Empty(t, rows)
	shortRows, err := yakit.GrepAITimelineHistory(context.Background(), db, "历史", yakit.AITimelineHistoryQuery{SessionID: "session-a", Mode: "bm25", Limit: 10})
	require.NoError(t, err)
	require.Len(t, shortRows, 1)
	_, err = yakit.GrepAITimelineHistory(context.Background(), db, "[invalid", yakit.AITimelineHistoryQuery{SessionID: "session-a", Mode: "regexp", Limit: 10})
	require.Error(t, err)
	// A failed checkpoint must not leave a partial history archive.
	fail := NewTimeline(nil, nil)
	fail.PushText(1, "must roll back")
	require.Error(t, fail.saveChecked(db, "missing-runtime"))
	rows, err = yakit.GrepAITimelineHistory(context.Background(), db, "", yakit.AITimelineHistoryQuery{SessionID: "missing-runtime", Mode: "literal", Limit: 10})
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestTimelineHistoryStableAcrossReassign(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.AutoMigrate(&schema.AIAgentRuntime{}).Error)
	require.NoError(t, db.Create(&schema.AIAgentRuntime{Uuid: "stable-runtime", PersistentSession: "stable"}).Error)
	tl := NewTimeline(nil, nil)
	tl.PushText(99, "same entry")
	require.NoError(t, tl.saveChecked(db, "stable"))
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	restored.ReassignIDs(func() int64 { return 1 })
	require.NoError(t, restored.saveChecked(db, "stable"))
	rows, err := yakit.GrepAITimelineHistory(context.Background(), db, "", yakit.AITimelineHistoryQuery{SessionID: "stable", Mode: "literal", Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
}
