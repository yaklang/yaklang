package loop_yaklangcode

import (
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

// buildInitTask creates the initialization task handler with file detection and initial code search
func buildInitTask(r aicommon.AIInvokeRuntime, docSearcher *ziputil.ZipGrepSearcher) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
		emitter := r.GetConfig().GetEmitter()

		// Step 1: 分析用户需求，生成搜索关键字和判断文件路径
		log.Infof("init task step 1: analyzing user requirements and generating search patterns")
		step1Result, err := r.InvokeLiteForge(
			task.GetContext(),
			"analyze-requirement-and-search",
			utils.MustRenderTemplate(
				`
你的目标是分析用户需求，完成两个任务：

【任务1：判断文件操作类型】
判断这是创建新文件还是修改已有文件：
- 如果用户明确提到文件路径（如"修改 /tmp/test.yak"），则是修改已有文件
- 如果用户只描述功能需求，没有提到具体文件，则是创建新文件

【任务2：生成代码搜索关键字】
根据用户需求，生成 2-4 个搜索模式（search_patterns），用于在 Yaklang 代码样例库中搜索相关示例：

搜索模式类型：
1. 函数名搜索：如 "servicescan\\.Scan", "poc\\.Get", "str\\.Split"
2. 关键词搜索：如 "端口扫描", "HTTP请求", "JSON解析"
3. 混合搜索：如 "mitm.*证书", "fuzz.*参数"

注意事项：
- 优先使用函数名搜索（使用 \\.  转义点号）
- 每个pattern要具体且相关，避免过于宽泛
- 如果涉及多个功能点，可以为每个功能点生成一个pattern
- 搜索模式需要是正则表达式或关键词

<|USER_INPUT_{{ .nonce }}|>
{{ .data }}
<|USER_INPUT_END_{{ .nonce }}|>
`,
				map[string]any{
					"nonce": utils.RandStringBytes(4),
					"data":  task.GetUserInput(),
				}),
			[]aitool.ToolOption{
				aitool.WithBoolParam("create_new_file", aitool.WithParam_Description("Is this task to create a new file or modify an existing file? If user mentions specific file path, set to false."), aitool.WithParam_Required(true)),
				aitool.WithStringParam("existed_filepath", aitool.WithParam_Description("Only when create_new_file is false. The file path to modify.")),
				aitool.WithStringArrayParam("search_patterns", aitool.WithParam_Description("2-4 search patterns for finding relevant Yaklang code examples. Each pattern should be a regex or keyword."), aitool.WithParam_Required(true)),
				// aitool.WithStringParam("reason", aitool.WithParam_Description("Explain your decision and why these search patterns are chosen."), aitool.WithParam_Required(true)),
			},
			// aicommon.WithGeneralConfigStreamableField("reason"),
		)
		if err != nil {
			log.Errorf("failed to invoke liteforge step 1: %v", err)
			return utils.Errorf("failed to analyze requirement: %v", err)
		}

		createNewFile := step1Result.GetBool("create_new_file")
		existed := step1Result.GetString("existed_filepath")
		reason := step1Result.GetString("reason")
		_ = reason
		searchPatterns := step1Result.GetStringSlice("search_patterns")

		log.Infof("identified create_new_file: %v, search_patterns count: %d", createNewFile, len(searchPatterns))

		// Step 2: 执行代码样例搜索（如果有 docSearcher）
		var initialSamples string
		if docSearcher != nil && len(searchPatterns) > 0 {
			log.Infof("init task step 2: searching code samples with %d patterns", len(searchPatterns))
			emitter.EmitThoughtStream(task.GetIndex(), "Searching for relevant code examples in Yaklang sample library...")

			var allResults strings.Builder
			searchedCount := 0
			maxPatterns := 4 // 最多搜索4个pattern
			if len(searchPatterns) > maxPatterns {
				searchPatterns = searchPatterns[:maxPatterns]
			}

			for idx, pattern := range searchPatterns {
				if pattern == "" {
					continue
				}

				log.Infof("searching pattern %d/%d: %s", idx+1, len(searchPatterns), pattern)

				// 执行 grep 搜索
				grepOpts := []ziputil.GrepOption{
					ziputil.WithGrepCaseSensitive(false),
					ziputil.WithContext(15),
				}

				results, err := docSearcher.GrepRegexp(pattern, grepOpts...)
				if err != nil {
					// 如果正则失败，尝试子字符串搜索
					results, err = docSearcher.GrepSubString(pattern, grepOpts...)
				}

				if err != nil || len(results) == 0 {
					log.Infof("no results found for pattern: %s", pattern)
					continue
				}

				searchedCount++
				allResults.WriteString(fmt.Sprintf("\n=== Search Pattern: %s (Found %d matches) ===\n", pattern, len(results)))

				// 限制每个pattern的结果数量
				maxResultsPerPattern := 10
				displayCount := len(results)
				if displayCount > maxResultsPerPattern {
					displayCount = maxResultsPerPattern
				}

				for i := 0; i < displayCount; i++ {
					result := results[i]
					allResults.WriteString(fmt.Sprintf("\n--- [%d] %s:%d ---\n", i+1, result.FileName, result.LineNumber))

					if len(result.ContextBefore) > 0 {
						for _, line := range result.ContextBefore {
							allResults.WriteString(fmt.Sprintf("  %s\n", line))
						}
					}

					allResults.WriteString(fmt.Sprintf(">>> %s\n", result.Line))

					if len(result.ContextAfter) > 0 {
						for _, line := range result.ContextAfter {
							allResults.WriteString(fmt.Sprintf("  %s\n", line))
						}
					}
				}

				if len(results) > maxResultsPerPattern {
					allResults.WriteString(fmt.Sprintf("\n... (%d more results not shown)\n", len(results)-maxResultsPerPattern))
				}
			}

			if searchedCount > 0 {
				rawResults := allResults.String()
				log.Infof("collected %d bytes of search results, attempting compression", len(rawResults))

				// 构建 patterns 字符串用于压缩
				var patternsStr strings.Builder
				for idx, pattern := range searchPatterns {
					if idx > 0 {
						patternsStr.WriteString(", ")
					}
					patternsStr.WriteString(pattern)
				}

				// 使用压缩功能精选代码片段
				initialSamples = compressSearchResults(rawResults, patternsStr.String(), r, nil, 6, 5, 20, "【精选初始代码样例】", true)

				if initialSamples != "" {
					emitter.EmitThoughtStream(task.GetIndex(), "Found relevant code samples:\n"+initialSamples)
					r.AddToTimeline("initial_code_samples", initialSamples)
					log.Infof("initial samples collected successfully, size: %d bytes", len(initialSamples))
				}
			} else {
				log.Infof("no search results found for any pattern")
			}
		}

		// Step 3: 处理文件路径
		if !createNewFile || existed != "" {
			targetPath := existed
			log.Infof("identified target path: %s", targetPath)
			filename := utils.GetFirstExistedFile(targetPath)
			if filename == "" {
				createFileErr := os.WriteFile(targetPath, []byte(""), 0644)
				if createFileErr != nil {
					return utils.Errorf("not found existed file and cannot create file to disk, failed: %v", createFileErr)
				}
				filename = targetPath
			}
			content, _ := os.ReadFile(targetPath)
			if len(content) > 0 {
				log.Infof("identified target file: %s, file size: %v", targetPath, len(content))
				loop.Set("full_code", string(content))
			}
			emitter.EmitPinFilename(filename)
			loop.Set("filename", filename)
			return nil
		}

		// 创建新文件
		filename := r.EmitFileArtifactWithExt("gen_code", ".yak", "")
		emitter.EmitPinFilename(filename)
		loop.Set("filename", filename)
		return nil
	}
}
