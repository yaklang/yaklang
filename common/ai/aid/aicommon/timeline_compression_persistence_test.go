package aicommon

import (
	"strconv"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

// Prove the former 1.5MiB limit was application policy, not the storage schema.
// Save must preserve ordinary entries, exact state and summary archives, both
// when persistence succeeds and when the database returns an error.
func TestTimelineCompressionSaveLargeSnapshotWithoutLoss(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	db.DB().SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&schema.AIAgentRuntime{}).Error)
	require.NoError(t, db.Create(&schema.AIAgentRuntime{Uuid: "compression-save", PersistentSession: "compression-save"}).Error)
	tl := NewTimeline(nil, nil)
	tl.SetTimelineContentLimit(1)
	tl.PushText(1, "BEGIN_FULL_HISTORY\n"+strings.Repeat("large historical input ", 100000)+"\nEND_FULL_HISTORY")
	tl.compressedHistory = []*TimelineCompressedHistoryNode{{Version: 1, Text: "ARCHIVED_SUMMARY_KEEP", CoveredEndItemID: 1}}
	before, err := MarshalTimeline(tl)
	require.NoError(t, err)
	require.Greater(t, len(before), 1536*1024)
	tl.Save(db, "compression-save")
	var row schema.AIAgentRuntime
	require.NoError(t, db.Where("uuid = ?", "compression-save").First(&row).Error)
	stored, err := strconv.Unquote(row.QuotedTimeline)
	require.NoError(t, err)
	require.Equal(t, before, stored)
	restored, err := UnmarshalTimeline(stored)
	require.NoError(t, err)
	require.Equal(t, "ARCHIVED_SUMMARY_KEEP", restored.compressedHistory[0].Text)
	item, ok := restored.idToTimelineItem.Get(1)
	require.True(t, ok)
	require.Contains(t, item.String(), "END_FULL_HISTORY")
	require.NoError(t, db.Close())
	tl.Save(db, "compression-save")
	after, err := MarshalTimeline(tl)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
