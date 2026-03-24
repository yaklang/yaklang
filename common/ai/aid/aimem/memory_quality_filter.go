package aimem

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

type memoryQualityFilterConfig struct {
	MinTemporality   float64
	MinRelevance     float64
	MinActionability float64
	MinPreference    float64
	MinConnectivity  float64
	MinContentRunes  int
}

var (
	transientVisitPattern = regexp.MustCompile(`(?i)(访问|打开|浏览|进入|点击|visited|opened|browsed).*(https?://|www\.)|(https?://|www\.).*(访问|打开|浏览|进入|点击|visited|opened|browsed)`)
	timestampPattern      = regexp.MustCompile(`(?i)(\b\d{1,2}:\d{2}(?::\d{2})?\b|\b20\d{2}[-/]\d{1,2}[-/]\d{1,2}\b|\b\d{1,2}月\d{1,2}日\b|\b\d{1,2}点\d{1,2}分\b)`)
	processNoisePattern   = regexp.MustCompile(`(?i)(react迭代|iteration|tool call|tool result|tool execution|trace|日志|log output|call\[|observation|timeline diff|执行步骤|过程记录)`)
	ambiguousPronouns     = []string{"该用户", "当前这个", "这次", "这里", "刚才", "上述", "前者", "后者", "this", "that", "it", "these", "those"}
)

func defaultMemoryQualityFilterConfig() memoryQualityFilterConfig {
	return memoryQualityFilterConfig{
		MinTemporality:   0.45,
		MinRelevance:     0.35,
		MinActionability: 0.30,
		MinPreference:    0.30,
		MinConnectivity:  0.30,
		MinContentRunes:  12,
	}
}

func (t *AIMemoryTriage) filterGeneratedMemoryEntities(entities []*aicommon.MemoryEntity) []*aicommon.MemoryEntity {
	if len(entities) == 0 {
		return nil
	}

	config := defaultMemoryQualityFilterConfig()
	kept := make([]*aicommon.MemoryEntity, 0, len(entities))
	for _, entity := range entities {
		entity = normalizeMemoryEntity(entity)
		if entity == nil {
			continue
		}

		keep, reason := shouldKeepMemoryEntity(entity, config)
		if !keep {
			log.Infof("dropping low-quality memory entity %s: %s", entity.Id, reason)
			continue
		}
		kept = append(kept, entity)
	}
	return kept
}

func normalizeMemoryEntity(entity *aicommon.MemoryEntity) *aicommon.MemoryEntity {
	if entity == nil {
		return nil
	}
	entity.Content = strings.TrimSpace(entity.Content)
	entity.Tags = deduplicateTrimmedStrings(entity.Tags)
	entity.PotentialQuestions = deduplicateTrimmedStrings(entity.PotentialQuestions)
	if entity.Content == "" {
		return nil
	}
	return entity
}

func shouldKeepMemoryEntity(entity *aicommon.MemoryEntity, config memoryQualityFilterConfig) (bool, string) {
	content := strings.TrimSpace(entity.Content)
	if content == "" {
		return false, "empty content"
	}
	if len([]rune(content)) < config.MinContentRunes {
		return false, "content too short"
	}
	if entity.T_Score < config.MinTemporality {
		return false, "temporality too low"
	}
	if entity.R_Score < config.MinRelevance && entity.A_Score < config.MinActionability && entity.P_Score < config.MinPreference && entity.C_Score < config.MinConnectivity {
		return false, "insufficient durable value"
	}
	if isSingleVisitEvent(content) {
		return false, "single visit event"
	}
	if isProcessNoise(content) {
		return false, "process noise"
	}
	if hasAmbiguousPronoun(content) {
		return false, "ambiguous pronoun"
	}
	return true, ""
}

func isSingleVisitEvent(content string) bool {
	return transientVisitPattern.MatchString(content) && timestampPattern.MatchString(content)
}

func isProcessNoise(content string) bool {
	return processNoisePattern.MatchString(content)
}

func hasAmbiguousPronoun(content string) bool {
	lower := strings.ToLower(content)
	for _, token := range ambiguousPronouns {
		if strings.Contains(lower, strings.ToLower(token)) {
			return true
		}
	}
	return false
}

func deduplicateTrimmedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}
