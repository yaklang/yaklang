package loop_fast_context

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func makeIndexSearchAction(
	actionName string,
	targetToolName string,
	desc string,
	toolOpts []aitool.ToolOption,
	forceParams func(params aitool.InvokeParams) aitool.InvokeParams,
	parseOutput func(string) []string,
) func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
		return reactloops.WithRegisterLoopAction(
			actionName,
			desc, toolOpts,
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				invoker := loop.GetInvoker()
				ctx := loop.GetConfig().GetContext()
				if task := loop.GetCurrentTask(); task != nil && !utils.IsNil(task.GetContext()) {
					ctx = task.GetContext()
				}

				params := action.GetParams()
				if forceParams != nil {
					params = forceParams(params)
				}

				loop.LoadingStatus(fmt.Sprintf("index search: %s", targetToolName))
				result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, targetToolName, params)
				if err != nil {
					log.Warnf("[FastContext] %s failed: %v", targetToolName, err)
					op.Feedback(fmt.Sprintf("%s failed: %v", targetToolName, err))
					op.Continue()
					return
				}

				content := ""
				if result != nil {
					content = toolOutputString(result.Data)
				}
				paths := parseOutput(content)
				added := mergePathsIntoFileIndex(loop, paths...)

				count := utils.InterfaceToInt(loop.Get(loopVarSearchRounds))
				loop.Set(loopVarSearchRounds, count+1)
				total := len(listFileIndex(loop))

				invoker.AddToTimeline(
					fmt.Sprintf("[FASTCONTEXT_%s]", actionName),
					fmt.Sprintf("pattern=%q added=%d total=%d", action.GetString("pattern"), added, total),
				)

				op.Feedback(compactSearchFeedback(targetToolName, added, total, samplePaths(paths, 5)))
				op.Continue()
			},
		)
	}
}

var grepFilesAction = makeIndexSearchAction(
	"grep_files", "grep",
	"Search with grep and append matching FILE PATHS only (forced files_with_matches). "+
		"For multiple patterns use grep_files_batch instead of calling this repeatedly.",
	[]aitool.ToolOption{
		aitool.WithStringParam("path",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Directory or file absolute path")),
		aitool.WithStringParam("pattern",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Regex or literal pattern")),
		aitool.WithStringParam("pattern-mode",
			aitool.WithParam_Description("regexp | substr | isubstr"),
			aitool.WithParam_Default("regexp")),
		aitool.WithStringParam("include-ext",
			aitool.WithParam_Description("Optional extension whitelist, e.g. go,php")),
		aitool.WithIntegerParam("limit",
			aitool.WithParam_Description("Max files returned"),
			aitool.WithParam_Default(grepFilesWithMatchesLimit)),
	},
	func(params aitool.InvokeParams) aitool.InvokeParams {
		if params == nil {
			params = aitool.InvokeParams{}
		}
		params["output-mode"] = "files_with_matches"
		if params.GetInt("limit") <= 0 {
			params["limit"] = grepFilesWithMatchesLimit
		}
		return params
	},
	parseGrepFilesWithMatchesOutput,
)

var findFilesAction = makeIndexSearchAction(
	"find_files", "find_file",
	"Find files by glob and append paths to the index.",
	[]aitool.ToolOption{
		aitool.WithStringParam("dir",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Root directory absolute path")),
		aitool.WithStringParam("pattern",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Filename glob")),
		aitool.WithIntegerParam("max",
			aitool.WithParam_Description("Max results"),
			aitool.WithParam_Default(findFileMaxResults)),
	},
	nil,
	parseFindFileOutput,
)
