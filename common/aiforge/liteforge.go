package aiforge

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	utils.Debug(func() {
		log.Info("liteforge.go is already registered aicommon.LiteForgeExecuteCallback")
	})
	aicommon.RegisterLiteForgeExecuteCallback(func(prompt string, opts ...any) (*aicommon.ForgeResult, error) {
		result, err := _executeLiteForgeTemp(prompt, opts...)
		if err != nil {
			return nil, err
		}
		final := &aicommon.ForgeResult{
			Action: result.Action,
		}
		if !utils.IsNil(result.Forge) {
			final.Name = result.Forge.Name
		} else {
			final.Name = "liteforge"
		}
		return final, nil
	})
}

type streamableField struct {
	AINodeId string
	FieldKey string
}

// LiteForge 被设计只允许提取数据，生成结构化（单步），如果需要多步拆解，不能使用 LiteForge
//
// 字段语义（关键词: aicache, PROMPT_SECTION, LiteForge 字段语义）：
//   - StaticInstruction: 系统侧稳定指令（不含用户输入/动态内容），渲染进 high-static 段，跨调用稳定哈希
//   - Prompt: 调用方动态上下文（可含用户输入、变化标签、动态参数等），渲染进 dynamic 段，外层 PROMPT_SECTION_dynamic_NONCE 已防 prompt-injection
type LiteForge struct {
	ForgeName           string
	Prompt              string
	StaticInstruction   string
	RequireSchema       string
	OutputSchema        string
	OutputActionName    string
	PreferSpeedPriority bool
	ExtendAIDOptions    []aicommon.ConfigOption

	streamFields         *omap.OrderedMap[string, *streamableField]
	fieldStreamCallbacks []*fieldStreamCallbackItem // user-defined callbacks for streaming fields
	emitter              *aicommon.Emitter

	OutputJsonHook []jsonextractor.CallbackOption
}

func WithLiteForge_Emitter(emitter *aicommon.Emitter) LiteForgeOption {
	return func(l *LiteForge) error {
		l.emitter = emitter
		return nil
	}
}

func WithLiteForge_StreamableFieldWithAINodeId(aiNodeId string, fieldKey string) LiteForgeOption {
	return func(l *LiteForge) error {
		l.streamFields.Set(fieldKey, &streamableField{
			AINodeId: aiNodeId,
			FieldKey: fieldKey,
		})
		return nil
	}
}

func WithLiteForge_StreamableField(fieldKey string) LiteForgeOption {
	return WithLiteForge_StreamableFieldWithAINodeId("thought", fieldKey)
}

// FieldStreamCallback is a callback for handling streaming field data
type FieldStreamCallback func(key string, r io.Reader)
type FieldStreamEmitterCallback func(key string, r io.Reader, emitter *aicommon.Emitter)

// fieldStreamCallbackItem stores callback info for streaming fields
type fieldStreamCallbackItem struct {
	FieldKeys []string
	Callback  FieldStreamEmitterCallback
}

// WithLiteForge_FieldStreamCallback registers a callback to be invoked when specified fields stream data.
// This enables extensibility for processing streaming JSON field data in real-time during LiteForge execution.
func WithLiteForge_FieldStreamCallback(fieldKeys []string, callback FieldStreamCallback) LiteForgeOption {
	return WithLiteForge_FieldStreamEmitterCallback(fieldKeys, func(key string, r io.Reader, _ *aicommon.Emitter) {
		callback(key, r)
	})
}

func WithLiteForge_FieldStreamEmitterCallback(fieldKeys []string, callback FieldStreamEmitterCallback) LiteForgeOption {
	return func(l *LiteForge) error {
		if l.fieldStreamCallbacks == nil {
			l.fieldStreamCallbacks = make([]*fieldStreamCallbackItem, 0)
		}
		l.fieldStreamCallbacks = append(l.fieldStreamCallbacks, &fieldStreamCallbackItem{
			FieldKeys: fieldKeys,
			Callback:  callback,
		})
		return nil
	}
}

type LiteForgeOption func(*LiteForge) error

func WithLiteForge_SpeedPriority(b ...bool) LiteForgeOption {
	return func(l *LiteForge) error {
		if len(b) == 0 || b[0] {
			l.PreferSpeedPriority = true
		}
		return nil
	}
}

func WithLiteForge_RequireParams(params ...aitool.ToolOption) LiteForgeOption {
	return func(l *LiteForge) error {
		t := aitool.NewWithoutCallback("", params...)
		for _, param := range params {
			param(t)
		}
		l.RequireSchema = t.ToJSONSchemaString()
		return nil
	}
}

func WithLiteForge_OutputSchema(params ...aitool.ToolOption) LiteForgeOption {
	return func(l *LiteForge) error {
		t := aitool.NewWithoutCallback(
			"output", params...)
		l.OutputSchema = t.ToJSONSchemaString()
		l.OutputActionName = "call-tool"
		return nil
	}
}

func WithLiteForge_OutputSchemaRaw(actionName string, outputSchema string) LiteForgeOption {
	return func(l *LiteForge) error {
		l.OutputActionName = actionName
		l.OutputSchema = outputSchema
		return nil
	}
}

func WithLiteForge_OutputJsonHook(hook ...jsonextractor.CallbackOption) LiteForgeOption {
	return func(l *LiteForge) error {
		if l.OutputJsonHook == nil {
			l.OutputJsonHook = make([]jsonextractor.CallbackOption, 0)
		}
		l.OutputJsonHook = append(l.OutputJsonHook, hook...)
		return nil
	}
}

func WithLiteForge_OutputMemoryOP() LiteForgeOption {
	return func(l *LiteForge) error {
		t := aitool.NewWithoutCallback(
			"output", aid.MemoryOpSchemaOption...)
		return WithLiteForge_OutputSchemaRaw(aid.MemoryOpAction, t.ParamsJsonSchemaString())(l)
	}
}

func WithExtendLiteForge_AIOption(opts ...aicommon.ConfigOption) LiteForgeOption {
	return func(l *LiteForge) error {
		if l.ExtendAIDOptions == nil {
			l.ExtendAIDOptions = make([]aicommon.ConfigOption, 0)
		}
		l.ExtendAIDOptions = append(l.ExtendAIDOptions, opts...)
		return nil
	}
}

func WithLiteForge_Prompt(i string) LiteForgeOption {
	return func(forge *LiteForge) error {
		forge.Prompt = i
		return nil
	}
}

// WithLiteForge_StaticInstruction 设置 LiteForge 的系统侧静态指令
// 该指令进入 high-static 段，跨调用稳定哈希
// 关键词: aicache, PROMPT_SECTION, StaticInstruction, WithLiteForge_StaticInstruction
func WithLiteForge_StaticInstruction(i string) LiteForgeOption {
	return func(forge *LiteForge) error {
		forge.StaticInstruction = i
		return nil
	}
}

func NewLiteForge(i string, opts ...LiteForgeOption) (*LiteForge, error) {
	lf := &LiteForge{
		ForgeName:    i,
		streamFields: omap.NewOrderedMap[string, *streamableField](make(map[string]*streamableField)),
	}
	for _, o := range opts {
		err := o(lf)
		if err != nil {
			return nil, err
		}
	}
	return lf, nil
}

func (l *LiteForge) Execute(ctx context.Context, params []*ypb.ExecParamItem, opts ...aicommon.ConfigOption) (*ForgeResult, error) {
	return l.ExecuteEx(ctx, params, nil, opts...)
}

func (l *LiteForge) ExecuteEx(ctx context.Context, params []*ypb.ExecParamItem, imageData []*aicommon.ImageData, opts ...aicommon.ConfigOption) (*ForgeResult, error) {
	cod, err := aid.NewCoordinatorContext(ctx, l.Prompt, append(l.ExtendAIDOptions, opts...)...)
	if err != nil {
		return nil, utils.Errorf("cannot create coordinator: %v", err)
	}

	if l.OutputSchema == "" {
		l.OutputSchema = cod.GetAIConfig().LiteForgeOutputSchema
		l.OutputActionName = cod.GetAIConfig().LiteForgeActionName
	}

	if l.OutputSchema == "" {
		return nil, utils.Error("liteforge output schema is required, you should set it via aiforge.WithLiteForge_OutputSchema or aicommon.WithLiteForgeOutputSchema config option")
	}

	nonce := strings.ToLower(utils.RandStringBytes(6))
	var callBuffer bytes.Buffer
	if len(params) == 1 {
		callBuffer.WriteString(params[0].Value)
	} else {
		for _, i := range params {
			if strings.Contains(i.Value, "\n") {
				callBuffer.WriteString(i.Key + ": \n")
				callBuffer.WriteString(utils.PrefixLines(i.Value, "  "))
			} else {
				callBuffer.WriteString(fmt.Sprintf("%v: %v\n", i.Key, i.Value))
			}
		}
	}
	call := callBuffer.String()

	rendered, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:             nonce,
		Prompt:            string(l.Prompt),
		StaticInstruction: string(l.StaticInstruction),
		Params:            call,
		Schema:            string(l.OutputSchema),
		PersistentMemory:  cod.ContextProvider.PersistentMemory(),
		TimelineDump:      cod.ContextProvider.TimelineDump(),
	})
	if err != nil {
		return nil, err
	}
	var action *aicommon.Action
	aiCallback := cod.CallAI
	if l.PreferSpeedPriority {
		aiCallback = cod.CallSpeedPriorityAI
	}
	transactionErr := aicommon.CallAITransaction(cod, rendered, aiCallback,
		func(response *aicommon.AIResponse) error {
			boundEmitter := response.BindEmitter(l.emitter)
			if l.ForgeName == "" {
				l.ForgeName = "LiteForge"
			}
			result := response.GetOutputStreamReader(fmt.Sprintf(`liteforge[%v]`, l.ForgeName), true, cod.GetEmitter())
			var mirrored bytes.Buffer
			var actionOpts = []aicommon.ActionMakerOption{
				aicommon.WithActionJSONCallback(l.OutputJsonHook...),
			}

			// add streamable fields handlers
			for _, i := range l.streamFields.Values() {
				i := i
				actionOpts = append(actionOpts, aicommon.WithActionFieldStreamHandler([]string{i.FieldKey}, func(key string, r io.Reader) {
					r = utils.JSONStringReader(r)
					if utils.IsNil(l.emitter) {
						r = io.TeeReader(r, os.Stdout)
						io.Copy(io.Discard, r)
						return
					}

					utils.Debug(func() {
						r = io.TeeReader(r, os.Stdout)
					})
					boundEmitter.EmitDefaultStreamEvent(i.AINodeId, r, response.GetTaskIndex())
				}))
			}

			// add user-defined field stream callbacks
			for _, item := range l.fieldStreamCallbacks {
				item := item
				actionOpts = append(actionOpts, aicommon.WithActionFieldStreamHandler(item.FieldKeys, func(key string, r io.Reader) {
					if item.Callback != nil {
						item.Callback(key, r, boundEmitter)
					}
				}))
			}

			actionNames := []string{}
			if l.OutputActionName == "" {
				actionNames = append(actionNames, "call-tool", "object")
			} else {
				actionNames = append(actionNames, l.OutputActionName)
			}
			actionOpts = append(actionOpts, aicommon.WithActionAlias(actionNames...))

			action, err = aicommon.ExtractValidActionFromStream(ctx, io.TeeReader(result, &mirrored), "object", actionOpts...)
			if err != nil {
				return utils.Errorf("extract action failed: %v", err)
			}
			if action == nil {
				return utils.Errorf("action is nil(unknown reason): \n%v", mirrored.String())
			}
			return nil
		},
		lo.Map(imageData, func(item *aicommon.ImageData, _ int) aicommon.AIRequestOption {
			return aicommon.WithAIRequest_ImageData(item)
		})...,
	)
	if transactionErr != nil {
		return nil, utils.Errorf("liteforge execute failed: %v", transactionErr)
	}
	result := &ForgeResult{Action: action}
	return result, nil
}

// liteForgePromptParams 是 LiteForge prompt 渲染时传入模板的字段集合
// 关键词: aicache, PROMPT_SECTION, LiteForge 模板, liteForgePromptParams
type liteForgePromptParams struct {
	Nonce             string
	Prompt            string
	StaticInstruction string
	Params            string
	Schema            string
	PersistentMemory  string
	TimelineDump      string
}

// liteForgePromptTemplate 是 LiteForge 的 prompt 模板，按 aicache 4 段 PROMPT_SECTION 框架包装：
//   - high-static 段：# Preset / # Output Formatter / # SCHEMA / # Instruction（系统侧静态指令，跨调用稳定）
//   - semi-dynamic 段：# 牢记（PersistentMemory，半动态）
//   - timeline 段：<timeline_NONCE>...</timeline_NONCE>（每次变化）
//   - dynamic 段：<context_NONCE>...</context_NONCE>（调用方动态上下文）+ <params_NONCE>...</params_NONCE>（用户参数）；
//     外层 PROMPT_SECTION_dynamic_NONCE 屏蔽 prompt-injection
//
// high-static 段内 <schema>/<instruction> 不带 nonce，确保同 forge 跨调用的 hash 真正稳定
// Prompt 字段从 high-static 段挪到 dynamic 段，调用方传入的"动态内容"不再污染 high-static 段的 hash
// 关键词: aicache, PROMPT_SECTION, LiteForge 模板, liteForgePromptTemplate
const liteForgePromptTemplate = `<|PROMPT_SECTION_high-static|>
# Preset
你现在在一个任务引擎中，是一个输出JSON的数据处理和总结提示小助手，我会为你提供一些基本信息和输入材料，你需要按照我的Schema生成一个JSON数据直接返回。

作为系统的一部分你应该直接返回JSON，避免多余的解释。

# Output Formatter

请你根据下面 SCHEMA 构建数据 , 注意事项：
1. 你必须严格按照 SCHEMA 格式生成数据，不能缺少任何字段，不能多余任何字段。
2. 所有字符串类型的数据必须使用双引号括起来，数字类型的数据不能使用引号括起来，布尔类型的数据必须使用 true 或 false 。
3. 如果某个字段是可选的，你可以选择不返回该字段，但如果返回了该字段，必须符合 SCHEMA 的要求。
4. 不要添加任何多余的解释或文本，只返回符合 SCHEMA 的 JSON 数据。
5. 不要输出压缩成一行的 JSON，请保持良好的可读性和缩进。
{{ if .Schema }}
# SCHEMA

<schema>
{{ .Schema }}
</schema>
{{ end }}{{ if .StaticInstruction }}
# Instruction

<instruction>
{{ .StaticInstruction }}
</instruction>
{{ end }}<|PROMPT_SECTION_END_high-static|>

<|PROMPT_SECTION_semi-dynamic|>
{{ if .PersistentMemory }}# 牢记
{{ .PersistentMemory }}{{ end }}
<|PROMPT_SECTION_END_semi-dynamic|>
{{ if .TimelineDump }}
<|PROMPT_SECTION_timeline|>
<timeline_{{ .Nonce }}>
{{ .TimelineDump }}
</timeline_{{ .Nonce }}>
<|PROMPT_SECTION_END_timeline|>
{{ end }}
<|PROMPT_SECTION_dynamic_{{ .Nonce }}|>
{{ if .Prompt }}<context_{{ .Nonce }}>
{{ .Prompt }}
</context_{{ .Nonce }}>
{{ end }}{{ if .Params }}<params_{{ .Nonce }}>
{{ .Params }}
</params_{{ .Nonce }}>{{ end }}
<|PROMPT_SECTION_dynamic_END_{{ .Nonce }}|>
`

// renderLiteForgePrompt 按 4 段 PROMPT_SECTION 框架渲染 LiteForge prompt
// 关键词: aicache, PROMPT_SECTION, LiteForge 模板, renderLiteForgePrompt
func renderLiteForgePrompt(p liteForgePromptParams) (string, error) {
	tmp, err := template.New("liteforge").Parse(liteForgePromptTemplate)
	if err != nil {
		return "", utils.Errorf("template parse failed: %v", err)
	}
	var buf bytes.Buffer
	if err := tmp.Execute(&buf, p); err != nil {
		return "", utils.Errorf("template execute failed: %v", err)
	}
	return buf.String(), nil
}
