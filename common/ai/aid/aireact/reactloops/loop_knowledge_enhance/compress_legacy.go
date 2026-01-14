package loop_knowledge_enhance

import (
	"fmt"
	"os"
	"path/filepath"
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

// compressKnowledgeResults compresses and refines knowledge search results using AI, filtering by text coordinates
// Deprecated: Use compressKnowledgeResultsWithScore for better scoring-based compression
func compressKnowledgeResults(resultStr string, queries []string, userContext string, invoker aicommon.AIInvokeRuntime, op *reactloops.LoopActionHandlerOperator, loop *reactloops.ReActLoop) string {
	if len(resultStr) == 0 {
		return resultStr
	}

	resultEditor := memedit.NewMemEditor(resultStr)
	dNonce := utils.RandStringBytes(4)

	promptTemplate := `
{{ if .userContext }}<|USER_CONTEXT_{{ .nonce }}|>
{{ .userContext }}
<|USER_CONTEXT_END_{{ .nonce }}|>

{{ end }}<|KNOWLEDGE_RESULTS_{{ .nonce }}|>
{{ .samples }}
<|KNOWLEDGE_RESULTS_END_{{ .nonce }}|>

<|INSTRUCT_{{ .nonce }}|>
【智能知识内容提取与精炼】

请严格根据用户查询从知识库搜索结果中提取最有价值的知识条目，按相关性和重要性排序：

【核心原则】
{{ if .userContext }}- 必须与用户需求直接相关
- 过滤掉所有无关的知识条目
- 优先选择能直接回答用户问题的知识
{{ else }}- 提取最具代表性和价值的知识内容
- 按主题相关性排序
- 去除重复和冗余信息
{{ end }}
【提取要求】
1. 最多提取 8 个最相关的知识条目
2. 每个条目应包含完整的上下文和关键信息
3. 按相关性从高到低排序（rank: 1最相关，数字越大越不相关）
4. 严格过滤无关内容

【重要性评判标准】（按优先级排序）
🔥 最高优先级 (rank 1-2)：
- 直接回答用户查询的核心知识
- 包含关键概念定义和解释
- 展示最佳实践和解决方案

⭐ 高优先级 (rank 3-5)：
- 包含重要补充信息和细节
- 相关示例和应用场景
- 重要的技术规范和要求

📝 中等优先级 (rank 6-8)：
- 辅助性信息和背景知识
- 相关术语解释和概念澄清
- 补充性的技术细节

【输出格式】
返回JSON数组，每个元素包含：
{
  "range": "startLine-endLine",
  "rank": 数字(1-8),
  "reason": "选择理由，例如：包含xxx核心概念"
}

【严格要求】
- 总输出控制在60行以内
- 避免重复或相似的知识条目
- 确保每个条目都有实际参考价值
{{ if .userContext }}- 必须与用户需求相关，无关内容一律排除{{ end }}

请按相关性排序输出ranges数组。
<|INSTRUCT_END_{{ .nonce }}|>
`

	materials, err := utils.RenderTemplate(promptTemplate, map[string]any{
		"nonce":       dNonce,
		"samples":     utils.PrefixLinesWithLineNumbers(resultStr),
		"queries":     strings.Join(queries, ", "),
		"userContext": userContext,
	})

	if err != nil {
		log.Errorf("compressKnowledgeResults: template render failed: %v", err)
		return resultStr
	}

	var context = invoker.GetConfig().GetContext()
	if op != nil {
		context = op.GetTask().GetContext()
	}

	forgeResult, err := invoker.InvokeLiteForge(
		context,
		"extract-ranked-knowledge",
		materials,
		[]aitool.ToolOption{
			aitool.WithStructArrayParam(
				"ranges",
				[]aitool.PropertyOption{
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("要提取的知识条目范围"),
				},
				nil,
				aitool.WithStringParam("range", aitool.WithParam_Required(true), aitool.WithParam_Description("行数范围，格式：startLine-endLine")),
				aitool.WithIntegerParam("rank", aitool.WithParam_Description("相关性排名，1-8，1最相关")),
				aitool.WithStringParam("reason", aitool.WithParam_Description("选择该条目的理由")),
			),
		},
	)

	if err != nil {
		log.Errorf("compressKnowledgeResults: forge failed: %v", err)
		return resultStr
	}

	if forgeResult == nil {
		log.Warnf("compressKnowledgeResults: forge result is nil")
		return resultStr
	}

	// 解析提取的ranges
	ranges := forgeResult.GetInvokeParamsArray("ranges")
	if len(ranges) == 0 {
		log.Warnf("compressKnowledgeResults: no ranges extracted")
		return resultStr
	}

	type RankedRange struct {
		StartLine int
		EndLine   int
		Rank      int
		Reason    string
	}

	var rankedRanges []RankedRange
	var totalLines int

	for _, r := range ranges {
		rangeStr := r.GetString("range")
		rank := r.GetInt("rank")
		reason := r.GetString("reason")

		if rangeStr == "" {
			log.Warnf("compressKnowledgeResults: empty range")
			continue
		}

		// 解析范围字符串
		parts := strings.Split(rangeStr, "-")
		if len(parts) != 2 {
			log.Warnf("compressKnowledgeResults: invalid range format: %s", rangeStr)
			continue
		}

		startLine, err1 := strconv.Atoi(parts[0])
		endLine, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			log.Errorf("compressKnowledgeResults: parse range failed: %s, errors: %v, %v", rangeStr, err1, err2)
			continue
		}

		if startLine < 1 || endLine < startLine {
			log.Warnf("compressKnowledgeResults: invalid range values: %s (start=%d, end=%d)", rangeStr, startLine, endLine)
			continue
		}

		// 检查是否有实际内容
		text := resultEditor.GetTextFromPositionInt(startLine, 1, endLine, 1)
		if strings.TrimSpace(text) == "" {
			log.Warnf("compressKnowledgeResults: empty text for range: %s", rangeStr)
			continue
		}

		// 控制总行数不超过100行
		lineCount := endLine - startLine + 1
		if totalLines+lineCount > 100 {
			log.Warnf("compressKnowledgeResults: would exceed 100 lines limit, stopping at range: %s", rangeStr)
			break
		}

		rankedRanges = append(rankedRanges, RankedRange{
			StartLine: startLine,
			EndLine:   endLine,
			Rank:      int(rank),
			Reason:    reason,
		})
		totalLines += lineCount
	}

	if len(rankedRanges) == 0 {
		log.Warnf("compressKnowledgeResults: no valid ranges extracted")
		return resultStr
	}

	// 按rank排序
	sort.Slice(rankedRanges, func(i, j int) bool {
		return rankedRanges[i].Rank < rankedRanges[j].Rank
	})

	// 构建压缩后的结果
	var finalResult strings.Builder
	finalResult.WriteString("【AI智能提取】按相关性排序的知识条目：\n\n")

	emitter := loop.GetEmitter()

	for i, r := range rankedRanges {
		text := resultEditor.GetTextFromPositionInt(r.StartLine, 1, r.EndLine, 1)
		finalResult.WriteString(fmt.Sprintf("=== [相关性排名 #%d] ===\n", i+1))
		finalResult.WriteString(fmt.Sprintf("选择理由：%s\n", r.Reason))
		finalResult.WriteString("内容：\n")
		finalResult.WriteString(text)
		finalResult.WriteString("\n\n")

		// 为重要知识条目创建 artifacts
		if r.Rank <= 3 {
			iteration := loop.GetCurrentIterationIndex()
			if iteration <= 0 {
				iteration = 1
			}
			loopDir := loop.Get("loop_directory")
			if loopDir == "" {
				filename := invoker.EmitFileArtifactWithExt(fmt.Sprintf("key_knowledge_rank_%d_iter_%d", r.Rank, iteration), ".txt", "")
				emitter.EmitPinFilename(filename)

				// 写入文件内容，包含元信息
				artifactContent := fmt.Sprintf("迭代轮数：%d\n相关性排名：#%d\n选择理由：%s\n\n内容：\n%s", iteration, r.Rank, r.Reason, text)
				err := os.WriteFile(filename, []byte(artifactContent), 0644)
				if err != nil {
					log.Warnf("failed to write key knowledge artifact rank %d: %v", r.Rank, err)
				}
			} else {
				artifactDir := filepath.Join(loopDir, "key_knowledge", fmt.Sprintf("iter_%d", iteration))
				if err := os.MkdirAll(artifactDir, 0755); err != nil {
					log.Warnf("failed to create key knowledge directory: %v", err)
				}
				filename := filepath.Join(artifactDir, fmt.Sprintf("key_knowledge_rank_%d_iter_%d_%s.txt", r.Rank, iteration, utils.DatetimePretty2()))
				emitter.EmitPinFilename(filename)

				// 写入文件内容，包含元信息
				artifactContent := fmt.Sprintf("迭代轮数：%d\n相关性排名：#%d\n选择理由：%s\n\n内容：\n%s", iteration, r.Rank, r.Reason, text)
				err := os.WriteFile(filename, []byte(artifactContent), 0644)
				if err != nil {
					log.Warnf("failed to write key knowledge artifact rank %d: %v", r.Rank, err)
				}
			}
		}
	}

	// 如果有创建的artifacts，在结果中提及
	if len(rankedRanges) > 0 && rankedRanges[0].Rank <= 3 {
		finalResult.WriteString("📌 重要知识条目已保存到 artifacts 中，可供后续详细查看。\n")
	}

	log.Infof("compressKnowledgeResults: compressed from %d chars to %d chars, %d ranges",
		len(resultStr), len(finalResult.String()), len(rankedRanges))

	return finalResult.String()
}
