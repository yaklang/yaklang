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
					yakCode := loop.Get("yak_code")
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
				reactloops.WithRegisterLoopAction(
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
								aitool.WithParam_Description(`Keywords or phrases to search in Yaklang documentation (supports both Chinese and English). Common patterns:

**High-Frequency Functions (use exact names)**:
• Network: 'poc.HTTP', 'poc.HTTPEx', 'poc.Get', 'poc.Post', 'servicescan.Scan', 'synscan.Scan'
• File: 'file.ReadFile', 'file.Save', 'filesys.Recursive', 'zip.CompressRaw', 'zip.Recursive'
• String: 'str.Split', 'str.Join', 'str.Contains', 'str.Replace', 'str.TrimPrefix'
• Codec: 'codec.DecodeBase64', 'codec.EncodeBase64', 'json.dumps', 'json.loads'
• Database: 'db.Query', 'db.Exec', 'risk.NewRisk'

**Function Options (exact option names)**:
• HTTP: 'poc.timeout', 'poc.json', 'poc.header', 'poc.cookie', 'poc.body', 'poc.retry'
• Scan: 'servicescan.concurrent', 'servicescan.active', 'servicescan.web', 'servicescan.all'
• File: 'filesys.onFileStat', 'file.IsDir', 'file.IsFile'

**Feature Keywords (Chinese or English)**:
• Chinese: 'HTTP发包', 'HTTP请求', '端口扫描', '服务扫描', '文件读取', '文件写入', '字符串处理', 'JSON解析', '并发编程', '错误处理', '正则匹配'
• English: 'send request', 'port scan', 'file operation', 'string processing', 'error handling', 'concurrent', 'goroutine', 'channel'

**Common Patterns**:
• Error handling: 'die(err)', '~', 'try-catch', 'defer-recover'
• Concurrency: 'go func', 'sync.NewWaitGroup', 'sync.NewSizedWaitGroup', 'channel'
• Fuzzing: 'fuzz.HTTPRequest', 'fuzztag', '{{参数}}'

**Example combinations**:
- For HTTP: ["poc.HTTP", "HTTP发包", "poc.timeout", "发送请求"]
- For scanning: ["servicescan.Scan", "端口扫描", "servicescan.concurrent", "指纹识别"]
- For files: ["file.ReadFile", "文件读取", "filesys.Recursive", "文件遍历"]`),
							),
							aitool.WithStringArrayParam(
								"regexp",
								aitool.WithParam_Description(`Regular expressions to match specific code patterns in Yaklang documentation. Use for precise structural matching:

**Function Call Patterns**:
• Library functions: '\w+\.\w+\(' - matches any library.function() calls
• Specific library: 'poc\.\w+\(' - matches all poc.* functions
• HTTP methods: 'poc\.(HTTP|HTTPEx|Get|Post|Do)\(' - matches HTTP-related functions
• File operations: 'file\.(ReadFile|Save|WriteFile)\(' - matches file functions
• String utils: 'str\.(Split|Join|Contains|Replace)\(' - matches string functions

**Configuration Options**:
• HTTP options: 'poc\.(timeout|json|header|cookie|body|query|postParams)\(' - matches HTTP config
• Scan options: 'servicescan\.(concurrent|timeout|active|web|all)\(' - matches scan config
• Context options: '\.(https|port|host|redirectTimes|retryTimes)\(' - matches connection config

**Control Flow & Error Handling**:
• Error handling: 'die\(|~\s*$|try\s*\{|defer.*recover\(' - matches error patterns
• Concurrency: 'go\s+func|sync\.New\w+WaitGroup|make\(chan\s+' - matches concurrent code
• Loops: 'for\s+\w+\s+in\s+|for\s+\w+\s*:?=?\s*range\s+' - matches for-in/range loops

**Code Structure**:
• Function definition: '(func|fn|def)\s+\w+\s*\(' - matches function declarations
• Variable assignment: '\w+\s*:?=\s*\w+\.\w+\(' - matches var = lib.func() pattern
• Method chaining: '\)\s*\.\s*\w+\(' - matches chained method calls

**Example patterns**:
- HTTP workflow: ['poc\.(HTTP|Get|Post)\(', 'poc\.(timeout|json|header)\(', '~\s*$']
- File processing: ['file\.\w+\(', 'filesys\.Recursive\(', 'for.*range.*']
- Error handling: ['die\(|~', 'try\s*\{.*\}\s*catch', 'defer.*recover\(']
- Concurrency: ['go\s+func', 'sync\.New.*WaitGroup', '<-.*chan|chan\s*<-']

**Note**: Patterns are case-sensitive. Use '\s+' for whitespace, '\w+' for identifiers, '.*' for wildcards.`),
							),
						),
					},
					func(r *reactloops.ReActLoop, action *aicommon.Action) error {
						payloads := action.GetInvokeParams("query_document_payload")
						if len(payloads.GetStringSlice("keywords")) == 0 && len(payloads.GetStringSlice("regexp")) == 0 {
							return utils.Error("query_document action must have at least one keyword or regexp in 'query_document_payload'")
						}
						return nil
					},
					func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {

						payloads := action.GetInvokeParams("query_document_payload")

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
						if documentResults != "" {
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
							msg += "--[DOCS]--\n" + documentResults
							op.Feedback(msg)
						}

						if len(documentResults) > 0 {
							log.Infof("================== document query =====================\n"+
								"%v\n===================== document result ===================\n"+
								"%v\n=================================================",
								utils.InterfaceToString(payloads),
								documentResults,
							)
							invoker.AddToTimeline("query_yaklang_docs_result", documentResults)
						}
					},
				),
				reactloops.WithRegisterLoopAction(
					"write_code",
					"if the current code is empty or need to create an initial version",
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
				reactloops.WithRegisterLoopAction(
					"modify_code",
					"do NOT use this action to create new code file, ONLY use it to modify existing code. Modify the code between the specified line numbers (inclusive). The line numbers are 1-based, meaning the first line of the file is line 1. Ensure that the 'modify_start_line' is less than or equal to 'modify_end_line'.",
					[]aitool.ToolOption{},
					func(l *reactloops.ReActLoop, action *aicommon.Action) error {
						start := action.GetInt("modify_start_line")
						end := action.GetInt("modify_end_line")
						if start <= 0 || end <= 0 || end < start {
							return utils.Error("modify_code action must have valid 'modify_start_line' and 'modify_end_line' parameters")
						}
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
					},
				),
			)
		},
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed", schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG)
	}
}
