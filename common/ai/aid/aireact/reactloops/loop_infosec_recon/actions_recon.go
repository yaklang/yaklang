package loop_infosec_recon

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const keyReconLog = "infosec_recon_log"

func appendInfosecReconLog(loop *reactloops.ReActLoop, content string) {
	old := loop.Get(keyReconLog)
	if old == "" {
		loop.Set(keyReconLog, content)
	} else {
		loop.Set(keyReconLog, old+"\n\n"+content)
	}
}

func makeReconRequireAction(
	actionName string,
	targetToolName string,
	desc string,
) func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
		return reactloops.WithRegisterLoopAction(
			actionName,
			desc,
			nil,
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				invoker := loop.GetInvoker()
				ctx := loop.GetConfig().GetContext()
				task := loop.GetCurrentTask()
				if task != nil && !utils.IsNil(task.GetContext()) {
					ctx = task.GetContext()
				}

				thought := action.GetString("human_readable_thought")
				if thought != "" {
					invoker.AddToTimeline(actionName+"_intent", thought)
				}
				reactloops.EmitActionLog(loop, infosecReconToolNodeID, fmt.Sprintf("开始: %s / Start: %s", actionName, targetToolName))
				reactloops.EmitStatus(loop, "执行侦察工具中 / Executing recon tool...")

				result, directly, err := invoker.ExecuteToolRequiredAndCall(ctx, targetToolName)
				if err != nil {
					log.Warnf("%s tool call failed: %v", targetToolName, err)
					failMsg := fmt.Sprintf(
						"%s FAILED: %v. Try different parameters or another tool.",
						actionName, err)
					invoker.AddToTimeline(actionName+"_failed", failMsg)
					op.Feedback(failMsg)
					op.Continue()
					return
				}
				if directly {
					invoker.AddToTimeline(actionName+"_skipped", "user chose to skip tool execution")
					op.Feedback(fmt.Sprintf("%s was skipped by user.", actionName))
					op.Continue()
					return
				}
				if result == nil {
					emptyMsg := fmt.Sprintf("%s returned no result.", actionName)
					invoker.AddToTimeline(actionName+"_empty", emptyMsg)
					op.Feedback(emptyMsg)
					op.Continue()
					return
				}
				content := utils.InterfaceToString(result.Data)
				if result.Error != "" {
					invoker.AddToTimeline(actionName+"_error", result.Error)
				}
				thoughtHint := utils.ShrinkString(thought, 120)
				entry := fmt.Sprintf("=== %s: %s ===\n%s", actionName, thoughtHint, utils.ShrinkString(content, 8192))
				appendInfosecReconLog(loop, entry)
				invoker.AddToTimeline(actionName+"_result", fmt.Sprintf("[%s] %s\n\n%s", targetToolName, thoughtHint, utils.ShrinkString(content, 4096)))
				op.Feedback(fmt.Sprintf("%s completed (%d bytes).", actionName, len(content)))
				reactloops.EmitStatus(loop, "完成 / Complete")
				reactloops.EmitActionLog(loop, infosecReconToolNodeID, fmt.Sprintf("完成: %s (%d bytes) / Done: %s (%d bytes)", actionName, len(content), targetToolName, len(content)))
				op.Continue()
			},
		)
	}
}

func makeToolForwardAction(
	actionName string,
	targetToolName string,
	desc string,
	toolOpts []aitool.ToolOption,
) func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
		return reactloops.WithRegisterLoopAction(
			actionName,
			desc,
			toolOpts,
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				invoker := loop.GetInvoker()
				ctx := loop.GetConfig().GetContext()
				task := loop.GetCurrentTask()
				if task != nil && !utils.IsNil(task.GetContext()) {
					ctx = task.GetContext()
				}
				reactloops.EmitActionLog(loop, infosecReconToolNodeID, fmt.Sprintf("开始: %s / Start: %s", actionName, targetToolName))
				reactloops.EmitStatus(loop, "执行侦察工具中 / Executing recon tool...")

				params := action.GetParams()
				result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, targetToolName, params)
				if err != nil {
					log.Warnf("%s call failed: %v", targetToolName, err)
					op.Feedback(fmt.Sprintf("%s failed: %v", targetToolName, err))
					op.Continue()
					return
				}
				content := ""
				if result != nil {
					content = utils.InterfaceToString(result.Data)
				}
				entry := fmt.Sprintf("=== %s ===\n%s", actionName, utils.ShrinkString(content, 8192))
				appendInfosecReconLog(loop, entry)
				invoker.AddToTimeline(fmt.Sprintf("%s_result", actionName), utils.ShrinkString(content, 4096))
				op.Feedback(fmt.Sprintf("%s completed (%d bytes)", targetToolName, len(content)))
				reactloops.EmitStatus(loop, "完成 / Complete")
				reactloops.EmitActionLog(loop, infosecReconToolNodeID, fmt.Sprintf("完成: %s (%d bytes) / Done: %s (%d bytes)", actionName, len(content), targetToolName, len(content)))
				op.Continue()
			},
		)
	}
}

var (
	scanPortAction      = makeReconRequireAction("scan_port", "scan_port", "Port scan (authorized targets only). Describe hosts and ports in human_readable_thought for the tool request phase.")
	simpleCrawlerAction = makeReconRequireAction("simple_crawler", "simple_crawler", "Lightweight web crawl to discover URLs and pages. Describe start URL and depth in human_readable_thought.")
	bannerGrabAction    = makeReconRequireAction("banner_grab", "banner_grab", "TCP banner grab on host:port. Describe targets in human_readable_thought.")
	digAction           = makeReconRequireAction("dig", "dig", "DNS lookups. Describe domain and record types in human_readable_thought.")
	subdomainScanAction = makeReconRequireAction("subdomain_scan", "subdomain_scan", "Subdomain brute / scan for a domain. Describe target domain in human_readable_thought.")
	networkSpaceAction  = makeReconRequireAction("network_space_search", "network_space_search", "Space search engine query (FOFA/Shodan etc., needs API keys). Describe engine and query in human_readable_thought.")
)

var webSearchAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"web_search",
		"OSINT web search for exposed docs, tech stack, or related assets (authorized use only).",
		[]aitool.ToolOption{
			aitool.WithStringParam("query", aitool.WithParam_Required(true), aitool.WithParam_Description("Search query.")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}
			query := action.GetString("query")
			reactloops.EmitActionLog(loop, infosecReconToolNodeID, fmt.Sprintf("开始: %s / Start: %s", query, query))
			reactloops.EmitStatus(loop, "联网搜索中 / Searching the web...")

			params := aitool.InvokeParams{"query": query}
			result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, "web_search", params)
			if err != nil {
				failMsg := fmt.Sprintf("web_search FAILED for '%s': %v", query, err)
				invoker.AddToTimeline("web_search_failed", failMsg)
				op.Feedback(failMsg)
				op.Continue()
				return
			}
			content := ""
			if result != nil {
				content = utils.InterfaceToString(result.Data)
			}
			appendInfosecReconLog(loop, fmt.Sprintf("=== web_search: %s ===\n%s", query, utils.ShrinkString(content, 4096)))
			invoker.AddToTimeline("web_search_result", utils.ShrinkString(content, 2048))
			op.Feedback(fmt.Sprintf("web_search completed for: '%s' (%d bytes)", query, len(content)))
			reactloops.EmitStatus(loop, "完成 / Complete")
			reactloops.EmitActionLog(loop, infosecReconToolNodeID, fmt.Sprintf("完成: %s (%d bytes) / Done: %s (%d bytes)", query, len(content), query, len(content)))
			op.Continue()
		},
	)
}

var (
	readFileAction = makeToolForwardAction(
		"read_file", "read_file",
		"Read a local text file (e.g. saved crawl or JS).",
		[]aitool.ToolOption{
			aitool.WithStringParam("path", aitool.WithParam_Required(true), aitool.WithParam_Description("Absolute file path.")),
			aitool.WithIntegerParam("offset", aitool.WithParam_Default(0)),
			aitool.WithIntegerParam("chunk_size", aitool.WithParam_Default(20480)),
		},
	)
	findFilesAction = makeToolForwardAction(
		"find_files", "find_file",
		"Find files under a directory by pattern.",
		[]aitool.ToolOption{
			aitool.WithStringParam("dir", aitool.WithParam_Required(true), aitool.WithParam_Description("Root directory.")),
			aitool.WithStringParam("pattern", aitool.WithParam_Required(true), aitool.WithParam_Description("Glob pattern.")),
			aitool.WithIntegerParam("max", aitool.WithParam_Default(20)),
		},
	)
	grepTextAction = makeToolForwardAction(
		"grep_text", "grep",
		"Search text / regex in files.",
		[]aitool.ToolOption{
			aitool.WithStringParam("path", aitool.WithParam_Required(true), aitool.WithParam_Description("File or directory path.")),
			aitool.WithStringParam("pattern", aitool.WithParam_Required(true)),
			aitool.WithIntegerParam("limit", aitool.WithParam_Default(20)),
		},
	)
	doHTTPAction = makeToolForwardAction(
		"do_http_request", "do_http_request",
		"Single HTTP request for probing endpoints.",
		[]aitool.ToolOption{
			aitool.WithStringParam("url", aitool.WithParam_Required(true)),
		},
	)
	batchHTTPAction = makeToolForwardAction(
		"batch_do_http_request", "batch_do_http_request",
		"Batch HTTP requests for two or more paths. Prefer this over repeated single-request calls when probing a public attack surface.",
		[]aitool.ToolOption{
			aitool.WithStringParam("base-url", aitool.WithParam_Description("Base URL for URL mode, e.g. https://target.example.com.")),
			aitool.WithStringParam("paths", aitool.WithParam_Required(true), aitool.WithParam_Description("Newline-separated paths to request in one batch.")),
			aitool.WithStringParam("packet", aitool.WithParam_Description("Optional raw HTTP template containing {{PATH}}; use instead of base-url for exact replay.")),
			aitool.WithStringParam("method", aitool.WithParam_Default("GET")),
			aitool.WithStringParam("headers", aitool.WithParam_Description("Optional shared request headers.")),
			aitool.WithStringParam("body", aitool.WithParam_Description("Optional shared request body.")),
			aitool.WithStringParam("content-type"),
			aitool.WithStringParam("query-params"),
			aitool.WithStringParam("https", aitool.WithParam_Default("auto")),
			aitool.WithIntegerParam("concurrent", aitool.WithParam_Default(5)),
			aitool.WithIntegerParam("timeout", aitool.WithParam_Default(10)),
			aitool.WithIntegerParam("redirect-times", aitool.WithParam_Default(3)),
			aitool.WithStringParam("include-code"),
			aitool.WithStringParam("exclude-code"),
			aitool.WithIntegerParam("max-body-size", aitool.WithParam_Default(4096)),
		},
	)
	urlSummaryAction = makeToolForwardAction(
		"url_content_summary", "url_content_summary",
		"Fetch URL and get text summary.",
		[]aitool.ToolOption{
			aitool.WithStringParam("url", aitool.WithParam_Required(true)),
		},
	)
)
