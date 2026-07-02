package loop_fast_context

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func grepFilesBatchAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"grep_files_batch",
		"Run multiple grep searches in parallel and append matching FILE PATHS only "+
			"(forced files_with_matches). Prefer this over serial grep_files when you have 3-8 known patterns.",
		[]aitool.ToolOption{
			aitool.WithStructArrayParam(
				"searches",
				[]aitool.PropertyOption{
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("Non-empty array of grep queries; each item must include path and pattern."),
				},
				nil,
				aitool.WithStringParam("id",
					aitool.WithParam_Description("Optional label for logging, e.g. sink_decode")),
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
					aitool.WithParam_Description("Max files returned per search"),
					aitool.WithParam_Default(grepFilesWithMatchesLimit)),
			),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			action.WaitStream(loop.GetCurrentTask().GetContext())
			searches, err := parseGrepBatchSearches(action)
			if err != nil {
				return err
			}
			loop.Set(loopVarGrepBatchSearches, searches)
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			invoker := loop.GetInvoker()

			searches, ok := loop.GetVariable(loopVarGrepBatchSearches).([]grepBatchSearch)
			if !ok || len(searches) == 0 {
				var err error
				searches, err = parseGrepBatchSearches(action)
				if err != nil {
					op.Feedback(fmt.Sprintf("grep_files_batch parse failed: %v\n"+
						"Provide searches: [{\"path\":\"/abs/project\",\"pattern\":\"sink\"}, ...]", err))
					op.Continue()
					return
				}
			}

			ctx := invoker.GetConfig().GetContext()
			if task := loop.GetCurrentTask(); task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}

			loop.LoadingStatus(fmt.Sprintf("grep_files_batch: %d parallel searches", len(searches)))
			batchResult := runGrepBatch(loop, invoker, ctx, searches)

			count := utils.InterfaceToInt(loop.Get(loopVarSearchRounds))
			loop.Set(loopVarSearchRounds, count+1)

			if batchResult.FirstFatal != nil && batchResult.BatchAdded == 0 {
				op.Feedback(fmt.Sprintf("grep_files_batch failed: %v\n%s", batchResult.FirstFatal,
					compactSearchFeedback("grep_files_batch", batchResult.BatchAdded, batchResult.Total,
						samplePaths(dedupeStrings(batchResult.BatchPaths), 5))))
				op.Continue()
				return
			}

			feedback := compactSearchFeedback("grep_files_batch", batchResult.BatchAdded, batchResult.Total,
				samplePaths(dedupeStrings(batchResult.BatchPaths), 8))
			if len(batchResult.ExecErrors) > 0 {
				feedback += "\nWarnings:\n  - " + strings.Join(batchResult.ExecErrors, "\n  - ")
			}
			op.Feedback(feedback)
			op.Continue()
		},
	)
}
