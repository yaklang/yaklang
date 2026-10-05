package aimem

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// MemoryEntityFromCompression converts one Timeline memory candidate without
// model calls or score estimation. The existing wire key "perference" is retained.
func MemoryEntityFromCompression(value any) (*aicommon.MemoryEntity, error) {
	var candidate struct {
		Content            string              `json:"content"`
		Tags               []string            `json:"tags"`
		PotentialQuestions []string            `json:"potential_questions"`
		Scores             map[string]*float64 `json:"scores"`
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode compression memory: %w", err)
	}
	if err := json.Unmarshal(raw, &candidate); err != nil {
		return nil, fmt.Errorf("decode compression memory: %w", err)
	}
	if strings.TrimSpace(candidate.Content) == "" {
		return nil, fmt.Errorf("compression memory requires content")
	}
	if len(candidate.Tags) > 5 {
		return nil, fmt.Errorf("compression memory allows at most 5 tags")
	}
	for _, values := range [][]string{candidate.Tags, candidate.PotentialQuestions} {
		for _, text := range values {
			if strings.TrimSpace(text) == "" {
				return nil, fmt.Errorf("compression memory contains an empty tag or question")
			}
		}
	}
	// The score index uses C, O, R, E, P, A, T, rather than the prompt's order.
	keys := []string{"connectivity", "origin", "relevance", "emotion", "perference", "actionability", "temporality"}
	vector := make([]float32, len(keys))
	for i, key := range keys {
		score := candidate.Scores[key]
		if score == nil || math.IsNaN(*score) || math.IsInf(*score, 0) || *score < 0 || *score > 1 {
			return nil, fmt.Errorf("compression memory score %q must be a number in [0, 1]", key)
		}
		vector[i] = float32(*score)
	}
	return &aicommon.MemoryEntity{
		Id: uuid.NewString(), CreatedAt: time.Now(), Content: candidate.Content,
		Tags: candidate.Tags, PotentialQuestions: candidate.PotentialQuestions,
		C_Score: *candidate.Scores["connectivity"], O_Score: *candidate.Scores["origin"],
		R_Score: *candidate.Scores["relevance"], E_Score: *candidate.Scores["emotion"],
		P_Score: *candidate.Scores["perference"], A_Score: *candidate.Scores["actionability"],
		T_Score: *candidate.Scores["temporality"], CorePactVector: vector,
	}, nil
}
