package loop_yaklangcode

import (
	"bytes"
	"github.com/jinzhu/gorm"

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
		"查询Yaklang代码文档和库函数。支持关键字搜索（使用动宾结构，如'端口扫描'、'文件读取'）、正则表达式匹配、库名查询（如'str'、'http'）和函数模糊搜索（如'*Split*'、'str.Join'）。当你需要了解某个功能如何实现、查找特定函数或学习库的用法时使用此工具。",
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
				aitool.WithStringArrayParam(
					"lib_names",
					aitool.WithParam_Description(`Names of Yaklang built-in libraries. If you want to view members of certain libraries, you can set this parameter to display the library's content and functions to help you find the desired functionality`),
				),
				aitool.WithStringArrayParam(
					"lib_function_globs",
					aitool.WithParam_Description(`Functions in built-in libraries. You can directly use this to search for function names and which library they belong to. This is particularly useful when you use this search, for example, if you search for '*Rand*', you can find Rand-related functions and their locations and basic declarations in yaklang`),
				),
			),
		},
		[]*reactloops.LoopStreamField{},
		func(r *reactloops.ReActLoop, action *aicommon.Action) error {
			payloads := action.GetInvokeParams("query_document_payload")

			// Check if at least one search parameter is provided
			hasKeywords := len(payloads.GetStringSlice("keywords")) > 0
			hasRegexp := len(payloads.GetStringSlice("regexp")) > 0
			hasLibNames := len(payloads.GetStringSlice("lib_names")) > 0
			hasLibFunctionGlobs := len(payloads.GetStringSlice("lib_function_globs")) > 0

			if !hasKeywords && !hasRegexp && !hasLibNames && !hasLibFunctionGlobs {
				return utils.Error("query_document action must have at least one of: keywords, regexp, lib_names, or lib_function_globs in 'query_document_payload'")
			}

			// Validate lib_names if provided
			if hasLibNames {
				libNames := payloads.GetStringSlice("lib_names")
				for _, libName := range libNames {
					if libName == "" {
						return utils.Error("lib_names cannot contain empty strings")
					}
				}
			}

			// Validate lib_function_globs if provided
			if hasLibFunctionGlobs {
				globs := payloads.GetStringSlice("lib_function_globs")
				for _, glob := range globs {
					if glob == "" {
						return utils.Error("lib_function_globs cannot contain empty strings")
					}
				}
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

var ragQueryDocumentAction = func(r aicommon.AIInvokeRuntime, db *gorm.DB, enhanceCollectionName string) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"rag_search",
		"Yaklang资料和库函数的资料补充增强工具，支持直接限定搜索某个内置库，同样支持通过提出问题（question）搜索对应的 Yaklang 文档内容和库函数用法 （answer）。当你需要了解某个功能如何实现、查找特定函数或学习库的用法时推荐使用此工具。",
		[]aitool.ToolOption{
			aitool.WithStructParam(
				"rag_search_payload",
				[]aitool.PropertyOption{
					aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'query_document'."),
				},
				aitool.WithStringArrayParam(
					"keywords",
					aitool.WithParam_Description(`Keywords or phrases to search in Yaklang documentation (supports both Chinese and English). Common patterns:`)),
				aitool.WithStringArrayParam(
					"question",
					aitool.WithParam_Description(`Questions to search in Yaklang documentation to get answers.`),
				),
			),
		},
		[]*reactloops.LoopStreamField{},
		func(r *reactloops.ReActLoop, action *aicommon.Action) error {
			payloads := action.GetInvokeParams("rag_search")

			// Check if at least one search parameter is provided
			hasKeywords := len(payloads.GetStringSlice("keywords")) > 0
			hasQuestion := len(payloads.GetStringSlice("question")) > 0

			if !hasKeywords && !hasQuestion {
				return utils.Error("query_document action must have at least one of: keywords or question, in 'rag_search_payload'")
			}

			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {

			payloads := action.GetInvokeParams("query_document")

			searching := payloads.Dump()
			loop.GetEmitter().EmitTextPlainTextStreamEvent(
				"rag_search_yaklang_document",
				bytes.NewReader([]byte(searching)),
				loop.GetCurrentTask().GetIndex(),
				func() {
					log.Infof("searching yaklang document: \n%v", searching)
				},
			)

			invoker := loop.GetInvoker()
			invoker.AddToTimeline("rag_search_yaklang_document", "AI decided to rag search with payload: "+utils.InterfaceToString(payloads))

			searchResult, ok := handleRAGQueryDocument(r, db, enhanceCollectionName, payloads)
			if !ok {
				invoker.AddToTimeline("rag_search_yaklang_result", "No document searcher available, cannot perform document query, maybe keyword or regexp is invalid: "+utils.InterfaceToString(payloads))
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

			if len(searchResult) > 0 {
				log.Infof("\n================== document query =====================\n"+
					"%v\n===================== document result ===================\n"+
					"%v\n=================================================",
					utils.InterfaceToString(payloads),
					searchResult,
				)
				invoker.AddToTimeline("query_yaklang_docs_result", searchResult)
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
					"docs":     searchResult,
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
