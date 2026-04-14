package loop_http_fuzztest

import (
	"bytes"
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed prompts/persistent_instruction.txt
var instruction string

//go:embed prompts/reactive_data.txt
var reactiveData string

//go:embed prompts/reflection_output_example.txt
var outputExample string

const LoopHTTPFuzztestName = "http_fuzztest"
const loopHTTPFuzztestHTTPSource = "reactloop_http_fuzztest"

func init() {
	err := reactloops.RegisterLoopFactory(
		LoopHTTPFuzztestName,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			// 创建预设选项
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(false),
				reactloops.WithAllowToolCall(true),
				reactloops.WithAITagFieldWithAINodeId("GEN_PACKET", generatedPacketContentField, "http_flow", aicommon.TypeCodeHTTPRequest),
				reactloops.WithAITagFieldWithAINodeId("GEN_MODIFIED_PACKET", modifiedPacketContentField, "http_flow", aicommon.TypeCodeHTTPRequest),
				reactloops.WithOverrideLoopAction(loopActionDirectlyAnswerHTTPFuzztest),
				reactloops.WithInitTask(buildInitTask(r)),
				BuildOnPostIterationHook(r),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					originalRequest := loop.Get("original_request")
					originalRequestSummary := loop.Get("original_request_summary")
					currentRequestSummary := getCurrentRequestSummary(loop)
					previousRequestSummary := loop.Get("previous_request_summary")
					requestChangeSummary := loop.Get("request_change_summary")
					requestModificationReason := loop.Get("request_modification_reason")
					requestReviewDecision := loop.Get("request_review_decision")
					representativeRequest := loop.Get("representative_request")
					representativeResponse := loop.Get("representative_response")
					representativeHiddenIndex := loop.Get("representative_httpflow_hidden_index")
					diffResult := loop.Get("diff_result")
					verificationResult := loop.Get("verification_result")
					securityKnowledge := loop.Get("security_knowledge")
					recentActionsSummary := buildLoopHTTPFuzzRecentActionsPrompt(loop)
					testedPayloadSummary := buildLoopHTTPFuzzTestedPayloadPrompt(loop)
					fuzztagReference := loop.Get(loopHTTPFuzzFuzztagReferenceKey)
					payloadGroupsReference := loop.Get(loopHTTPFuzzPayloadGroupsReferenceKey)

					renderMap := map[string]any{
						"OriginalRequest":           originalRequest,
						"OriginalRequestSummary":    originalRequestSummary,
						"CurrentRequestSummary":     currentRequestSummary,
						"PreviousRequestSummary":    previousRequestSummary,
						"RequestChangeSummary":      requestChangeSummary,
						"RequestModificationReason": requestModificationReason,
						"RequestReviewDecision":     requestReviewDecision,
						"RepresentativeRequest":     representativeRequest,
						"RepresentativeResponse":    representativeResponse,
						"RepresentativeHiddenIndex": representativeHiddenIndex,
						"DiffResult":                diffResult,
						"VerificationResult":        verificationResult,
						"SecurityKnowledge":         securityKnowledge,
						"RecentActionsSummary":      recentActionsSummary,
						"TestedPayloadSummary":      testedPayloadSummary,
						"FuzztagReference":          fuzztagReference,
						"PayloadGroupsReference":    payloadGroupsReference,
						"Nonce":                     nonce,
						"FeedbackMessages":          feedbacker.String(),
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				// Register set_http_request action (must be called first)
				setHTTPRequestAction(r),
				patchHTTPRequestAction(r),
				modifyHTTPRequestAction(r),
				// Register fuzz actions
				fuzzMethodAction(r),
				fuzzPathAction(r),
				fuzzHeaderAction(r),
				fuzzGetParamsAction(r),
				fuzzBodyAction(r),
				fuzzCookieAction(r),
				generateAndSendPacketAction(r),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(LoopHTTPFuzztestName, r, preset...)
		},
		reactloops.WithLoopDescription("HTTP request fuzzing and response diff analysis for security testing"),
		reactloops.WithLoopDescriptionZh("HTTP 安全模糊测试模式：对 HTTP 请求进行变异、发送和响应差异分析，用于发现潜在安全问题。"),
		reactloops.WithVerboseName("HTTP Fuzz Test"),
		reactloops.WithVerboseNameZh("HTTP 安全模糊测试"),
		reactloops.WithLoopUsagePrompt("Use when user wants to fuzz HTTP requests and analyze security-relevant response differences. First use 'set_http_request' to set the target request, then use 'patch_http_request' for fine-grained single-step packet edits, auth/header/body format transforms, or repair, fuzz actions (fuzz_method, fuzz_path, fuzz_header, fuzz_get_params, fuzz_body, fuzz_cookie), 'modify_http_request' when the current packet must be revised with visible merge details via a full raw packet, 'generate_and_send_packet' when a complete raw packet must be constructed and sent, or 'directly_answer' for short testing-process Q&A."),
		reactloops.WithLoopOutputExample(`
* When user requests to fuzz HTTP request:
  {"@action": "http_fuzztest", "human_readable_thought": "I need to fuzz HTTP request parameters to find vulnerabilities"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", LoopHTTPFuzztestName, err)
	}
}

var urlPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

// buildInitTask creates the initialization task handler
func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		emitter := r.GetConfig().GetEmitter()
		config := r.GetConfig()

		invoker := loop.GetInvoker()
		bootstrapLoopHTTPFuzzFuzztagContext(loop, config.GetDB())

		// TBD: 检查是否已经有 fuzz_request 了（可能是用户之前的交互设置的），如果有就直接继续
		haveReq := loop.Get("fuzz_request") // Just to ensure the key exists in the loop state
		if haveReq == "" {
			// TBD: 如果没有，就尝试从用户输入中引导提取 HTTP 请求信息来初始化 fuzz_request
			bootstrapResult := tryBootstrapFuzzRequestFromUserInput(r, loop, task)
			switch bootstrapResult {
			case "raw":
				loop.Set("bootstrap_source", "user_input_raw")
				emitter.EmitThoughtStream(task.GetIndex(), "Initialized fuzz request from extracted HTTP packet in user input.")
			case "url":
				loop.Set("bootstrap_source", "user_input_url")
				emitter.EmitThoughtStream(task.GetIndex(), "No raw packet found. Initialized fuzz request from extracted URL.")
			default:
				if restoreLoopHTTPFuzzSessionContext(loop, r) {
					emitter.EmitThoughtStream(task.GetIndex(), "Restored the original HTTP packet and latest vulnerability analysis from the current session.")
					invoker.AddToTimeline("http_fuzztest_restore", "Restored HTTP fuzz session context from persistent session history")
				} else {
					// TBD: 不知道怎么测试，也不知道数据包
					emitter.EmitThoughtStream(task.GetIndex(), "No valid HTTP packet/URL extracted from user input, and no previous packet was restored from this session. Please call set_http_request or provide a URL/raw packet before fuzz actions.")
					operator.Done()
					return
				}
			}
		}

		// TBD: 使用 liteforge 来处理一下
		action, err := invoker.InvokeSpeedPriorityLiteForge(task.GetContext(), "http_fuzztest_init_booststrap", `
从安全模糊测试的角度来说，这个 HTTP 请求可能有哪些测试要点和灵感提示？请结合请求的结构、参数、头部等信息，给出一些模糊测试的思路和建议，帮助后续的 fuzzing 设计。

案例如下：
1. 看起来这是一个登录接口的请求，URL 中有 /login，参数里有 username 和 password，这些都是典型的模糊测试目标。可以尝试在 username 和 password 参数里进行 SQL 注入、XSS、越权访问等测试。
2. 请求头里有一个 User-Agent 字段，可以尝试在这个字段里进行模糊测试，比如注入恶意 payload 来测试服务器对 User-Agent 的处理。
3. 如果请求里有 Cookie，Cookie 也是一个重要的模糊测试目标，可以尝试修改 Cookie 的值来测试会话管理和权限控制等方面的安全性。
4. 如果请求里有 JSON 或者其他结构化的 body，可以针对这些参数进行模糊测试，尝试注入特殊字符、超长字符串、边界值等来测试服务器的健壮性和安全性。
5. 如果参数名像 id、uid、userId、orderId、file、path、tenant、account 这一类对象标识符，要考虑参数遍历、对象切换、资源编号枚举，验证是否存在 IDOR、越权读取或信息泄漏。
6. 如果响应里可能包含用户名、邮箱、手机号、路径、版本号、调试报错、SQL 报错、模板报错等内容，也要把信息泄漏当作漏洞方向，而不是只盯着 SQL 注入和 XSS。
7. 如果当前数据包明显不合理，例如缺失 Host、User-Agent、Accept、Content-Type，或者 method/path/body 组合明显不匹配，先考虑修复数据包再测，尽量让请求更像真实客户端流量。
8. 如果需要修复数据包，可以优先补齐 User-Agent、Accept、Accept-Language、Connection、Referer、Origin 等常见头部，并保持与当前接口语义一致，再继续做 fuzz。

`, []aitool.ToolOption{
			aitool.WithStringParam("thought", aitool.WithParam_Description("针对这个 HTTP 请求的模糊测试要点和灵感提示")),
		}, aicommon.WithGeneralConfigStreamableFieldWithNodeId("thought", "quick_plan"))
		if err != nil {
			log.Warnf("http_fuzztest init booststrap failed: %v", err)
			return
		}
		invoker.AddToTimeline("http_fuzztest_init_booststrap", "Bootstrap insights: "+action.GetString("thought"))
	}
}

func tryBootstrapFuzzRequestFromUserInput(r aicommon.AIInvokeRuntime, loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) string {
	userInput := strings.TrimSpace(task.GetUserInput())
	if userInput == "" {
		return ""
	}

	prompt := `
请从用户输入中提取可用于 HTTP 安全测试的请求信息。

输出规则：
1) 如果用户提供了原始 HTTP 请求报文（请求行 + Host 头），将完整报文放到 raw_http_request。
2) 如果没有原始报文但有 URL，提取到 url，并给出 method（无明确时使用 GET）。
3) 若无法提取，返回空字符串。

<|USER_INPUT_{{ .nonce }}|>
{{ .userInput }}
<|USER_INPUT_END_{{ .nonce }}|>
`

	renderedPrompt := utils.MustRenderTemplate(prompt, map[string]any{
		"nonce":     utils.RandStringBytes(4),
		"userInput": userInput,
	})

	action, err := r.InvokeSpeedPriorityLiteForge(
		task.GetContext(),
		"extract-http-request-from-user-input",
		renderedPrompt,
		[]aitool.ToolOption{
			aitool.WithStringParam("raw_http_request", aitool.WithParam_Description("完整原始 HTTP 请求报文，无法提取则为空")),
			aitool.WithStringParam("url", aitool.WithParam_Description("提取到的 URL，无法提取则为空")),
			aitool.WithStringParam("method", aitool.WithParam_Description("提取到的 HTTP 方法，默认 GET")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("说明提取依据和置信度")),
		},
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("http_flow", "raw_http_request"),
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("thought", "url"),
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("thought", "reason"),
	)
	if err != nil {
		log.Warnf("failed to extract HTTP request from user input: %v", err)
		return ""
	}

	rawPacket := ""
	urlStr := ""
	method := "GET"
	reason := ""
	if action != nil {
		rawPacket = strings.TrimSpace(action.GetString("raw_http_request"))
		urlStr = strings.TrimSpace(action.GetString("url"))
		method = strings.TrimSpace(action.GetString("method"))
		reason = strings.TrimSpace(action.GetString("reason"))
	}

	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(method)

	if rawPacket != "" {
		rawIsHTTPS := strings.HasPrefix(strings.ToLower(urlStr), "https://")
		if initFuzzRequestFromRaw(loop, r, rawPacket, rawIsHTTPS) {
			r.AddToTimeline("http_request_bootstrap", fmt.Sprintf("Initialized from extracted raw packet (%s)", reason))
			return "raw"
		}
	}

	if urlStr == "" {
		urlStr = extractURLFromUserInput(userInput)
	}
	if urlStr != "" {
		if initFuzzRequestFromURL(loop, r, urlStr, method) {
			r.AddToTimeline("http_request_bootstrap", fmt.Sprintf("Initialized from extracted URL: %s (%s)", urlStr, reason))
			return "url"
		}
	}

	if reason != "" {
		r.AddToTimeline("http_request_bootstrap", fmt.Sprintf("Initialization skipped: %s", reason))
	}
	return "none"
}

func initFuzzRequestFromRaw(loop *reactloops.ReActLoop, runtime aicommon.AIInvokeRuntime, rawPacket string, isHTTPS bool) bool {
	_, err := applyLoopHTTPFuzzRequestChange(loop, runtime, &loopHTTPFuzzRequestChange{
		RawRequest:          rawPacket,
		IsHTTPS:             isHTTPS,
		SourceAction:        "user_input_raw",
		EventOp:             loopHTTPFuzzRequestEventOpReplace,
		ResetBaseline:       true,
		ClearActionTracking: true,
		EmitEvent:           true,
		EmitEditablePacket:  true,
		PersistSession:      true,
		Task:                loop.GetCurrentTask(),
	})
	if err != nil {
		log.Warnf("failed to build fuzz request from extracted raw packet: %v", err)
		return false
	}
	return true
}

func initFuzzRequestFromURL(loop *reactloops.ReActLoop, runtime aicommon.AIInvokeRuntime, urlStr, method string) bool {
	isHTTPS, packet, err := lowhttp.ParseUrlToHttpRequestRaw(method, urlStr)
	if err != nil {
		log.Warnf("failed to build request from URL %s: %v", urlStr, err)
		return false
	}

	_, err = applyLoopHTTPFuzzRequestChange(loop, runtime, &loopHTTPFuzzRequestChange{
		RawRequest:          string(packet),
		IsHTTPS:             isHTTPS,
		SourceAction:        "user_input_url",
		EventOp:             loopHTTPFuzzRequestEventOpReplace,
		ResetBaseline:       true,
		ClearActionTracking: true,
		EmitEvent:           true,
		EmitEditablePacket:  true,
		PersistSession:      true,
		Task:                loop.GetCurrentTask(),
	})
	if err != nil {
		log.Warnf("failed to build fuzz request from URL packet: %v", err)
		return false
	}
	return true
}

func extractURLFromUserInput(userInput string) string {
	if userInput == "" {
		return ""
	}
	matches := urlPattern.FindAllString(userInput, -1)
	if len(matches) == 0 {
		return ""
	}
	return strings.TrimSpace(matches[0])
}

// searchByKeywords searches knowledge base by keywords
func searchByKeywords(db *gorm.DB, keywords []string) string {
	if db == nil || len(keywords) == 0 {
		return ""
	}

	var results strings.Builder
	results.WriteString("\n=== Keyword Search Results ===\n")

	foundCount := 0
	seenIDs := make(map[uint]bool) // 用于去重

	for _, keyword := range keywords {
		if keyword == "" {
			continue
		}

		// 使用现有的 API 搜索知识库
		filter := &ypb.SearchKnowledgeBaseEntryFilter{
			Keyword: keyword,
		}
		paging := &ypb.Paging{
			Page:  1,
			Limit: 5,
		}

		_, entries, err := yakit.QueryKnowledgeBaseEntryPaging(db, filter, paging)
		if err != nil {
			log.Warnf("keyword search failed for '%s': %v", keyword, err)
			continue
		}

		if len(entries) > 0 {
			results.WriteString(fmt.Sprintf("\n--- Keyword: %s (Found %d) ---\n", keyword, len(entries)))
			for i, entry := range entries {
				// 去重
				if seenIDs[entry.ID] {
					continue
				}
				seenIDs[entry.ID] = true

				results.WriteString(fmt.Sprintf("[%d] %s\n", i+1, entry.KnowledgeTitle))
				content := entry.KnowledgeDetails
				if ytoken.CalcTokenCount(content) > 500 {
					content = content[:500] + "..."
				}
				results.WriteString(content + "\n\n")
				foundCount++
			}
		}
	}

	if foundCount == 0 {
		return ""
	}

	results.WriteString("=== End of Keyword Search Results ===\n")
	return results.String()
}

// searchBySemantic searches knowledge base by semantic questions
func searchBySemantic(db *gorm.DB, collectionName string, questions []string) string {
	if db == nil || len(questions) == 0 {
		return ""
	}

	ragSys, err := rag.GetRagSystem(collectionName, rag.WithDB(db))
	if err != nil {
		log.Warnf("RAG system not available: %v", err)
		return ""
	}

	var results strings.Builder
	results.WriteString("\n=== Semantic Search Results ===\n")

	// 使用 map 去重
	type ResultKey struct {
		DocID string
	}
	allResultsMap := make(map[ResultKey]rag.SearchResult)

	for _, question := range questions {
		if question == "" {
			continue
		}

		log.Infof("semantic searching: %s", question)

		searchResults, err := ragSys.QueryTopN(question, 10, 0.3)
		if err != nil {
			log.Warnf("semantic search failed for '%s': %v", question, err)
			continue
		}

		for _, result := range searchResults {
			var docID string
			if result.KnowledgeBaseEntry != nil {
				docID = fmt.Sprintf("kb_%d_%s", result.KnowledgeBaseEntry.ID, result.KnowledgeBaseEntry.KnowledgeTitle)
			} else if result.Document != nil {
				docID = result.Document.ID
			} else {
				continue
			}

			key := ResultKey{DocID: docID}
			existing, exists := allResultsMap[key]
			if !exists || result.Score > existing.Score {
				allResultsMap[key] = *result
			}
		}
	}

	if len(allResultsMap) == 0 {
		return ""
	}

	results.WriteString(fmt.Sprintf("Found %d unique matches:\n\n", len(allResultsMap)))

	displayCount := 0
	maxDisplay := 10
	for _, result := range allResultsMap {
		if displayCount >= maxDisplay {
			break
		}

		results.WriteString(fmt.Sprintf("--- [%d] Score: %.3f ---\n", displayCount+1, result.Score))

		var content string
		if result.KnowledgeBaseEntry != nil {
			results.WriteString(fmt.Sprintf("Title: %s\n", result.KnowledgeBaseEntry.KnowledgeTitle))
			content = result.KnowledgeBaseEntry.KnowledgeDetails
		} else if result.Document != nil {
			content = result.Document.Content
		}

		if ytoken.CalcTokenCount(content) > 800 {
			content = content[:800] + "\n[... content truncated ...]"
		}

		results.WriteString(content + "\n\n")
		displayCount++
	}

	if len(allResultsMap) > maxDisplay {
		results.WriteString(fmt.Sprintf("\n... (%d more results not shown)\n", len(allResultsMap)-maxDisplay))
	}

	results.WriteString("=== End of Semantic Search Results ===\n")
	return results.String()
}
