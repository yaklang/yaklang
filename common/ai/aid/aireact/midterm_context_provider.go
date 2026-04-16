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

	query := strings.TrimSpace(buildMidtermRecallQuery(react))
	if query == "" {
		return "", nil
	}

	result, err := react.config.TimelineArchiveStore.SearchArchivedBatches(
		react.config.GetContext(),
		&aicommon.TimelineArchiveSearchQuery{
			Query:      query,
			BytesLimit: midtermContextBytesLimit,
		},
	)
	if err != nil {
		return "", err
	}
	if result == nil || strings.TrimSpace(result.TotalContent) == "" {
		return "", nil
	}

	var buf strings.Builder
	nowStr := time.Now().Format(utils.DefaultTimeFormat3)
	buf.WriteString(fmt.Sprintf("--[%s] midterm-memory:\n", nowStr))
	buf.WriteString(fmt.Sprintf("     search-query: %s\n", utils.ShrinkString(query, 240)))
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
	if react == nil {
		return ""
	}

	parts := make([]string, 0, 12)
	if task := react.GetCurrentTask(); task != nil {
		parts = append(parts,
			task.GetIndex(),
			task.GetName(),
			task.GetOriginUserInput(),
			task.GetUserInput(),
			task.GetSummary(),
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

	return strings.Join(deduplicateMidtermQueryParts(parts), " ")
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
