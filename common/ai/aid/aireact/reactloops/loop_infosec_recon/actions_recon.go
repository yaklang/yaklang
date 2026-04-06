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
				loop.LoadingStatus(fmt.Sprintf("executing %s: %s", targetToolName, utils.ShrinkString(thought, 80)))

				result, directly, err := invoker.ExecuteToolRequiredAndCall(ctx, targetToolName)
				if err != nil {
					log.Warnf("%s tool call failed: %v", targetToolName, err)
					failMsg := fmt.Sprintf("%s FAILED: %v.", actionName, err)
					invoker.AddToTimeline(actionName+"_failed", failMsg)
					op.Feedback(failMsg + " Try different parameters or another tool.")
					op.Continue()
					return
				}
				if directly {
					op.Feedback(fmt.Sprintf("%s was skipped by user.", actionName))
					op.Continue()
					return
				}
				if result == nil {
					op.Feedback(fmt.Sprintf("%s returned no result.", actionName))
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
				op.Feedback(fmt.Sprintf("%s completed.", actionName))
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
				loop.LoadingStatus(fmt.Sprintf("calling tool: %s", targetToolName))
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
				op.Continue()
			},
		)
	}
}

var (
	scanPortAction        = makeReconRequireAction("scan_port", "scan_port", "Port scan (authorized targets only). Describe hosts and ports in human_readable_thought for the tool request phase.")
	simpleCrawlerAction   = makeReconRequireAction("simple_crawler", "simple_crawler", "Lightweight web crawl to discover URLs and pages. Describe start URL and depth in human_readable_thought.")
	bannerGrabAction      = makeReconRequireAction("banner_grab", "banner_grab", "TCP banner grab on host:port. Describe targets in human_readable_thought.")
	digAction             = makeReconRequireAction("dig", "dig", "DNS lookups. Describe domain and record types in human_readable_thought.")
	subdomainScanAction   = makeReconRequireAction("subdomain_scan", "subdomain_scan", "Subdomain brute / scan for a domain. Describe target domain in human_readable_thought.")
	networkSpaceAction    = makeReconRequireAction("network_space_search", "network_space_search", "Space search engine query (FOFA/Shodan etc., needs API keys). Describe engine and query in human_readable_thought.")
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
			params := aitool.InvokeParams{"query": query}
			result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, "web_search", params)
			if err != nil {
				op.Feedback(fmt.Sprintf("web_search failed: %v", err))
				op.Continue()
				return
			}
			content := ""
			if result != nil {
				content = utils.InterfaceToString(result.Data)
			}
			appendInfosecReconLog(loop, fmt.Sprintf("=== web_search: %s ===\n%s", query, utils.ShrinkString(content, 4096)))
			invoker.AddToTimeline("web_search_result", utils.ShrinkString(content, 2048))
			op.Feedback("web_search completed")
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
		"Batch HTTP requests with constrained concurrency.",
		[]aitool.ToolOption{
			aitool.WithStringParam("requests", aitool.WithParam_Description("Batch request spec per tool docs.")),
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
