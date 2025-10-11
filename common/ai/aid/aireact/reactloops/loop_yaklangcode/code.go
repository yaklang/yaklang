package loop_yaklangcode

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

// createDocumentSearcher creates a document searcher from aikb path
func createDocumentSearcher(aikbPath string) *ziputil.ZipGrepSearcher {
	var zipPath string

	// Use custom aikb path if provided
	if aikbPath != "" {
		zipPath = aikbPath
		log.Infof("using custom aikb path: %s", zipPath)
	} else {
		// Get default yaklang-aikb binary path
		path, err := thirdparty_bin.GetBinaryPath("yaklang-aikb")
		if err != nil {
			log.Warnf("failed to get yaklang-aikb binary: %v", err)
			return nil
		}
		zipPath = path
	}

	// Create searcher
	searcher, err := ziputil.NewZipGrepSearcher(zipPath)
	if err != nil {
		log.Warnf("failed to create document searcher from %s: %v", zipPath, err)
		return nil
	}

	log.Infof("document searcher created successfully from: %s", zipPath)
	return searcher
}

//go:embed prompts/persistent_instruction.txt
var instruction string

//go:embed prompts/reflection_output_example.txt
var outputExample string

//go:embed prompts/reactive_data.txt
var reactiveData string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG,
		func(r aicommon.AIInvokeRuntime) (*reactloops.ReActLoop, error) {
			config := r.GetConfig()
			aikbPath := config.GetConfigString("aikb_path")
			docSearcher := createDocumentSearcher(aikbPath)
			filename := r.EmitFileArtifactWithExt("gen_code", ".yak", "")
			return reactloops.NewReActLoop(
				schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG,
				r,
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithAITagFieldWithAINodeId("GEN_CODE", "yak_code", "re-act-loop-answer-payload"),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					yakCode := loop.Get("full_code")
					codeWithLine := utils.PrefixLinesWithLineNumbers(yakCode)

					feedbacks := feedbacker.String()
					feedbacks = strings.TrimSpace(feedbacks)
					renderMap := map[string]any{
						"Code":                      yakCode,
						"CurrentCodeWithLineNumber": codeWithLine,
						"Nonce":                     nonce,
						"FeedbackMessages":          feedbacks,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				reactloops.WithRegisterLoopActionWithStreamField(
					"query_document",
					"Query the document database or sample code to find relevant information.",
					[]aitool.ToolOption{
						aitool.WithStructParam(
							"query_document_payload",
							[]aitool.PropertyOption{
								aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'query_document'. Provide the exact search pattern of the document you need to query (e.g., 'json.dump', 'servicescan.Scan', 'file.ReadFile', '端口扫描', '打开文件'). Another system will handle the parameter generation based on this name."),
							},
							aitool.WithBoolParam(
								"case_sensitive",
								aitool.WithParam_Description("Indicates whether the search should be case-sensitive. If true, the search will differentiate between uppercase and lowercase letters. If false, the search will be case-insensitive."),
							),
							aitool.WithStringArrayParam(
								"keywords",
								aitool.WithParam_Description(`Keywords or phrases to search in Yaklang documentation (supports both Chinese and English). Common patterns:`)),
							aitool.WithStringArrayParam(
								"regexp",
								aitool.WithParam_Description(`Regular expressions to match specific code patterns in Yaklang documentation.
**Note**: Patterns are case-sensitive. Use '\s+' for whitespace, '\w+' for identifiers, '.*' for wildcards.`),
							),
						),
					},
					[]*reactloops.LoopStreamField{},
					func(r *reactloops.ReActLoop, action *aicommon.Action) error {
						payloads := action.GetInvokeParams("query_document_payload")
						if len(payloads.GetStringSlice("keywords")) == 0 && len(payloads.GetStringSlice("regexp")) == 0 {
							return utils.Error("query_document action must have at least one keyword or regexp in 'query_document_payload'")
						}
						return nil
					},
					func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {

						payloads := action.GetInvokeParams("query_document_payload")

						searching := payloads.Dump()
						loop.GetEmitter().EmitTextPlainTextStreamEvent(
							"query_yaklang_document",
							bytes.NewReader([]byte(searching)),
							loop.GetCurrentTask().GetIndex(),
							func() {
								log.Infof("searching yaklang document: \n%v", searching)
							},
						)

						invoker := loop.GetInvoker()
						invoker.AddToTimeline("start_query_yaklang_docs", "AI decided to query document with payload: "+utils.InterfaceToString(payloads))
						documentResults, ok := handleQueryDocument(r, docSearcher, payloads)
						if !ok {
							invoker.AddToTimeline("query_yaklang_docs_result", "No document searcher available, cannot perform document query, maybe keyword or regexp is invalid: "+utils.InterfaceToString(payloads))
							log.Warn("document searcher is not available, cannot perform document query")
							op.Continue()
							return
						}
						var msg string
						fullcode := loop.Get("full_code")
						if fullcode != "" {
							errMsg, blocking := checkCodeAndFormatErrors(fullcode)
							if blocking {
								op.DisallowNextLoopExit()
							}
							if errMsg != "" {
								msg += "LINT ERR:\n" + errMsg + "\n\n"
							}
						}
						if msg != "" {
							op.Feedback(msg)
						}

						if len(documentResults) > 0 {
							log.Infof("\n================== document query =====================\n"+
								"%v\n===================== document result ===================\n"+
								"%v\n=================================================",
								utils.InterfaceToString(payloads),
								documentResults,
							)
							invoker.AddToTimeline("query_yaklang_docs_result", documentResults)
							nonce := utils.RandBytes(4)
							targetPrompt, err := utils.RenderTemplate(`
<|QUERY_PARAM_{{ .nonce }}|>
{{ .payloads }}
<|QUERY_PARAM_END_{{ .nonce }}|>

<|DOC_{{ .nonce }}|>
{{ .docs }}
<|DOC_END_{{ .nonce }}|>

根据查询参数以及查询结果给出总结，如果查询结果和意图不相关，则你需要在 summary 中说明

summary 回答的内容需要为：

* 描述查询参数和查询结果的意图
* 增加相关的代码示例，告诉后续的行动如何在 modify_code 中修改代码修复错误
* 吸取经验教训，告诉后面不要犯什么错误
`, map[string]any{
								"nonce":    string(nonce),
								"payloads": payloads.Dump(),
								"docs":     documentResults,
							})
							if err != nil {
								invoker.AddToTimeline("failed_to_render_prompt", "Failed to render document summarization prompt: "+err.Error())
								op.Fail(err)
								return
							}
							action, err := invoker.InvokeLiteForge(
								loop.GetCurrentTask().GetContext(),
								"yaklang_doc_summarizer",
								targetPrompt,
								[]aitool.ToolOption{
									aitool.WithStringParam(
										"summary",
										aitool.WithParam_Required(true),
										aitool.WithParam_Description("The summary of the document, use it to generate the code"),
									),
								},
							)
							if err != nil {
								r.AddToTimeline("error", "Failed to invoke liteforge: "+err.Error())
								op.Continue()
								return
							}
							summary := action.GetParams().GetString("summary")
							if summary == "" {
								r.AddToTimeline("error", "No summary generated in 'yaklang_doc_summarizer' action")
								op.Continue()
								return
							} else {
								r.AddToTimeline("summary-request-docs", summary)
							}
						}
					},
				),
				reactloops.WithRegisterLoopAction(
					"write_code",
					"If there is NO CODE, you need to create a new file, then use 'write_code'. If there is already code, it is forbidden to use 'write_code' as it will forcibly overwrite the previous code. You must use 'modify_code' to modify the code.",
					nil,
					func(l *reactloops.ReActLoop, action *aicommon.Action) error {
						return nil
					},
					func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
						invoker := loop.GetInvoker()

						invoker.AddToTimeline("initialize", "AI decided to initialize the code file: "+filename)
						code := loop.Get("yak_code")
						loop.Set("full_code", code)
						if code == "" {
							r.AddToTimeline("error", "No code generated in write_code action")
							operator.Fail("No code generated in 'write_code' action")
							return
						}
						err := os.WriteFile(filename, []byte(code), 0644)
						if err != nil {
							r.AddToTimeline("error", "Failed to write code to file: "+err.Error())
							operator.Fail(err)
							return
						}
						errMsg, blocking := checkCodeAndFormatErrors(code)
						if blocking {
							operator.DisallowNextLoopExit()
							loop.RemoveAction("write_code")
						}
						msg := utils.ShrinkTextBlock(code, 256)
						if errMsg != "" {
							msg += "\n\n--[linter]--\nWriting Code Linter Check:\n" + utils.PrefixLines(utils.ShrinkTextBlock(errMsg, 2048), "  ")
							operator.Feedback(errMsg)
						} else {
							msg += "\n\n--[linter]--\nNo issues found in the modified code segment."
						}
						r.AddToTimeline("initial-yaklang-code", msg)
						log.Infof("write_code done: hasBlockingErrors=%v, will show errors in next iteration", blocking)
						loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "write_code", code)
					},
				),
				reactloops.WithRegisterLoopActionWithStreamField(
					"modify_code",
					"do NOT use this action to create new code file, ONLY use it to modify existing code. Modify the code between the specified line numbers (inclusive). The line numbers are 1-based, meaning the first line of the file is line 1. Ensure that the 'modify_start_line' is less than or equal to 'modify_end_line'.",
					[]aitool.ToolOption{
						aitool.WithIntegerParam("modify_start_line"),
						aitool.WithIntegerParam("modify_end_line"),
						aitool.WithStringParam("modify_code_reason", aitool.WithParam_Description(`Fix code errors or issues, and summarize the fixing approach and lessons learned, keeping the original code content for future reference value`)),
					},
					[]*reactloops.LoopStreamField{
						{
							FieldName: "modify_code_reason",
							AINodeId:  "re-act-loop-thought",
						},
					},
					func(l *reactloops.ReActLoop, action *aicommon.Action) error {
						start := action.GetInt("modify_start_line")
						end := action.GetInt("modify_end_line")
						if start <= 0 || end <= 0 || end < start {
							return utils.Error("modify_code action must have valid 'modify_start_line' and 'modify_end_line' parameters")
						}
						l.GetEmitter().EmitTextPlainTextStreamEvent(
							"thought",
							bytes.NewReader([]byte(fmt.Sprintf("Preparing modify line:%v-%v", start, end))), l.GetCurrentTask().GetIndex())
						return nil
					},
					func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
						invoker := loop.GetInvoker()

						fullCode := loop.Get("full_code")
						partialCode := loop.Get("yak_code")
						editor := memedit.NewMemEditor(fullCode)
						modifyStartLine := action.GetInt("modify_start_line")
						modifyEndLine := action.GetInt("modify_end_line")

						msg := fmt.Sprintf("decided to modify code file, from start_line[%v] to end_line:[%v]", modifyStartLine, modifyEndLine)
						invoker.AddToTimeline("modify_code", msg)

						reason := action.GetString("modify_code_reason")
						if reason != "" {
							r.AddToTimeline("modify_reason", reason)
						}

						log.Infof("start to modify code lines %d to %d", modifyStartLine, modifyEndLine)
						err := editor.ReplaceLineRange(modifyStartLine, modifyEndLine, partialCode)
						if err != nil {
							r.AddToTimeline("modify_failed", "Failed to replace line range: "+err.Error())
							//return filename, utils.Errorf("Failed to replace line range: %v", err)
							op.Fail("failed to replace line range: " + err.Error())
							return
						}
						fmt.Println("=================================================")
						fmt.Println(string(partialCode))
						fmt.Println("=================================================")
						fullCode = editor.GetSourceCode()
						loop.Set("full_code", fullCode)

						os.RemoveAll(filename)
						os.WriteFile(filename, []byte(fullCode), 0644)

						errMsg, hasBlockingErrors := checkCodeAndFormatErrors(fullCode)
						if hasBlockingErrors {
							op.DisallowNextLoopExit()
						}
						msg = utils.ShrinkTextBlock(fmt.Sprintf("line[%v-%v]:\n", modifyStartLine, modifyEndLine)+partialCode, 256)
						if errMsg != "" {
							msg += "\n\n--[linter]--\nWriting Code Linter Check:\n" + utils.PrefixLines(utils.ShrinkTextBlock(errMsg, 2048), "  ")
							op.Feedback(errMsg)
						} else {
							msg += "\n\n--[linter]--\nNo issues found in the modified code segment."
						}
						r.AddToTimeline("code_modified", msg)
						log.Infof("modify_code done: hasBlockingErrors=%v, will show errors in next iteration", hasBlockingErrors)
						loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "modify_code", partialCode)

						if errMsg != "" {
							invoker.AddToTimeline("advice", "use 'query_document' to find more syntax sample or docs")
						}
					},
				),
			)
		},
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed", schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG)
	}
}
