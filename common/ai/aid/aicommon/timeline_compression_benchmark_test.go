package aicommon

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// This recorder also runs on the parent revision. It measures the removed host
// archive preparation without adding synthetic database or embedding latency.
type compressionArchiveRecorder struct {
	calls int
	bytes int
}

func (r *compressionArchiveRecorder) ArchiveCompressedBatch(_ context.Context, batch *TimelineArchiveBatch) (*TimelineArchiveRef, error) {
	r.calls++
	r.bytes += len(batch.MergedContent)
	return &TimelineArchiveRef{ArchiveID: batch.ArchiveID, ReducerKeyID: batch.ReducerKeyID}, nil
}

func (*compressionArchiveRecorder) SearchArchivedBatches(context.Context, *TimelineArchiveSearchQuery) (*TimelineArchiveSearchResult, error) {
	panic("compression must never search an archive")
}

func compressionBenchmarkFixture(cfg *Config) *Timeline {
	timeline := NewTimeline(cfg, nil)
	timeline.SoftBindConfig(cfg, cfg)
	timeline.totalDumpContentLimit = 50 * 1024
	timeline.autoCompressDisabled = true
	for id := int64(1); id <= 60; id++ {
		timeline.PushText(id, strings.Repeat("fixed tool evidence; request completed successfully. ", 12))
	}
	return timeline
}

func TestTimelineCompress_BatchPreservesSummaryWithoutArchive(t *testing.T) {
	registerTimelineTestLiteForge(t)
	store := &compressionArchiveRecorder{}
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true), WithTimelineArchiveStore(store),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			return (&mockedAI{}).CallSpeedPriorityAI(req)
		}),
	)
	timeline := compressionBenchmarkFixture(cfg)
	items := timeline.idToTimelineItem.Values()
	timeline.batchCompressOldestWithRecent(items[:50], items[50:])
	require.Zero(t, store.calls)
	require.Empty(t, timeline.archiveRefs.Values())
	require.NotNil(t, timeline.compressedHead)
	require.Contains(t, timeline.compressedHead.Text, "finding via ai")
	require.EqualValues(t, 50, timeline.compressedHead.CoveredEndItemID)
	require.Len(t, timeline.getActiveTimelineItemIDs(), 10)

	raw, err := MarshalTimeline(timeline)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, timeline.compressedHead, restored.compressedHead)
	require.Len(t, restored.getActiveTimelineItemIDs(), 10)
	require.Empty(t, restored.archiveRefs.Values())
}

// BenchmarkTimelineCompression measures compression and persistence with fixed
// input and a deterministic reducer. Fixture construction is outside the timer.
func BenchmarkTimelineCompression(b *testing.B) {
	registerTimelineTestLiteForge(b)
	for _, mode := range []string{"batch", "emergency"} {
		b.Run(mode, func(b *testing.B) {
			store := &compressionArchiveRecorder{}
			cfg := NewConfig(context.Background(), WithDisableAutoSkills(true), WithTimelineArchiveStore(store),
				WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
					return (&mockedAI{}).CallSpeedPriorityAI(req)
				}),
			)
			fixture, err := MarshalTimeline(compressionBenchmarkFixture(cfg))
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				timeline, err := UnmarshalTimeline(fixture)
				if err != nil {
					b.Fatal(err)
				}
				timeline.SoftBindConfig(cfg, cfg)
				timeline.totalDumpContentLimit = 50 * 1024
				items := timeline.idToTimelineItem.Values()
				b.StartTimer()
				if mode == "batch" {
					timeline.batchCompressOldestWithRecent(items[:50], items[50:])
				} else {
					timeline.emergencyCompress(8 * 1024)
				}
				_, err = MarshalTimeline(timeline)
				if err != nil {
					b.Fatal(err)
				}
				if timeline.compressedHead == nil || timeline.compressedHead.Text == "" {
					b.Fatal("compression lost the summary")
				}
				if mode == "batch" && len(timeline.getActiveTimelineItemIDs()) != 10 {
					b.Fatal("batch compression changed the retained tail")
				}
			}
			b.ReportMetric(float64(store.calls)/float64(b.N), "archives/op")
			b.ReportMetric(float64(store.bytes)/float64(b.N), "archive_bytes/op")
		})
	}
}
