package knowledgebench

import (
	"bytes"
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
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// BenchScoredRange mirrors aireact.ScoredRange for bench isolation.
type BenchScoredRange struct {
	StartLine int
	EndLine   int
	Score     float64
}

// BenchCompressLongText performs the same logic as ReAct.CompressLongTextWithDestination
// but with configurable parameters for benchmarking. Returns (result, aiCallCount, err).
func BenchCompressLongText(
	ctx context.Context,
	invoker aicommon.AIInvokeRuntime,
	rawText string,
	destination string,
	opts CompressOptions,
) (string, int, error) {
	opts = opts.Merge(DefaultCompressOptions())

	if rawText == "" {
		return "", 0, utils.Error("empty text")
	}

	targetTokenSize := opts.TargetTokenSize
	if int64(ytoken.CalcTokenCount(rawText)) < (targetTokenSize / 2) {
		return rawText, 0, nil
	}

	maxChunkSize := opts.MaxChunkSizeBytes
	maxChunks := opts.MaxChunks
	scoreThreshold := opts.ScoreThreshold

	editor := memedit.NewMemEditor(rawText)

	var mu sync.Mutex
	var allRanges []BenchScoredRange
	currentBlockSize := 0
	aiCallCount := 0

	const maxAlreadyExtractedSize = 8 * 1024
	var alreadyExtracted strings.Builder

	reducer, err := aireducer.NewReducerFromString(
		rawText,
		aireducer.WithContext(ctx),
		aireducer.WithEnableLineNumber(true),
		aireducer.WithChunkSize(int64(maxChunkSize)),
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			currentBlockSize++
			if currentBlockSize > maxChunks {
				return nil
			}

			dumped := chunk.DumpWithOverlap(128)

			mu.Lock()
			extracted := alreadyExtracted.String()
			mu.Unlock()

			ranges := benchCompressChunk(ctx, invoker, editor, dumped, destination, extracted, scoreThreshold)
			if len(ranges) > 0 {
				mu.Lock()
				aiCallCount++
				allRanges = append(allRanges, ranges...)
				for _, item := range ranges {
					if alreadyExtracted.Len() >= maxAlreadyExtractedSize {
						break
					}
					text := editor.GetTextFromPositionInt(item.StartLine, 1, item.EndLine+1, 1)
					if text != "" {
						remaining := maxAlreadyExtractedSize - alreadyExtracted.Len()
						if len(text) > remaining {
							text = text[:remaining]
						}
						alreadyExtracted.WriteString(fmt.Sprintf("--- [Score: %.2f, Line %d-%d] ---\n%s\n",
							item.Score, item.StartLine, item.EndLine, text))
					}
				}
				mu.Unlock()
			} else {
				mu.Lock()
				aiCallCount++
				mu.Unlock()
			}
			return nil
		}),
	)
	if err != nil {
		return utils.ShrinkTextBlock(rawText, int(targetTokenSize/2)), 0, nil
	}

	if err := reducer.Run(); err != nil {
		return utils.ShrinkTextBlock(rawText, int(targetTokenSize/2)), aiCallCount, nil
	}

	if len(allRanges) == 0 {
		return "", aiCallCount, nil
	}

	sort.Slice(allRanges, func(i, j int) bool {
		return allRanges[i].Score > allRanges[j].Score
	})
	allRanges = deduplicateRanges(allRanges)

	var result strings.Builder
	totalExtracted := 0
	for i, item := range allRanges {
		text := editor.GetTextFromPositionInt(item.StartLine, 1, item.EndLine+1, 1)
		if text == "" {
			continue
		}
		textTokens := ytoken.CalcTokenCount(text)
		if totalExtracted+textTokens > int(targetTokenSize) {
			result.WriteString(fmt.Sprintf("\n[... reached %d token limit, %d ranges remaining ...]\n",
				targetTokenSize, len(allRanges)-i))
			break
		}
		result.WriteString(fmt.Sprintf("=== [%d] Score: %.2f (line %d-%d) ===\n", i+1, item.Score, item.StartLine, item.EndLine))
		result.WriteString(text)
		result.WriteString("\n\n")
		totalExtracted += textTokens
	}

	return result.String(), aiCallCount, nil
}

func benchCompressChunk(
	ctx context.Context,
	invoker aicommon.AIInvokeRuntime,
	editor *memedit.MemEditor,
	chunkContent string,
	userQuery string,
	alreadyExtracted string,
	scoreThreshold float64,
) []BenchScoredRange {
	dNonce := utils.RandStringBytes(4)

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
Extract the most relevant knowledge snippets from this chunk for the user question.

Output up to 8 ranges, each 3-20 lines, with relevance score 0.00-1.00.
Only output ranges with score >= %.2f.

Output the ranges array.
<|INSTRUCT_END_{{ .nonce }}|>
`

	materials, err := utils.RenderTemplate(fmt.Sprintf(promptTemplate, scoreThreshold), map[string]any{
		"nonce":                   dNonce,
		"samples":                 chunkContent,
		"userQuery":               userQuery,
		"alreadyExtractedSection": alreadyExtractedSection,
	})
	if err != nil {
		log.Errorf("bench compress chunk template: %v", err)
		return nil
	}

	forgeResult, err := invoker.InvokeSpeedPriorityLiteForge(
		ctx,
		"knowledge-compress-bench",
		materials,
		[]aitool.ToolOption{
			aitool.WithStructArrayParam(
				"ranges",
				[]aitool.PropertyOption{
					aitool.WithParam_Description("knowledge snippet ranges"),
				},
				nil,
				aitool.WithStringParam("range", aitool.WithParam_Description("line range start-end")),
				aitool.WithNumberParam("score", aitool.WithParam_Description("relevance 0.0-1.0")),
			),
		},
		aicommon.WithGeneralConfigStreamableFieldEmitterCallback([]string{
			"ranges",
		}, func(key string, r io.Reader, emitter *aicommon.Emitter) {
			jsonextractor.ExtractStructuredJSONFromStream(r, jsonextractor.WithObjectCallback(func(data map[string]interface{}) {
				// streaming callback — results are also collected below via forgeResult
			}))
		}),
	)
	if err != nil {
		log.Errorf("bench compress LiteForge: %v", err)
		return nil
	}
	if forgeResult == nil {
		return nil
	}

	rangeItems := forgeResult.GetInvokeParamsArray("ranges")
	var results []BenchScoredRange
	for _, item := range rangeItems {
		rangeStr := item.GetString("range")
		score := item.GetFloat("score")
		if rangeStr == "" || score < scoreThreshold {
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
		results = append(results, BenchScoredRange{StartLine: startLine, EndLine: endLine, Score: score})
	}
	return results
}

func deduplicateRanges(ranges []BenchScoredRange) []BenchScoredRange {
	if len(ranges) <= 1 {
		return ranges
	}
	var result []BenchScoredRange
	for _, r := range ranges {
		overlaps := false
		for _, existing := range result {
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

// suppress unused import warnings
var _ = bytes.NewBuffer
