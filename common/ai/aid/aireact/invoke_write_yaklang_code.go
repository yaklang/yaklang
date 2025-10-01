package aireact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/utils/ziputil"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) invokeWriteYaklangCode(ctx context.Context, approach string) error {
	// start to write code:
	// * (optional)query code snippets
	// * write to file
	// * check syntax and lint
	// * (optional) run test cases
	// * (query document && modify to file)
	// * return

	iterationCount := 0
	currentCode := ""
	errorMessages := ""
	userQuery := ""
	if r.config.memory != nil {
		userQuery = r.config.memory.Query
	}

	filename := r.EmitYaklangCodeArtifact("generated_code", "")

	log.Info("================================================================")
	log.Infof("Generating yaklang code to file: %s", filename)
	log.Infof("in terminal, use `code %#v` for open current in editor", filename)
	log.Info("================================================================")

	// Get available tools
	tools := buildinaitools.GetAllTools()
	nonceStr := utils.RandStringBytes(4)

LOOP:
	for {
		iterationCount++

		if r.config.maxIterations > 0 && iterationCount > r.config.maxIterations {
			break
		}

		log.Infof("start to generate yaklang code, iteration %d", iterationCount)
		prompt, err := r.promptManager.GenerateYaklangCodeActionLoop(
			userQuery+"\n\n"+approach,   // userQuery
			currentCode,                 // currentCode
			errorMessages,               // errorMessages
			iterationCount,              // iterationCount
			tools,                       // tools
			nonceStr,                    // nonce for build boundary safe tag
			r.config.enableUserInteract, // allow ask for clarification
		)
		if err != nil {
			log.Errorf("Failed to generate prompt for yaklang code action loop: %v", err)
			return err
		}
		errorMessages = ""

		var actionName string
		var payload string
		var generatedCode string
		var action *aicommon.Action
		var actionErr error
		var modifyStartLine, modifyEndLine int

		cb := utils.NewCondBarrier()
		codeBarrier := cb.CreateBarrier("code")

		transactionErr := aicommon.CallAITransaction(
			r.config, prompt, r.config.CallAI,
			func(resp *aicommon.AIResponse) error {
				stream := resp.GetOutputStreamReader("yaklang-code-loop", true, r.config.Emitter)

				// debug io
				stream = io.TeeReader(stream, os.Stdout)

				stream = utils.CreateUTF8StreamMirror(stream, func(reader io.Reader) {
					aitag.Parse(reader, aitag.WithCallback("GEN_CODE", nonceStr, func(reader io.Reader) {
						var result bytes.Buffer
						resultReader := io.TeeReader(reader, &result)
						r.EmitStreamEvent("yaklang-code", time.Now(), resultReader, resp.GetTaskIndex(), func() {
							code := result.String()
							if code == "" {
								return
							}
							if strings.HasPrefix(code, "\n") {
								code = code[1:]
							}
							if strings.HasSuffix(code, "\n") {
								code = code[:len(code)-1]
							}
							generatedCode = code
							codeBarrier.Done()
						})
					}))
				})

				action, actionErr = aicommon.ExtractActionFromStreamWithJSONExtractOptions(
					stream,
					"write_code",
					[]string{
						"query_document",
						"require_tool",
						"modify_code",
						"ask_for_clarification",
						"finish",
					},
					[]jsonextractor.CallbackOption{
						jsonextractor.WithRegisterMultiFieldStreamHandler(
							[]string{
								"query_document",
								"tool_require_payload",
								"human_readable_thought",
								"question", // only emit when parent is ask_for_clarification_payload
							},
							func(key string, reader io.Reader, parents []string) {
								if key == "question" {
									if ret := len(parents); !(ret > 0 && strings.Contains(parents[ret-1], "ask_for_clarification_payload")) {
										return
									}
								}

								pr, pw := utils.NewPipe()
								go func() {
									defer pw.Close()
									switch key {
									case "query_document":
										pw.WriteString("查询文档：")
									case "tool_require_payload":
										pw.WriteString("调用工具：")
									}
									io.Copy(pw, utils.JSONStringReader(reader))
								}()
								r.Emitter.EmitStreamEvent(
									"re-act-loop-thought",
									time.Now(),
									pr,
									resp.GetTaskIndex(),
								)
							},
						),
					})
				if actionErr != nil {
					return utils.Errorf("Failed to parse action: %v", actionErr)
				}
				actionName = action.Name()
				switch actionName {
				case "write_code":
					return nil
				case "modify_code":
					payload = action.GetString("code")
					start := action.GetInt("modify_start_line")
					end := action.GetInt("modify_end_line")
					if start <= 0 || end <= 0 || end < start {
						return utils.Error("modify_code action must have valid 'modify_start_line' and 'modify_end_line' fields")
					}
					modifyStartLine = int(start)
					modifyEndLine = int(end)
				case "query_document":
					// query_document uses query_document_payload, no simple payload field needed
					payloads := action.GetInvokeParams("query_document_payload")
					if len(payloads.GetStringSlice("keywords")) == 0 && len(payloads.GetStringSlice("regexp")) == 0 {
						return utils.Error("query_document action must have at least one keyword or regexp in 'query_document_payload'")
					}
				case "require_tool":
					payload = action.GetString("tool_require_payload")
					if payload == "" {
						return utils.Error("require_tool action must have 'tool_require_payload' field")
					}
				case "ask_for_clarification":
					result := action.GetInvokeParams("ask_for_clarification_payload")
					if result.GetString("question") == "" {
						return utils.Error("ask_for_clarification action must have 'question' field in 'ask_for_clarification_payload'")
					}
				case "finish":
					return nil
				default:
					// For other actions, we don't have specific payload requirements
					return utils.Errorf("unknown action: %s", actionName)
				}
				return nil
			})
		if transactionErr != nil {
			return utils.Wrap(transactionErr, "AI transaction failed in code generation loop")
		}

		if actionName == "write_code" || actionName == "modify_code" {
			log.Info("start to wait code in conditional barrier")
			cberr := cb.Wait("code")
			if cberr != nil {
				log.Warnf("Failed to wait for code generation: %v", cberr)
			}
			payload = generatedCode
			if actionName == "write_code" && payload == "" {
				errorMessages += "AI did not provide any code in write_code action; "
				continue
			}
			log.Infof("end to wait code in conditional barrier, code received, len: %v, shrinked: %v", len(generatedCode), utils.ShrinkString(generatedCode, 128))
		}

		// Handle different action types
		switch actionName {
		case "finish":
			break LOOP
		case "modify_code":
			// Apply modification to current code using new edit methods
			editor := memedit.NewMemEditor(currentCode)
			log.Infof("start to modify code lines %d to %d", modifyStartLine, modifyEndLine)
			err = editor.ReplaceLineRange(modifyStartLine, modifyEndLine, payload)
			if err != nil {
				return utils.Errorf("Failed to replace line range: %v", err)
			}
			fmt.Println("=================================================")
			fmt.Println(string(payload))
			fmt.Println("=================================================")
			fullCode := editor.GetSourceCode()
			os.RemoveAll(filename)
			os.WriteFile(filename, []byte(fullCode), 0644)
			currentCode = fullCode

			result := static_analyzer.YaklangScriptChecking(currentCode, "yak")
			var buf bytes.Buffer
			for _, msg := range result {
				buf.WriteString(msg.String())
				buf.WriteString("\n")
			}
			r.addToTimeline("code_modified",
				utils.ShrinkString(fmt.Sprintf("line[%v-%v]:", modifyStartLine, modifyEndLine)+strconv.Quote(currentCode), 128))
			errorMessages += buf.String()
			r.EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, actionName, payload)
			continue
		case "write_code":
			// Update current code
			code := payload
			err := os.WriteFile(filename, []byte(code), 0644)
			if err != nil {
				r.addToTimeline("error", "Failed to write code to file: "+err.Error())
				return utils.Errorf("Failed to write code to file: %v", err)
			}
			currentCode = code
			result := static_analyzer.YaklangScriptChecking(code, "yak")
			var buf bytes.Buffer
			for _, msg := range result {
				buf.WriteString(msg.String())
				buf.WriteString("\n")
			}
			r.addToTimeline("code_generated", utils.ShrinkString(strconv.Quote(code), 128))
			errorMessages += buf.String()
			r.EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, actionName, payload)
			continue
		case "require_tool":
			toolPayload := payload
			toolcallResult, directlyAnswerRequired, err := r.handleRequireTool(
				toolPayload,
			)
			if err != nil {
				errorMessages += err.Error()
				continue
			}
			if directlyAnswerRequired {
				errorMessages += "Tool call resulted in direct answer"
				continue
			}
			result := toolcallResult.StringWithoutID()
			r.addToTimeline("tool_call", "Tool call result: "+result)
			continue
		case "ask_for_clarification":
			result := action.GetInvokeParams("ask_for_clarification_payload")
			question := result.GetString("question")
			options := result.GetStringSlice("options")
			suggestion := r.invokeAskForClarification(question, options)
			if suggestion == "" {
				suggestion = "user did not provide a valid suggestion, using default 'continue' action"
			}
			continue
		case "query_document":
			zipName, err := thirdparty_bin.GetBinaryPath("yaklang-aikb")
			if err != nil {
				log.Errorf("Failed to get yaklang-aikb binary: %v", err)
				errorMessages += fmt.Sprintf("Failed to get yaklang-aikb binary: %v; ", err)
				continue
			}

			payloads := action.GetInvokeParams("query_document_payload")
			caseSensitive := payloads.GetBool("case_sensitive")
			contextLines := payloads.GetInt("context_lines")
			if contextLines == 0 {
				contextLines = 2 // default context
			}
			limit := payloads.GetInt("limit")
			if limit == 0 {
				limit = 20 // default limit
			}

			// Create searcher for better performance with multiple searches
			searcher, err := ziputil.NewZipGrepSearcher(zipName)
			if err != nil {
				log.Errorf("Failed to create zip searcher: %v", err)
				errorMessages += fmt.Sprintf("Failed to create document searcher: %v; ", err)
				continue
			}

			var results []*ziputil.GrepResult

			// Build grep options
			grepOpts := []ziputil.GrepOption{
				ziputil.WithGrepCaseSensitive(caseSensitive),
				ziputil.WithContext(int(contextLines)),
			}

			// Add path filters if specified
			includePathSubString := payloads.GetStringSlice("include_path_substring")
			if len(includePathSubString) > 0 {
				grepOpts = append(grepOpts, ziputil.WithIncludePathSubString(includePathSubString...))
			}

			excludePathSubString := payloads.GetStringSlice("exclude_path_substring")
			if len(excludePathSubString) > 0 {
				grepOpts = append(grepOpts, ziputil.WithExcludePathSubString(excludePathSubString...))
			}

			includePathRegexp := payloads.GetStringSlice("include_path_regexp")
			if len(includePathRegexp) > 0 {
				grepOpts = append(grepOpts, ziputil.WithIncludePathRegexp(includePathRegexp...))
			}

			excludePathRegexp := payloads.GetStringSlice("exclude_path_regexp")
			if len(excludePathRegexp) > 0 {
				grepOpts = append(grepOpts, ziputil.WithExcludePathRegexp(excludePathRegexp...))
			}

			// Search by keywords
			for _, keyword := range payloads.GetStringSlice("keywords") {
				searchResult, err := searcher.GrepSubString(keyword, grepOpts...)
				if err != nil {
					log.Warnf("Failed to grep keyword '%s': %v", keyword, err)
					continue
				}
				results = append(results, searchResult...)
			}

			// Search by regexp
			for _, reg := range payloads.GetStringSlice("regexp") {
				searchResults, err := searcher.GrepRegexp(reg, grepOpts...)
				if err != nil {
					log.Warnf("Failed to grep regexp '%s': %v", reg, err)
					continue
				}
				results = append(results, searchResults...)
			}

			if len(results) == 0 {
				errorMessages += "No matching documents found for the query; "
				r.addToTimeline("query_document", "No results found")
				continue
			}

			// Apply RRF ranking to merge and rank results from multiple searches
			results = ziputil.MergeGrepResults(results)
			rankedResults := utils.RRFRankWithDefaultK(results)

			// Apply limit
			if limit > 0 && len(rankedResults) > int(limit) {
				rankedResults = rankedResults[:limit]
			}

			// Format results for AI consumption
			var docBuffer bytes.Buffer
			docBuffer.WriteString("\n=== Document Query Results ===\n")
			docBuffer.WriteString(fmt.Sprintf("Found %d relevant documents:\n\n", len(rankedResults)))

			for i, result := range rankedResults {
				docBuffer.WriteString(fmt.Sprintf("--- Result %d (Score: %.4f) ---\n", i+1, result.Score))
				docBuffer.WriteString(result.String())
				docBuffer.WriteString("\n")
			}
			docBuffer.WriteString("=== End of Document Query Results ===\n")

			documentResults := docBuffer.String()
			errorMessages += documentResults

			r.addToTimeline("query_document", fmt.Sprintf("Found %d documents", len(rankedResults)))
			r.EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "query_document", documentResults)
			continue
		}
	}
	return nil
}
