package aimem

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// HandleMemory 处理输入内容，自动构造记忆并去重保存
func (t *AIMemoryTriage) HandleMemory(i any) error {
	// 转换输入为字符串
	inputText := utils.InterfaceToString(i)
	if strings.TrimSpace(inputText) == "" {
		log.Infof("input is empty, skipping memory handling")
		return nil
	}

	log.Infof("handling memory for input: %s", utils.ShrinkString(inputText, 100))

	// 1. 使用 AddRawText 构造记忆实体
	entities, err := t.AddRawText(inputText)
	if err != nil {
		return utils.Errorf("failed to build memory entities: %v", err)
	}

	if len(entities) == 0 {
		log.Infof("no memory entities generated from input")
		return nil
	}

	log.Infof("generated %d memory entities from input", len(entities))

	// 2. 使用去重功能判断是否有重复
	worthSaving := t.ShouldSaveMemoryEntities(entities)

	// 3. 处理重复和非重复的记忆
	duplicateCount := len(entities) - len(worthSaving)
	if duplicateCount > 0 {
		log.Infof("detected %d duplicate memory entities, skipping them", duplicateCount)

		// 记录被跳过的重复记忆
		savedIds := make(map[string]bool)
		for _, saved := range worthSaving {
			savedIds[saved.Id] = true
		}

		for _, entity := range entities {
			if !savedIds[entity.Id] {
				log.Infof("skipping duplicate memory: %s (content: %s)",
					entity.Id, utils.ShrinkString(entity.Content, 50))
			}
		}
	}

	// 4. 保存非重复的记忆
	if len(worthSaving) > 0 {
		if err := t.SaveMemoryEntities(worthSaving...); err != nil {
			return utils.Errorf("failed to save memory entities: %v", err)
		}
		log.Infof("successfully saved %d new memory entities", len(worthSaving))
	} else {
		log.Infof("no new memories to save after deduplication")
	}

	return nil
}

// SearchMemoryResult 搜索记忆的结果
type SearchMemoryResult struct {
	Memories      []*MemoryEntity `json:"memories"`
	TotalContent  string          `json:"total_content"`
	ContentBytes  int             `json:"content_bytes"`
	SearchSummary string          `json:"search_summary"`
}

// SearchMemory 根据输入内容搜索相关记忆，限制总内容字节数
func (t *AIMemoryTriage) SearchMemory(origin any, bytesLimit int) (*SearchMemoryResult, error) {
	// 转换输入为字符串
	queryText := utils.InterfaceToString(origin)
	if strings.TrimSpace(queryText) == "" {
		return &SearchMemoryResult{
			Memories:      []*MemoryEntity{},
			TotalContent:  "",
			ContentBytes:  0,
			SearchSummary: "empty query provided",
		}, nil
	}

	log.Infof("searching memories for query: %s (bytes limit: %d)",
		utils.ShrinkString(queryText, 100), bytesLimit)

	var allMemories []*MemoryEntity
	var searchSteps []string

	// 1. 使用 SelectTags 获取相关标签
	ctx := context.Background()
	relevantTags, err := t.SelectTags(ctx, queryText)
	if err != nil {
		log.Warnf("failed to select tags: %v", err)
		relevantTags = []string{} // 继续执行，但没有标签
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

	return &SearchMemoryResult{
		Memories:      selectedMemories,
		TotalContent:  totalContent,
		ContentBytes:  contentBytes,
		SearchSummary: searchSummary,
	}, nil
}

// deduplicateMemories 去重记忆列表
func (t *AIMemoryTriage) deduplicateMemories(memories []*MemoryEntity) []*MemoryEntity {
	seen := make(map[string]bool)
	var unique []*MemoryEntity

	for _, memory := range memories {
		if memory != nil && !seen[memory.Id] {
			seen[memory.Id] = true
			unique = append(unique, memory)
		}
	}

	return unique
}

// rankMemoriesByRelevance 基于 C.O.R.E. P.A.C.T. 原则对记忆进行重排序
func (t *AIMemoryTriage) rankMemoriesByRelevance(memories []*MemoryEntity, query string) []*MemoryEntity {
	// 为每个记忆计算综合相关性分数
	type ScoredMemory struct {
		Memory         *MemoryEntity
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
	var rankedMemories []*MemoryEntity
	for _, scored := range scoredMemories {
		if scored.RelevanceScore >= 0.3 {
			rankedMemories = append(rankedMemories, scored.Memory)
		}
	}

	return rankedMemories
}

// calculateRelevanceScore 基于 C.O.R.E. P.A.C.T. 原则计算记忆的相关性分数
func (t *AIMemoryTriage) calculateRelevanceScore(memory *MemoryEntity, query string) float64 {
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

	// 内容相关性加成：检查查询词是否出现在记忆内容或标签中
	queryLower := strings.ToLower(query)
	contentLower := strings.ToLower(memory.Content)

	contentBonus := 0.0
	if strings.Contains(contentLower, queryLower) {
		contentBonus += 0.1 // 内容匹配加成
	}

	// 标签匹配加成
	for _, tag := range memory.Tags {
		if strings.Contains(strings.ToLower(tag), queryLower) ||
			strings.Contains(queryLower, strings.ToLower(tag)) {
			contentBonus += 0.05 // 每个匹配标签加成
		}
	}

	// 问题匹配加成
	for _, question := range memory.PotentialQuestions {
		if strings.Contains(strings.ToLower(question), queryLower) {
			contentBonus += 0.03 // 每个匹配问题加成
		}
	}

	// 限制加成不超过0.2
	if contentBonus > 0.2 {
		contentBonus = 0.2
	}

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

// selectMemoriesByBytesLimit 根据字节限制选择记忆
func (t *AIMemoryTriage) selectMemoriesByBytesLimit(memories []*MemoryEntity, bytesLimit int) ([]*MemoryEntity, string, int) {
	if bytesLimit <= 0 {
		return []*MemoryEntity{}, "", 0
	}

	var selectedMemories []*MemoryEntity
	var contentParts []string
	totalBytes := 0

	for _, memory := range memories {
		// 构建记忆的文本表示
		memoryText := fmt.Sprintf("【记忆】%s\n标签：%s\n内容：%s\n",
			memory.Id[:8], // 只显示ID前8位
			strings.Join(memory.Tags, ", "),
			memory.Content)

		memoryBytes := len([]byte(memoryText))

		// 检查是否超过限制
		if totalBytes+memoryBytes > bytesLimit {
			// 如果是第一个记忆就超过限制，尝试截断
			if len(selectedMemories) == 0 && memoryBytes > bytesLimit {
				// 截断内容以适应限制
				availableBytes := bytesLimit - len([]byte(fmt.Sprintf("【记忆】%s\n标签：%s\n内容：",
					memory.Id[:8], strings.Join(memory.Tags, ", "))))
				if availableBytes > 20 { // 至少保留20字节的内容
					truncatedContent := string([]byte(memory.Content)[:availableBytes-10]) + "..."
					memoryText = fmt.Sprintf("【记忆】%s\n标签：%s\n内容：%s\n",
						memory.Id[:8], strings.Join(memory.Tags, ", "), truncatedContent)
					selectedMemories = append(selectedMemories, memory)
					contentParts = append(contentParts, memoryText)
					totalBytes = len([]byte(memoryText))
				}
			}
			break
		}

		selectedMemories = append(selectedMemories, memory)
		contentParts = append(contentParts, memoryText)
		totalBytes += memoryBytes
	}

	totalContent := strings.Join(contentParts, "\n")
	return selectedMemories, totalContent, totalBytes
}
