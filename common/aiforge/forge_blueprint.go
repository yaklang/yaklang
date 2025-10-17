// Package aiforge 提供了AI Forge的核心功能，用于构建和配置AI助手
package aiforge

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/information"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

//go:embed forgeprompts/forge.txt
var forgeTemplate string

// ForgeBlueprint 定义了AI Forge的蓝图结构，包含配置AI助手所需的所有元素
type ForgeBlueprint struct {
	Name string

	// Plan
	PlanMocker func(config *aid.Config) *aid.PlanResponse

	// InitializePrompt 是AI助手初始化时使用的提示词，用于设置AI的基本行为和知识
	InitializePrompt string

	// PersistentPrompt 是在AI助手整个会话过程中持续存在的提示词，用于维持AI的行为一致性
	PersistentPrompt string

	// ResultPrompt 是AI助手生成结果时使用的提示词，用于设置AI的输出格式和内容
	ResultPrompt  string
	ResultHandler func(string, error)

	// Tools 是AI助手可以使用的工具列表，这些工具可以扩展AI的能力
	Tools []*aitool.Tool

	// AIDOptions 是AI助手的其他配置选项
	AIDOptions []aid.Option

	// ParameterRuleYaklangCode 是原始的Yaklang CLI代码，用于AI理解和操作Yaklang环境
	// 不执行这段代码，只通过代码生成表单
	ParameterRuleYaklangCode string

	ToolKeywords []string
}

func NewForgeBlueprint(name string, opts ...Option) *ForgeBlueprint {
	forge := &ForgeBlueprint{
		Name: name,
	}
	for _, opt := range opts {
		opt(forge)
	}
	return forge
}

// Option 是一个函数类型，用于实现选项模式来配置ForgeBlueprint
type Option func(*ForgeBlueprint)

// WithAIDOptions 设置AI助手的配置选项
func WithAIDOptions(options ...aid.Option) Option {
	return func(f *ForgeBlueprint) {
		f.AIDOptions = append(f.AIDOptions, options...)
	}
}

// WithPlanMocker 设置AI助手的计划生成器
func WithPlanMocker(plan func(config *aid.Config) *aid.PlanResponse) Option {
	return func(f *ForgeBlueprint) {
		f.PlanMocker = plan
	}
}

// WithInitializePrompt 设置AI助手的初始化提示词
// 这个提示词会在AI助手启动时被使用，用于定义AI的初始状态和行为
func WithInitializePrompt(prompt string) Option {
	return func(f *ForgeBlueprint) {
		f.InitializePrompt = prompt
	}
}

// WithResultPrompt 设置AI助手的生成结果提示词
// 这个提示词会在AI助手生成结果时被使用，用于定义AI的输出格式和内容
func WithResultPrompt(prompt string) Option {
	return func(f *ForgeBlueprint) {
		f.ResultPrompt = prompt
	}
}

// WithResultHandler 设置AI助手的结果处理函数
// 这个函数会在AI助手生成结果后被调用，用于处理AI的输出
func WithResultHandler(handler func(string, error)) Option {
	return func(f *ForgeBlueprint) {
		f.ResultHandler = handler
	}
}

// WithPersistentPrompt 设置AI助手的持久提示词
// 这个提示词会在整个会话过程中持续存在，确保AI行为的一致性
func WithPersistentPrompt(persistentPrompt string) Option {
	return func(f *ForgeBlueprint) {
		f.PersistentPrompt = persistentPrompt
	}
}

// WithToolKeywords 设置AI助手的工具关键词
// 这些关键词可以扩展AI的能力，使其能够执行特定的任务
func WithToolKeywords(keywords []string) Option {
	return func(f *ForgeBlueprint) {
		f.ToolKeywords = append(f.ToolKeywords, keywords...)
	}
}

// WithTools 为AI助手添加可用的工具
// 这些工具可以扩展AI的能力，使其能够执行特定的任务
func WithTools(tools ...*aitool.Tool) Option {
	return func(f *ForgeBlueprint) {
		f.Tools = append(f.Tools, tools...)
	}
}

// WithOriginYaklangCliCode 设置原始的Yaklang CLI代码
// 这个结构需要 Yak 引擎根据 CLI 代码构建出正确的用户需要输入的工具
// 这个结构是表单构建的核心依据，可以使用 Yak 原声插件基础设施直接构建表单
func WithOriginYaklangCliCode(originYaklangCliCode string) Option {
	return func(f *ForgeBlueprint) {
		f.ParameterRuleYaklangCode = originYaklangCliCode
	}
}

// GenerateParameter
func (f *ForgeBlueprint) GenerateParameter() *ypb.YaklangInspectInformationResponse {
	prog, err := static_analyzer.SSAParse(f.ParameterRuleYaklangCode, "yak")
	if err != nil {
		log.Errorf("parse yaklang code failed: %v", err)
		return nil
	}
	cliInfo, uiInfo, pluginEnvKey := information.ParseCliParameter(prog)
	ret := &ypb.YaklangInspectInformationResponse{
		CliParameter: cliParam2grpc(cliInfo),
		UIInfo:       uiInfo2grpc(uiInfo),
		PluginEnvKey: pluginEnvKey,
	}
	return ret
}

// GenerateFirstPromptWithMemoryOption 用户根据 Origin
func (f *ForgeBlueprint) GenerateFirstPromptWithMemoryOption(
	params []*ypb.ExecParamItem,
) (string, []aid.Option, error) {
	initPrompt, err := f.renderInitPrompt("", params...)
	if err != nil {
		return "", nil, utils.Errorf("render init prompt failed: %v", err)
	}

	fmt.Println("initPrompt", initPrompt)
	persistentPrompt, err := f.renderPersistentPrompt("")
	if err != nil {
		return "", nil, utils.Errorf("render persistent prompt failed: %v", err)
	}

	var opts []aid.Option
	_ = persistentPrompt
	if persistentPrompt != "" {
		opts = append(opts, aid.WithAppendPersistentMemory(persistentPrompt))
	}

	if f.PlanMocker != nil {
		opts = append(opts, aid.WithPlanMocker(f.PlanMocker))
	}

	if len(f.Tools) > 0 {
		opts = append(opts, aid.WithTools(f.Tools...))
	}

	opts = append(opts, f.AIDOptions...)
	if f.ResultPrompt != "" && f.ResultHandler != nil {
		opts = append(opts, aid.WithResultHandler(func(config *aid.Config) {
			prompt, err := f.renderResultPrompt(config.GetMemory())
			if err != nil {
				f.ResultHandler("", utils.Errorf("render result prompt failed: %v", err))
				return
			}

			rsp, err := config.CallAI(aicommon.NewAIRequest(prompt))
			if err != nil {
				f.ResultHandler("", utils.Errorf("render result failed: %v", err))
				return
			}
			rspReader := rsp.GetOutputStreamReader("forge", true, config.GetEmitter())
			raw, err := io.ReadAll(rspReader)
			if err == io.EOF {
				f.ResultHandler(string(raw), nil)
			} else {
				f.ResultHandler(string(raw), err)
			}
		}))
	}

	return initPrompt, opts, nil
}

func (f *ForgeBlueprint) GenerateFirstPromptWithMemoryOptionWithQuery(
	query string,
) (string, []aid.Option, error) {
	params := []*ypb.ExecParamItem{
		{
			Key:   "query",
			Value: query,
		},
	}
	return f.GenerateFirstPromptWithMemoryOption(params)
}

func cliParam2grpc(params []*information.CliParameter) []*ypb.YakScriptParam {
	ret := make([]*ypb.YakScriptParam, 0, len(params))

	for _, param := range params {
		defaultValue := ""
		if param.Default != nil {
			defaultValue = fmt.Sprintf("%v", param.Default)
		}
		extra := []byte{}
		if param.Type == "select" {
			paramSelect := &PluginParamSelect{
				Double: param.MultipleSelect,
				Data:   make([]PluginParamSelectData, 0),
			}
			param.SelectOption.ForEach(func(k string, v any) {
				paramSelect.Data = append(paramSelect.Data, PluginParamSelectData{
					Key:   k,
					Label: k,
					Value: codec.AnyToString(v),
				})
			})
			extra, _ = json.Marshal(paramSelect)
		}

		ret = append(ret, &ypb.YakScriptParam{
			Field:                    param.Name,
			DefaultValue:             string(defaultValue),
			TypeVerbose:              param.Type,
			FieldVerbose:             param.NameVerbose,
			Help:                     param.Help,
			Required:                 param.Required,
			Group:                    param.Group,
			SuggestionDataExpression: param.SuggestionValueExpression,
			ExtraSetting:             string(extra),
			MethodType:               param.MethodType,
			JsonSchema:               param.JsonSchema,
			UISchema:                 param.UISchema,
		})
	}

	return ret
}

func uiInfo2grpc(info []*information.UIInfo) []*ypb.YakUIInfo {
	ret := make([]*ypb.YakUIInfo, 0, len(info))
	for _, i := range info {
		ret = append(ret, &ypb.YakUIInfo{
			Typ:            i.Typ,
			Effected:       i.Effected,
			WhenExpression: i.WhenExpression,
		})
	}
	return ret
}

type PluginParamSelect struct {
	Double bool                    `json:"double"`
	Data   []PluginParamSelectData `json:"data"`
}

type PluginParamSelectData struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
}
