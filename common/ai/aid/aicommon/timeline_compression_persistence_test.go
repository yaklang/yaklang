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

func TestTimelineCompressionRestoreSummaryOnlyRemapsCoverage(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(9000, "completed investigation")
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) { return compressionMockSummary("confirmed finding"), nil })
	_, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	var seq int64
	last := restored.ReassignIDs(func() int64 { seq++; return seq })
	require.EqualValues(t, 1, last)
	require.EqualValues(t, 1, restored.GetMaxID())
	require.EqualValues(t, 1, restored.compressedHead.CoveredEndItemID)
	restored.PushText(2, "new work stays open")
	view := RenderTimelineFrozenOpen(restored)
	require.Contains(t, view.Frozen, "confirmed finding")
	require.Contains(t, view.Open, "new work stays open")
	require.NotContains(t, view.Frozen, "new work stays open")
}

func TestTimelineCompressionCopyDoesNotRetireOriginal(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "shared original")
	copy := tl.CopyReducibleTimelineWithMemory()
	bindCompressionMock(t, copy, func(*AIRequest) (string, error) { return compressionMockSummary("copy summary"), nil })
	_, err := copy.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, []int64{1}, tl.GetTimelineItemIDs())
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "shared original")
	require.Empty(t, copy.GetTimelineItemIDs())
}

func TestTimelineCompressionRollbackAndDeletionIsolation(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "history already summarized")
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) { return compressionMockSummary("confirmed finding"), nil })
	_, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	tl.PushText(2, "later observation")
	before, err := MarshalTimeline(tl)
	require.NoError(t, err)
	require.Error(t, tl.TruncateAfter(0))
	after, err := MarshalTimeline(tl)
	require.NoError(t, err)
	require.Equal(t, before, after, "cannot recover part of a summary: preserve everything")
	restored, err := UnmarshalTimeline(before)
	require.NoError(t, err)
	copy := restored.CopyReducibleTimelineWithMemory()
	require.NoError(t, copy.TruncateAfter(1))
	require.Contains(t, RenderTimelineFrozenOpen(restored).Open, "later observation")
	require.Empty(t, RenderTimelineFrozenOpen(copy).Open)
	ts, ok := copy.idToTs.Get(2)
	require.True(t, ok)
	item, _ := copy.tsToTimelineItem.Get(ts)
	require.True(t, item.deleted, "both restored indexes must retire the same entry")
	restored.SoftDelete(2)
	require.Empty(t, RenderTimelineFrozenOpen(restored).Open)
	require.Contains(t, RenderTimelineFrozenOpen(restored).Frozen, "confirmed finding")
}
