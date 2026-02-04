package aireact

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/jsonextractor"
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

func (r *ReAct) CompressLongTextWithDestination(
	ctx context.Context,
	i any,
	destination string,
	targetByteSize int64,
) (string, error) {
	if targetByteSize <= 1024 {
		targetByteSize = 10 * 1024
	}
	var rawText string
	switch ret := i.(type) {
	case io.Reader:
		rawTextBytes, _ := io.ReadAll(ret)
		rawText = string(rawTextBytes)
	default:
		rawText = utils.InterfaceToString(ret)
	}

	if rawText == "" {
		return "", utils.Error("cannot compress empty text")
	}

	if int64(len(rawText)) < (targetByteSize / 2) {
		return rawText, nil
	}

	var emergencyLimit = targetByteSize / 2
	fallbackResult := utils.ShrinkTextBlock(rawText, int(emergencyLimit))

	// For large content (>30KB), use chunked processing
	const maxChunkSize = 30 * 1024 // 40KB per chunk
	const overlapLines = 20        // 20 lines overlap
	const maxChunks = 20           // max 10 chunks

	editor := memedit.NewMemEditor(rawText)

	var mu sync.Mutex
	var allScoredRanges []ScoredRange
	isOversize := false
	_ = isOversize
	currentBlockSize := 0

	// alreadyExtractedContent stores content that has been extracted, max 8KB
	const maxAlreadyExtractedSize = 8 * 1024
	var alreadyExtractedContent strings.Builder

	reducer, err := aireducer.NewReducerFromString(
		rawText,
		aireducer.WithContext(ctx),
		aireducer.WithEnableLineNumber(true),         // 自动添加行号，格式：N | content
		aireducer.WithChunkSize(int64(maxChunkSize)), // 块大小硬限制
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			currentBlockSize++
			if currentBlockSize > maxChunks {
				isOversize = true
				return nil // 超出最大块数，跳过
			}

			dumpped := chunk.DumpWithOverlap(128)

			utils.Debug(func() {
				fmt.Println("--------------------------------chunk--------------------------------")
				fmt.Println(dumpped)
				fmt.Println("--------------------------------chunk--------------------------------")
			})

			// Get already extracted content for deduplication hint
			mu.Lock()
			alreadyExtracted := alreadyExtractedContent.String()
			mu.Unlock()

			// 对当前 chunk 进行 AI 筛选
			chunkRanges := compressKnowledgeChunkWithScore(editor, ctx, dumpped, destination, r, alreadyExtracted)
			if len(chunkRanges) > 0 {
				mu.Lock()
				allScoredRanges = append(allScoredRanges, chunkRanges...)
				// Update already extracted content (max 8KB)
				for _, item := range chunkRanges {
					if alreadyExtractedContent.Len() >= maxAlreadyExtractedSize {
						break
					}
					text := editor.GetTextFromPositionInt(item.StartLine, 1, item.EndLine+1, 1)
					if text != "" {
						remaining := maxAlreadyExtractedSize - alreadyExtractedContent.Len()
						if len(text) > remaining {
							text = text[:remaining]
						}
						alreadyExtractedContent.WriteString(fmt.Sprintf("--- [Score: %.2f, Line %d-%d] ---\n%s\n", item.Score, item.StartLine, item.EndLine, text))
					}
				}
				mu.Unlock()
			}
			return nil
		}),
	)
	if err != nil {
		log.Errorf("CompressLongTextWithDestination: failed to create reducer: %v", err)
		return fallbackResult, nil
	}

	if err := reducer.Run(); err != nil {
		log.Errorf("CompressLongTextWithDestination: reduce failed: %v", err)
		return fallbackResult, nil
	}

	if len(allScoredRanges) == 0 {
		// no result
		return "", nil
	}

	sort.Slice(allScoredRanges, func(i, j int) bool {
		return allScoredRanges[i].Score > allScoredRanges[j].Score
	})
	// Deduplicate
	allScoredRanges = deduplicateScoredRanges(allScoredRanges)
	var result strings.Builder
	result.WriteString(fmt.Sprintf("【AI 智能筛选】从 %d 字节内容中提取的 %d 个最相关知识片段：\n\n", len(rawText), len(allScoredRanges)))

	totalExtractedBytes := 0

	for i, item := range allScoredRanges {
		text := editor.GetTextFromPositionInt(item.StartLine, 1, item.EndLine+1, 1)
		if text == "" {
			continue
		}

		textBytes := len(text)
		if totalExtractedBytes+textBytes > int(targetByteSize) {
			result.WriteString(fmt.Sprintf("\n[... 已达到 %d 字节限制，剩余 %d 个片段未展示 ...]\n", targetByteSize, len(allScoredRanges)-i))
			break
		}

		result.WriteString(fmt.Sprintf("=== [%d] Score: %.2f (行 %d-%d) ===\n", i+1, item.Score, item.StartLine, item.EndLine))
		result.WriteString(text)
		result.WriteString("\n\n")

		totalExtractedBytes += textBytes
	}

	finalResult := result.String()
	return finalResult, nil
}

// compressKnowledgeChunkWithScore processes a single chunk for AI filtering
// alreadyExtracted contains previously extracted content (max 8KB) to avoid duplicate extraction
func compressKnowledgeChunkWithScore(
	editor *memedit.MemEditor,
	ctx context.Context,
	chunkContentWithLineNum string,
	userQuery string,
	invoker aicommon.AIInvokeRuntime,
	alreadyExtracted string,
) []ScoredRange {
	dNonce := utils.RandStringBytes(4)
	minLines := 3
	maxLines := 20
	maxRanges := 8

	// Build already extracted section if available
	alreadyExtractedSection := ""
	if alreadyExtracted != "" {
		alreadyExtractedSection = fmt.Sprintf(`<|ALREADY_EXTRACTED_%s|>
%s
<|ALREADY_EXTRACTED_END_%s|>

`, dNonce, alreadyExtracted, dNonce)
	}

	promptTemplate := `<|USER_QUERY_{{ .nonce }}|>
{{ .userQuery }}
<|USER_QUERY_END_{{ .nonce }}|>

{{ .alreadyExtractedSection }}<|KNOWLEDGE_CHUNK_{{ .nonce }}|>
{{ .samples }}
<|KNOWLEDGE_CHUNK_END_{{ .nonce }}|>

<|INSTRUCT_{{ .nonce }}|>
【智能知识筛选】请从当前分片中提取与用户问题最相关的知识片段。

【核心任务】
从上述带行号的知识内容中，提取与用户问题直接相关的片段。
{{ if .hasAlreadyExtracted }}
【重要：去重要求】
ALREADY_EXTRACTED 部分包含了之前已经提取过的内容。请注意：
- 如果当前分片中的内容与已提取内容完全相同或高度重复，应大幅降低评分或不提取
- 如果内容仅部分重复但有新的补充信息，可以适当降低评分后提取
- 优先提取与已提取内容不重复的新信息
{{ end }}
【输出要求】
1. 最多提取 %d 个片段
2. 每个片段 %d-%d 行
3. 使用原始行号（第一列数字）
4. 给出 0.00-1.00 的相关性评分（score），越高越相关

【评分标准】
- 0.80-1.00: 直接回答用户问题的核心内容（且未被提取过）
- 0.60-0.80: 相关背景/技术细节（且未被提取过）
- 0.40-0.60: 补充性信息（或与已提取内容部分重复但有新信息）
- 0.00-0.40: 弱相关、无关内容、或与已提取内容完全重复（不输出）

尽量使用，精确到小数点后两位来表示

请输出 ranges 数组。
<|INSTRUCT_END_{{ .nonce }}|>
`

	materials, err := utils.RenderTemplate(fmt.Sprintf(promptTemplate, maxRanges, minLines, maxLines), map[string]any{
		"nonce":                   dNonce,
		"samples":                 chunkContentWithLineNum,
		"userQuery":               userQuery,
		"alreadyExtractedSection": alreadyExtractedSection,
		"hasAlreadyExtracted":     alreadyExtracted != "",
	})

	if err != nil {
		log.Errorf("compressKnowledgeChunkWithScore: template render failed: %v", err)
		return nil
	}

	// Get task index for emit
	var taskIndex string
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
		aicommon.WithGeneralConfigStreamableFieldCallback([]string{
			"ranges",
		}, func(key string, r io.Reader) {
			jsonextractor.ExtractStructuredJSONFromStream(r, jsonextractor.WithObjectCallback(func(data map[string]interface{}) {
				score := 0.0
				score = utils.MapGetFloat64(data, "score")
				if score < 0.4 {
					return
				}
				rangeStr := utils.MapGetString(data, "range")
				if rangeStr == "" {
					return
				}
				parts := strings.Split(rangeStr, "-")
				if len(parts) != 2 {
					return
				}
				// utils.Int
				startLine := utils.InterfaceToInt(strings.TrimSpace(parts[0]))
				endLine := utils.InterfaceToInt(strings.TrimSpace(parts[1]))
				if startLine <= 0 || endLine < startLine {
					return
				}
				pr, pw := utils.NewPipe()
				text := editor.GetTextFromPositionInt(startLine, 1, endLine, 1)
				pw.WriteString(fmt.Sprintf("[权重：%v] 片段范围: %v-%v(切片大小:%v)；", score, startLine, endLine, utils.ByteSize(uint64(len(text)))))
				// Start streaming output with unified nodeId
				if invoker != nil {
					emitter := invoker.GetConfig().GetEmitter()
					if event, _ := emitter.EmitDefaultStreamEvent(
						"knowledge-compress",
						pr,
						taskIndex,
					); event != nil {
						streamId := event.GetStreamEventWriterId()
						emitter.EmitTextReferenceMaterial(streamId, text)
					}
				}
				pw.Close()
			}))
		}),
	)

	if err != nil {
		log.Errorf("compressKnowledgeChunkWithScore: LiteForge failed: %v", err)
		return nil
	}

	if forgeResult == nil {
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
		results = append(results, ScoredRange{
			Range:     rangeStr,
			StartLine: startLine,
			EndLine:   endLine,
			Score:     score,
		})
	}
	return results
}
