package loop_internet_research

import (
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

const maxReadURLContentBytes = 8192

func makeReadURLAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	desc := "访问指定 URL 并提取页面文本内容。用于深入阅读搜索结果中感兴趣的页面，获取更详细的信息。"

	toolOpts := []aitool.ToolOption{
		aitool.WithStringParam("url",
			aitool.WithParam_Description("要访问的网页 URL"),
			aitool.WithParam_Required(true)),
	}

	return reactloops.WithRegisterLoopAction(
		"read_url",
		desc, toolOpts,
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			loop.LoadingStatus("validating read_url parameters")
			url := action.GetString("url")
			if url == "" {
				return utils.Error("url is required")
			}
			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
				return utils.Error("url must start with http:// or https://")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			url := action.GetString("url")

			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}

			userQuery := loop.Get("user_query")
			emitter := loop.GetEmitter()

			loop.LoadingStatus(fmt.Sprintf("fetching page: %s", url))
			log.Infof("internet research: reading URL '%s'", url)

			pageText := fetchAndExtractText(url, 15*time.Second)
			if pageText == "" {
				op.Feedback(fmt.Sprintf("failed to extract text content from %s. the page may be unavailable or contain non-text content.", url))
				op.Continue()
				return
			}

			if len(pageText) > maxReadURLContentBytes {
				pageText = pageText[:maxReadURLContentBytes] + "\n...(truncated)"
			}

			loop.LoadingStatus("compressing page content")
			log.Infof("internet research: page content size: %d bytes from %s", len(pageText), url)

			rawContent := fmt.Sprintf("=== Page Content: %s ===\n\n%s", url, pageText)
			compressedResult, err := invoker.CompressLongTextWithDestination(ctx, rawContent, userQuery, 8*1024)
			if err != nil {
				log.Warnf("failed to compress page content: %v", err)
				compressedResult = rawContent
				if len(compressedResult) > 8*1024 {
					compressedResult = compressedResult[:8*1024] + "\n...(truncated)"
				}
			}

			feedNewKnowledge := func(knowledge string) {
				oldKnowledges := loop.Get("search_results_summary")
				if oldKnowledges == "" {
					loop.Set("search_results_summary", knowledge)
				} else {
					newKnowledge := oldKnowledges + "\n\n" + knowledge
					compressed, err := invoker.CompressLongTextWithDestination(ctx, newKnowledge, userQuery, 10*1024)
					if err != nil {
						log.Warnf("failed to compress accumulated knowledge: %v", err)
						loop.Set("search_results_summary", newKnowledge)
					} else {
						loop.Set("search_results_summary", compressed)
					}
				}
			}

			feedNewKnowledge(compressedResult)

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
				fmt.Sprintf("internet_research_read_url_%d_%d_%s", iteration, searchCount, utils.DatetimePretty2()),
				".md",
				"",
			)
			emitter.EmitPinFilename(artifactFilename)

			artifactContent := fmt.Sprintf(`# URL Content - Round %d Read #%d

URL: %s
Time: %s

## Extracted Content

%s
`, iteration, searchCount, url, time.Now().Format("2006-01-02 15:04:05"), compressedResult)

			if err := os.WriteFile(artifactFilename, []byte(artifactContent), 0644); err != nil {
				log.Warnf("failed to write URL read artifact: %v", err)
			}

			loop.Set(fmt.Sprintf("artifact_round_%d_%d", iteration, searchCount), artifactFilename)
			loop.Set(fmt.Sprintf("compressed_result_round_%d_%d", iteration, searchCount), compressedResult)

			searchHistory := loop.Get("search_history")
			if searchHistory != "" {
				searchHistory += "\n"
			}
			searchHistory += fmt.Sprintf("[%s] #%d read_url: %s -> %d bytes",
				time.Now().Format("15:04:05"), searchCount, url, len(compressedResult))
			loop.Set("search_history", searchHistory)

			feedback := fmt.Sprintf("=== URL Read Complete ===\nURL: %s\nContent: %d bytes\nSaved to: %s\n\nPlease continue your research or call final_summary to generate the report.",
				url, len(compressedResult), artifactFilename)
			op.Feedback(feedback)
			op.Continue()
		},
	)
}

var readURLAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return makeReadURLAction(r)
}
