package aireact

import (
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

const midtermContextBytesLimit = 3 * 1024

func buildTimelineDumpWithMidtermMemory(react *ReAct, timeline *aicommon.Timeline) string {
	baseTimeline := ""
	if timeline != nil {
		baseTimeline = timeline.Dump()
	}
	midtermPrefix, err := buildMidtermTimelinePrefix(react)
	if err != nil {
		return baseTimeline
	}
	if midtermPrefix == "" {
		return baseTimeline
	}
	if strings.TrimSpace(baseTimeline) == "" {
		return "timeline:\n" + midtermPrefix
	}
	body := strings.TrimPrefix(baseTimeline, "timeline:\n")
	return "timeline:\n" + midtermPrefix + body
}

func buildMidtermTimelinePrefix(react *ReAct) (string, error) {
	if react == nil || react.config == nil || react.config.TimelineArchiveStore == nil {
		return "", nil
	}

	queries := buildMidtermRecallQueryParts(react)
	if len(queries) == 0 {
		return "", nil
	}

	result, err := searchMidtermTimelineQueries(react, queries)
	if err != nil {
		return "", err
	}
	if result == nil || strings.TrimSpace(result.TotalContent) == "" {
		return "", nil
	}

	var buf strings.Builder
	nowStr := time.Now().Format(utils.DefaultTimeFormat3)
	buf.WriteString(fmt.Sprintf("--[%s] midterm-memory:\n", nowStr))
	if len(queries) == 1 {
		buf.WriteString(fmt.Sprintf("     search-query: %s\n", utils.ShrinkString(queries[0], 240)))
	} else {
		buf.WriteString("     search-queries:\n")
		for _, query := range queries {
			buf.WriteString(fmt.Sprintf("       - %s\n", utils.ShrinkString(query, 240)))
		}
	}
	for _, line := range utils.ParseStringToRawLines(strings.TrimSpace(result.TotalContent)) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		buf.WriteString("     ")
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	return buf.String(), nil
}

func buildMidtermRecallQuery(react *ReAct) string {
	return strings.Join(buildMidtermRecallQueryParts(react), " ")
}

func buildMidtermRecallQueryParts(react *ReAct) []string {
	if react == nil {
		return nil
	}

	parts := make([]string, 0, 12)
	if task := react.GetCurrentTask(); task != nil {
		parts = append(parts,
			// task.GetIndex(),
			task.GetName(),
			task.GetOriginUserInput(),
			// task.GetUserInput(),
			// task.GetSummary(),
		)
		parts = append(parts, task.GetUserInput())
		if info := task.GetTaskRetrievalInfo(); info != nil {
			parts = append(parts, info.Target)
			parts = append(parts, info.Questions...)
			parts = append(parts, info.Tags...)
		}
	}

	history := react.config.GetUserInputHistory()
	if n := len(history); n > 0 {
		parts = append(parts, history[n-1].UserInput)
	}

	return deduplicateMidtermQueryParts(parts)
}

func searchMidtermTimelineQueries(react *ReAct, queries []string) (*aicommon.TimelineArchiveSearchResult, error) {
	if react == nil || react.config == nil || react.config.TimelineArchiveStore == nil || len(queries) == 0 {
		return nil, nil
	}

	archiveRefs := make([]*aicommon.TimelineArchiveRef, 0)
	selectedMemories := make([]*aicommon.MemoryEntity, 0)
	searchSummaries := make([]string, 0, len(queries))

	seenArchiveIDs := make(map[string]struct{})
	seenMemoryIDs := make(map[string]struct{})

	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		result, err := react.config.TimelineArchiveStore.SearchArchivedBatches(
			react.config.GetContext(),
			&aicommon.TimelineArchiveSearchQuery{
				Query:      query,
				BytesLimit: midtermContextBytesLimit,
			},
		)
		if err != nil {
			return nil, err
		}
		if result == nil {
			continue
		}
		if summary := strings.TrimSpace(result.SearchSummary); summary != "" {
			searchSummaries = append(searchSummaries, summary)
		}
		for _, ref := range result.ArchiveRefs {
			if ref == nil || strings.TrimSpace(ref.ArchiveID) == "" {
				continue
			}
			if _, ok := seenArchiveIDs[ref.ArchiveID]; ok {
				continue
			}
			seenArchiveIDs[ref.ArchiveID] = struct{}{}
			archiveRefs = append(archiveRefs, ref)
		}
		if len(result.SelectedMemory) > 0 {
			for _, memory := range result.SelectedMemory {
				if memory == nil || strings.TrimSpace(memory.Id) == "" {
					continue
				}
				if _, ok := seenMemoryIDs[memory.Id]; ok {
					continue
				}
				seenMemoryIDs[memory.Id] = struct{}{}
				selectedMemories = append(selectedMemories, memory)
			}
		} else if content := strings.TrimSpace(result.TotalContent); content != "" {
			pseudoID := "__midterm_content__:" + utils.CalcSha256(content)
			if _, ok := seenMemoryIDs[pseudoID]; !ok {
				seenMemoryIDs[pseudoID] = struct{}{}
				selectedMemories = append(selectedMemories, &aicommon.MemoryEntity{
					Id:      pseudoID,
					Content: content,
				})
			}
		}
	}

	totalContent := mergeMidtermMemoryContent(selectedMemories, midtermContextBytesLimit)
	return &aicommon.TimelineArchiveSearchResult{
		ArchiveRefs:    archiveRefs,
		TotalContent:   totalContent,
		ContentBytes:   len([]byte(totalContent)),
		SearchSummary:  strings.Join(searchSummaries, " | "),
		SelectedMemory: selectedMemories,
	}, nil
}

func mergeMidtermMemoryContent(memories []*aicommon.MemoryEntity, limit int) string {
	if len(memories) == 0 || limit <= 0 {
		return ""
	}

	var buf strings.Builder
	for _, memory := range memories {
		if memory == nil {
			continue
		}
		content := strings.TrimSpace(memory.Content)
		if content == "" {
			continue
		}
		if buf.Len() > 0 {
			if buf.Len()+1 > limit {
				break
			}
			buf.WriteByte('\n')
		}
		remaining := limit - buf.Len()
		if remaining <= 0 {
			break
		}
		if len(content) > remaining {
			buf.WriteString(utils.ShrinkString(content, remaining))
			break
		}
		buf.WriteString(content)
	}
	return strings.TrimSpace(buf.String())
}

func deduplicateMidtermQueryParts(parts []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}
