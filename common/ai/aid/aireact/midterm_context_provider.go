package aireact

import (
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const midtermContextBytesLimit = 3 * 1024

type midtermPerceptionSnapshot struct {
	Summary  string
	Topics   []string
	Keywords []string
}

type midtermTimelineSearchQuery struct {
	Query                 string
	DisableSemanticSearch bool
}

func buildTimelineDumpWithMidtermMemory(react *ReAct, timeline *aicommon.Timeline) string {
	baseTimeline := ""
	if timeline != nil {
		baseTimeline = timeline.Dump()
	}
	queries := react.consumePendingMidtermTimelineQueries()
	if len(queries) == 0 {
		return baseTimeline
	}

	midtermPrefix, err := buildMidtermTimelinePrefix(react, queries)
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

func (r *ReAct) ScheduleMidtermTimelineRecall(summary string) {
	r.ScheduleMidtermTimelineRecallFromPerception(summary, nil, nil)
}

func (r *ReAct) ScheduleMidtermTimelineRecallFromPerception(summary string, topics []string, keywords []string) {
	if r == nil {
		return
	}

	summary = strings.TrimSpace(summary)
	topics = deduplicateMidtermQueryParts(topics)
	keywords = deduplicateMidtermQueryParts(keywords)
	r.midtermRecallMutex.Lock()
	defer r.midtermRecallMutex.Unlock()

	if summary == "" && len(topics) == 0 && len(keywords) == 0 {
		r.pendingMidtermTimelineRecall = false
		r.pendingMidtermPerception = nil
		return
	}

	r.pendingMidtermTimelineRecall = true
	r.pendingMidtermPerception = &midtermPerceptionSnapshot{
		Summary:  summary,
		Topics:   append([]string{}, topics...),
		Keywords: append([]string{}, keywords...),
	}
}

func (r *ReAct) consumePendingMidtermTimelineQueries() []midtermTimelineSearchQuery {
	if r == nil {
		return nil
	}

	r.midtermRecallMutex.Lock()
	defer r.midtermRecallMutex.Unlock()

	if !r.pendingMidtermTimelineRecall {
		return nil
	}

	snapshot := r.pendingMidtermPerception
	r.pendingMidtermTimelineRecall = false
	r.pendingMidtermPerception = nil
	if snapshot == nil {
		return nil
	}

	return buildMidtermRecallQueries(snapshot)
}

func buildMidtermRecallQueries(snapshot *midtermPerceptionSnapshot) []midtermTimelineSearchQuery {
	if snapshot == nil {
		return nil
	}

	seen := make(map[string]struct{})
	result := make([]midtermTimelineSearchQuery, 0, 2+len(snapshot.Keywords))
	appendQuery := func(query string, disableSemantic bool) {
		query = strings.TrimSpace(query)
		if query == "" {
			return
		}
		key := fmt.Sprintf("%t:%s", disableSemantic, query)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		result = append(result, midtermTimelineSearchQuery{
			Query:                 query,
			DisableSemanticSearch: disableSemantic,
		})
	}

	appendQuery(snapshot.Summary, false)
	appendQuery(strings.Join(deduplicateMidtermQueryParts(snapshot.Topics), " "), false)
	for _, keyword := range deduplicateMidtermQueryParts(snapshot.Keywords) {
		appendQuery(keyword, true)
	}
	return result
}

func buildMidtermTimelinePrefix(react *ReAct, queries []midtermTimelineSearchQuery) (string, error) {
	if react == nil || react.config == nil || react.config.TimelineArchiveStore == nil {
		return "", nil
	}

	queries = deduplicateMidtermSearchQueries(queries)
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
		buf.WriteString(fmt.Sprintf("     search-query: %s\n", utils.ShrinkString(queries[0].Query, 240)))
	} else {
		buf.WriteString("     search-queries:\n")
		for _, query := range queries {
			buf.WriteString(fmt.Sprintf("       - %s\n", utils.ShrinkString(query.Query, 240)))
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

func searchMidtermTimelineQueries(react *ReAct, queries []midtermTimelineSearchQuery) (finalResult *aicommon.TimelineArchiveSearchResult, finalErr error) {
	if react == nil || react.config == nil || react.config.TimelineArchiveStore == nil || len(queries) == 0 {
		return nil, nil
	}

	totalStartedAt := time.Now()
	archiveRefs := make([]*aicommon.TimelineArchiveRef, 0)
	selectedMemories := make([]*aicommon.MemoryEntity, 0)
	searchSummaries := make([]string, 0, len(queries))
	searchedQueryCount := 0

	seenArchiveIDs := make(map[string]struct{})
	seenMemoryIDs := make(map[string]struct{})

	defer func() {
		if finalErr != nil {
			log.Debugf("midterm timeline search finished with error: queries=%d total=%s err=%v", searchedQueryCount, time.Since(totalStartedAt), finalErr)
			return
		}
		log.Debugf("midterm timeline search finished: queries=%d total=%s", searchedQueryCount, time.Since(totalStartedAt))
	}()

	for _, query := range queries {
		query.Query = strings.TrimSpace(query.Query)
		if query.Query == "" {
			continue
		}
		searchedQueryCount++
		queryStartedAt := time.Now()
		result, err := react.config.TimelineArchiveStore.SearchArchivedBatches(
			react.config.GetContext(),
			&aicommon.TimelineArchiveSearchQuery{
				Query:                 query.Query,
				BytesLimit:            midtermContextBytesLimit,
				DisableSemanticSearch: query.DisableSemanticSearch,
			},
		)
		if err != nil {
			log.Debugf("midterm timeline search query failed: query=%q disable_semantic=%v duration=%s err=%v", utils.ShrinkString(query.Query, 240), query.DisableSemanticSearch, time.Since(queryStartedAt), err)
			return nil, err
		}
		log.Debugf("midterm timeline search query finished: query=%q disable_semantic=%v duration=%s", utils.ShrinkString(query.Query, 240), query.DisableSemanticSearch, time.Since(queryStartedAt))
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
	finalResult = &aicommon.TimelineArchiveSearchResult{
		ArchiveRefs:    archiveRefs,
		TotalContent:   totalContent,
		ContentBytes:   len([]byte(totalContent)),
		SearchSummary:  strings.Join(searchSummaries, " | "),
		SelectedMemory: selectedMemories,
	}
	return finalResult, nil
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

func deduplicateMidtermSearchQueries(queries []midtermTimelineSearchQuery) []midtermTimelineSearchQuery {
	seen := make(map[string]struct{})
	result := make([]midtermTimelineSearchQuery, 0, len(queries))
	for _, query := range queries {
		query.Query = strings.TrimSpace(query.Query)
		if query.Query == "" {
			continue
		}
		key := fmt.Sprintf("%t:%s", query.DisableSemanticSearch, query.Query)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, query)
	}
	return result
}
