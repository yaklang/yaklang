package aireact

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

const midtermContextBytesLimit = 3 * 1024

func NewMidtermSessionMemoryContextProvider(react *ReAct) aicommon.ContextProvider {
	return func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
		_ = emitter
		_ = key
		if react == nil || react.config == nil || react.config.TimelineArchiveStore == nil {
			return "", nil
		}

		query := strings.TrimSpace(buildMidtermRecallQuery(react))
		if query == "" {
			return "", nil
		}

		result, err := react.config.TimelineArchiveStore.SearchArchivedBatches(
			config.GetContext(),
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

		return fmt.Sprintf(
			"## Midterm Session Memory\nSearch query: %s\n%s",
			utils.ShrinkString(query, 240),
			strings.TrimSpace(result.TotalContent),
		), nil
	}
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
