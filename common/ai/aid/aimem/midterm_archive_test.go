package aimem

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestBuildTimelineArchiveMemoryEntities_SplitsIntoChunkedMemories(t *testing.T) {
	originSplitter := timelineArchiveSplitText
	t.Cleanup(func() {
		timelineArchiveSplitText = originSplitter
	})

	timelineArchiveSplitText = func(text string, maxLength int, opts ...any) ([]string, error) {
		require.Equal(t, midtermArchiveChunkContentLimit, maxLength)
		return []string{
			strings.Repeat("A", midtermArchiveChunkContentLimit+120),
			"keep only the necessary timeline details for recall",
		}, nil
	}

	batch := &aicommon.TimelineArchiveBatch{
		ArchiveID:           "timeline-archive-abc",
		Reason:              aicommon.TimelineArchiveReasonBatchCompress,
		Summary:             "timeline summary",
		MergedContent:       strings.Repeat("raw timeline item\n", 200),
		ReducerKeyID:        20,
		SourceStartID:       1,
		SourceEndID:         20,
		ItemCount:           20,
		RepresentativeSnips: []string{"snippet one", "snippet two"},
	}

	entities, err := buildTimelineArchiveMemoryEntities(batch)
	require.NoError(t, err)
	require.Len(t, entities, 3)

	for index, entity := range entities {
		require.NotNil(t, entity)
		require.Contains(t, entity.Id, "timeline-archive-abc-chunk-")
		require.LessOrEqual(t, len([]rune(entity.Content)), midtermArchiveChunkLimit)
		require.Contains(t, entity.Content, "archive_id: timeline-archive-abc")
		require.Contains(t, entity.Tags, midtermMemoryKindTag)
		require.Contains(t, entity.Tags, midtermTagChunkPrefix+strconv.Itoa(index+1))
		require.Contains(t, entity.Tags, midtermTagChunkTotal+strconv.Itoa(len(entities)))
		require.NotEmpty(t, entity.PotentialQuestions)
	}
}

func TestBuildTimelineArchiveSearchResult_DeduplicatesArchiveRefs(t *testing.T) {
	result := buildTimelineArchiveSearchResult(&aicommon.SearchMemoryResult{
		Memories: []*aicommon.MemoryEntity{
			{
				Id:      "timeline-archive-1-chunk-001",
				Content: "chunk 1",
				Tags: []string{
					midtermMemoryKindTag,
					midtermTagArchiveIDPrefix + "timeline-archive-1",
					midtermTagStartPrefix + "1",
					midtermTagEndPrefix + "10",
				},
			},
			{
				Id:      "timeline-archive-1-chunk-002",
				Content: "chunk 2",
				Tags: []string{
					midtermMemoryKindTag,
					midtermTagArchiveIDPrefix + "timeline-archive-1",
					midtermTagStartPrefix + "1",
					midtermTagEndPrefix + "10",
				},
			},
			{
				Id:      "timeline-archive-2-chunk-001",
				Content: "chunk 3",
				Tags: []string{
					midtermMemoryKindTag,
					midtermTagArchiveIDPrefix + "timeline-archive-2",
					midtermTagStartPrefix + "11",
					midtermTagEndPrefix + "20",
				},
			},
		},
		TotalContent:  "chunk 1\nchunk 2\nchunk 3",
		ContentBytes:  len("chunk 1\nchunk 2\nchunk 3"),
		SearchSummary: "ok",
	})

	require.Len(t, result.ArchiveRefs, 2)
	require.Equal(t, "timeline-archive-1", result.ArchiveRefs[0].ArchiveID)
	require.Equal(t, "timeline-archive-2", result.ArchiveRefs[1].ArchiveID)
	require.Len(t, result.SelectedMemory, 3)
	require.Equal(t, "chunk 1\nchunk 2\nchunk 3", result.TotalContent)
}
