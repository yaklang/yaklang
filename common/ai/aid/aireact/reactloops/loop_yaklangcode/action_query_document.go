package loop_yaklangcode

import (
	"bytes"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

var queryDocumentAction = func(r aicommon.AIInvokeRuntime, docSearcher *ziputil.ZipGrepSearcher) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
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
				nonce := utils.RandStringBytes(4)
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
	)
}
