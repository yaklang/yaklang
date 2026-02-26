package loop_internet_research

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const maxSearchResults = 8
const defaultSearchPageSize = 5

func makeWebSearchAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	desc := "搜索互联网获取与查询相关的信息。每次搜索提交一个查询条件，系统会返回搜索结果并自动提取页面内容。"

	toolOpts := []aitool.ToolOption{
		aitool.WithStringParam("search_query",
			aitool.WithParam_Description("搜索查询关键词，用于互联网搜索"),
			aitool.WithParam_Required(true)),
		aitool.WithIntegerParam("max_results",
			aitool.WithParam_Description("最大搜索结果数，默认 5")),
	}

	return reactloops.WithRegisterLoopAction(
		"web_search",
		desc, toolOpts,
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			loop.LoadingStatus("validating web search parameters")
			searchQuery := action.GetString("search_query")
			if searchQuery == "" {
				return utils.Error("search_query is required for web search")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			feedNewKnowledge := func(knowledge string, query string) {
				var finalKnowledge string
				oldKnowledges := loop.Get("search_results_summary")
				if oldKnowledges == "" {
					finalKnowledge = knowledge
				} else {
					newKnowledge := oldKnowledges + "\n\n" + knowledge
					ctx := loop.GetCurrentTask().GetContext()
					compressed, err := loop.GetInvoker().CompressLongTextWithDestination(ctx, newKnowledge, query, 10*1024)
					if err != nil {
						log.Warnf("failed to compress accumulated knowledge: %v", err)
						finalKnowledge = newKnowledge
					} else {
						finalKnowledge = compressed
					}
				}
				loop.Set("search_results_summary", finalKnowledge)
			}

			searchQuery := action.GetString("search_query")
			maxResults := action.GetInt("max_results")
			if maxResults <= 0 {
				maxResults = defaultSearchPageSize
			}
			if maxResults > maxSearchResults {
				maxResults = maxSearchResults
			}

			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}

			userQuery := loop.Get("user_query")
			emitter := loop.GetEmitter()

			loop.LoadingStatus(fmt.Sprintf("searching internet: %s", searchQuery))
			log.Infof("internet research: searching for '%s' with max_results=%d", searchQuery, maxResults)

			params := aitool.InvokeParams{
				"query":       searchQuery,
				"max_results": maxResults,
			}
			toolResult, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, "web_search", params)
			if err != nil {
				log.Warnf("web_search tool call failed for '%s': %v", searchQuery, err)
				op.Feedback(fmt.Sprintf("search failed: %v\nplease try a different query.", err))
				op.Continue()
				return
			}

			content := ""
			if toolResult != nil {
				content = utils.InterfaceToString(toolResult.Data)
			}
			if content == "" {
				op.Feedback(fmt.Sprintf("search for '%s' returned no useful content. try a different query.", searchQuery))
				op.Continue()
				return
			}

			log.Infof("internet research: web_search tool returned %d bytes for '%s'", len(content), searchQuery)

			loop.LoadingStatus("recording search results")
			feedNewKnowledge(content, userQuery)

			iteration := loop.GetCurrentIterationIndex()
			if iteration <= 0 {
				iteration = 1
			}

			searchCountStr := loop.Get("search_count")
			searchCount := 1
			if searchCountStr != "" {
				if c, err := strconv.Atoi(searchCountStr); err == nil {
					searchCount = c + 1
				}
			}
			loop.Set("search_count", fmt.Sprintf("%d", searchCount))

			artifactFilename := invoker.EmitFileArtifactWithExt(
				fmt.Sprintf("internet_research_round_%d_search_%d_%s", iteration, searchCount, utils.DatetimePretty2()),
				".md",
				"",
			)
			emitter.EmitPinFilename(artifactFilename)

			artifactContent := fmt.Sprintf(`# Internet Research Result - Round %d Search #%d

Query: %s
Time: %s

## Content

%s
`, iteration, searchCount, searchQuery, time.Now().Format("2006-01-02 15:04:05"), content)

			if err := os.WriteFile(artifactFilename, []byte(artifactContent), 0644); err != nil {
				log.Warnf("failed to write research artifact: %v", err)
			} else {
				log.Infof("research artifact saved to: %s", artifactFilename)
			}

			loop.Set(fmt.Sprintf("artifact_round_%d_%d", iteration, searchCount), artifactFilename)
			loop.Set(fmt.Sprintf("compressed_result_round_%d_%d", iteration, searchCount), content)

			searchHistory := loop.Get("search_history")
			if searchHistory != "" {
				searchHistory += "\n"
			}
			searchHistory += fmt.Sprintf("[%s] #%d web_search: %s -> %d bytes",
				time.Now().Format("15:04:05"), searchCount, searchQuery, len(content))
			loop.Set("search_history", searchHistory)

			loop.LoadingStatus("evaluating search progress")
			evalResult := evaluateNextMovements(ctx, invoker, loop, userQuery, searchQuery, content, searchCount)
			if evalResult.Finished {
				log.Infof("internet research finished, summary: %s", evalResult.Summary)
				loop.Set("final_summary", evalResult.Summary)
				feedback := fmt.Sprintf("=== Search Complete ===\nQuery: %s\nContent: %d bytes\nSaved to: %s\n\n=== Research Complete ===\n%s\n\nGenerating final report...",
					searchQuery, len(content), artifactFilename, evalResult.Summary)
				op.Feedback(feedback)
				r.AddToTimeline("internet_research_finished", feedback)
				op.Exit()
			} else {
				currentNextMovements := loop.Get("next_movements_summary")
				if currentNextMovements != "" {
					currentNextMovements += "\n\n"
				}
				currentNextMovements += fmt.Sprintf("【Search #%d: %s】\n%s", searchCount, searchQuery, evalResult.NextMovements)
				loop.Set("next_movements_summary", currentNextMovements)

				feedback := fmt.Sprintf("=== Search Complete ===\nQuery: %s\nContent: %d bytes\nSaved to: %s\n\n=== Next Steps ===\n%s\n\nPlease continue searching based on suggestions.",
					searchQuery, len(content), artifactFilename, evalResult.NextMovements)
				loop.Set("next_movements", feedback)
				op.Continue()
			}
		},
	)
}

type EvaluateResult struct {
	NextMovements string
	Finished      bool
	Summary       string
}

func evaluateNextMovements(
	ctxAny any,
	invoker aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	userQuery string,
	currentQuery string,
	currentResult string,
	searchCount int,
) EvaluateResult {
	ctx, ok := ctxAny.(context.Context)
	if !ok {
		log.Warnf("evaluateNextMovements: context conversion failed")
		ctx = context.Background()
	}
	dNonce := utils.RandStringBytes(4)

	searchHistory := loop.Get("search_history")

	if searchCount >= maxSearchResults {
		log.Infof("evaluateNextMovements: search count reached limit (%d), stopping", searchCount)
		return EvaluateResult{
			Finished: true,
			Summary:  "reached maximum search count limit",
		}
	}

	promptTemplate := `<|USER_QUERY_{{ .nonce }}|>
{{ .userQuery }}
<|USER_QUERY_END_{{ .nonce }}|>

<|SEARCH_HISTORY_{{ .nonce }}|>
{{ .searchHistory }}
<|SEARCH_HISTORY_END_{{ .nonce }}|>

<|CURRENT_SEARCH_{{ .nonce }}|>
Query: {{ .currentQuery }}
Search Count: #{{ .searchCount }}

Result Summary:
{{ .currentResult }}
<|CURRENT_SEARCH_END_{{ .nonce }}|>

<|INSTRUCT_{{ .nonce }}|>
Evaluate whether the current search results are sufficient to answer the user's question, and provide next-step recommendations.

Evaluation Criteria:
1. Do current results directly answer the user's core question?
2. Are there important aspects not yet covered?
3. Is additional information from different angles needed?

Output Requirements:
- finished: boolean, true if information is sufficient, false otherwise
- next_movements: if finished is false, provide specific search suggestions (keywords/queries); if finished is true, output empty string
- summary: if finished is true, briefly summarize collected knowledge; if finished is false, output empty string

Constraints:
- Total searches should not exceed 8
- Avoid repeating identical or similar searches
- Prioritize uncovered aspects of the user's question
<|INSTRUCT_END_{{ .nonce }}|>
`

	resultPreview := currentResult
	if len(resultPreview) > 2000 {
		resultPreview = resultPreview[:2000] + "\n...(truncated)"
	}

	materials, err := utils.RenderTemplate(promptTemplate, map[string]any{
		"nonce":         dNonce,
		"userQuery":     userQuery,
		"searchHistory": searchHistory,
		"currentQuery":  currentQuery,
		"searchCount":   searchCount,
		"currentResult": resultPreview,
	})

	if err != nil {
		log.Errorf("evaluateNextMovements: template render failed: %v", err)
		return EvaluateResult{Finished: true, Summary: "template render failed"}
	}

	forgeResult, err := invoker.InvokeLiteForge(
		ctx,
		"evaluate-internet-research-next",
		materials,
		[]aitool.ToolOption{
			aitool.WithBoolParam("finished",
				aitool.WithParam_Description("whether information collection is complete"),
				aitool.WithParam_Required(true)),
			aitool.WithStringParam("next_movements",
				aitool.WithParam_Description("next search suggestions if not finished")),
			aitool.WithStringParam("summary",
				aitool.WithParam_Description("brief summary of collected knowledge if finished")),
		},
	)

	if err != nil {
		log.Errorf("evaluateNextMovements: LiteForge failed: %v", err)
		return EvaluateResult{Finished: true, Summary: "LiteForge evaluation failed"}
	}

	if forgeResult == nil {
		return EvaluateResult{Finished: true, Summary: "LiteForge returned nil"}
	}

	finished := forgeResult.GetBool("finished")
	nextMovements := strings.TrimSpace(forgeResult.GetString("next_movements"))
	summary := strings.TrimSpace(forgeResult.GetString("summary"))

	return EvaluateResult{
		NextMovements: nextMovements,
		Finished:      finished,
		Summary:       summary,
	}
}

var webSearchAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return makeWebSearchAction(r)
}
