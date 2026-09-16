package loop_syntaxflow_rule

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/syntaxflowdoc"
	sfdoc "github.com/yaklang/yaklang/common/syntaxflow/syntaxflowdoc/doc"
	"github.com/yaklang/yaklang/common/utils"
)

func syntaxflowdocActions(_ aicommon.AIInvokeRuntime) []reactloops.ReActLoopOption {
	return []reactloops.ReActLoopOption{
		syntaxflowdocSearchAction(),
		syntaxflowdocListNativeCallsAction(),
		syntaxflowdocNativeCallDetailsAction(),
		syntaxflowdocListBuiltinLibsAction(),
		syntaxflowdocBuiltinLibDetailsAction(),
	}
}

func syntaxflowdocEmitStart(loop *reactloops.ReActLoop) {
	reactloops.EmitStatusI18n(loop, "查询 SyntaxFlow 文档中", "Querying SyntaxFlow Document...")
}

func syntaxflowdocHandleSuccess(
	loop *reactloops.ReActLoop,
	op *reactloops.LoopActionHandlerOperator,
	actionName, timelineKey, result string,
) {
	invoker := loop.GetInvoker()
	summary, reference := reactloops.SpillLongContent(loop, "syntaxflowdoc_"+actionName, result)
	finishLine := fmt.Sprintf("完成: %s", actionName)
	reactloops.EmitActionLog(loop, "query_syntaxflow_document", finishLine, reference)
	invoker.AddToTimeline(timelineKey, fmt.Sprintf("%s\n%s", finishLine, summary))
	log.Infof("%s: query completed", actionName)
	op.Continue()
}

func syntaxflowdocHandleError(
	loop *reactloops.ReActLoop,
	op *reactloops.LoopActionHandlerOperator,
	actionName, queryKey string,
	err error,
) {
	msg := fmt.Sprintf(`【SyntaxFlowDoc 查询失败】%v

【建议】：
1. 使用 syntaxflowdoc_list_native_calls / syntaxflowdoc_list_builtin_libs 确认名称
2. 使用 syntaxflowdoc_search 按关键词模糊搜索
3. 再用 syntaxflowdoc_native_call_details / syntaxflowdoc_builtin_lib_details 查详情`, err)
	log.Warnf("%s failed: %v", actionName, err)
	loop.GetInvoker().AddToTimeline(actionName+"_error", msg)
	op.Feedback(msg)
	if queryKey != "" {
		loop.Set(queryKey, "")
	}
	op.Continue()
}

func syntaxflowdocCheckDuplicate(loop *reactloops.ReActLoop, op *reactloops.LoopActionHandlerOperator, queryKey, currentQuery string) bool {
	last := loop.Get(queryKey)
	if last == "" || last != currentQuery {
		return false
	}
	msg := fmt.Sprintf(`【严重错误】检测到重复的 SyntaxFlowDoc 查询！

上次查询：%s
本次查询：%s

【拒绝执行】：请调整参数后再查询。`, last, currentQuery)
	invoker := loop.GetInvoker()
	invoker.AddToTimeline("syntaxflowdoc_duplicate_query_error", msg)
	op.Feedback(msg)
	op.Continue()
	return true
}

func syntaxflowdocSearchAction() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"syntaxflowdoc_search",
		`按关键词模糊搜索 SyntaxFlow 文档（NativeCall / operator / opcode / desc / builtin lib）

【使用场景】：
- 不确定 <nativeCall>、运算符或 include 库名时
- 编写规则前按功能词探索可用 API（如 "include"、"dataflow"、"gin"）

【参数】：
- query (string, 必需) - 搜索关键词
- category (string, 可选) - 限定分类：native_call / operator / opcode / desc_key / builtin_lib / syntax
- limit (int, 可选) - 返回条数，默认 20，最大 64

【示例】：
syntaxflowdoc_search(query="gin context include")
syntaxflowdoc_search(query="dataflow", category="native_call")`,
		[]aitool.ToolOption{
			aitool.WithStringParam(
				"query",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Keywords describing the desired SyntaxFlow feature"),
			),
			aitool.WithStringParam(
				"category",
				aitool.WithParam_Description("Optional category filter: native_call/operator/opcode/desc_key/builtin_lib/syntax"),
			),
			aitool.WithIntegerParam(
				"limit",
				aitool.WithParam_Description("Max results, default 20"),
			),
		},
		nil,
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(action.GetString("query")) == "" {
				return utils.Error("syntaxflowdoc_search requires 'query' parameter")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			query := strings.TrimSpace(action.GetString("query"))
			category := strings.TrimSpace(action.GetString("category"))
			limit := int(action.GetInt("limit"))
			currentQuery := fmt.Sprintf("search:%s:%s:%d", query, category, limit)
			const queryKey = "last_syntaxflowdoc_query"
			if syntaxflowdocCheckDuplicate(loop, op, queryKey, currentQuery) {
				return
			}
			loop.Set(queryKey, currentQuery)

			syntaxflowdocEmitStart(loop)
			if !sfdoc.IsDocumentAvailable() {
				syntaxflowdocHandleError(loop, op, "syntaxflowdoc_search", queryKey, utils.Error("SyntaxFlowDoc embed unavailable"))
				return
			}
			hits := sfdoc.SearchDocument(query, limit, category)
			syntaxflowdocHandleSuccess(loop, op, "syntaxflowdoc_search", "syntaxflowdoc_search", syntaxflowdoc.FormatSearchHits(query, hits))
		},
	)
}

func syntaxflowdocListNativeCallsAction() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"syntaxflowdoc_list_native_calls",
		`列出全部 SyntaxFlow <nativeCall> 名称

【使用场景】：
- 不确定可用 nativeCall 时先拉全量列表
- 再配合 syntaxflowdoc_native_call_details 查详情

【示例】：
syntaxflowdoc_list_native_calls()`,
		nil,
		nil,
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			const queryKey = "last_syntaxflowdoc_query"
			if syntaxflowdocCheckDuplicate(loop, op, queryKey, "list_native_calls") {
				return
			}
			loop.Set(queryKey, "list_native_calls")

			syntaxflowdocEmitStart(loop)
			h := sfdoc.GetDefaultDocumentHelper()
			if h == nil || h.Total() == 0 {
				syntaxflowdocHandleError(loop, op, "syntaxflowdoc_list_native_calls", queryKey, utils.Error("SyntaxFlowDoc embed unavailable"))
				return
			}
			names := h.ListNativeCallNames()
			syntaxflowdocHandleSuccess(loop, op, "syntaxflowdoc_list_native_calls", "syntaxflowdoc_native_calls", syntaxflowdoc.FormatNameList("native_call(s)", names))
		},
	)
}

func syntaxflowdocNativeCallDetailsAction() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"syntaxflowdoc_native_call_details",
		`查询 SyntaxFlow <nativeCall> 详情与示例

【参数】：
- name ([]string, 必需) - nativeCall 名称列表，如 ["include", "dataflow"]

【示例】：
syntaxflowdoc_native_call_details(name=["include"])
syntaxflowdoc_native_call_details(name=["include", "eval"])`,
		[]aitool.ToolOption{
			aitool.WithStringArrayParam(
				"name",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("NativeCall names to query"),
			),
		},
		nil,
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			if len(action.GetStringSlice("name")) == 0 {
				return utils.Error("syntaxflowdoc_native_call_details requires 'name' parameter")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			names := action.GetStringSlice("name")
			currentQuery := "native_call_details:" + strings.Join(names, ",")
			const queryKey = "last_syntaxflowdoc_query"
			if syntaxflowdocCheckDuplicate(loop, op, queryKey, currentQuery) {
				return
			}
			loop.Set(queryKey, currentQuery)

			syntaxflowdocEmitStart(loop)
			h := sfdoc.GetDefaultDocumentHelper()
			found, missing := syntaxflowdoc.ResolveNativeCalls(h, names)
			if len(found) == 0 {
				err := utils.Errorf("native_call not found: %s", strings.Join(missing, ", "))
				syntaxflowdocHandleError(loop, op, "syntaxflowdoc_native_call_details", queryKey, err)
				return
			}
			body := syntaxflowdoc.FormatNativeCallDetails(found)
			if len(missing) > 0 {
				body += "\n\n[missing] " + strings.Join(missing, ", ")
			}
			syntaxflowdocHandleSuccess(loop, op, "syntaxflowdoc_native_call_details", "syntaxflowdoc_native_call_details", body)
		},
	)
}

func syntaxflowdocListBuiltinLibsAction() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"syntaxflowdoc_list_builtin_libs",
		`列出全部 SyntaxFlow <include('lib')> 内置库名称

【使用场景】：
- 不确定该用 golang-gin-context 还是其他 lib 时
- 再配合 syntaxflowdoc_builtin_lib_details 查路径/语言/示例

【示例】：
syntaxflowdoc_list_builtin_libs()`,
		nil,
		nil,
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			const queryKey = "last_syntaxflowdoc_query"
			if syntaxflowdocCheckDuplicate(loop, op, queryKey, "list_builtin_libs") {
				return
			}
			loop.Set(queryKey, "list_builtin_libs")

			syntaxflowdocEmitStart(loop)
			h := sfdoc.GetDefaultDocumentHelper()
			if h == nil || h.Total() == 0 {
				syntaxflowdocHandleError(loop, op, "syntaxflowdoc_list_builtin_libs", queryKey, utils.Error("SyntaxFlowDoc embed unavailable"))
				return
			}
			names := h.ListBuiltinLibNames()
			syntaxflowdocHandleSuccess(loop, op, "syntaxflowdoc_list_builtin_libs", "syntaxflowdoc_builtin_libs", syntaxflowdoc.FormatNameList("builtin_lib(s)", names))
		},
	)
}

func syntaxflowdocBuiltinLibDetailsAction() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"syntaxflowdoc_builtin_lib_details",
		`查询 SyntaxFlow 内置 include 库详情（路径 / 语言 / 示例）

【参数】：
- name ([]string, 必需) - lib 名称，如 ["golang-gin-context"]

【示例】：
syntaxflowdoc_builtin_lib_details(name=["golang-gin-context"])`,
		[]aitool.ToolOption{
			aitool.WithStringArrayParam(
				"name",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Builtin include library names"),
			),
		},
		nil,
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			if len(action.GetStringSlice("name")) == 0 {
				return utils.Error("syntaxflowdoc_builtin_lib_details requires 'name' parameter")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			names := action.GetStringSlice("name")
			currentQuery := "builtin_lib_details:" + strings.Join(names, ",")
			const queryKey = "last_syntaxflowdoc_query"
			if syntaxflowdocCheckDuplicate(loop, op, queryKey, currentQuery) {
				return
			}
			loop.Set(queryKey, currentQuery)

			syntaxflowdocEmitStart(loop)
			h := sfdoc.GetDefaultDocumentHelper()
			found, missing := syntaxflowdoc.ResolveBuiltinLibs(h, names)
			if len(found) == 0 {
				err := utils.Errorf("builtin_lib not found: %s", strings.Join(missing, ", "))
				syntaxflowdocHandleError(loop, op, "syntaxflowdoc_builtin_lib_details", queryKey, err)
				return
			}
			body := syntaxflowdoc.FormatBuiltinLibDetails(found)
			if len(missing) > 0 {
				body += "\n\n[missing] " + strings.Join(missing, ", ")
			}
			syntaxflowdocHandleSuccess(loop, op, "syntaxflowdoc_builtin_lib_details", "syntaxflowdoc_builtin_lib_details", body)
		},
	)
}
