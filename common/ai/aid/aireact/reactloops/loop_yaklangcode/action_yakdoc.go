package loop_yaklangcode

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func yakdocActions(r aicommon.AIInvokeRuntime) []reactloops.ReActLoopOption {
	return []reactloops.ReActLoopOption{
		yakdocGetAllLibraryNamesAction(r),
		yakdocLibraryDetailsAction(r),
		yakdocFunctionDetailsAction(r),
		yakdocVariableDetailsAction(r),
	}
}

func yakdocHandleSuccess(
	loop *reactloops.ReActLoop,
	op *reactloops.LoopActionHandlerOperator,
	actionName, timelineKey, streamKey, result string,
) {
	invoker := loop.GetInvoker()
	emitter := loop.GetEmitter()
	emitter.EmitThoughtStream(streamKey, result)
	invoker.AddToTimeline(timelineKey, result)
	log.Infof("%s: query completed", actionName)
	op.Continue()
}

func yakdocHandleError(
	loop *reactloops.ReActLoop,
	op *reactloops.LoopActionHandlerOperator,
	actionName, queryKey string,
	err error,
) {
	msg := fmt.Sprintf("【YakDocument 查询失败】%v\n\n【建议】：\n1. 使用 yakdoc_get_all_library_names 确认库名\n2. 使用 yakdoc_library_details 列出函数/变量名\n3. 再用 yakdoc_function_details / yakdoc_variable_details 查详情", err)
	log.Warnf("%s failed: %v", actionName, err)
	loop.GetInvoker().AddToTimeline(actionName+"_error", msg)
	op.Feedback(msg)
	if queryKey != "" {
		loop.Set(queryKey, "")
	}
	op.Continue()
}

func yakdocCheckDuplicate(loop *reactloops.ReActLoop, op *reactloops.LoopActionHandlerOperator, queryKey, currentQuery string) bool {
	last := loop.Get(queryKey)
	if last == "" || last != currentQuery {
		return false
	}
	msg := fmt.Sprintf(`【严重错误】检测到重复的 YakDocument 查询！

上次查询：%s
本次查询：%s

【拒绝执行】：请调整 library/function/variable 参数后再查询。`, last, currentQuery)
	loop.GetInvoker().AddToTimeline("yakdoc_duplicate_query_error", msg)
	op.Feedback(msg)
	op.Continue()
	return true
}

func yakdocGetAllLibraryNamesAction(_ aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"yakdoc_get_all_library_names",
		`查询 Yaklang 标准库名称列表（YakDocument）

【使用场景】：
- 不确定库名时，先列出所有标准库
- 编写代码前探索可用模块

【示例】：
yakdoc_get_all_library_names()`,
		nil,
		nil,
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			const queryKey = "last_yakdoc_query"
			if yakdocCheckDuplicate(loop, op, queryKey, "all_libraries") {
				return
			}
			loop.Set(queryKey, "all_libraries")

			names, err := QueryAllLibraryNames()
			if err != nil {
				yakdocHandleError(loop, op, "yakdoc_get_all_library_names", queryKey, err)
				return
			}
			yakdocHandleSuccess(loop, op, "yakdoc_get_all_library_names", "yakdoc_all_libraries", "yakdoc_all_libraries_result", FormatAllLibraryNames(names))
		},
	)
}

func yakdocLibraryDetailsAction(_ aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"yakdoc_library_details",
		`查询 Yaklang 标准库概览：函数名与变量名列表（YakDocument）

【使用场景】：
- API 报错 ExternLib don't has 时，列出该库真实可用的函数/变量
- 在 yakdoc_function_details 之前缩小候选范围

【参数】：
- library ([]string, 可选) - 库名；留空表示 GLOBAL 内置函数/变量

【示例】：
yakdoc_library_details(library=["synscan"])
yakdoc_library_details(library=["str", "file"])`,
		[]aitool.ToolOption{
			aitool.WithStringArrayParam(
				"library",
				aitool.WithParam_Description("Standard library names; empty means GLOBAL builtins"),
			),
		},
		nil,
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			libNames := action.GetStringSlice("library")
			currentQuery := "library_details:" + strings.Join(libNames, ",")
			const queryKey = "last_yakdoc_query"
			if yakdocCheckDuplicate(loop, op, queryKey, currentQuery) {
				return
			}
			loop.Set(queryKey, currentQuery)

			details, err := QueryLibraryDetails(libNames)
			if err != nil {
				yakdocHandleError(loop, op, "yakdoc_library_details", queryKey, err)
				return
			}
			yakdocHandleSuccess(loop, op, "yakdoc_library_details", "yakdoc_library_details", "yakdoc_library_details_result", FormatLibraryDetails(details))
		},
	)
}

func yakdocFunctionDetailsAction(_ aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"yakdoc_function_details",
		`查询 Yaklang 标准函数文档：签名、参数、返回值、说明（YakDocument）

【使用场景】：
- ExternLib API 不存在错误后，确认正确函数名与参数
- 需要权威 API 说明（比 grep 样例更准确）

【参数】：
- library (string, 可选) - 库名；空表示 GLOBAL
- function ([]string, 必需) - 函数名列表

【示例】：
yakdoc_function_details(library="synscan", function=["Scan"])
yakdoc_function_details(library="str", function=["Split", "Contains"])`,
		[]aitool.ToolOption{
			aitool.WithStringParam(
				"library",
				aitool.WithParam_Description("Library name; empty means GLOBAL function"),
			),
			aitool.WithStringArrayParam(
				"function",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Function names to query"),
			),
		},
		nil,
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			if len(action.GetStringSlice("function")) == 0 {
				return utils.Error("yakdoc_function_details requires 'function' parameter")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			libName := action.GetString("library")
			funcNames := action.GetStringSlice("function")
			currentQuery := fmt.Sprintf("function_details:%s:%s", libName, strings.Join(funcNames, ","))
			const queryKey = "last_yakdoc_query"
			if yakdocCheckDuplicate(loop, op, queryKey, currentQuery) {
				return
			}
			loop.Set(queryKey, currentQuery)

			results, err := QueryFunctionDetails(libName, funcNames)
			if err != nil {
				yakdocHandleError(loop, op, "yakdoc_function_details", queryKey, err)
				return
			}
			yakdocHandleSuccess(loop, op, "yakdoc_function_details", "yakdoc_function_details", "yakdoc_function_details_result", FormatFunctionDetails(results))
		},
	)
}

func yakdocVariableDetailsAction(_ aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"yakdoc_variable_details",
		`查询 Yaklang 标准库变量/实例文档（YakDocument）

【使用场景】：
- 确认库级常量、预定义实例的名称与类型

【参数】：
- library (string, 可选) - 库名；空表示 GLOBAL
- variable ([]string, 必需) - 变量名列表

【示例】：
yakdoc_variable_details(library="yakit", variable=["Status"])`,
		[]aitool.ToolOption{
			aitool.WithStringParam(
				"library",
				aitool.WithParam_Description("Library name; empty means GLOBAL variable"),
			),
			aitool.WithStringArrayParam(
				"variable",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Variable names to query"),
			),
		},
		nil,
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			if len(action.GetStringSlice("variable")) == 0 {
				return utils.Error("yakdoc_variable_details requires 'variable' parameter")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			libName := action.GetString("library")
			varNames := action.GetStringSlice("variable")
			currentQuery := fmt.Sprintf("variable_details:%s:%s", libName, strings.Join(varNames, ","))
			const queryKey = "last_yakdoc_query"
			if yakdocCheckDuplicate(loop, op, queryKey, currentQuery) {
				return
			}
			loop.Set(queryKey, currentQuery)

			results, err := QueryVariableDetails(libName, varNames)
			if err != nil {
				yakdocHandleError(loop, op, "yakdoc_variable_details", queryKey, err)
				return
			}
			yakdocHandleSuccess(loop, op, "yakdoc_variable_details", "yakdoc_variable_details", "yakdoc_variable_details_result", FormatVariableDetails(results))
		},
	)
}
