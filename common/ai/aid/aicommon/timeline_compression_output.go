package aicommon

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
)

// ParseTimelineCompressionMemoryEntities normalizes optional memory candidates
// from a compression action. Parsing and validation belong to the action layer;
// Timeline only stores the accepted candidates.
func ParseTimelineCompressionMemoryEntities(action *Action) []any {
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
