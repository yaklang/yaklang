package aimem

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	midtermSessionPrefix      = "timeline-midterm:"
	midtermMemoryKindTag      = "timeline_midterm"
	midtermTagArchiveIDPrefix = "archive_id:"
	midtermTagReasonPrefix    = "reason:"
	midtermTagStartPrefix     = "source_start:"
	midtermTagEndPrefix       = "source_end:"
	midtermTagCountPrefix     = "item_count:"
)

func PersistentSessionToMidtermMemorySessionID(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	return midtermSessionPrefix + sessionID
}

func (r *AIMemoryTriage) ArchiveCompressedBatch(ctx context.Context, batch *aicommon.TimelineArchiveBatch) (*aicommon.TimelineArchiveRef, error) {
	_ = ctx
	if batch == nil {
		return nil, utils.Error("timeline archive batch is nil")
	}
	entity := buildTimelineArchiveMemoryEntity(batch)
	if err := r.SaveMemoryEntities(entity); err != nil {
		return nil, err
	}
	return buildTimelineArchiveRef(batch), nil
}

func (r *AIMemoryTriage) SearchArchivedBatches(ctx context.Context, query *aicommon.TimelineArchiveSearchQuery) (*aicommon.TimelineArchiveSearchResult, error) {
	_ = ctx
	if query == nil {
		query = &aicommon.TimelineArchiveSearchQuery{}
	}
	result, err := r.SearchMemoryWithoutAI(query.Query, query.BytesLimit)
	if err != nil {
		return nil, err
	}
	archiveRefs := make([]*aicommon.TimelineArchiveRef, 0, len(result.Memories))
	for _, memory := range result.Memories {
		if ref := parseTimelineArchiveRef(memory); ref != nil {
			archiveRefs = append(archiveRefs, ref)
		}
	}
	return &aicommon.TimelineArchiveSearchResult{
		ArchiveRefs:    archiveRefs,
		TotalContent:   result.TotalContent,
		ContentBytes:   result.ContentBytes,
		SearchSummary:  result.SearchSummary,
		SelectedMemory: result.Memories,
	}, nil
}

func buildTimelineArchiveMemoryEntity(batch *aicommon.TimelineArchiveBatch) *aicommon.MemoryEntity {
	summary := strings.TrimSpace(batch.Summary)
	if summary == "" {
		summary = "timeline archive without summary"
	}

	var content strings.Builder
	content.WriteString("[Timeline Midterm Archive]\n")
	content.WriteString(fmt.Sprintf("archive_id: %s\n", batch.ArchiveID))
	content.WriteString(fmt.Sprintf("reason: %s\n", batch.Reason))
	content.WriteString(fmt.Sprintf("source_range: %d-%d\n", batch.SourceStartID, batch.SourceEndID))
	content.WriteString(fmt.Sprintf("item_count: %d\n", batch.ItemCount))
	if !batch.SourceStartAt.IsZero() || !batch.SourceEndAt.IsZero() {
		content.WriteString(fmt.Sprintf("time_range: %s -> %s\n",
			batch.SourceStartAt.Format(time.RFC3339),
			batch.SourceEndAt.Format(time.RFC3339),
		))
	}
	content.WriteString("\nSummary:\n")
	content.WriteString(summary)

	if len(batch.RepresentativeSnips) > 0 {
		content.WriteString("\n\nRepresentative snippets:\n")
		for _, snippet := range batch.RepresentativeSnips {
			if snippet = strings.TrimSpace(snippet); snippet != "" {
				content.WriteString("- ")
				content.WriteString(snippet)
				content.WriteString("\n")
			}
		}
	}

	tags := []string{
		midtermMemoryKindTag,
		midtermTagArchiveIDPrefix + batch.ArchiveID,
		midtermTagReasonPrefix + string(batch.Reason),
		midtermTagStartPrefix + strconv.FormatInt(batch.SourceStartID, 10),
		midtermTagEndPrefix + strconv.FormatInt(batch.SourceEndID, 10),
		midtermTagCountPrefix + strconv.Itoa(batch.ItemCount),
	}
	tags = append(tags, batch.Tags...)
	tags = deduplicateTrimmedStrings(tags)

	questions := []string{summary}
	questions = append(questions, batch.RepresentativeSnips...)
	questions = deduplicateTrimmedStrings(questions)

	return &aicommon.MemoryEntity{
		Id:                 batch.ArchiveID,
		CreatedAt:          time.Now(),
		Content:            content.String(),
		Tags:               tags,
		PotentialQuestions: questions,
		C_Score:            0.85,
		O_Score:            0.95,
		R_Score:            0.90,
		E_Score:            0.50,
		P_Score:            0.20,
		A_Score:            0.65,
		T_Score:            0.85,
		CorePactVector:     []float32{0.85, 0.95, 0.90, 0.50, 0.20, 0.65, 0.85},
	}
}

func buildTimelineArchiveRef(batch *aicommon.TimelineArchiveBatch) *aicommon.TimelineArchiveRef {
	if batch == nil {
		return nil
	}
	return &aicommon.TimelineArchiveRef{
		ArchiveID:      batch.ArchiveID,
		Reason:         batch.Reason,
		SummaryPreview: utils.ShrinkString(strings.TrimSpace(batch.Summary), 160),
		ReducerKeyID:   batch.ReducerKeyID,
		SourceStartID:  batch.SourceStartID,
		SourceEndID:    batch.SourceEndID,
		ItemCount:      batch.ItemCount,
		CreatedAt:      time.Now(),
	}
}

func parseTimelineArchiveRef(memory *aicommon.MemoryEntity) *aicommon.TimelineArchiveRef {
	if memory == nil {
		return nil
	}
	ref := &aicommon.TimelineArchiveRef{
		ArchiveID:      memory.Id,
		SummaryPreview: utils.ShrinkString(strings.TrimSpace(memory.Content), 160),
		CreatedAt:      memory.CreatedAt,
	}
	for _, tag := range memory.Tags {
		switch {
		case tag == midtermMemoryKindTag:
		case strings.HasPrefix(tag, midtermTagArchiveIDPrefix):
			ref.ArchiveID = strings.TrimPrefix(tag, midtermTagArchiveIDPrefix)
		case strings.HasPrefix(tag, midtermTagReasonPrefix):
			ref.Reason = aicommon.TimelineArchiveReason(strings.TrimPrefix(tag, midtermTagReasonPrefix))
		case strings.HasPrefix(tag, midtermTagStartPrefix):
			ref.SourceStartID, _ = strconv.ParseInt(strings.TrimPrefix(tag, midtermTagStartPrefix), 10, 64)
		case strings.HasPrefix(tag, midtermTagEndPrefix):
			ref.SourceEndID, _ = strconv.ParseInt(strings.TrimPrefix(tag, midtermTagEndPrefix), 10, 64)
		case strings.HasPrefix(tag, midtermTagCountPrefix):
			ref.ItemCount, _ = strconv.Atoi(strings.TrimPrefix(tag, midtermTagCountPrefix))
		}
	}
	return ref
}
