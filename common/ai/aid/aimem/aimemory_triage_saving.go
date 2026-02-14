package aimem

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// DeduplicationConfig 去重配置
type DeduplicationConfig struct {
	TagOverlapThreshold         float64 // 标签交集比例阈值，默认0.8
	QuestionSimilarityThreshold float64 // 问题相似度阈值，默认0.85
	ContentSimilarityThreshold  float64 // 内容相似度阈值，默认0.9
}

// DefaultDeduplicationConfig 默认去重配置
func DefaultDeduplicationConfig() *DeduplicationConfig {
	return &DeduplicationConfig{
		TagOverlapThreshold:         0.8,
		QuestionSimilarityThreshold: 0.85,
		ContentSimilarityThreshold:  0.9,
	}
}

// ShouldSaveMemoryEntities 判断哪些记忆实体值得保存（去重后）
func (t *AIMemoryTriage) ShouldSaveMemoryEntities(entities []*aicommon.MemoryEntity) []*aicommon.MemoryEntity {
	if len(entities) <= 0 {
		return nil
	}

	config := DefaultDeduplicationConfig()

	// 第一步：批量进行无AI版本的重复检查
	nonRepeatedIndices, err := t.BatchIsRepeatedMemoryEntities(entities, config)
	if err != nil {
		log.Errorf("batch repetition check failed: %v", err)
		// 出错时保守处理，返回所有记忆
		return entities
	}

	// 如果无AI版本已经过滤掉了所有重复，直接返回
	if len(nonRepeatedIndices) == len(entities) {
		log.Infof("no repetition detected by non-AI check, saving all %d memories", len(entities))
		return entities
	}

	// 第二步：对可能重复的记忆使用AI进一步判别
	ctx := context.Background()
	finalIndices, err := t.BatchIsRepeatedMemoryEntitiesByAI(ctx, entities, nonRepeatedIndices, config)
	if err != nil {
		log.Errorf("AI batch repetition check failed: %v", err)
		// AI检查失败时，使用无AI版本的结果
		var result []*aicommon.MemoryEntity
		for _, idx := range nonRepeatedIndices {
			if idx >= 0 && idx < len(entities) {
				result = append(result, entities[idx])
			}
		}
		return result
	}

	// 根据AI返回的索引构建最终结果
	var worthSaving []*aicommon.MemoryEntity
	for _, idx := range finalIndices {
		if idx >= 0 && idx < len(entities) {
			worthSaving = append(worthSaving, entities[idx])
			log.Infof("memory entity %s (index %d) marked for saving", entities[idx].Id, idx)
		}
	}

	log.Infof("deduplication completed: %d/%d memories worth saving", len(worthSaving), len(entities))
	return worthSaving
}

// BatchIsRepeatedMemoryEntities 批量无AI版本的重复检查
func (t *AIMemoryTriage) BatchIsRepeatedMemoryEntities(entities []*aicommon.MemoryEntity, config *DeduplicationConfig) ([]int, error) {
	if len(entities) == 0 {
		return []int{}, nil
	}

	var nonRepeatedIndices []int

	// 对每个记忆实体进行重复检查
	for i, entity := range entities {
		// 1. 基于tags的数据库查询去重
		tagRepeated, err := t.checkTagRepetition(entity, config.TagOverlapThreshold)
		if err != nil {
			log.Warnf("tag repetition check failed for entity %s: %v", entity.Id, err)
			tagRepeated = false // 出错时假设不重复
		}

		// 2. 基于potential_questions的RAG搜索去重
		questionRepeated, err := t.checkQuestionRepetition(entity, config.QuestionSimilarityThreshold)
		if err != nil {
			log.Warnf("question repetition check failed for entity %s: %v", entity.Id, err)
			questionRepeated = false // 出错时假设不重复
		}

		// 3. 基于内容的相似度检查
		contentRepeated, err := t.checkContentRepetition(entity, config.ContentSimilarityThreshold)
		if err != nil {
			log.Warnf("content repetition check failed for entity %s: %v", entity.Id, err)
			contentRepeated = false // 出错时假设不重复
		}

		// 综合判断：如果在多个维度上都高度相似，则认为重复
		repetitionScore := 0
		if tagRepeated {
			repetitionScore++
		}
		if questionRepeated {
			repetitionScore++
		}
		if contentRepeated {
			repetitionScore++
		}

		// 如果在2个或以上维度重复，则认为是重复记忆
		isRepeated := repetitionScore >= 2
		if !isRepeated {
			nonRepeatedIndices = append(nonRepeatedIndices, i)
			log.Infof("memory entity %s (index %d) passed non-AI repetition check", entity.Id, i)
		} else {
			log.Infof("memory entity %s (index %d) flagged as potentially repeated by non-AI check", entity.Id, i)
		}
	}

	return nonRepeatedIndices, nil
}

// BatchIsRepeatedMemoryEntitiesByAI AI版本的批量重复检查
func (t *AIMemoryTriage) BatchIsRepeatedMemoryEntitiesByAI(ctx context.Context, entities []*aicommon.MemoryEntity, candidateIndices []int, config *DeduplicationConfig) ([]int, error) {
	if len(candidateIndices) == 0 {
		return []int{}, nil
	}

	// 收集所有候选记忆的相似记忆作为上下文
	allSimilarMemories, err := t.findSimilarMemoriesForBatch(entities, candidateIndices, 10) // 最多找10个相似记忆
	if err != nil {
		return nil, utils.Errorf("failed to find similar memories for batch: %v", err)
	}

	// 构建AI批量判别的prompt
	prompt, err := t.buildBatchDeduplicationPrompt(entities, candidateIndices, allSimilarMemories)
	if err != nil {
		return nil, utils.Errorf("failed to build batch deduplication prompt: %v", err)
	}

	// 调用AI进行批量高级判别
	action, err := t.invoker.InvokeLiteForge(ctx, "batch-memory-deduplication", prompt, []aitool.ToolOption{
		aitool.WithStringArrayParam("non_duplicate_indices", aitool.WithParam_Description("不重复的记忆索引列表，例如: [\"1\", \"3\", \"5\"]。只返回确实不重复且值得保存的记忆索引")),
		aitool.WithStringParam("analysis", aitool.WithParam_Description("详细分析每个记忆的重复情况和保留理由")),
	})
	if err != nil {
		return nil, utils.Errorf("AI batch deduplication check failed: %v", err)
	}

	nonDuplicateIndicesStr := action.GetStringSlice("non_duplicate_indices")
	analysis := action.GetString("analysis")

	// 转换字符串索引为整数
	var finalIndices []int
	for _, idxStr := range nonDuplicateIndicesStr {
		if idx := utils.InterfaceToInt(idxStr); idx >= 0 && idx < len(entities) {
			finalIndices = append(finalIndices, idx)
		}
	}

	log.Infof("AI batch deduplication result: %d/%d memories selected as non-duplicate",
		len(finalIndices), len(candidateIndices))
	log.Infof("AI analysis: %s", analysis)

	return finalIndices, nil
}

// checkTagRepetition 检查标签重复
func (t *AIMemoryTriage) checkTagRepetition(entity *aicommon.MemoryEntity, threshold float64) (bool, error) {
	if len(entity.Tags) == 0 {
		return false, nil
	}

	db := consts.GetGormProjectDatabase()
	if db == nil {
		return false, utils.Errorf("database connection is nil")
	}

	// 查询所有现有记忆的标签
	var existingEntities []schema.AIMemoryEntity
	if err := db.Where("session_id = ?", t.sessionID).Find(&existingEntities).Error; err != nil {
		return false, utils.Errorf("failed to query existing entities: %v", err)
	}

	entityTagSet := make(map[string]bool)
	for _, tag := range entity.Tags {
		entityTagSet[strings.ToLower(tag)] = true
	}

	// 检查与每个现有记忆的标签重叠度
	for _, existing := range existingEntities {
		if existing.MemoryID == entity.Id {
			continue // 跳过自己
		}

		existingTags := []string(existing.Tags)
		if len(existingTags) == 0 {
			continue
		}

		// 计算Jaccard相似度
		intersection := 0
		existingTagSet := make(map[string]bool)
		for _, tag := range existingTags {
			tagLower := strings.ToLower(tag)
			existingTagSet[tagLower] = true
			if entityTagSet[tagLower] {
				intersection++
			}
		}

		union := len(entityTagSet) + len(existingTagSet) - intersection
		if union == 0 {
			continue
		}

		jaccardSimilarity := float64(intersection) / float64(union)
		if jaccardSimilarity >= threshold {
			log.Infof("high tag overlap detected: entity %s vs existing %s, similarity=%.3f",
				entity.Id, existing.MemoryID, jaccardSimilarity)
			return true, nil
		}
	}

	return false, nil
}

// checkQuestionRepetition 检查问题重复
func (t *AIMemoryTriage) checkQuestionRepetition(entity *aicommon.MemoryEntity, threshold float64) (bool, error) {
	if len(entity.PotentialQuestions) == 0 {
		return false, nil
	}

	// 如果RAG系统不可用，直接返回false（不检查重复）
	if t.rag == nil {
		log.Debugf("RAG system not initialized, skipping question repetition check")
		return false, nil
	}

	// 使用RAG系统搜索相似问题
	for _, question := range entity.PotentialQuestions {
		if strings.TrimSpace(question) == "" {
			continue
		}

		// 在RAG系统中搜索相似问题
		results, err := t.rag.QueryTopN(question, 5) // 搜索最相似的5个文档
		if err != nil {
			log.Warnf("RAG search failed for question '%s': %v", question, err)
			continue
		}

		for _, result := range results {
			// RAG的相似度分数通常是0-1之间，1表示最相似
			if result.Score >= threshold {
				log.Infof("high question similarity detected: '%s' vs existing document, similarity=%.3f",
					question, result.Score)
				return true, nil
			}
		}
	}

	return false, nil
}

// checkContentRepetition 检查内容重复
func (t *AIMemoryTriage) checkContentRepetition(entity *aicommon.MemoryEntity, threshold float64) (bool, error) {
	if strings.TrimSpace(entity.Content) == "" {
		return false, nil
	}

	// 如果RAG系统不可用，直接返回false（不检查重复）
	if t.rag == nil {
		log.Debugf("RAG system not initialized, skipping content repetition check")
		return false, nil
	}

	// 使用RAG系统搜索相似内容
	results, err := t.rag.QueryTopN(entity.Content, 3) // 搜索最相似的3个文档
	if err != nil {
		log.Warnf("RAG content search failed: %v", err)
		return false, nil
	}

	for _, result := range results {
		if result.Score >= threshold {
			log.Infof("high content similarity detected for entity %s, similarity=%.3f",
				entity.Id, result.Score)
			return true, nil
		}
	}

	return false, nil
}

// findSimilarMemories 查找相似的记忆实体
func (t *AIMemoryTriage) findSimilarMemories(entity *aicommon.MemoryEntity, limit int) ([]*aicommon.MemoryEntity, error) {
	var similarMemories []*aicommon.MemoryEntity

	// 1. 基于标签查找相似记忆
	if len(entity.Tags) > 0 {
		tagSimilar, err := t.findSimilarByTags(entity, limit)
		if err == nil {
			similarMemories = append(similarMemories, tagSimilar...)
		}
	}

	// 2. 基于内容查找相似记忆
	if strings.TrimSpace(entity.Content) != "" {
		contentSimilar, err := t.findSimilarByContent(entity, limit)
		if err == nil {
			similarMemories = append(similarMemories, contentSimilar...)
		}
	}

	// 去重并限制数量
	seen := make(map[string]bool)
	var uniqueSimilar []*aicommon.MemoryEntity
	for _, mem := range similarMemories {
		if !seen[mem.Id] && mem.Id != entity.Id {
			seen[mem.Id] = true
			uniqueSimilar = append(uniqueSimilar, mem)
			if len(uniqueSimilar) >= limit {
				break
			}
		}
	}

	return uniqueSimilar, nil
}

// findSimilarByTags 基于标签查找相似记忆
func (t *AIMemoryTriage) findSimilarByTags(entity *aicommon.MemoryEntity, limit int) ([]*aicommon.MemoryEntity, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}

	var existingEntities []schema.AIMemoryEntity
	if err := db.Where("session_id = ? AND memory_id != ?", t.sessionID, entity.Id).
		Limit(limit * 2). // 查询更多以便过滤
		Find(&existingEntities).Error; err != nil {
		return nil, utils.Errorf("failed to query existing entities: %v", err)
	}

	var similar []*aicommon.MemoryEntity
	entityTagSet := make(map[string]bool)
	for _, tag := range entity.Tags {
		entityTagSet[strings.ToLower(tag)] = true
	}

	for _, existing := range existingEntities {
		existingTags := []string(existing.Tags)
		if len(existingTags) == 0 {
			continue
		}

		// 计算标签相似度
		intersection := 0
		for _, tag := range existingTags {
			if entityTagSet[strings.ToLower(tag)] {
				intersection++
			}
		}

		if intersection > 0 {
			memEntity := &aicommon.MemoryEntity{
				Id:                 existing.MemoryID,
				Content:            existing.Content,
				Tags:               existingTags,
				PotentialQuestions: []string(existing.PotentialQuestions),
				C_Score:            existing.C_Score,
				O_Score:            existing.O_Score,
				R_Score:            existing.R_Score,
				E_Score:            existing.E_Score,
				P_Score:            existing.P_Score,
				A_Score:            existing.A_Score,
				T_Score:            existing.T_Score,
			}
			similar = append(similar, memEntity)
			if len(similar) >= limit {
				break
			}
		}
	}

	return similar, nil
}

// findSimilarByContent 基于内容查找相似记忆
func (t *AIMemoryTriage) findSimilarByContent(entity *aicommon.MemoryEntity, limit int) ([]*aicommon.MemoryEntity, error) {
	// 使用RAG搜索相似内容
	results, err := t.rag.QueryTopN(entity.Content, limit)
	if err != nil {
		return nil, utils.Errorf("RAG search failed: %v", err)
	}

	var similar []*aicommon.MemoryEntity
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}

	for _, result := range results {
		// 从文档ID中提取记忆ID（假设格式为 memoryID-questionHash）
		parts := strings.Split(result.Document.ID, "-")
		if len(parts) < 1 {
			continue
		}
		memoryID := parts[0]

		if memoryID == entity.Id {
			continue // 跳过自己
		}

		var existing schema.AIMemoryEntity
		if err := db.Where("memory_id = ? AND session_id = ?", memoryID, t.sessionID).
			First(&existing).Error; err != nil {
			continue // 记忆不存在，跳过
		}

		memEntity := &aicommon.MemoryEntity{
			Id:                 existing.MemoryID,
			Content:            existing.Content,
			Tags:               []string(existing.Tags),
			PotentialQuestions: []string(existing.PotentialQuestions),
			C_Score:            existing.C_Score,
			O_Score:            existing.O_Score,
			R_Score:            existing.R_Score,
			E_Score:            existing.E_Score,
			P_Score:            existing.P_Score,
			A_Score:            existing.A_Score,
			T_Score:            existing.T_Score,
		}
		similar = append(similar, memEntity)
	}

	return similar, nil
}

// findSimilarMemoriesForBatch 为批量记忆查找相似记忆
func (t *AIMemoryTriage) findSimilarMemoriesForBatch(entities []*aicommon.MemoryEntity, candidateIndices []int, limit int) ([]*aicommon.MemoryEntity, error) {
	var allSimilarMemories []*aicommon.MemoryEntity
	seen := make(map[string]bool)

	// 为每个候选记忆查找相似记忆
	for _, idx := range candidateIndices {
		if idx >= 0 && idx < len(entities) {
			entity := entities[idx]
			similarMemories, err := t.findSimilarMemories(entity, limit/len(candidateIndices)+1)
			if err != nil {
				log.Warnf("failed to find similar memories for entity %s: %v", entity.Id, err)
				continue
			}

			// 去重添加
			for _, mem := range similarMemories {
				if !seen[mem.Id] {
					seen[mem.Id] = true
					allSimilarMemories = append(allSimilarMemories, mem)
					if len(allSimilarMemories) >= limit {
						break
					}
				}
			}

			if len(allSimilarMemories) >= limit {
				break
			}
		}
	}

	return allSimilarMemories, nil
}

// buildBatchDeduplicationPrompt 构建批量去重判别的prompt
func (t *AIMemoryTriage) buildBatchDeduplicationPrompt(entities []*aicommon.MemoryEntity, candidateIndices []int, similarMemories []*aicommon.MemoryEntity) (string, error) {
	nonce := utils.RandStringBytes(4)

	// 构建候选记忆列表
	var candidateMemoriesText strings.Builder
	for _, idx := range candidateIndices {
		if idx >= 0 && idx < len(entities) {
			entity := entities[idx]
			candidateMemoriesText.WriteString(fmt.Sprintf("Index %d:\n", idx))
			candidateMemoriesText.WriteString(fmt.Sprintf("  内容: %s\n", entity.Content))
			candidateMemoriesText.WriteString(fmt.Sprintf("  标签: %s\n", strings.Join(entity.Tags, ", ")))
			candidateMemoriesText.WriteString(fmt.Sprintf("  问题: %s\n", strings.Join(entity.PotentialQuestions, "; ")))
			candidateMemoriesText.WriteString(fmt.Sprintf("  评分: C=%.2f, O=%.2f, R=%.2f, E=%.2f, P=%.2f, A=%.2f, T=%.2f\n\n",
				entity.C_Score, entity.O_Score, entity.R_Score, entity.E_Score, entity.P_Score, entity.A_Score, entity.T_Score))
		}
	}

	// 构建现有记忆列表
	var existingMemoriesText strings.Builder
	for i, mem := range similarMemories {
		existingMemoriesText.WriteString(fmt.Sprintf("现有记忆%d:\n", i+1))
		existingMemoriesText.WriteString(fmt.Sprintf("  内容: %s\n", mem.Content))
		existingMemoriesText.WriteString(fmt.Sprintf("  标签: %s\n", strings.Join(mem.Tags, ", ")))
		existingMemoriesText.WriteString(fmt.Sprintf("  问题: %s\n", strings.Join(mem.PotentialQuestions, "; ")))
		existingMemoriesText.WriteString(fmt.Sprintf("  评分: C=%.2f, O=%.2f, R=%.2f, E=%.2f, P=%.2f, A=%.2f, T=%.2f\n\n",
			mem.C_Score, mem.O_Score, mem.R_Score, mem.E_Score, mem.P_Score, mem.A_Score, mem.T_Score))
	}

	// 读取C.O.R.E. P.A.C.T.原则
	corepactPrinciples := `## C.O.R.E. P.A.C.T. Framework Scoring Guide (All scores normalized to 0.0-1.0):

### T - Temporality (时效性) - How long should this memory be retained?
- **0.0-0.3**: Transient memory - Only for current conversation flow, almost useless after session ends
- **0.3-0.6**: Short-term memory - Valid within a project/topic discussion cycle (days/weeks)
- **0.6-0.8**: Mid-term memory - User's phased preferences or facts, stable in short term
- **0.8-1.0**: Long-term/Core memory - User's core identity, basic preferences, unchanging instructions

### A - Actionability (可操作性) - Can AI learn and improve future behavior from this?
- **0.0-0.3**: Low value information - Simple facts, small talk, no clear learning value
- **0.3-0.6**: Implicit feedback - User behavior patterns suggest tendencies or successful paths
- **0.6-0.8**: Explicit feedback - User directly provided positive/negative evaluation of AI output
- **0.8-1.0**: Generalizable rule - User gave clear instructions/rules for AI to follow in future

### P - Preference (个人偏好) - Does this bind to user's personal style, taste, or work methods?
- **0.0-0.3**: Impersonal - Universal knowledge or information
- **0.3-0.6**: Contextual preference - Preferences shown in specific domains/projects
- **0.6-0.8**: Personal style preference - User's communication style, knowledge level, format requirements
- **0.8-1.0**: Core identity - User's core identity, long-term goals, values

### O - Origin (来源与确定性) - Where does this information come from? How reliable?
- **0.0-0.2**: AI inferred - AI's own inference from context, not confirmed by user
- **0.2-0.5**: Indirect source - Information from third parties or vague sources mentioned by user
- **0.5-0.7**: External tool/document - Information from APIs, specific documents, or web pages
- **0.7-0.9**: User statement - Facts or opinions explicitly stated by user
- **0.9-1.0**: User's core directive - Direct instructions about AI behavior or personal core facts

### E - Emotion (情感基调) - User's emotional state when expressing this information?
- **0.0-0.2**: Strong negative - User shows obvious frustration, anger, or disappointment
- **0.2-0.4**: Slight negative - User shows confusion, uncertainty, or mild dissatisfaction
- **0.4-0.6**: Neutral - Pure information exchange, no obvious emotional coloring
- **0.6-0.8**: Slight positive - User shows satisfaction, curiosity, or approval
- **0.8-1.0**: Strong positive - User shows excitement, praise, or high satisfaction

### R - Relevance (相关性) - How critical is this information to user's goals?
- **0.0-0.3**: Trivial/auxiliary information - Nice-to-have details, no harm if missing
- **0.3-0.6**: Relevant context - Helps better understand tasks, but not core elements
- **0.6-0.8**: Important requirement - Directly affects task output quality
- **0.8-1.0**: Critical/blocking information - Task success depends on this (deadlines, core constraints)

### C - Connectivity (关联度) - How many other memories is this connected to?
- **0.0-0.3**: Isolated memory - One-time fact/question with almost no connection to other information
- **0.3-0.6**: Linearly connected - Part of a conversation flow, sequential relationship with other memories
- **0.6-0.8**: Thematic node - Key information in a theme/project, connects multiple other memories in that theme
- **0.8-1.0**: Core hub - Connects multiple different themes/domains, fundamental to user's worldview`

	prompt, err := utils.RenderTemplate(`
你是一个AI记忆去重专家。请批量判断哪些新记忆不重复且值得保存。

{{ .CorePactPrinciples }}

<|CANDIDATE_MEMORIES_{{ .Nonce }}|>
{{ .CandidateMemories }}
<|CANDIDATE_MEMORIES_END_{{ .Nonce }}|>

<|EXISTING_MEMORIES_{{ .Nonce }}|>
{{ .ExistingMemories }}
<|EXISTING_MEMORIES_END_{{ .Nonce }}|>

判断标准:
1. 如果候选记忆与现有记忆在内容、标签、问题三个维度上高度相似(>90%)，则视为重复
2. 如果候选记忆提供了显著的增量价值（新标签、新问题、新视角），则不应视为重复
3. 基于C.O.R.E. P.A.C.T.评分系统，综合考虑记忆的价值：
   - 高原创性(O)和高优先级(P)的记忆更倾向于保留
   - 高关联度(C)和高可操作性(A)的记忆有助于构建知识网络
   - 高时效性(T)和高相关性(R)的记忆对当前任务更重要
4. 即使内容相似，如果新记忆能补充或完善现有记忆，也应保留
5. 保持记忆多样性，避免过度去重导致信息丢失

请返回不重复且值得保存的记忆索引列表。例如，如果Index 1, 3, 5的记忆不重复，则返回["1", "3", "5"]。
`, map[string]any{
		"CorePactPrinciples": corepactPrinciples,
		"Nonce":              nonce,
		"CandidateMemories":  candidateMemoriesText.String(),
		"ExistingMemories":   existingMemoriesText.String(),
	})

	if err != nil {
		return "", utils.Errorf("failed to render batch prompt template: %v", err)
	}

	return prompt, nil
}
