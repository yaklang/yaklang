package loop_internet_research

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/omnisearch"
	"github.com/yaklang/yaklang/common/omnisearch/ostype"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/html"
)

const maxSearchResults = 8
const maxContentPerPage = 4096
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

			results, err := omnisearch.Search(searchQuery, ostype.WithPageSize(maxResults))
			if err != nil {
				log.Warnf("omnisearch error for query '%s': %v", searchQuery, err)
				op.Feedback(fmt.Sprintf("search failed: %v\nplease try a different query.", err))
				op.Continue()
				return
			}

			if len(results) == 0 {
				op.Feedback(fmt.Sprintf("no results found for '%s'. please try a different query.", searchQuery))
				op.Continue()
				return
			}

			loop.LoadingStatus(fmt.Sprintf("found %d results, extracting content", len(results)))
			log.Infof("internet research: found %d results for '%s'", len(results), searchQuery)

			var resultBuilder strings.Builder
			resultBuilder.WriteString(fmt.Sprintf("=== Web Search: %s ===\n\n", searchQuery))

			for i, result := range results {
				idx := i + 1
				resultBuilder.WriteString(fmt.Sprintf("## %d. %s\n", idx, result.Title))
				resultBuilder.WriteString(fmt.Sprintf("URL: %s\n", result.URL))
				if result.Content != "" {
					resultBuilder.WriteString(fmt.Sprintf("Snippet: %s\n", result.Content))
				}

				if result.URL != "" {
					loop.LoadingStatus(fmt.Sprintf("fetching page %d/%d: %s", idx, len(results), result.URL))
					pageText := fetchAndExtractText(result.URL, 10*time.Second)
					if pageText != "" {
						if len(pageText) > maxContentPerPage {
							pageText = pageText[:maxContentPerPage] + "\n...(truncated)"
						}
						resultBuilder.WriteString(fmt.Sprintf("\nPage Content:\n%s\n", pageText))
					}
				}
				resultBuilder.WriteString("\n---\n\n")
			}

			rawContent := resultBuilder.String()
			log.Infof("internet research: raw content size: %d bytes", len(rawContent))

			loop.LoadingStatus("compressing search results")
			compressedResult, err := invoker.CompressLongTextWithDestination(ctx, rawContent, searchQuery, 10*1024)
			if err != nil {
				log.Warnf("failed to compress search result: %v", err)
				if len(rawContent) > 10*1024 {
					compressedResult = rawContent[:10*1024] + "\n...(truncated)"
				} else {
					compressedResult = rawContent
				}
			}
			if compressedResult == "" {
				op.Feedback(fmt.Sprintf("search for '%s' returned results but no relevant content could be extracted. try a different query.", searchQuery))
				op.Continue()
				return
			}

			loop.LoadingStatus("recording search results")
			feedNewKnowledge(compressedResult, userQuery)

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
Result Count: %d
Time: %s

## Compressed Content

%s
`, iteration, searchCount, searchQuery, len(results), time.Now().Format("2006-01-02 15:04:05"), compressedResult)

			if err := os.WriteFile(artifactFilename, []byte(artifactContent), 0644); err != nil {
				log.Warnf("failed to write research artifact: %v", err)
			} else {
				log.Infof("research artifact saved to: %s", artifactFilename)
			}

			loop.Set(fmt.Sprintf("artifact_round_%d_%d", iteration, searchCount), artifactFilename)
			loop.Set(fmt.Sprintf("compressed_result_round_%d_%d", iteration, searchCount), compressedResult)

			searchHistory := loop.Get("search_history")
			if searchHistory != "" {
				searchHistory += "\n"
			}
			searchHistory += fmt.Sprintf("[%s] #%d web_search: %s -> %d results, %d bytes",
				time.Now().Format("15:04:05"), searchCount, searchQuery, len(results), len(compressedResult))
			loop.Set("search_history", searchHistory)

			loop.LoadingStatus("evaluating search progress")
			evalResult := evaluateNextMovements(ctx, invoker, loop, userQuery, searchQuery, compressedResult, searchCount)
			if evalResult.Finished {
				log.Infof("internet research finished, summary: %s", evalResult.Summary)
				loop.Set("final_summary", evalResult.Summary)
				feedback := fmt.Sprintf("=== Search Complete ===\nQuery: %s\nResults: %d, Compressed: %d bytes\nSaved to: %s\n\n=== Research Complete ===\n%s\n\nGenerating final report...",
					searchQuery, len(results), len(compressedResult), artifactFilename, evalResult.Summary)
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

				feedback := fmt.Sprintf("=== Search Complete ===\nQuery: %s\nResults: %d, Compressed: %d bytes\nSaved to: %s\n\n=== Next Steps ===\n%s\n\nPlease continue searching based on suggestions.",
					searchQuery, len(results), len(compressedResult), artifactFilename, evalResult.NextMovements)
				loop.Set("next_movements", feedback)
				op.Continue()
			}
		},
	)
}

func fetchAndExtractText(url string, timeout time.Duration) string {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		log.Infof("failed to fetch %s: %v", url, err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(contentType), "html") && contentType != "" {
		bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxContentPerPage))
		if err != nil {
			return ""
		}
		return string(bodyBytes)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return ""
	}

	return extractTextFromHTML(bodyBytes)
}

func extractTextFromHTML(body []byte) string {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return ""
	}

	var textParts []string
	skipTags := map[string]bool{
		"script": true, "style": true, "noscript": true,
		"iframe": true, "svg": true, "head": true,
	}

	var extractText func(*html.Node)
	extractText = func(n *html.Node) {
		if n.Type == html.ElementNode && skipTags[n.Data] {
			return
		}
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				textParts = append(textParts, text)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractText(c)
		}
	}
	extractText(doc)

	return strings.Join(textParts, " ")
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
