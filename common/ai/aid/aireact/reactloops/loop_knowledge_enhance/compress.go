package loop_knowledge_enhance

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// ScoredRange represents a line range with relevance score
type ScoredRange struct {
	Range     string
	StartLine int
	EndLine   int
	Score     float64 // 相关性评分，0.0-1.0，越高越相关
	Text      string
}

// deduplicateScoredRanges removes overlapping ranges, keeping higher scored ones
func deduplicateScoredRanges(ranges []ScoredRange) []ScoredRange {
	if len(ranges) <= 1 {
		return ranges
	}

	var result []ScoredRange
	for _, r := range ranges {
		overlaps := false
		for _, existing := range result {
			// Check for overlap
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

// compressKnowledgeResultsWithScore compresses knowledge content using AI with 0.0-1.0 scoring
// Reference: invoke_enhance_knowlege_answer.go
func compressKnowledgeResultsWithScore(
	resultStr string,
	userQuery string,
	invoker aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	maxBytes int,
) string {
	if len(resultStr) == 0 {
		return resultStr
	}

	// Skip compression for small content (< 3KB)
	if len(resultStr) < 3000 {
		log.Infof("compressKnowledgeResultsWithScore: content too short (%d chars), skip compression", len(resultStr))
		return resultStr
	}

	// Set default maxBytes
	if maxBytes <= 0 {
		maxBytes = 10 * 1024 // 10KB default
	}

	// For large content (>40KB), use chunked processing
	const maxChunkSize = 40 * 1024 // 40KB per chunk
	const overlapLines = 20        // 20 lines overlap
	const maxChunks = 10           // max 10 chunks

	ctx := invoker.GetConfig().GetContext()
	if loop != nil && loop.GetCurrentTask() != nil && !utils.IsNil(loop.GetCurrentTask().GetContext()) {
		ctx = loop.GetCurrentTask().GetContext()
	}

	if len(resultStr) > maxChunkSize {
		log.Infof("compressKnowledgeResultsWithScore: content too large (%d bytes), using chunked processing", len(resultStr))
		return compressKnowledgeResultsChunkedWithScore(ctx, resultStr, userQuery, invoker, loop, maxBytes, maxChunkSize, overlapLines, maxChunks)
	}

	// For smaller content, use single compression
	return compressKnowledgeResultsSingleWithScore(ctx, resultStr, userQuery, invoker, loop, maxBytes)
}

// compressKnowledgeResultsSingleWithScore handles compression for content < 40KB
func compressKnowledgeResultsSingleWithScore(
	ctx context.Context,
	knowledgeContent string,
	userQuery string,
	invoker aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	maxBytes int,
) string {
	resultEditor := memedit.NewMemEditor(knowledgeContent)
	dNonce := utils.RandStringBytes(4)

	minLines := 5
	maxLines := 30
	maxRanges := 15

	promptTemplate := `<|USER_QUERY_{{ .nonce }}|>
{{ .userQuery }}
<|USER_QUERY_END_{{ .nonce }}|>

<|KNOWLEDGE_RESULTS_{{ .nonce }}|>
{{ .samples }}
<|KNOWLEDGE_RESULTS_END_{{ .nonce }}|>

<|INSTRUCT_{{ .nonce }}|>
【智能知识筛选与排序】

请严格根据用户问题从上述知识搜索结果中提取最有价值的知识片段，按相关性评分排序：

【核心原则】
- 必须与用户问题直接相关
- 过滤掉所有无关的知识片段
- 优先选择能直接回答用户问题的知识
- 保留完整的知识条目，避免截断

【提取要求】
1. 最多提取 %d 个知识片段
2. 每个片段 %d-%d 行，确保上下文完整
3. 给出 0.0-1.0 的相关性评分（score），越高越相关
4. 严格过滤与用户问题无关的知识

【评分标准】
🔥 高度相关 (0.8-1.0)：
- 直接回答用户问题的知识
- 包含用户问题中提到的关键实体/概念
- 提供具体解决方案或操作步骤

⭐ 较高相关 (0.6-0.8)：
- 与用户问题领域相关的知识
- 提供背景信息或相关概念解释
- 包含相关的技术细节或配置

📝 一般相关 (0.4-0.6)：
- 可能对理解问题有帮助的知识
- 提供补充性信息
- 相关但不直接回答问题

❌ 弱相关 (0.0-0.4)：不输出

【输出格式】
返回JSON数组，每个元素包含：
{
  "range": "start-end", 
  "score": 0.0-1.0的小数
}

请按相关性评分从高到低输出ranges数组。
<|INSTRUCT_END_{{ .nonce }}|>
`

	materials, err := utils.RenderTemplate(fmt.Sprintf(promptTemplate, maxRanges, minLines, maxLines), map[string]any{
		"nonce":     dNonce,
		"samples":   utils.PrefixLinesWithLineNumbers(knowledgeContent),
		"userQuery": userQuery,
	})

	if err != nil {
		log.Errorf("compressKnowledgeResultsSingleWithScore: template render failed: %v", err)
		return knowledgeContent
	}

	// Create pipe for streaming output
	pr, pw := utils.NewPipe()

	// Get task index for emit
	var taskIndex string
	if loop != nil && loop.GetCurrentTask() != nil {
		taskIndex = loop.GetCurrentTask().GetIndex()
	}

	// Start streaming output with unified nodeId
	if loop != nil {
		loop.GetEmitter().EmitDefaultStreamEvent(
			"knowledge-compress",
			pr,
			taskIndex,
		)
	}

	forgeResult, err := invoker.InvokeLiteForge(
		ctx,
		"knowledge-compress",
		materials,
		[]aitool.ToolOption{
			aitool.WithStructArrayParam(
				"ranges",
				[]aitool.PropertyOption{
					aitool.WithParam_Description("按相关性评分排序的知识片段范围数组"),
				},
				nil,
				aitool.WithStringParam("range", aitool.WithParam_Description("行范围，格式: start-end，例如 18-45")),
				aitool.WithNumberParam("score", aitool.WithParam_Description("相关性评分，0.0-1.0，越高越相关")),
			),
		},
	)

	if err != nil {
		log.Errorf("compressKnowledgeResultsSingleWithScore: LiteForge failed: %v", err)
		pw.Close()
		return knowledgeContent
	}

	if forgeResult == nil {
		log.Warnf("compressKnowledgeResultsSingleWithScore: forge result is nil")
		pw.Close()
		return knowledgeContent
	}

	rangeItems := forgeResult.GetInvokeParamsArray("ranges")

	if len(rangeItems) == 0 {
		log.Warnf("compressKnowledgeResultsSingleWithScore: no ranges extracted")
		pw.Close()
		return knowledgeContent
	}

	var scoredRanges []ScoredRange

	for _, item := range rangeItems {
		rangeStr := item.GetString("range")
		score := item.GetFloat("score")

		if rangeStr == "" {
			continue
		}

		// Filter out low score items (< 0.4)
		if score < 0.4 {
			continue
		}

		parts := strings.Split(rangeStr, "-")
		if len(parts) != 2 {
			log.Warnf("compressKnowledgeResultsSingleWithScore: invalid range format: %s", rangeStr)
			continue
		}

		startLine, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		endLine, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))

		if err1 != nil || err2 != nil {
			log.Errorf("compressKnowledgeResultsSingleWithScore: parse range failed: %s, errors: %v, %v", rangeStr, err1, err2)
			continue
		}

		if startLine <= 0 || endLine < startLine {
			log.Warnf("compressKnowledgeResultsSingleWithScore: invalid range values: %s (start=%d, end=%d)", rangeStr, startLine, endLine)
			continue
		}

		text := resultEditor.GetTextFromPositionInt(startLine, 1, endLine, 1)
		if text == "" {
			log.Warnf("compressKnowledgeResultsSingleWithScore: empty text for range: %s", rangeStr)
			continue
		}

		// Write to stream: 片段：[Score: 0.x] startLine-endLine
		pw.WriteString(fmt.Sprintf("片段：[Score: %.2f] %d-%d\n", score, startLine, endLine))

		scoredRanges = append(scoredRanges, ScoredRange{
			Range:     rangeStr,
			StartLine: startLine,
			EndLine:   endLine,
			Score:     score,
			Text:      text,
		})
	}

	pw.Close()

	if len(scoredRanges) == 0 {
		log.Warnf("compressKnowledgeResultsSingleWithScore: no valid ranges extracted")
		return knowledgeContent
	}

	// Sort by score descending (higher score = more relevant)
	sort.Slice(scoredRanges, func(i, j int) bool {
		return scoredRanges[i].Score > scoredRanges[j].Score
	})

	var result strings.Builder
	result.WriteString("【AI 智能筛选】按相关性评分排序的知识片段：\n\n")

	currentBytes := 0
	for i, item := range scoredRanges {
		if currentBytes+len(item.Text) > maxBytes {
			log.Infof("compressKnowledgeResultsSingleWithScore: reached %d bytes limit, stopping at %d ranges", maxBytes, i)
			break
		}
		result.WriteString(fmt.Sprintf("=== [%d] Score: %.2f ===\n", i+1, item.Score))
		result.WriteString(item.Text)
		result.WriteString("\n\n")
		currentBytes += len(item.Text)
	}

	finalResult := result.String()

	log.Infof("compressKnowledgeResultsSingleWithScore: compressed from %d chars to %d chars (%d bytes), %d ranges extracted",
		len(knowledgeContent), len(finalResult), currentBytes, len(scoredRanges))

	return finalResult
}

// compressKnowledgeResultsChunkedWithScore handles compression for content > 40KB using chunked processing
func compressKnowledgeResultsChunkedWithScore(
	ctx context.Context,
	knowledgeContent string,
	userQuery string,
	invoker aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	maxBytes int,
	chunkSize int,
	overlapLines int,
	maxChunks int,
) string {
	// Step 1: Split by lines
	originalLines := strings.Split(knowledgeContent, "\n")
	totalLines := len(originalLines)

	log.Infof("compressKnowledgeResultsChunkedWithScore: processing %d bytes, %d lines, chunkSize=%d, overlapLines=%d, maxChunks=%d",
		len(knowledgeContent), totalLines, chunkSize, overlapLines, maxChunks)

	// Step 2: Calculate lines per chunk
	avgLineLen := len(knowledgeContent)/totalLines + 10
	linesPerChunk := chunkSize / avgLineLen
	if linesPerChunk < 50 {
		linesPerChunk = 50
	}

	// Adjust to ensure not exceeding maxChunks
	effectiveLinesPerChunk := linesPerChunk - overlapLines
	if effectiveLinesPerChunk <= 0 {
		effectiveLinesPerChunk = linesPerChunk / 2
	}
	estimatedChunks := (totalLines + effectiveLinesPerChunk - 1) / effectiveLinesPerChunk
	if estimatedChunks > maxChunks {
		effectiveLinesPerChunk = (totalLines + maxChunks - 1) / maxChunks
		linesPerChunk = effectiveLinesPerChunk + overlapLines
		log.Infof("compressKnowledgeResultsChunkedWithScore: adjusted linesPerChunk to %d to limit chunks to %d", linesPerChunk, maxChunks)
	}

	// Step 3: Process chunks
	var allScoredRanges []ScoredRange

	chunkIndex := 0
	for startLineIdx := 0; startLineIdx < totalLines && chunkIndex < maxChunks; chunkIndex++ {
		startLine := startLineIdx + 1
		endLineIdx := startLineIdx + linesPerChunk
		if endLineIdx > totalLines {
			endLineIdx = totalLines
		}
		endLine := endLineIdx

		chunkLines := originalLines[startLineIdx:endLineIdx]

		// Build chunk content with line numbers
		var chunkBuilder strings.Builder
		for i, line := range chunkLines {
			lineNum := startLineIdx + i + 1
			chunkBuilder.WriteString(fmt.Sprintf("%d | %s\n", lineNum, line))
		}
		chunkContent := chunkBuilder.String()

		// Add overlap context
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

		// Process chunk
		chunkRanges := compressKnowledgeChunkWithScore(ctx, chunkWithOverlap, userQuery, invoker, loop, startLine, endLine)

		if len(chunkRanges) > 0 {
			allScoredRanges = append(allScoredRanges, chunkRanges...)
			log.Infof("compressKnowledgeResultsChunkedWithScore: chunk %d extracted %d ranges", chunkIndex+1, len(chunkRanges))
		}

		// Move to next chunk
		startLineIdx = endLineIdx - overlapLines
		if startLineIdx < 0 {
			startLineIdx = 0
		}
		if startLineIdx <= (endLineIdx - linesPerChunk) {
			startLineIdx = endLineIdx
		}
	}

	log.Infof("compressKnowledgeResultsChunkedWithScore: processed %d chunks total", chunkIndex)

	if len(allScoredRanges) == 0 {
		log.Warnf("compressKnowledgeResultsChunkedWithScore: no valid ranges extracted from any chunk")
		if len(knowledgeContent) > 50000 {
			return knowledgeContent[:50000] + "\n\n[... 内容过长，已截断 ...]"
		}
		return knowledgeContent
	}

	// Sort by score descending
	sort.Slice(allScoredRanges, func(i, j int) bool {
		return allScoredRanges[i].Score > allScoredRanges[j].Score
	})

	// Deduplicate
	allScoredRanges = deduplicateScoredRanges(allScoredRanges)

	// Extract final results
	resultEditor := memedit.NewMemEditor(knowledgeContent)
	var result strings.Builder
	result.WriteString(fmt.Sprintf("【AI 智能筛选】从 %d 字节内容中提取的 %d 个最相关知识片段：\n\n", len(knowledgeContent), len(allScoredRanges)))

	totalExtractedBytes := 0

	for i, item := range allScoredRanges {
		text := resultEditor.GetTextFromPositionInt(item.StartLine, 1, item.EndLine, 1)
		if text == "" {
			continue
		}

		textBytes := len(text)
		if totalExtractedBytes+textBytes > maxBytes {
			result.WriteString(fmt.Sprintf("\n[... 已达到 %d 字节限制，剩余 %d 个片段未展示 ...]\n", maxBytes, len(allScoredRanges)-i))
			break
		}

		result.WriteString(fmt.Sprintf("=== [%d] Score: %.2f (行 %d-%d) ===\n", i+1, item.Score, item.StartLine, item.EndLine))
		result.WriteString(text)
		result.WriteString("\n\n")

		totalExtractedBytes += textBytes
	}

	finalResult := result.String()

	log.Infof("compressKnowledgeResultsChunkedWithScore: compressed from %d chars to %d chars (%d bytes), %d ranges",
		len(knowledgeContent), len(finalResult), totalExtractedBytes, len(allScoredRanges))

	return finalResult
}

// compressKnowledgeChunkWithScore processes a single chunk for AI filtering
func compressKnowledgeChunkWithScore(
	ctx context.Context,
	chunkContentWithLineNum string,
	userQuery string,
	invoker aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	chunkStartLine int,
	chunkEndLine int,
) []ScoredRange {
	dNonce := utils.RandStringBytes(4)
	minLines := 3
	maxLines := 20
	maxRanges := 8

	promptTemplate := `<|USER_QUERY_{{ .nonce }}|>
{{ .userQuery }}
<|USER_QUERY_END_{{ .nonce }}|>

<|KNOWLEDGE_CHUNK_{{ .nonce }}|>
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
4. 给出 0.0-1.0 的相关性评分（score），越高越相关

【评分标准】
- 0.8-1.0: 直接回答用户问题的核心内容
- 0.6-0.8: 相关背景/技术细节
- 0.4-0.6: 补充性信息
- 0.0-0.4: 弱相关或无关内容（不输出）

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
		log.Errorf("compressKnowledgeChunkWithScore: template render failed: %v", err)
		return nil
	}

	// Create pipe for streaming output
	pr, pw := utils.NewPipe()

	// Get task index for emit
	var taskIndex string
	if loop != nil && loop.GetCurrentTask() != nil {
		taskIndex = loop.GetCurrentTask().GetIndex()
	}

	// Start streaming output with unified nodeId
	if loop != nil {
		loop.GetEmitter().EmitDefaultStreamEvent(
			"knowledge-compress",
			pr,
			taskIndex,
		)
	}

	forgeResult, err := invoker.InvokeLiteForge(
		ctx,
		"knowledge-compress",
		materials,
		[]aitool.ToolOption{
			aitool.WithStructArrayParam(
				"ranges",
				[]aitool.PropertyOption{
					aitool.WithParam_Description("按相关性评分排序的知识片段范围数组"),
				},
				nil,
				aitool.WithStringParam("range", aitool.WithParam_Description("原始行范围，格式: start-end")),
				aitool.WithNumberParam("score", aitool.WithParam_Description("相关性评分，0.0-1.0，越高越相关")),
			),
		},
	)

	if err != nil {
		log.Errorf("compressKnowledgeChunkWithScore: LiteForge failed: %v", err)
		pw.Close()
		return nil
	}

	if forgeResult == nil {
		pw.Close()
		return nil
	}

	rangeItems := forgeResult.GetInvokeParamsArray("ranges")
	var results []ScoredRange

	for _, item := range rangeItems {
		rangeStr := item.GetString("range")
		score := item.GetFloat("score")

		if rangeStr == "" {
			continue
		}

		// Filter out low score items (< 0.4)
		if score < 0.4 {
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

		// Write to stream
		pw.WriteString(fmt.Sprintf("片段：[Score: %.2f] %d-%d\n", score, startLine, endLine))

		results = append(results, ScoredRange{
			Range:     rangeStr,
			StartLine: startLine,
			EndLine:   endLine,
			Score:     score,
		})
	}

	pw.Close()
	return results
}
