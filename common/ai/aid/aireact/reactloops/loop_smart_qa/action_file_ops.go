package loop_smart_qa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func makeToolForwardAction(
	actionName string,
	targetToolName string,
	desc string,
	toolOpts []aitool.ToolOption,
) func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
		return reactloops.WithRegisterLoopAction(
			actionName,
			desc, toolOpts,
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

				invoker.AddToTimeline(
					fmt.Sprintf("%s_result", actionName),
					fmt.Sprintf("[%s] %s", targetToolName, utils.ShrinkString(content, 2048)),
				)

				op.Feedback(fmt.Sprintf("%s completed (%d bytes)", targetToolName, len(content)))
				op.Continue()
			},
		)
	}
}

var readFileAction = makeToolForwardAction(
	"read_file", "read_file",
	"Read the content of a local TEXT file by path. "+
		"Uses mimetype (magic bytes) to detect binary files and recommends appropriate built-in tools instead of outputting garbled data. "+
		"For Excel use read_excel_info/query_excel_data, for Word/PPT use read_word_structure/parse_office_to_text, for ZIP use zip_viewer, for PCAP use analyze_pcap.",
	[]aitool.ToolOption{
		aitool.WithStringParam("path",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Absolute path of the file to read.")),
		aitool.WithIntegerParam("offset",
			aitool.WithParam_Description("Byte offset to start reading from."),
			aitool.WithParam_Default(0)),
		aitool.WithIntegerParam("chunk_size",
			aitool.WithParam_Description("Maximum bytes to read."),
			aitool.WithParam_Default(20480)),
	},
)

var findFilesAction = makeToolForwardAction(
	"find_files", "find_file",
	"Search for files by name pattern in a directory.",
	[]aitool.ToolOption{
		aitool.WithStringParam("dir",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Root directory to search.")),
		aitool.WithStringParam("pattern",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Pattern to match file paths.")),
		aitool.WithIntegerParam("max",
			aitool.WithParam_Description("Maximum results."),
			aitool.WithParam_Default(10)),
	},
)

var grepTextAction = makeToolForwardAction(
	"grep_text", "grep",
	"Search for text patterns in files or directories.",
	[]aitool.ToolOption{
		aitool.WithStringParam("path",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("File or directory path to search in.")),
		aitool.WithStringParam("pattern",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Text pattern to search for.")),
		aitool.WithIntegerParam("limit",
			aitool.WithParam_Description("Maximum matched results."),
			aitool.WithParam_Default(10)),
	},
)
