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
				reactloops.EmitStatusI18n(loop, "执行侦察工具中", "Executing recon tool...")

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
				reactloops.EmitStatusI18n(loop, "完成", "Complete")
				reactloops.EmitActionLog(loop, infosecReconToolNodeID, fmt.Sprintf("完成: %s (%d bytes) / Done: %s (%d bytes)", actionName, len(content), targetToolName, len(content)))
				op.Continue()
			},
		)
	}
}

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
			reactloops.EmitStatusI18n(loop, "联网搜索中", "Searching the web...")

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
			reactloops.EmitStatusI18n(loop, "完成", "Complete")
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
		"Batch HTTP requests with constrained concurrency.",
		[]aitool.ToolOption{
			aitool.WithStringParam("requests", aitool.WithParam_Description("Batch request spec per tool docs.")),
		},
	)
)
