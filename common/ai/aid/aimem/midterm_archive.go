package aimem

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
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
	midtermTagChunkPrefix     = "chunk:"
	midtermTagChunkTotal      = "chunk_total:"

	midtermArchiveChunkLimit        = 1000
	midtermArchiveChunkContentLimit = 760
)

var timelineArchiveSplitText = aiforge.SplitTextSafe

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
	entities, err := buildTimelineArchiveMemoryEntities(batch)
	if err != nil {
		return nil, err
	}
	if err := r.SaveMemoryEntities(entities...); err != nil {
		return nil, err
	}
	return buildTimelineArchiveRef(batch), nil
}

func (r *AIMemoryTriage) SearchArchivedBatches(ctx context.Context, query *aicommon.TimelineArchiveSearchQuery) (*aicommon.TimelineArchiveSearchResult, error) {
	_ = ctx
	if query == nil {
		query = &aicommon.TimelineArchiveSearchQuery{}
	}
	var (
		result *aicommon.SearchMemoryResult
		err    error
	)
	if query.DisableSemanticSearch {
		result, err = r.SearchMemoryWithoutAIAndSemantics(query.Query, query.BytesLimit)
	} else {
		result, err = r.SearchMemoryWithoutAI(query.Query, query.BytesLimit)
	}
	if err != nil {
		return nil, err
	}
	return buildTimelineArchiveSearchResult(result), nil
}

func buildTimelineArchiveSearchResult(result *aicommon.SearchMemoryResult) *aicommon.TimelineArchiveSearchResult {
	if result == nil {
		return &aicommon.TimelineArchiveSearchResult{}
	}

	archiveRefs := make([]*aicommon.TimelineArchiveRef, 0, len(result.Memories))
	seenArchiveIDs := make(map[string]struct{}, len(result.Memories))
	for _, memory := range result.Memories {
		ref := parseTimelineArchiveRef(memory)
		if ref == nil || strings.TrimSpace(ref.ArchiveID) == "" {
			continue
		}
		if _, ok := seenArchiveIDs[ref.ArchiveID]; ok {
			continue
		}
		seenArchiveIDs[ref.ArchiveID] = struct{}{}
		archiveRefs = append(archiveRefs, ref)
	}

	return &aicommon.TimelineArchiveSearchResult{
		ArchiveRefs:    archiveRefs,
		TotalContent:   result.TotalContent,
		ContentBytes:   len([]byte(result.TotalContent)),
		SearchSummary:  result.SearchSummary,
		SelectedMemory: result.Memories,
	}
}

func buildTimelineArchiveMemoryEntities(batch *aicommon.TimelineArchiveBatch) ([]*aicommon.MemoryEntity, error) {
	if batch == nil {
		return nil, utils.Error("timeline archive batch is nil")
	}

	summary := strings.TrimSpace(batch.Summary)
	if summary == "" {
		summary = "timeline archive without summary"
	}

	chunks, err := buildTimelineArchiveChunks(batch, summary)
	if err != nil {
		return nil, err
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

	entities := make([]*aicommon.MemoryEntity, 0, len(chunks))
	createdAt := time.Now()
	for index, chunk := range chunks {
		chunkTags := append([]string{}, tags...)
		chunkTags = append(chunkTags,
			midtermTagChunkPrefix+strconv.Itoa(index+1),
			midtermTagChunkTotal+strconv.Itoa(len(chunks)),
		)
		chunkTags = deduplicateTrimmedStrings(chunkTags)

		questions := []string{
			summary,
			utils.ShrinkString(chunk, 240),
			fmt.Sprintf("timeline archive %s range %d-%d", batch.Reason, batch.SourceStartID, batch.SourceEndID),
		}
		questions = append(questions, batch.RepresentativeSnips...)
		questions = deduplicateTrimmedStrings(questions)

		content := buildTimelineArchiveChunkContent(batch, summary, chunk, index+1, len(chunks))
		entities = append(entities, &aicommon.MemoryEntity{
			Id:                 buildTimelineArchiveChunkID(batch.ArchiveID, index+1),
			CreatedAt:          createdAt,
			Content:            content,
			Tags:               chunkTags,
			PotentialQuestions: questions,
			C_Score:            0.85,
			O_Score:            0.95,
			R_Score:            0.90,
			E_Score:            0.50,
			P_Score:            0.20,
			A_Score:            0.65,
			T_Score:            0.85,
			CorePactVector:     []float32{0.85, 0.95, 0.90, 0.50, 0.20, 0.65, 0.85},
		})
	}

	return entities, nil
}

func buildTimelineArchiveChunks(batch *aicommon.TimelineArchiveBatch, summary string) ([]string, error) {
	input := strings.TrimSpace(batch.MergedContent)
	if input == "" {
		input = strings.TrimSpace(summary)
	}
	if input == "" {
		input = "timeline archive without summary"
	}

	chunks, err := timelineArchiveSplitText(input, midtermArchiveChunkContentLimit)
	if err != nil {
		log.Warnf("split timeline archive content failed, fallback to pre-merge source chunks: %v", err)
		return fallbackTimelineArchiveSourceChunks(batch, summary), nil
	}
	if len(chunks) == 0 {
		return fallbackTimelineArchiveSourceChunks(batch, summary), nil
	}

	result := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		for _, part := range splitChunkByRuneLimit(chunk, midtermArchiveChunkContentLimit) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			result = append(result, part)
		}
	}
	if len(result) == 0 {
		result = []string{utils.ShrinkString(input, midtermArchiveChunkContentLimit)}
	}
	return result, nil
}

func fallbackTimelineArchiveSourceChunks(batch *aicommon.TimelineArchiveBatch, summary string) []string {
	if batch == nil {
		return nil
	}
	result := make([]string, 0, len(batch.SourceChunks))
	for _, chunk := range batch.SourceChunks {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		result = append(result, shrinkRunes(chunk, midtermArchiveChunkLimit))
	}
	if len(result) > 0 {
		return result
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = "timeline archive without summary"
	}
	return []string{shrinkRunes(summary, midtermArchiveChunkLimit)}
}

func shrinkRunes(input string, limit int) string {
	input = strings.TrimSpace(input)
	if input == "" || limit <= 0 {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= limit {
		return input
	}
	return strings.TrimSpace(string(runes[:limit]))
}

func buildTimelineArchiveChunkContent(batch *aicommon.TimelineArchiveBatch, summary string, chunk string, chunkIndex int, chunkTotal int) string {
	var content strings.Builder
	content.WriteString("[Timeline Midterm Archive Chunk]\n")
	content.WriteString(fmt.Sprintf("archive_id: %s\n", batch.ArchiveID))
	content.WriteString(fmt.Sprintf("reason: %s\n", batch.Reason))
	content.WriteString(fmt.Sprintf("source_range: %d-%d\n", batch.SourceStartID, batch.SourceEndID))
	content.WriteString(fmt.Sprintf("item_count: %d\n", batch.ItemCount))
	content.WriteString(fmt.Sprintf("chunk: %d/%d\n", chunkIndex, chunkTotal))
	if !batch.SourceStartAt.IsZero() || !batch.SourceEndAt.IsZero() {
		content.WriteString(fmt.Sprintf("time_range: %s -> %s\n",
			batch.SourceStartAt.Format(time.RFC3339),
			batch.SourceEndAt.Format(time.RFC3339),
		))
	}
	if chunkIndex == 1 && summary != "" {
		content.WriteString("\nSummary:\n")
		content.WriteString(utils.ShrinkString(summary, 180))
		content.WriteString("\n")
	}
	content.WriteString("\nKey timeline details:\n")
	content.WriteString(strings.TrimSpace(chunk))

	result := content.String()
	if len([]rune(result)) <= midtermArchiveChunkLimit {
		return result
	}

	available := midtermArchiveChunkLimit - len([]rune(result)) + len([]rune(chunk))
	if available < 80 {
		available = 80
	}

	content.Reset()
	content.WriteString("[Timeline Midterm Archive Chunk]\n")
	content.WriteString(fmt.Sprintf("archive_id: %s\n", batch.ArchiveID))
	content.WriteString(fmt.Sprintf("reason: %s\n", batch.Reason))
	content.WriteString(fmt.Sprintf("source_range: %d-%d\n", batch.SourceStartID, batch.SourceEndID))
	content.WriteString(fmt.Sprintf("chunk: %d/%d\n", chunkIndex, chunkTotal))
	content.WriteString("\nKey timeline details:\n")
	content.WriteString(utils.ShrinkString(strings.TrimSpace(chunk), available))
	result = content.String()
	if len([]rune(result)) <= midtermArchiveChunkLimit {
		return result
	}
	return string([]rune(result)[:midtermArchiveChunkLimit])
}

func buildTimelineArchiveChunkID(archiveID string, chunkIndex int) string {
	return fmt.Sprintf("%s-chunk-%03d", archiveID, chunkIndex)
}

func splitChunkByRuneLimit(input string, limit int) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	runes := []rune(input)
	if len(runes) <= limit {
		return []string{input}
	}

	result := make([]string, 0, len(runes)/limit+1)
	for len(runes) > 0 {
		end := limit
		if end > len(runes) {
			end = len(runes)
		}
		if end < len(runes) {
			for candidate := end; candidate >= end/2; candidate-- {
				if candidate >= len(runes) {
					continue
				}
				switch runes[candidate] {
				case '\n', '。', '！', '？', '.', '!', '?', ';', '；':
					end = candidate + 1
					goto appendChunk
				}
			}
		}
	appendChunk:
		part := strings.TrimSpace(string(runes[:end]))
		if part != "" {
			result = append(result, part)
		}
		runes = runes[end:]
	}
	return result
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
