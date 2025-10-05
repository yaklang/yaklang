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

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils"

	resultSpec "github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

// checkCodeAndFormatErrors performs static analysis and formats error messages
// Returns: errorMessages string, hasBlockingErrors bool
func checkCodeAndFormatErrors(code string) (string, bool) {
	result := static_analyzer.YaklangScriptChecking(code, "yak")
	var buf bytes.Buffer
	hasBlockingErrors := false

	for _, msg := range result {
		buf.WriteString(msg.String())
		buf.WriteString("\n")

		// Check if there are any errors (not just warnings/hints)
		if !hasBlockingErrors && msg.Severity == resultSpec.Error {
			hasBlockingErrors = true
		}
	}

	return buf.String(), hasBlockingErrors
}

func (r *ReAct) invokeWriteYaklangCode(ctx context.Context, approach string) (string, error) {
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
	hasBlockingErrors := false
	userQuery := ""
	if r.config.memory != nil {
		userQuery = r.config.memory.Query
	}

	filename := r.EmitYaklangCodeArtifact("generated_code", "")

	log.Info("================================================================")
	log.Infof("Generating yaklang code to file: %s", filename)
	log.Infof("in terminal, use `code %#v` for open current in editor", filename)
	log.Infof("Code generation loop maxIterations: %d", r.config.maxIterations)
	log.Info("================================================================")

	// Create document searcher before entering loop for better performance
	// This will be nil if aikb is not available, which is handled gracefully
	documentSearcher := r.createDocumentSearcher()

	// Get available tools
	tools := buildinaitools.GetAllTools()
	nonceStr := utils.RandStringBytes(4)

LOOP:
	for {
		iterationCount++

		if r.config.maxIterations > 0 && iterationCount > r.config.maxIterations {
			log.Warnf("Reached max iterations (%d), stopping code generation loop", r.config.maxIterations)
			break
		}

		// Before generating prompt, ALWAYS check current code status to determine errors and finish availability
		// This ensures we have the most up-to-date error state for the prompt
		if currentCode != "" {
			errorMessages, hasBlockingErrors = checkCodeAndFormatErrors(currentCode)
			log.Infof("iteration %d: checked code, hasBlockingErrors=%v, errorMsgLen=%d, finish_allowed=%v",
				iterationCount, hasBlockingErrors, len(errorMessages), !hasBlockingErrors)
		} else {
			errorMessages = ""
			hasBlockingErrors = false
			log.Infof("iteration %d: no code yet, hasBlockingErrors=%v, finish_allowed=%v", iterationCount, hasBlockingErrors, !hasBlockingErrors)
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
			!hasBlockingErrors,          // allow finish only when no blocking errors
		)
		if err != nil {
			log.Errorf("Failed to generate prompt for yaklang code action loop: %v", err)
			return "", err
		}
		// Don't clear errorMessages here - it's already set correctly above based on current code state

		var actionName string
		var payload string
		var generatedCode string
		var action *aicommon.Action
		var actionErr error
		var modifyStartLine, modifyEndLine int

		cb := utils.NewCondBarrier()
		codeBarrier := cb.CreateBarrier("code")
		streamFinished := make(chan struct{})

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
					},
					func() {
						// Called when stream reading is finished
						close(streamFinished)
					},
				)
				if actionErr != nil {
					return utils.Errorf("Failed to parse action: %v", actionErr)
				}
				actionName = action.Name()
				switch actionName {
				case "write_code":
					return nil
				case "modify_code":
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
			return "", utils.Wrap(transactionErr, "AI transaction failed in code generation loop")
		}

		if actionName == "write_code" || actionName == "modify_code" {
			log.Info("start to wait for stream to finish, then wait for code")

			// First, wait for the stream to finish reading
			<-streamFinished
			log.Info("stream finished, now waiting for code barrier with 30s timeout")

			// After stream finishes, wait up to 30 seconds for code to be generated
			waitDone := make(chan error, 1)
			go func() {
				waitDone <- cb.Wait("code")
			}()

			select {
			case cberr := <-waitDone:
				if cberr != nil {
					log.Warnf("Failed to wait for code generation: %v", cberr)
					errorMessages += fmt.Sprintf("Code generation failed: %v. AI MUST provide code in <|GEN_CODE_...|> tags when using %s action. ", cberr, actionName)
					continue
				}
			case <-time.After(30 * time.Second):
				log.Warnf("Code generation timeout: stream finished but no code received within 30 seconds")
				errorMessages += fmt.Sprintf("Code generation TIMEOUT! Stream finished but AI did not provide code in <|GEN_CODE_...|> tags within 30 seconds after stream ended. CRITICAL: You MUST generate code inside <|GEN_CODE_...|> tags when using '%s' action! ", actionName)
				continue
			}

			payload = generatedCode
			if payload == "" {
				errorMessages += fmt.Sprintf("AI did not provide any code in %s action. CRITICAL: You MUST generate code inside <|GEN_CODE_...|> tags when using %s action! ", actionName, actionName)
				continue
			}
			log.Infof("code barrier passed, code received, len: %v, shrinked: %v", len(generatedCode), utils.ShrinkString(generatedCode, 128))
		}

		// Handle different action types
		switch actionName {
		case "finish":
			log.Info("start to check code for finish action")
			errMsg, hasErrors := checkCodeAndFormatErrors(currentCode)
			hasBlockingErrors = hasErrors

			if errMsg != "" {
				fmt.Println("=================================================")
				fmt.Println(currentCode)
				fmt.Println("=================================================")
				if hasBlockingErrors {
					log.Warnf("finish action, but code has ERRORS: %v", errMsg)
					errorMessages = "⚠️ CRITICAL: You attempted to use 'finish' action, but the code still has ERRORS that MUST be fixed:\n\n" + errMsg + "\n\n⚠️ You MUST fix all errors before using 'finish' action again. Use 'modify_code' or 'query_document' to resolve these issues.\n"
					fmt.Println(errorMessages)
					fmt.Println("=================================================")
					// Don't break - continue the loop to give AI a chance to fix errors
					continue
				} else {
					log.Infof("finish action with warnings/hints: %v", errMsg)
					errorMessages = errMsg
				}
				fmt.Println(errorMessages)
				fmt.Println("=================================================")
			}
			break LOOP
		case "modify_code":
			// Apply modification to current code using new edit methods
			editor := memedit.NewMemEditor(currentCode)
			log.Infof("start to modify code lines %d to %d", modifyStartLine, modifyEndLine)
			err = editor.ReplaceLineRange(modifyStartLine, modifyEndLine, payload)
			if err != nil {
				return filename, utils.Errorf("Failed to replace line range: %v", err)
			}
			fmt.Println("=================================================")
			fmt.Println(string(payload))
			fmt.Println("=================================================")
			fullCode := editor.GetSourceCode()
			os.RemoveAll(filename)
			os.WriteFile(filename, []byte(fullCode), 0644)
			currentCode = fullCode

			errMsg, hasErrors := checkCodeAndFormatErrors(currentCode)
			hasBlockingErrors = hasErrors
			errorMessages = errMsg // Set error messages (don't accumulate)
			r.AddToTimeline("code_modified",
				utils.ShrinkString(fmt.Sprintf("line[%v-%v]:", modifyStartLine, modifyEndLine)+strconv.Quote(currentCode), 128))
			log.Infof("modify_code done: hasBlockingErrors=%v, will show errors in next iteration", hasBlockingErrors)
			r.EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, actionName, payload)
			continue
		case "write_code":
			// Update current code
			code := payload
			err := os.WriteFile(filename, []byte(code), 0644)
			if err != nil {
				r.AddToTimeline("error", "Failed to write code to file: "+err.Error())
				return filename, utils.Errorf("Failed to write code to file: %v", err)
			}
			currentCode = code
			errMsg, hasErrors := checkCodeAndFormatErrors(code)
			hasBlockingErrors = hasErrors
			errorMessages = errMsg // Set error messages (don't accumulate)
			r.AddToTimeline("code_generated", utils.ShrinkString(strconv.Quote(code), 128))
			log.Infof("write_code done: hasBlockingErrors=%v, will show errors in next iteration", hasBlockingErrors)
			r.EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, actionName, payload)
			continue
		case "require_tool":
			toolPayload := payload
			toolcallResult, directlyAnswerRequired, err := r.ExecuteToolRequiredAndCall(
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
			r.AddToTimeline("tool_call", "Tool call result: "+result)
			continue
		case "ask_for_clarification":
			result := action.GetInvokeParams("ask_for_clarification_payload")
			question := result.GetString("question")
			options := result.GetStringSlice("options")
			suggestion := r.AskForClarification(question, options)
			if suggestion == "" {
				suggestion = "user did not provide a valid suggestion, using default 'continue' action"
			}
			continue
		case "query_document":
			payloads := action.GetInvokeParams("query_document_payload")
			documentResults, ok := r.handleQueryDocument(documentSearcher, payloads)
			errorMessages += documentResults
			if !ok {
				// If searcher is not available or no results found, still continue the loop
				log.Warn("query_document action did not complete successfully")
			}

			if len(documentResults) > 0 {
				log.Infof("================== document query =====================\n"+
					"%v\n===================== document result ===================\n"+
					"%v\n=================================================", string(payload), string(documentResults))
			}
			continue
		}
	}
	return filename, nil
}
