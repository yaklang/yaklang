package yakit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestRecoveryHistoryIndexMigrationPreservesResults(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AiOutputEvent{}).Error)
	create := func(session, block string, anchor, deleted bool) uint {
		t.Helper()
		event := &schema.AiOutputEvent{SessionId: session, RecoveryIndexID: block, IsRecoveryBlock: anchor}
		require.NoError(t, db.Create(event).Error)
		if deleted {
			require.NoError(t, db.Delete(event).Error)
		}
		return event.ID
	}
	oldest := create("session", "", true, false)
	toolStart := create("session", "shared-tool", true, false)
	create("other-session", "shared-tool", true, false)
	create("other-session", "shared-tool", false, false)
	create("session", "shared-tool", false, true)
	toolResult := create("session", "shared-tool", false, false)
	single := create("session", "", true, false)
	create("session", "", true, true)
	streamStart := create("session", "stream", true, false)
	streamDelta := create("session", "stream", false, false)
	streamEnd := create("session", "stream", false, false)

	// Exercise the unchanged public recovery API before and after the same
	// patch applied when Yakit/Memfit opens a project. Pagination, grouping,
	// soft deletion and session isolation must all retain their old meanings.
	for _, phase := range []string{"released indexes", "upgraded indexes"} {
		t.Run(phase, func(t *testing.T) {
			if phase == "upgraded indexes" {
				schema.ApplyPatches(db, schema.KEY_SCHEMA_YAKIT_DATABASE)
				var count int
				require.NoError(t, db.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name IN (?, ?)",
					"idx_ai_output_events_session_recovery_anchor", "idx_ai_output_events_session_recovery_block").Row().Scan(&count))
				require.Equal(t, 2, count)
			}
			collect := func(session string, startID int64) ([]uint, *AIEventRecoveryHistoryResult) {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				stream, result, err := YieldAIEventRecoveryHistory(ctx, db, session, startID, 2)
				require.NoError(t, err)
				var ids []uint
				for event := range stream {
					ids = append(ids, event.ID)
				}
				require.NoError(t, ctx.Err())
				return ids, result
			}
			ids, first := collect("session", 0)
			require.Equal(t, []uint{streamStart, streamDelta, streamEnd, single}, ids)
			require.Equal(t, 2, first.BlockCount)
			require.Equal(t, 4, first.EventCount)
			require.Equal(t, int64(single), first.NextStartID)
			require.True(t, first.HasMore)
			ids, last := collect("session", first.NextStartID)
			require.Equal(t, []uint{toolStart, toolResult, oldest}, ids)
			require.Equal(t, 2, last.BlockCount)
			require.Equal(t, 3, last.EventCount)
			require.Equal(t, int64(oldest), last.NextStartID)
			require.False(t, last.HasMore)
			ids, empty := collect("new-session", 0)
			require.Empty(t, ids)
			require.Equal(t, &AIEventRecoveryHistoryResult{}, empty)
		})
	}
	// A write from an old client after migration still uses the same table.
	newAnchor := create("session", "", true, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, result, err := YieldAIEventRecoveryHistory(ctx, db, "session", 0, 2)
	require.NoError(t, err)
	var ids []uint
	for event := range stream {
		ids = append(ids, event.ID)
	}
	require.NoError(t, ctx.Err())
	require.Equal(t, []uint{newAnchor, streamStart, streamDelta, streamEnd}, ids)
	require.Equal(t, int64(streamStart), result.NextStartID)
	require.True(t, result.HasMore)
}
