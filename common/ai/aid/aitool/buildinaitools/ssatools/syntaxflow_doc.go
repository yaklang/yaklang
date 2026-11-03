package ssatools

import (
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/syntaxflow/syntaxflowdoc"
	sfdoc "github.com/yaklang/yaklang/common/syntaxflow/syntaxflowdoc/doc"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	SyntaxFlowToolName_DocSearch         = "syntaxflowdoc_search"
	SyntaxFlowToolName_BuiltinLibDetails = "syntaxflowdoc_builtin_lib_details"
)

// registerSyntaxFlowDocTools exposes the same SyntaxFlowDoc index that the
// write_syntaxflow_rule focus mode queries through loop actions, so skills can
// reach it from any mode. The index is the embedded doc.gob.zst snapshot with
// live NativeCall rows overlaid by sfdoc.GetDefaultDocumentHelper.
func registerSyntaxFlowDocTools(factory *aitool.ToolFactory) error {
	if factory == nil {
		return utils.Error("tool factory is nil")
	}
	if err := registerSyntaxFlowDocSearchTool(factory); err != nil {
		return err
	}
	return registerSyntaxFlowDocBuiltinLibDetailsTool(factory)
}

func registerSyntaxFlowDocSearchTool(factory *aitool.ToolFactory) error {
	return factory.RegisterTool(
		SyntaxFlowToolName_DocSearch,
		aitool.WithVerboseName("SyntaxFlowDoc Search"),
		aitool.WithVerboseNameZh("SyntaxFlow文档检索"),
		aitool.WithDescription("按关键词搜索 SyntaxFlow 文档（NativeCall / operator / opcode / desc / builtin lib / syntax）。编写或修改 SyntaxFlow 规则前，用它确认真实存在的 native call、include 库名与语法关键字，禁止臆造。"),
		aitool.WithKeywords([]string{
			"syntaxflow", "规则", "include", "native call", "builtin lib", "文档检索", "syntax",
		}),
		aitool.WithStringParam("query",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("搜索关键词，如 gin、include、dataflow、alert、desc"),
		),
		aitool.WithStringParam("category",
			aitool.WithParam_Description("可选分类过滤：native_call / operator / opcode / desc_key / builtin_lib / syntax"),
		),
		aitool.WithIntegerParam("limit",
			aitool.WithParam_Default(10),
			aitool.WithParam_Description("返回条数上限，默认 10"),
		),
		aitool.WithSimpleCallback(syntaxFlowDocSearchCallback),
	)
}

func syntaxFlowDocSearchCallback(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
	query := strings.TrimSpace(params.GetString("query"))
	if query == "" {
		return nil, utils.Error("query is required")
	}
	limit := int(params.GetInt("limit", 10))
	if limit <= 0 {
		limit = 10
	}
	helper := sfdoc.GetDefaultDocumentHelper()
	if helper == nil {
		return nil, utils.Error("SyntaxFlowDoc helper is unavailable")
	}
	hits := syntaxflowdoc.SearchDocument(helper, query, limit, strings.TrimSpace(params.GetString("category")))
	return syntaxflowdoc.FormatSearchHits(query, hits), nil
}

func registerSyntaxFlowDocBuiltinLibDetailsTool(factory *aitool.ToolFactory) error {
	return factory.RegisterTool(
		SyntaxFlowToolName_BuiltinLibDetails,
		aitool.WithVerboseName("SyntaxFlowDoc Builtin Lib Details"),
		aitool.WithVerboseNameZh("SyntaxFlow内置库详情"),
		aitool.WithDescription("查询 SyntaxFlow 内置 include 库详情（库名 / 语言 / 相对路径 / 描述）。返回的 path 是内置库在规则库中的相对路径，可用于定位或确认库名；库的导出变量需读取对应 .sf 源码确认。"),
		aitool.WithKeywords([]string{
			"builtin lib", "include", "golang-gin-context", "库详情", "syntaxflow",
		}),
		aitool.WithStringArrayParam("name",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("内置 include 库名数组，如 [\"golang-gin-context\"]"),
		),
		aitool.WithSimpleCallback(syntaxFlowDocBuiltinLibDetailsCallback),
	)
}

func syntaxFlowDocBuiltinLibDetailsCallback(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
	names := params.GetStringSlice("name")
	if len(names) == 0 {
		return nil, utils.Error("name is required")
	}
	helper := sfdoc.GetDefaultDocumentHelper()
	if helper == nil {
		return nil, utils.Error("SyntaxFlowDoc helper is unavailable")
	}
	found, missing := syntaxflowdoc.ResolveBuiltinLibs(helper, names)
	if len(found) == 0 {
		return nil, utils.Errorf("builtin_lib not found: %s", strings.Join(missing, ", "))
	}
	result := syntaxflowdoc.FormatBuiltinLibDetails(found)
	if len(missing) > 0 {
		result += "\nmissing: " + strings.Join(missing, ", ") + "\n"
	}
	return result, nil
}
