package aicommon

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/utils"
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

func parseTimelineCompressionOutput(action *Action, snapshot *timelineCompressionSnapshot, summary string) *timelineCompressionOutput {
	rawRange, _ := action.LookupCanonicalParam("ratain_timeline_item_range")
	rawRetire, _ := action.LookupCanonicalParam("retire_promoted_entries")
	rawSkills, _ := action.LookupCanonicalParam("retire_skill_names")
	output := compressionSelection(snapshot, summary, rawRange, rawRetire, rawSkills)
	// Optional extensions must not reject a usable summary. Normalize detached
	// JSON data and admit valid candidates independently; never invent scores.
	rawMemory, _ := action.LookupCanonicalParam("memory_entities")
	raw, err := json.Marshal(rawMemory)
	var entities []any
	if err != nil || json.Unmarshal(raw, &entities) != nil {
		return output
	}
	for _, value := range entities {
		if validCompressionMemoryEntity(value) {
			delete(value.(map[string]any), "title")
			output.MemoryEntities = append(output.MemoryEntities, value)
		}
	}
	return output
}

func compressionSelection(snapshot *timelineCompressionSnapshot, summary string, rawRange any, rawRetire any, rawSkills any) *timelineCompressionOutput {
	output := &timelineCompressionOutput{Summary: strings.TrimSpace(summary), MemoryEntities: []any{}}
	if text, ok := rawRange.(string); ok {
		if ids, err := parseCompressionRetainedRange(text, snapshot.Items); err == nil {
			output.RetainedRange, output.RetainedIDs = strings.TrimSpace(text), ids
		}
	}
	output.RetiredPromotedEntries = sortedCompressionIDs(append(
		parsePromotedRetirementSelection(snapshot, rawRetire),
		parseSkillRetirementSelection(snapshot, rawSkills)...))
	return output
}

// parsePromotedRetirementSelection accepts only evidence item IDs listed in
// this snapshot. Skill names and tool-cache IDs are not accepted here: skills
// resolve through parseSkillRetirementSelection, and tool cache is not
// deletable by the model. Unknown IDs are ignored per item.
func parsePromotedRetirementSelection(snapshot *timelineCompressionSnapshot, raw any) []int64 {
	if snapshot == nil {
		return nil
	}
	offered := make(map[int64]bool, len(snapshot.PromotedItems))
	for _, item := range snapshot.PromotedItems {
		if item.Kind == TimelinePromotedKindEvidence {
			offered[item.ID] = true
		}
	}
	selected := make(map[int64]bool)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 || !offered[id] {
			return
		}
		selected[id] = true
	}
	switch value := raw.(type) {
	case string:
		for _, part := range strings.Split(value, ",") {
			add(part)
		}
	}
	if len(selected) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// parseSkillRetirementSelection resolves skill names to every original item of
// those skills. A name matches the skill_name exactly, or a part label copied
// with that name as its prefix. Unknown names and tool-cache names delete nothing.
func parseSkillRetirementSelection(snapshot *timelineCompressionSnapshot, raw any) []int64 {
	text, ok := raw.(string)
	if snapshot == nil || !ok || strings.TrimSpace(text) == "" {
		return nil
	}
	var requested []string
	for _, part := range strings.Split(text, ",") {
		name := strings.TrimSpace(part)
		if name == skillCatalogKey {
			name = "catalog"
		}
		if name == "" || name == "SKILL_NAME" {
			continue
		}
		requested = append(requested, name)
	}
	if len(requested) == 0 {
		return nil
	}
	selected := make(map[int64]bool)
	for _, item := range snapshot.PromotedItems {
		actual := compressionSkillRetirementName(item)
		if actual == "" {
			continue
		}
		for _, name := range requested {
			if name == actual || strings.HasPrefix(name, actual+":") || strings.HasPrefix(name, actual+" ") {
				selected[item.ID] = true
				break
			}
		}
	}
	if len(selected) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
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

var compressionRetainedRangePattern = regexp.MustCompile(`^[0-9]+(-[0-9]+)?$`)

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
		// The shared parser skips invalid input and eagerly expands ranges.
		// Check endpoints and bound expansion before calling it.
		bounds := utils.ParseStringToInts(strings.ReplaceAll(part, "-", ","))
		if !compressionRetainedRangePattern.MatchString(part) || len(bounds) == 0 ||
			bounds[0] <= 0 || !eligible[int64(bounds[0])] || !eligible[int64(bounds[len(bounds)-1])] ||
			(len(bounds) == 2 && bounds[1]-bounds[0] > 1_000_000) {
			return nil, fmt.Errorf("invalid retained timeline range %q", part)
		}
		ids := utils.ParseStringToInts(part)
		if len(ids) == 0 {
			return nil, fmt.Errorf("invalid retained timeline range %q", part)
		}
		for _, id := range ids {
			if eligible[int64(id)] {
				selected[int64(id)] = true
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
