package aimem

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (t *AIMemoryTriage) SearchMemory(origin any, bytesLimit int) (*aicommon.SearchMemoryResult, error) {
	return t.searchMemoryWithAIOption(origin, bytesLimit, false)
}

func (t *AIMemoryTriage) SearchMemoryWithoutAI(origin any, bytesLimit int) (*aicommon.SearchMemoryResult, error) {
	return t.searchMemoryWithAIOption(origin, bytesLimit, true)
}

// SearchMemory 根据输入内容搜索相关记忆，限制总内容字节数
func (t *AIMemoryTriage) searchMemoryWithAIOption(origin any, bytesLimit int, disableAI bool) (*aicommon.SearchMemoryResult, error) {
	// 转换输入为字符串
	queryText := utils.InterfaceToString(origin)
	if strings.TrimSpace(queryText) == "" {
		return &aicommon.SearchMemoryResult{
			Memories:      []*aicommon.MemoryEntity{},
			TotalContent:  "",
			ContentBytes:  0,
			SearchSummary: "empty query provided",
		}, nil
	}

	log.Infof("searching memories for query: %s (bytes limit: %d)",
		utils.ShrinkString(queryText, 100), bytesLimit)

	var allMemories []*aicommon.MemoryEntity
	var searchSteps []string

	// 1. 使用 SelectTags 获取相关标签
	ctx := context.Background()
	var relevantTags []string
	var err error
	if !disableAI {
		// AI 作用主要是在关键词搜索上可以更智能地选择标签
		relevantTags, err = t.SelectTags(ctx, queryText)
		if err != nil {
			log.Warnf("failed to select tags: %v", err)
			relevantTags = []string{} // 继续执行，但没有标签
		}
	}

	searchSteps = append(searchSteps, fmt.Sprintf("selected %d relevant tags: %v", len(relevantTags), relevantTags))

	// 2. 基于标签搜索记忆
	if len(relevantTags) > 0 {
		tagMemories, err := t.SearchByTags(relevantTags, false, 20) // 不要求全匹配，最多20个
		if err != nil {
			log.Warnf("failed to search by tags: %v", err)
		} else {
			allMemories = append(allMemories, tagMemories...)
			searchSteps = append(searchSteps, fmt.Sprintf("found %d memories by tags", len(tagMemories)))
		}
	}

	// 3. 基于语义搜索扩展
	semanticResults, err := t.SearchBySemantics(queryText, 15) // 最多15个语义搜索结果
	if err != nil {
		log.Warnf("failed to search by semantics: %v", err)
	} else {
		for _, result := range semanticResults {
			allMemories = append(allMemories, result.Entity)
		}
		searchSteps = append(searchSteps, fmt.Sprintf("found %d memories by semantics", len(semanticResults)))
	}

	// 4. 去重合并
	uniqueMemories := t.deduplicateMemories(allMemories)
	searchSteps = append(searchSteps, fmt.Sprintf("deduplicated to %d unique memories", len(uniqueMemories)))

	// 5. 基于 C.O.R.E. P.A.C.T. 原则进行重排序和过滤
	rankedMemories := t.rankMemoriesByRelevance(uniqueMemories, queryText)
	searchSteps = append(searchSteps, "ranked memories by C.O.R.E. P.A.C.T. relevance")

	// 6. 根据字节限制选择记忆
	selectedMemories, totalContent, contentBytes := t.selectMemoriesByBytesLimit(rankedMemories, bytesLimit)
	searchSteps = append(searchSteps, fmt.Sprintf("selected %d memories within %d bytes limit", len(selectedMemories), bytesLimit))

	searchSummary := strings.Join(searchSteps, " -> ")

	log.Infof("memory search completed: %d memories, %d bytes content", len(selectedMemories), contentBytes)

	return &aicommon.SearchMemoryResult{
		Memories:      selectedMemories,
		TotalContent:  totalContent,
		ContentBytes:  contentBytes,
		SearchSummary: searchSummary,
	}, nil
}

// deduplicateMemories 去重记忆列表
func (t *AIMemoryTriage) deduplicateMemories(memories []*aicommon.MemoryEntity) []*aicommon.MemoryEntity {
	seen := make(map[string]bool)
	var unique []*aicommon.MemoryEntity

	for _, memory := range memories {
		if memory != nil && !seen[memory.Id] {
			seen[memory.Id] = true
			unique = append(unique, memory)
		}
	}

	return unique
}

// rankMemoriesByRelevance 基于 C.O.R.E. P.A.C.T. 原则对记忆进行重排序
// 现已集成改进的关键词系统
func (t *AIMemoryTriage) rankMemoriesByRelevance(memories []*aicommon.MemoryEntity, query string) []*aicommon.MemoryEntity {
	// 关键词匹配器在创建时已初始化

	// 为每个记忆计算综合相关性分数
	type ScoredMemory struct {
		Memory         *aicommon.MemoryEntity
		RelevanceScore float64
	}

	var scoredMemories []ScoredMemory

	for _, memory := range memories {
		// 基于 C.O.R.E. P.A.C.T. 计算综合分数
		relevanceScore := t.calculateRelevanceScore(memory, query)
		scoredMemories = append(scoredMemories, ScoredMemory{
			Memory:         memory,
			RelevanceScore: relevanceScore,
		})
	}

	// 按相关性分数降序排序
	sort.Slice(scoredMemories, func(i, j int) bool {
		return scoredMemories[i].RelevanceScore > scoredMemories[j].RelevanceScore
	})

	// 过滤低分记忆（相关性分数低于0.3的记忆）
	var rankedMemories []*aicommon.MemoryEntity
	for _, scored := range scoredMemories {
		if scored.RelevanceScore >= 0.3 {
			rankedMemories = append(rankedMemories, scored.Memory)
		}
	}

	log.Infof("ranked %d memories from %d, filtered threshold: 0.3", len(rankedMemories), len(scoredMemories))
	return rankedMemories
}

// calculateRelevanceScore 基于 C.O.R.E. P.A.C.T. 原则计算记忆的相关性分数
// 现已集成改进的关键词匹配系统
func (t *AIMemoryTriage) calculateRelevanceScore(memory *aicommon.MemoryEntity, query string) float64 {
	// 关键词匹配器在创建时已初始化，这里直接使用

	// 权重设计基于搜索场景的重要性
	weights := map[string]float64{
		"R": 0.25, // Relevance - 最重要，直接影响搜索相关性
		"C": 0.20, // Connectivity - 重要，关联度高的记忆更有价值
		"T": 0.15, // Temporality - 时效性，新鲜的记忆更重要
		"A": 0.15, // Actionability - 可操作性，能指导行为的记忆更有价值
		"P": 0.10, // Preference - 个人偏好，个性化相关
		"O": 0.10, // Origin - 来源可信度
		"E": 0.05, // Emotion - 情感，在搜索中权重较低
	}

	// 计算加权分数
	relevanceScore := weights["R"]*memory.R_Score +
		weights["C"]*memory.C_Score +
		weights["T"]*memory.T_Score +
		weights["A"]*memory.A_Score +
		weights["P"]*memory.P_Score +
		weights["O"]*memory.O_Score +
		weights["E"]*memory.E_Score

	// 使用改进的关键词匹配系统计算内容相关性加成
	contentBonus := t.calculateKeywordBonus(memory, query)

	finalScore := relevanceScore + contentBonus

	// 确保分数在0-1范围内
	if finalScore > 1.0 {
		finalScore = 1.0
	}
	if finalScore < 0.0 {
		finalScore = 0.0
	}

	return finalScore
}

// calculateKeywordBonus 使用关键词系统计算匹配加成
func (t *AIMemoryTriage) calculateKeywordBonus(memory *aicommon.MemoryEntity, query string) float64 {
	if t.keywordMatcher == nil {
		// 防御性编程：即使没有初始化，也返回0
		return 0.0
	}

	contentBonus := 0.0

	// 1. 内容关键词匹配分数 (权重: 0.1)
	contentMatchScore := t.keywordMatcher.MatchScore(query, memory.Content)
	contentBonus += contentMatchScore * 0.1

	// 2. 标签关键词匹配 (权重: 0.08)
	tagContent := strings.Join(memory.Tags, " ")
	tagMatchScore := t.keywordMatcher.MatchScore(query, tagContent)
	contentBonus += tagMatchScore * 0.08

	// 3. 问题关键词匹配 (权重: 0.05)
	questionContent := strings.Join(memory.PotentialQuestions, " ")
	questionMatchScore := t.keywordMatcher.MatchScore(query, questionContent)
	contentBonus += questionMatchScore * 0.05

	// 4. 直接关键词包含检查 (权重: 0.05)
	if t.keywordMatcher.ContainsKeyword(query, memory.Content) {
		contentBonus += 0.05
	}

	// 5. 所有关键词都包含的奖励 (权重: 0.03)
	if t.keywordMatcher.MatchAllKeywords(query, memory.Content) {
		contentBonus += 0.03
	}

	// 限制加成不超过0.3
	if contentBonus > 0.3 {
		contentBonus = 0.3
	}

	log.Debugf("keyword bonus calculation for query '%s': content_match=%.3f, tag_match=%.3f, "+
		"question_match=%.3f, has_keyword=%v, all_keywords=%v, total_bonus=%.3f",
		utils.ShrinkString(query, 50),
		contentMatchScore*0.1, tagMatchScore*0.08, questionMatchScore*0.05,
		t.keywordMatcher.ContainsKeyword(query, memory.Content),
		t.keywordMatcher.MatchAllKeywords(query, memory.Content),
		contentBonus)

	return contentBonus
}

// selectMemoriesByBytesLimit 根据字节限制选择记忆
func (t *AIMemoryTriage) selectMemoriesByBytesLimit(memories []*aicommon.MemoryEntity, bytesLimit int) ([]*aicommon.MemoryEntity, string, int) {
	if bytesLimit <= 0 {
		return []*aicommon.MemoryEntity{}, "", 0
	}

	var selectedMemories []*aicommon.MemoryEntity
	memoryTextMap := make(map[string]string)
	totalBytes := 0

	for _, memory := range memories {
		// 构建记忆的文本表示
		memoryText := fmt.Sprintf("[%s] 【记忆】%s\n标签：%s\n内容：%s\n",
			memory.CreatedAt.Format("2006-01-02 15:04:05"),
			memory.Id[:8], // 只显示ID前8位
			strings.Join(memory.Tags, ", "),
			memory.Content)

		memoryBytes := len([]byte(memoryText))

		// 检查是否超过限制
		if totalBytes+memoryBytes > bytesLimit {
			// 如果是第一个记忆就超过限制，尝试截断
			if len(selectedMemories) == 0 && memoryBytes > bytesLimit {
				// 截断内容以适应限制
				availableBytes := bytesLimit - len([]byte(fmt.Sprintf("[%s] 【记忆】%s\n标签：%s\n内容：",
					memory.CreatedAt.Format("2006-01-02 15:04:05"),
					memory.Id[:8], strings.Join(memory.Tags, ", "))))
				if availableBytes > 20 { // 至少保留20字节的内容
					truncatedContent := string([]byte(memory.Content)[:availableBytes-10]) + "..."
					memoryText = fmt.Sprintf("[%s] 【记忆】%s\n标签：%s\n内容：%s\n",
						memory.CreatedAt.Format("2006-01-02 15:04:05"),
						memory.Id[:8], strings.Join(memory.Tags, ", "), truncatedContent)
					selectedMemories = append(selectedMemories, memory)
					memoryTextMap[memory.Id] = memoryText
					totalBytes = len([]byte(memoryText))
				}
			}
			break
		}

		selectedMemories = append(selectedMemories, memory)
		memoryTextMap[memory.Id] = memoryText
		totalBytes += memoryBytes
	}

	// 对选中的记忆按时间排序 (Timeline: Oldest -> Newest)
	sort.Slice(selectedMemories, func(i, j int) bool {
		return selectedMemories[i].CreatedAt.Before(selectedMemories[j].CreatedAt)
	})

	// 重新构建内容字符串，确保顺序正确
	var sortedContentParts []string
	for _, memory := range selectedMemories {
		if text, ok := memoryTextMap[memory.Id]; ok {
			sortedContentParts = append(sortedContentParts, text)
		}
	}

	totalContent := strings.Join(sortedContentParts, "\n")
	return selectedMemories, totalContent, totalBytes
}
