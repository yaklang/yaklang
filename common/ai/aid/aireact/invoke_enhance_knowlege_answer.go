package aireact

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func (r *ReAct) EnhanceKnowledgeAnswer(ctx context.Context, userQuery string) (string, error) {
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}

	currentTask := r.GetCurrentTask()
	enhanceID := uuid.NewString()
	config := r.config

	ekm := config.EnhanceKnowledgeManager

	if ekm == nil {
		log.Errorf("enhanceKnowledgeManager is not configured, but ai choice knowledge enhance answer action, check config! use temp rag knowledge manager")
		ekm = rag.NewRagEnhanceKnowledgeManager()
		ekm.SetEmitter(r.Emitter)
	}

	enhanceData, err := ekm.FetchKnowledge(ctx, userQuery)
	if err != nil {
		return "", utils.Errorf("enhanceKnowledgeManager.FetchKnowledge(%s) failed: %v", userQuery, err)
	}

	// Collect all knowledge items for summary artifact
	var knowledgeList []aicommon.EnhanceKnowledge
	for enhanceDatum := range enhanceData {
		r.EmitKnowledge(enhanceID, enhanceDatum)
		ekm.AppendKnowledge(currentTask.GetId(), enhanceDatum)
		knowledgeList = append(knowledgeList, enhanceDatum)
	}
	knowledgeCount := len(knowledgeList)

	// Save all knowledge to a single artifact file
	if knowledgeCount > 0 {
		r.EmitKnowledgeReferenceArtifact(knowledgeList, userQuery)
	}

	var queryBuf bytes.Buffer
	queryBuf.WriteString(userQuery)

	enhance := r.DumpCurrentEnhanceData()

	// 如果知识条目过多（超过 5 条），使用 AI 智能压缩
	// 参考 loop_yaklangcode 中的上下文压缩技术
	if enhance != "" && knowledgeCount > 5 {
		log.Infof("EnhanceKnowledgeAnswer: %d knowledge items found, attempting AI compression", knowledgeCount)
		compressedEnhance := r.compressKnowledgeResults(ctx, enhance, userQuery, 15)
		if len(compressedEnhance) < len(enhance) {
			log.Infof("EnhanceKnowledgeAnswer: compressed from %d to %d chars", len(enhance), len(compressedEnhance))
			enhance = compressedEnhance
		}
	}

	if enhance != "" {
		enhancePayload, err := utils.RenderTemplate(`<|ENHANCE_DATA_{{ .Nonce }}|>
{{ .EnhanceData }}
<|ENHANCE_DATA_{{ .Nonce }}|>
`, map[string]interface{}{
			"Nonce":       nonce(),
			"EnhanceData": enhance,
		})
		if err != nil {
			log.Warnf("enhanceKnowledgeAnswer.DumpCurrentEnhanceData() failed: %v", err)
		}
		if enhancePayload != "" {
			queryBuf.WriteString("\n\n")
			queryBuf.WriteString(enhancePayload)
		}
	}

	// Build reference material content with original query and knowledge data
	referenceMaterial := ""
	if enhance != "" {
		referenceMaterial, _ = utils.RenderTemplate(`<|ORIGINAL_QUERY|>
{{ .OriginalQuery }}
<|ORIGINAL_QUERY_END|>

<|KNOWLEDGE_ENHANCED_DATA|>
{{ .EnhanceData }}
<|KNOWLEDGE_ENHANCED_DATA_END|>

知识条目数量: {{ .KnowledgeCount }} (已通过 AI 智能筛选)
`, map[string]any{
			"OriginalQuery":  userQuery,
			"EnhanceData":    enhance,
			"KnowledgeCount": knowledgeCount,
		})
	}

	// Pass reference material to DirectlyAnswer for emission with stream
	var opts []any
	if referenceMaterial != "" {
		opts = append(opts, WithReferenceMaterial(referenceMaterial, 1))
	}

	finalResult, err := r.DirectlyAnswer(ctx, queryBuf.String(), nil, opts...)
	// Note: DirectlyAnswer already emits the result via stream
	// EmitTextArtifact only saves to file for reference, doesn't show duplicate UI
	if finalResult != "" {
		r.EmitTextArtifact("enhance_directly_answer", finalResult)
	}
	return finalResult, err
}

func (r *ReAct) EnhanceKnowledgeGetRandomN(ctx context.Context, n int, collections ...string) (string, error) {
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}
	_ = ctx // 预留 ctx 供后续使用

	if n <= 0 {
		n = 10
	}

	db := consts.GetGormProfileDatabase()
	var allEntries []*schema.KnowledgeBaseEntry

	// 遍历每个知识库获取随机条目
	for _, collectionName := range collections {
		// 获取知识库信息
		kb, err := yakit.GetKnowledgeBaseByName(db, collectionName)
		if err != nil {
			log.Warnf("failed to get knowledge base %s: %v", collectionName, err)
			continue
		}

		// 使用随机排序获取条目
		var entries []*schema.KnowledgeBaseEntry
		err = db.Model(&schema.KnowledgeBaseEntry{}).
			Where("knowledge_base_id = ?", kb.ID).
			Order("RANDOM()").
			Limit(n).
			Find(&entries).Error
		if err != nil {
			log.Warnf("failed to get random entries from knowledge base %s: %v", collectionName, err)
			continue
		}

		allEntries = append(allEntries, entries...)
	}

	if len(allEntries) == 0 {
		return "", nil
	}

	// 格式化输出
	var result bytes.Buffer
	result.WriteString(fmt.Sprintf("=== 知识库样本数据 (共 %d 条) ===\n\n", len(allEntries)))

	for i, entry := range allEntries {
		result.WriteString(fmt.Sprintf("【条目 %d】\n", i+1))
		result.WriteString(fmt.Sprintf("标题: %s\n", entry.KnowledgeTitle))
		if entry.Summary != "" {
			result.WriteString(fmt.Sprintf("摘要: %s\n", entry.Summary))
		}
		if len(entry.Keywords) > 0 {
			result.WriteString(fmt.Sprintf("关键词: %s\n", strings.Join(entry.Keywords, ", ")))
		}
		if entry.KnowledgeType != "" {
			result.WriteString(fmt.Sprintf("类型: %s\n", entry.KnowledgeType))
		}
		if entry.KnowledgeDetails != "" {
			result.WriteString(fmt.Sprintf("详细内容: %s\n", entry.KnowledgeDetails))
		}
		result.WriteString("\n")
	}

	return result.String(), nil
}

func (r *ReAct) EnhanceKnowledgeGetter(ctx context.Context, userQuery string, collections ...string) (string, error) {
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}

	currentTask := r.GetCurrentTask()
	enhanceID := uuid.NewString()
	config := r.config

	ekm := config.EnhanceKnowledgeManager
	if ekm == nil {
		log.Errorf("enhanceKnowledgeManager is not configured, but ai choice knowledge enhance answer action, check config! use temp rag knowledge manager")
		ekm = rag.NewRagEnhanceKnowledgeManager()
		ekm.SetEmitter(r.Emitter)
	}

	enhanceData, err := ekm.FetchKnowledgeWithCollections(ctx, collections, userQuery)
	if err != nil {
		return "", utils.Errorf("enhanceKnowledgeManager.FetchKnowledge(%s) failed: %v", userQuery, err)
	}

	for enhanceDatum := range enhanceData {
		r.EmitKnowledge(enhanceID, enhanceDatum)
		ekm.AppendKnowledge(currentTask.GetId(), enhanceDatum)
	}

	var queryBuf bytes.Buffer
	queryBuf.WriteString(userQuery)

	enhance := r.DumpCurrentEnhanceData()
	if enhance != "" {
		enhancePayload, err := utils.RenderTemplate(`<|ENHANCE_DATA_{{ .Nonce }}|>
{{ .EnhanceData }}
<|ENHANCE_DATA_{{ .Nonce }}|>
`, map[string]interface{}{
			"Nonce":       nonce(),
			"EnhanceData": enhance,
		})
		if err != nil {
			log.Warnf("enhanceKnowledgeAnswer.DumpCurrentEnhanceData() failed: %v", err)
		}
		if enhancePayload != "" {
			queryBuf.WriteString("\n\n")
			queryBuf.WriteString(enhancePayload)
		}
	}

	return enhance, nil
}

// compressKnowledgeResults 使用 AI 智能压缩知识搜索结果
// 参考 loop_yaklangcode 和 aireducer 中的上下文压缩技术
// 将长内容带行号展示，让 AI 筛选出与用户问题最相关的片段
// 对于超大内容（>20KB），使用分片 + overlap 技术分批处理
func (r *ReAct) compressKnowledgeResults(ctx context.Context, knowledgeContent string, userQuery string, maxRanges int) string {
	if len(knowledgeContent) == 0 {
		return knowledgeContent
	}

	// 如果内容不够长，不需要压缩
	if len(knowledgeContent) < 3000 {
		log.Infof("compressKnowledgeResults: content too short (%d chars), skip compression", len(knowledgeContent))
		return knowledgeContent
	}

	// 设置默认参数
	if maxRanges <= 0 {
		maxRanges = 15
	}

	// 对于超大内容（>20KB），使用分片处理
	const maxChunkSize = 20 * 1024 // 20KB per chunk
	const overlapLines = 20        // 20 行 overlap
	const maxChunks = 10           // 最多 10 个分片

	if len(knowledgeContent) > maxChunkSize {
		log.Infof("compressKnowledgeResults: content too large (%d bytes), using chunked processing", len(knowledgeContent))
		return r.compressKnowledgeResultsChunked(ctx, knowledgeContent, userQuery, maxRanges, maxChunkSize, overlapLines, maxChunks)
	}

	// 对于较小的内容，直接处理
	return r.compressKnowledgeResultsSingle(ctx, knowledgeContent, userQuery, maxRanges)
}

// compressKnowledgeResultsChunked 使用分片方式处理超大内容
// 借鉴 aireducer 的设计：先给整个内容添加行号，然后按大小分片
// 使用行号 overlap 确保上下文连贯
func (r *ReAct) compressKnowledgeResultsChunked(ctx context.Context, knowledgeContent string, userQuery string, maxRanges int, chunkSize int, overlapLines int, maxChunks int) string {
	// 步骤1: 先按行分割原始内容
	originalLines := strings.Split(knowledgeContent, "\n")
	totalLines := len(originalLines)

	log.Infof("compressKnowledgeResultsChunked: processing %d bytes, %d lines, chunkSize=%d, overlapLines=%d, maxChunks=%d",
		len(knowledgeContent), totalLines, chunkSize, overlapLines, maxChunks)

	// 步骤2: 计算每个 chunk 应该包含多少行
	// 估算平均每行长度（考虑行号前缀约 10 字符）
	avgLineLen := len(knowledgeContent)/totalLines + 10
	linesPerChunk := chunkSize / avgLineLen
	if linesPerChunk < 50 {
		linesPerChunk = 50
	}

	// 调整以确保不超过 maxChunks
	effectiveLinesPerChunk := linesPerChunk - overlapLines
	if effectiveLinesPerChunk <= 0 {
		effectiveLinesPerChunk = linesPerChunk / 2
	}
	estimatedChunks := (totalLines + effectiveLinesPerChunk - 1) / effectiveLinesPerChunk
	if estimatedChunks > maxChunks {
		effectiveLinesPerChunk = (totalLines + maxChunks - 1) / maxChunks
		linesPerChunk = effectiveLinesPerChunk + overlapLines
		log.Infof("compressKnowledgeResultsChunked: adjusted linesPerChunk to %d to limit chunks to %d", linesPerChunk, maxChunks)
	}

	// 步骤3: 分片处理
	type ChunkResult struct {
		ChunkIndex int
		StartLine  int
		EndLine    int
		Ranges     []RankedRange
	}
	var allChunkResults []ChunkResult

	chunkIndex := 0
	for startLineIdx := 0; startLineIdx < totalLines && chunkIndex < maxChunks; chunkIndex++ {
		// 计算当前 chunk 的行范围（1-based 行号）
		startLine := startLineIdx + 1
		endLineIdx := startLineIdx + linesPerChunk
		if endLineIdx > totalLines {
			endLineIdx = totalLines
		}
		endLine := endLineIdx

		// 提取当前 chunk 的行
		chunkLines := originalLines[startLineIdx:endLineIdx]

		// 构建带行号的 chunk 内容
		var chunkBuilder strings.Builder
		for i, line := range chunkLines {
			lineNum := startLineIdx + i + 1
			chunkBuilder.WriteString(fmt.Sprintf("%d | %s\n", lineNum, line))
		}
		chunkContent := chunkBuilder.String()

		// 添加 overlap 上下文（从前面取 overlapLines 行）
		var chunkWithOverlap string
		if startLineIdx > 0 && overlapLines > 0 {
			overlapStartIdx := startLineIdx - overlapLines
			if overlapStartIdx < 0 {
				overlapStartIdx = 0
			}
			overlapLinesContent := originalLines[overlapStartIdx:startLineIdx]
			var overlapBuilder strings.Builder
			overlapBuilder.WriteString("--- [上下文开始] ---\n")
			for i, line := range overlapLinesContent {
				lineNum := overlapStartIdx + i + 1
				overlapBuilder.WriteString(fmt.Sprintf("%d | %s\n", lineNum, line))
			}
			overlapBuilder.WriteString("--- [上下文结束] ---\n\n")
			chunkWithOverlap = overlapBuilder.String() + chunkContent
		} else {
			chunkWithOverlap = chunkContent
		}

		// 打印 chunk 内容摘要日志
		chunkPreview := utils.ShrinkString(chunkContent, 300)
		log.Infof("compressKnowledgeResultsChunked: chunk %d/%d (lines %d-%d, %d lines, size=%d bytes):\n%s",
			chunkIndex+1, maxChunks, startLine, endLine, len(chunkLines), len(chunkContent), chunkPreview)

		// 对当前 chunk 进行 AI 筛选
		chunkRanges := r.compressKnowledgeChunk(ctx, chunkWithOverlap, "", userQuery, maxRanges/2+1, startLine, endLine)

		if len(chunkRanges) > 0 {
			allChunkResults = append(allChunkResults, ChunkResult{
				ChunkIndex: chunkIndex,
				StartLine:  startLine,
				EndLine:    endLine,
				Ranges:     chunkRanges,
			})
			log.Infof("compressKnowledgeResultsChunked: chunk %d extracted %d ranges", chunkIndex+1, len(chunkRanges))
		} else {
			log.Infof("compressKnowledgeResultsChunked: chunk %d extracted 0 ranges", chunkIndex+1)
		}

		// 移动到下一个 chunk（减去 overlap 行数）
		startLineIdx = endLineIdx - overlapLines
		if startLineIdx < 0 {
			startLineIdx = 0
		}
		// 确保向前推进
		if startLineIdx <= (endLineIdx - linesPerChunk) {
			startLineIdx = endLineIdx
		}
	}

	log.Infof("compressKnowledgeResultsChunked: processed %d chunks total", chunkIndex)

	// 合并所有 chunk 的结果
	var allRanges []RankedRange
	for _, cr := range allChunkResults {
		allRanges = append(allRanges, cr.Ranges...)
	}

	if len(allRanges) == 0 {
		log.Warnf("compressKnowledgeResultsChunked: no valid ranges extracted from any chunk")
		// 返回截断的原始内容
		if len(knowledgeContent) > 50000 {
			return knowledgeContent[:50000] + "\n\n[... 内容过长，已截断 ...]"
		}
		return knowledgeContent
	}

	// 按 rank 排序
	sort.Slice(allRanges, func(i, j int) bool {
		return allRanges[i].Rank < allRanges[j].Rank
	})

	// 限制最终结果数量
	if len(allRanges) > maxRanges {
		allRanges = allRanges[:maxRanges]
	}

	// 去重（基于行范围重叠）
	allRanges = deduplicateRanges(allRanges)

	// 从原始内容中提取最终结果
	resultEditor := memedit.NewMemEditor(knowledgeContent)
	var result strings.Builder
	result.WriteString(fmt.Sprintf("【AI 智能筛选】从 %d 字节内容中提取的 %d 个最相关知识片段：\n\n", len(knowledgeContent), len(allRanges)))

	totalExtracted := 0
	maxTotalLines := 200

	for i, item := range allRanges {
		text := resultEditor.GetTextFromPositionInt(item.StartLine, 1, item.EndLine, 1)
		if text == "" {
			continue
		}

		lineCount := strings.Count(text, "\n") + 1
		if totalExtracted+lineCount > maxTotalLines {
			result.WriteString(fmt.Sprintf("\n[... 已达到 %d 行限制，剩余 %d 个片段未展示 ...]\n", maxTotalLines, len(allRanges)-i))
			break
		}

		result.WriteString(fmt.Sprintf("=== [%d] 相关性排序: %d (行 %d-%d) ===\n", i+1, item.Rank, item.StartLine, item.EndLine))
		if item.Reason != "" {
			result.WriteString(fmt.Sprintf("相关性说明: %s\n", item.Reason))
		}
		result.WriteString(text)
		result.WriteString("\n\n")

		totalExtracted += lineCount
	}

	finalResult := result.String()

	log.Infof("compressKnowledgeResultsChunked: compressed from %d chars to %d chars, %d ranges from %d chunks",
		len(knowledgeContent), len(finalResult), len(allRanges), len(allChunkResults))

	return finalResult
}

// RankedRange 表示一个带排名的行范围
type RankedRange struct {
	Range     string
	StartLine int
	EndLine   int
	Rank      int
	Reason    string
	Text      string
}

// deduplicateRanges 去除重叠的范围
func deduplicateRanges(ranges []RankedRange) []RankedRange {
	if len(ranges) <= 1 {
		return ranges
	}

	var result []RankedRange
	for _, r := range ranges {
		overlaps := false
		for _, existing := range result {
			// 检查是否重叠
			if r.StartLine <= existing.EndLine && r.EndLine >= existing.StartLine {
				overlaps = true
				break
			}
		}
		if !overlaps {
			result = append(result, r)
		}
	}
	return result
}

// compressKnowledgeChunk 对单个 chunk 进行 AI 筛选
func (r *ReAct) compressKnowledgeChunk(ctx context.Context, chunkContentWithLineNum string, overlapContext string, userQuery string, maxRanges int, chunkStartLine int, chunkEndLine int) []RankedRange {
	dNonce := utils.RandStringBytes(4)
	minLines := 3
	maxLines := 20

	var overlapSection string
	if overlapContext != "" {
		overlapSection = fmt.Sprintf(`<|OVERLAP_CONTEXT_{{ .nonce }}|>
%s
<|OVERLAP_CONTEXT_END_{{ .nonce }}|>

`, overlapContext)
	}

	promptTemplate := `<|USER_QUERY_{{ .nonce }}|>
{{ .userQuery }}
<|USER_QUERY_END_{{ .nonce }}|>

` + overlapSection + `<|KNOWLEDGE_CHUNK_{{ .nonce }}|>
当前处理分片: 行 {{ .chunkStart }} - {{ .chunkEnd }}
{{ .samples }}
<|KNOWLEDGE_CHUNK_END_{{ .nonce }}|>

<|INSTRUCT_{{ .nonce }}|>
【智能知识筛选】请从当前分片中提取与用户问题最相关的知识片段。

【核心任务】
从上述带行号的知识内容中，提取与用户问题直接相关的片段。

【输出要求】
1. 最多提取 %d 个片段
2. 每个片段 %d-%d 行
3. 使用原始行号（第一列数字）
4. 按相关性排序（1最相关）

【评判标准】
- rank 1-3: 直接回答用户问题
- rank 4-7: 相关背景/技术细节
- rank 8+: 补充性信息

请输出 ranges 数组。
<|INSTRUCT_END_{{ .nonce }}|>
`

	materials, err := utils.RenderTemplate(fmt.Sprintf(promptTemplate, maxRanges, minLines, maxLines), map[string]any{
		"nonce":      dNonce,
		"samples":    chunkContentWithLineNum,
		"userQuery":  userQuery,
		"chunkStart": chunkStartLine,
		"chunkEnd":   chunkEndLine,
	})

	if err != nil {
		log.Errorf("compressKnowledgeChunk: template render failed: %v", err)
		return nil
	}

	forgeResult, err := aicommon.InvokeLiteForge(
		materials,
		aicommon.WithContext(ctx),
		aicommon.WithLiteForgeOutputSchemaFromAIToolOptions(
			aitool.WithStructArrayParam(
				"ranges",
				[]aitool.PropertyOption{
					aitool.WithParam_Description("按相关性排序的知识片段范围数组"),
				},
				nil,
				aitool.WithStringParam("range", aitool.WithParam_Description("原始行范围，格式: start-end")),
				aitool.WithIntegerParam("rank", aitool.WithParam_Description("相关性排序，1最相关")),
				aitool.WithStringParam("relevance_reason", aitool.WithParam_Description("相关性说明")),
			),
		),
	)

	if err != nil {
		log.Errorf("compressKnowledgeChunk: LiteForge failed: %v", err)
		return nil
	}

	if forgeResult == nil {
		return nil
	}

	rangeItems := forgeResult.GetInvokeParamsArray("ranges")
	var results []RankedRange

	for _, item := range rangeItems {
		rangeStr := item.GetString("range")
		rank := item.GetInt("rank")
		reason := item.GetString("relevance_reason")

		if rangeStr == "" {
			continue
		}

		parts := strings.Split(rangeStr, "-")
		if len(parts) != 2 {
			continue
		}

		startLine, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		endLine, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))

		if err1 != nil || err2 != nil || startLine <= 0 || endLine < startLine {
			continue
		}

		results = append(results, RankedRange{
			Range:     rangeStr,
			StartLine: startLine,
			EndLine:   endLine,
			Rank:      int(rank),
			Reason:    reason,
		})
	}

	return results
}

// compressKnowledgeResultsSingle 对较小的内容直接进行压缩（不分片）
func (r *ReAct) compressKnowledgeResultsSingle(ctx context.Context, knowledgeContent string, userQuery string, maxRanges int) string {
	resultEditor := memedit.NewMemEditor(knowledgeContent)
	dNonce := utils.RandStringBytes(4)

	minLines := 5
	maxLines := 30

	promptTemplate := `<|USER_QUERY_{{ .nonce }}|>
{{ .userQuery }}
<|USER_QUERY_END_{{ .nonce }}|>

<|KNOWLEDGE_RESULTS_{{ .nonce }}|>
{{ .samples }}
<|KNOWLEDGE_RESULTS_END_{{ .nonce }}|>

<|INSTRUCT_{{ .nonce }}|>
【智能知识筛选与排序】

请严格根据用户问题从上述知识搜索结果中提取最有价值的知识片段，按相关性排序：

【核心原则】
- 必须与用户问题直接相关
- 过滤掉所有无关的知识片段
- 优先选择能直接回答用户问题的知识
- 保留完整的知识条目，避免截断

【提取要求】
1. 最多提取 %d 个知识片段
2. 每个片段 %d-%d 行，确保上下文完整
3. 按相关性从高到低排序（rank: 1最相关，数字越大越不相关）
4. 严格过滤与用户问题无关的知识

【相关性评判标准】（按优先级排序）
🔥 最高相关 (rank 1-3)：
- 直接回答用户问题的知识
- 包含用户问题中提到的关键实体/概念
- 提供具体解决方案或操作步骤

⭐ 高度相关 (rank 4-7)：
- 与用户问题领域相关的知识
- 提供背景信息或相关概念解释
- 包含相关的技术细节或配置

📝 一般相关 (rank 8-15)：
- 可能对理解问题有帮助的知识
- 提供补充性信息
- 相关但不直接回答问题

【输出格式】
返回JSON数组，每个元素包含：
{
  "range": "start-end", 
  "rank": 数字(1-15),
  "relevance_reason": "与用户问题的相关性说明"
}

【严格要求】
- 总内容控制在合理范围内
- 避免重复或高度相似的知识片段
- 优先选择信息密度高的知识
- 确保每个片段都对回答用户问题有价值

请按相关性排序输出ranges数组。
<|INSTRUCT_END_{{ .nonce }}|>
`

	materials, err := utils.RenderTemplate(fmt.Sprintf(promptTemplate, maxRanges, minLines, maxLines), map[string]any{
		"nonce":     dNonce,
		"samples":   utils.PrefixLinesWithLineNumbers(knowledgeContent),
		"userQuery": userQuery,
	})

	if err != nil {
		log.Errorf("compressKnowledgeResultsSingle: template render failed: %v", err)
		return knowledgeContent
	}

	forgeResult, err := aicommon.InvokeLiteForge(
		materials,
		aicommon.WithContext(ctx),
		aicommon.WithLiteForgeOutputSchemaFromAIToolOptions(
			aitool.WithStructArrayParam(
				"ranges",
				[]aitool.PropertyOption{
					aitool.WithParam_Description("按相关性排序的知识片段范围数组"),
				},
				nil,
				aitool.WithStringParam("range", aitool.WithParam_Description("行范围，格式: start-end，例如 18-45")),
				aitool.WithIntegerParam("rank", aitool.WithParam_Description("相关性排序，1最相关，数字越大越不相关")),
				aitool.WithStringParam("relevance_reason", aitool.WithParam_Description("与用户问题的相关性说明")),
			),
		),
	)

	if err != nil {
		log.Errorf("compressKnowledgeResultsSingle: LiteForge failed: %v", err)
		return knowledgeContent
	}

	if forgeResult == nil {
		log.Warnf("compressKnowledgeResultsSingle: forge result is nil")
		return knowledgeContent
	}

	rangeItems := forgeResult.GetInvokeParamsArray("ranges")

	if len(rangeItems) == 0 {
		log.Warnf("compressKnowledgeResultsSingle: no ranges extracted")
		return knowledgeContent
	}

	var rankedRanges []RankedRange
	totalLines := 0
	maxTotalLines := 150

	for _, item := range rangeItems {
		rangeStr := item.GetString("range")
		rank := item.GetInt("rank")
		reason := item.GetString("relevance_reason")

		if rangeStr == "" {
			continue
		}

		parts := strings.Split(rangeStr, "-")
		if len(parts) != 2 {
			log.Warnf("compressKnowledgeResultsSingle: invalid range format: %s", rangeStr)
			continue
		}

		startLine, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		endLine, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))

		if err1 != nil || err2 != nil {
			log.Errorf("compressKnowledgeResultsSingle: parse range failed: %s, errors: %v, %v", rangeStr, err1, err2)
			continue
		}

		if startLine <= 0 || endLine < startLine {
			log.Warnf("compressKnowledgeResultsSingle: invalid range values: %s (start=%d, end=%d)", rangeStr, startLine, endLine)
			continue
		}

		text := resultEditor.GetTextFromPositionInt(startLine, 1, endLine, 1)
		if text == "" {
			log.Warnf("compressKnowledgeResultsSingle: empty text for range: %s", rangeStr)
			continue
		}

		lineCount := strings.Count(text, "\n") + 1
		if totalLines+lineCount > maxTotalLines {
			log.Warnf("compressKnowledgeResultsSingle: would exceed %d lines limit, stopping at range: %s", maxTotalLines, rangeStr)
			break
		}

		rankedRanges = append(rankedRanges, RankedRange{
			Range:     rangeStr,
			StartLine: startLine,
			EndLine:   endLine,
			Rank:      int(rank),
			Reason:    reason,
			Text:      text,
		})

		totalLines += lineCount
	}

	if len(rankedRanges) == 0 {
		log.Warnf("compressKnowledgeResultsSingle: no valid ranges extracted")
		return knowledgeContent
	}

	sort.Slice(rankedRanges, func(i, j int) bool {
		return rankedRanges[i].Rank < rankedRanges[j].Rank
	})

	var result strings.Builder
	result.WriteString("【AI 智能筛选】按相关性排序的知识片段：\n\n")

	for i, item := range rankedRanges {
		result.WriteString(fmt.Sprintf("=== [%d] 相关性排序: %d ===\n", i+1, item.Rank))
		if item.Reason != "" {
			result.WriteString(fmt.Sprintf("相关性说明: %s\n", item.Reason))
		}
		result.WriteString(item.Text)
		result.WriteString("\n\n")
	}

	finalResult := result.String()

	log.Infof("compressKnowledgeResultsSingle: compressed from %d chars to %d chars, %d ranges extracted",
		len(knowledgeContent), len(finalResult), len(rankedRanges))

	return finalResult
}
