package aicommon

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
)

// Selection references whole original items. Memory candidates are delivered
// with the committed summary; compression does not write a memory database.
type timelineCompressionOutput struct {
	Summary       string
	RetainedRange string
	RetainedIDs   []int64
	// Original promoted item IDs explicitly blacklisted by the model. Missing,
	// malformed or unknown IDs keep content by default.
	RetiredPromotedEntries []int64
	MemoryEntities         []any
}

func parseTimelineCompressionMemories(action *Action) []any {
	// Optional extensions must not reject a usable summary. Normalize detached
	// JSON data and admit valid candidates independently; never invent scores.
	rawMemory, _ := action.LookupCanonicalParam("memory_entities")
	raw, err := json.Marshal(rawMemory)
	var entities []any
	if err != nil || json.Unmarshal(raw, &entities) != nil {
		return nil
	}
	var candidates []any
	for _, value := range entities {
		if validCompressionMemoryEntity(value) {
			delete(value.(map[string]any), "title")
			candidates = append(candidates, value)
		}
	}
	return candidates
}

func validCompressionMemoryEntity(value any) bool {
	entity, ok := value.(map[string]any)
	if !ok {
		return false
	}
	content, ok := entity["content"].(string)
	if !ok || strings.TrimSpace(content) == "" || strings.Contains(content, aiprojection.Nonce()) {
		return false
	}
	for _, key := range []string{"tags", "potential_questions"} {
		values, ok := entity[key].([]any)
		if !ok || (key == "tags" && len(values) > 5) {
			return false
		}
		for _, value := range values {
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" || strings.Contains(text, aiprojection.Nonce()) {
				return false
			}
		}
	}
	scores, ok := entity["scores"].(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"temporality", "actionability", "perference", "origin", "emotion", "relevance", "connectivity"} {
		score, ok := scores[key].(float64)
		if !ok || math.IsNaN(score) || math.IsInf(score, 0) || score < 0 || score > 1 {
			return false
		}
	}
	return true
}

// Select from the snapshot instead of expanding numeric ranges. Sparse IDs
// and arbitrarily large ranges cannot select protected entries or allocate
// memory proportional to the range's width.
func parseCompressionRetainedRange(text string, items []timelineCompressionSnapshotItem) ([]int64, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	eligible := make(map[int64]bool, len(items))
	for _, item := range items {
		if item.PromptText != "" {
			eligible[item.ID] = true
		}
	}
	selected := make(map[int64]bool)
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		first, last, ranged := strings.Cut(part, "-")
		if !ranged {
			last = first
		}
		start, startErr := strconv.ParseInt(first, 10, 64)
		end, endErr := strconv.ParseInt(last, 10, 64)
		if startErr != nil || endErr != nil || start <= 0 || end < start ||
			strings.HasPrefix(first, "+") || strings.HasPrefix(last, "+") || !eligible[start] || !eligible[end] {
			return nil, fmt.Errorf("retain_timeline 范围 %q 无效；使用动态区列出的普通历史 ID，例如 12,18-23，起止 ID 都必须存在且递增", part)
		}
		for id := range eligible {
			if id >= start && id <= end {
				selected[id] = true
			}
		}
	}
	ids := make([]int64, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (o *timelineCompressionOutput) budgetText(snapshot *timelineCompressionSnapshot) string {
	var text strings.Builder
	text.WriteString(o.Summary)
	selected := make(map[int64]bool, len(o.RetainedIDs))
	for _, id := range o.RetainedIDs {
		selected[id] = true
	}
	for _, item := range snapshot.Items {
		if selected[item.ID] {
			text.WriteByte('\n')
			text.WriteString(item.PromptText)
		}
	}
	raw, _ := json.Marshal(o.MemoryEntities)
	text.Write(raw)
	return text.String()
}

func cloneCompressionMemoryEntities(entities []any) []any {
	// Only validated JSON data reaches this boundary. Each observer owns its
	// nested objects as well as the slice, independently of other observers.
	raw, _ := json.Marshal(entities)
	var cloned []any
	_ = json.Unmarshal(raw, &cloned)
	if cloned == nil {
		return []any{}
	}
	// Older responses and snapshots may contain a title; it is no longer part
	// of the memory contract and must not propagate to callbacks or history.
	for _, value := range cloned {
		if entity, ok := value.(map[string]any); ok {
			delete(entity, "title")
		}
	}
	return cloned
}
