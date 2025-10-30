package aiforge

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag/rag_search_tool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ForgeFactory 负责基于 schema.AIForge 的检索与执行
type ForgeFactory struct{}

func NewForgeFactory() *ForgeFactory {
	return &ForgeFactory{}
}

// ForgeQueryOption 定义 Query 的可选项
type ForgeQueryOption func(*ypb.AIForgeFilter, *ypb.Paging)

func (f ForgeFactory) NLQuery(ctx context.Context, query string, aiChatFunc func(string) (io.Reader, error), opts ...aicommon.ForgeQueryOption) ([]*schema.AIForge, error) {
	data, err := f.Query(ctx, opts...)
	if err != nil {
		return nil, err
	}
	seacher := rag_search_tool.NewComprehensiveSearcher[*schema.AIForge](rag_search_tool.ForgeVectorIndexName, aiChatFunc)
	return seacher(query, data)
}

// Query 从 Profile 数据库中查询 schema.AIForge 列表
func (ForgeFactory) Query(ctx context.Context, opts ...aicommon.ForgeQueryOption) ([]*schema.AIForge, error) {
	queryConfig := aicommon.NewForgeQueryConfig(opts...)
	db := consts.GetGormProfileDatabase()
	log.Debugf("ForgeFactory.Query: keyword=%q limit=%d", queryConfig.Filter.GetKeyword(), queryConfig.Paging.GetLimit())
	_, data, err := yakit.QueryAIForge(db, queryConfig.Filter, queryConfig.Paging)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (f *ForgeFactory) GenerateAIForgeListForPrompt(forges []*schema.AIForge) (string, error) {
	if forges == nil || len(forges) == 0 {
		return "", nil
	}
	result, err := utils.RenderTemplate(`<|AI_BLUEPRINT_{{ .nonce }}_START|>`+
		`{{range .forges}}
* '{{ .ForgeName }}': {{ .Description }}{{ if .ForgeVerboseName }}(Short: {{ .ForgeVerboseName }}){{end}}{{end}}
<|AI_BLUEPRINT_{{ .nonce }}_END|>`, map[string]any{
		"forges": forges,
		"nonce":  utils.RandStringBytes(4),
	})
	return result, err
}

// GetAIForge 根据名称从数据库中获取单个 AIForge
func (f *ForgeFactory) GetAIForge(name string) (*schema.AIForge, error) {
	if name == "" {
		return nil, utils.Errorf("forge name cannot be empty")
	}

	db := consts.GetGormProfileDatabase()
	log.Debugf("ForgeFactory.GetAIForge: name=%q", name)

	return yakit.GetAIForgeByName(db, name)
}

// GenerateAIJSONSchemaFromSchemaAIForge  从 AIForge 生成对应的 aitool.ToolOption 选项
// 这个函数解析 AIForge.Params 中的 Yak 语言 CLI 参数定义代码，并生成相应的 aitool.ToolOption 配置
func (f *ForgeFactory) GenerateAIJSONSchemaFromSchemaAIForge(forge *schema.AIForge) (string, error) {
	if forge == nil {
		return "", utils.Errorf("forge cannot be nil")
	}

	var options []any
	var params []aitool.ToolOption

	options = append(options, aitool.WithAction("call-ai-blueprint"))

	// 如果 forge.Params 为空，只返回基本选项
	if forge.Params != "" {
		// 解析 Yak CLI 代码获取参数选项
		parsedParams := aitool.ConvertYaklangCliCodeToToolOptions(forge.Params)
		params = append(params, parsedParams...)
	} else if forge.ForgeContent != "" {
		parsedParams := aitool.ConvertYaklangCliCodeToToolOptions(forge.ForgeContent)
		params = append(params, parsedParams...)
	} else {
		params = append(params, aitool.WithStringParam("query", aitool.WithParam_Description("Some input for helping the AI blueprint execute plan and executing")))

	}

	// 如果有参数，添加到 params 结构体中
	if len(params) > 0 {
		options = append(options, aitool.WithStructParam("params", nil, params...))
	} else {
		// 如果没有参数，创建一个空的 params 对象
		options = append(options, aitool.WithStructParam("params", nil))
	}

	return aitool.NewObjectSchema(options...), nil
}

// Execute 透明转发到内置 ExecuteForge
func (ForgeFactory) Execute(ctx context.Context, forgeName string, params []*ypb.ExecParamItem, opts ...aicommon.ConfigOption) (*ForgeResult, error) {
	return ExecuteForge(forgeName, ctx, params, opts...)
}
