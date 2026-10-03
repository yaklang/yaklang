package aiforge

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	aidliteforge "github.com/yaklang/yaklang/common/ai/aid/liteforge"
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
//   - StaticInstruction: 系统侧稳定指令（不含用户输入/动态内容），渲染进 semi-dynamic 段，同一用途跨调用稳定
//   - Prompt: 调用方动态上下文（可含用户输入、变化标签、动态参数等），渲染进 dynamic 段，外层 PROMPT_SECTION_dynamic_NONCE 已防 prompt-injection
type LiteForge struct {
	ForgeName           string
	Prompt              string
	StaticInstruction   string
	RequireSchema       string
	OutputSchema        string
	OutputActionName    string
	PreferSpeedPriority bool
	DisableTimeline     bool
	maxPromptTokens     int
	ExtendAIDOptions    []aicommon.ConfigOption

	streamFields         *omap.OrderedMap[string, *streamableField]
	fieldStreamCallbacks []*fieldStreamCallbackItem // user-defined callbacks for streaming fields
	emitter              *aicommon.Emitter

	OutputJsonHook  []jsonextractor.CallbackOption
	OutputValidator func(*aicommon.Action) error

	// extraRequestOpts carries AIRequestOption values that are appended to
	// reqOpts during Execute, allowing callers to inject parameters like
	// aispec.WithThinkingLevel("none") through the LiteForge option chain.
	extraRequestOpts []aicommon.AIRequestOption
	responseHandler  aicommon.AuxiliaryResponseHandler
}

// WithLiteForge_ResponseHandler uses a complete caller-supplied prompt and
// response protocol for one transaction, retaining LiteForge's model invocation,
// retries and cancellation. The handler only parses/validates the response.
func WithLiteForge_ResponseHandler(handler aicommon.AuxiliaryResponseHandler) LiteForgeOption {
	return func(l *LiteForge) error { l.responseHandler = handler; return nil }
}

// WithLiteForge_OutputValidator adds caller-specific validation to the existing
// transaction retry loop. It must not perform persistence or other side effects.
func WithLiteForge_OutputValidator(validate func(*aicommon.Action) error) LiteForgeOption {
	return func(l *LiteForge) error { l.OutputValidator = validate; return nil }
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
	FieldKeys        []string
	Callback         FieldStreamEmitterCallback
	ResponseCallback aicommon.StreamableFieldResponseCallback
}

func WithLiteForge_FieldStreamResponseCallback(fieldKeys []string, callback aicommon.StreamableFieldResponseCallback) LiteForgeOption {
	return func(l *LiteForge) error {
		l.fieldStreamCallbacks = append(l.fieldStreamCallbacks, &fieldStreamCallbackItem{
			FieldKeys: fieldKeys, ResponseCallback: callback,
		})
		return nil
	}
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

func WithLiteForge_MaxPromptTokens(limit int) LiteForgeOption {
	return func(l *LiteForge) error { l.maxPromptTokens = limit; return nil }
}

// WithLiteForge_DisableTimeline is for callers whose dynamic prompt already
// carries a deliberately bounded trace. It prevents LiteForge from appending
// a second recent Timeline copy to the same lightweight request.
func WithLiteForge_DisableTimeline() LiteForgeOption {
	return func(l *LiteForge) error {
		l.DisableTimeline = true
		return nil
	}
}

func WithLiteForge_SpeedPriority(b ...bool) LiteForgeOption {
	return func(l *LiteForge) error {
		if len(b) == 0 || b[0] {
			l.PreferSpeedPriority = true
		}
		return nil
	}
}

// WithLiteForge_RequireParams 设置 LiteForge 的输入参数 schema（导出名为 aiagent.liteForgedRequireParams）
// 参数:
//   - params: 一个或多个 aitool 参数选项
//
// 返回值:
//   - LiteForge 可选项
//
// Example:
// ```
// opt = aiagent.liteForgedRequireParams(aitool.WithStringParam("target"))
// println(opt)
// ```
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

// WithLiteForge_OutputSchema 设置 LiteForge 的输出结构 schema（导出名为 aiagent.liteForgeOutputSchema）
// 参数:
//   - params: 一个或多个 aitool 参数选项，描述期望的输出字段
//
// 返回值:
//   - LiteForge 可选项
//
// Example:
// ```
// opt = aiagent.liteForgeOutputSchema(aitool.WithStringParam("title"))
// println(opt)
// ```
func WithLiteForge_OutputSchema(params ...aitool.ToolOption) LiteForgeOption {
	return func(l *LiteForge) error {
		t := aitool.NewWithoutCallback(
			"output", params...)
		l.OutputSchema = t.ToJSONSchemaString()
		l.OutputActionName = "call-tool"
		return nil
	}
}

// WithLiteForge_OutputSchemaRaw 通过原始 JSON Schema 字符串设置输出结构（导出名为 aiagent.liteForgeOutputSchemaRaw）
// 参数:
//   - actionName: action 名称
//   - outputSchema: 原始 JSON Schema 字符串
//
// 返回值:
//   - LiteForge 可选项
//
// Example:
// ```
// opt = aiagent.liteForgeOutputSchemaRaw("call-tool", `{"type":"object"}`)
// println(opt)
// ```
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

func WithExtendLiteForge_AIOption(opts ...aicommon.ConfigOption) LiteForgeOption {
	return func(l *LiteForge) error {
		if l.ExtendAIDOptions == nil {
			l.ExtendAIDOptions = make([]aicommon.ConfigOption, 0)
		}
		l.ExtendAIDOptions = append(l.ExtendAIDOptions, opts...)
		return nil
	}
}

// WithLiteForge_Prompt 设置 LiteForge 的"上下文/动态" prompt 文本。
//
// 渲染后该字段被包在 <|PROMPT_SECTION_dynamic_<nonce>|>...<|PROMPT_SECTION_dynamic_END_<nonce>|>
// 内的 <context_<nonce>>...</context_<nonce>> 子标签里, 即 dynamic 段。dynamic
// 段每次请求都包含 nonce 因此 byte-hash 必定不同, 没有跨调用 prefix cache 命中
// 价值, **不要把任何静态指令** (例如 INSTRUCTION / CRITICAL RULES / Selection
// rules) 塞进这里。
//
// 调用约定 (P0-B4):
//   - 真正动态的内容 (USER_QUERY / PARENT_TASK / CURRENT_TASK / 当次具体输入) ->
//     WithLiteForge_Prompt
//   - 跨调用稳定的指令文本 (规则 / Schema / 角色定义) ->
//     WithLiteForge_StaticInstruction (进 semi-dynamic 段) 或写到调用方传给
//     NewLiteForge 的 schema 里
//
// 关键词: WithLiteForge_Prompt, dynamic 段, 调用方约定
// 参数:
//   - i: 动态 prompt 文本
//
// 返回值:
//   - LiteForge 可选项
//
// Example:
// ```
// opt = aiagent.liteForgePrompt("extract the title from the html")
// println(opt)
// ```
func WithLiteForge_Prompt(i string) LiteForgeOption {
	return func(forge *LiteForge) error {
		forge.Prompt = i
		return nil
	}
}

// WithLiteForge_StaticInstruction 设置 LiteForge 的系统侧"静态指令"。
//
// 渲染后该字段进入 <|PROMPT_SECTION_semi-dynamic|> 段 (P0-B1: 历史上曾在
// high-static 段, 但因 schema / instruction 通常按 forge 维度变化, 留在
// high-static 会让该段跨 forge 永远 miss; 下移后高频调用同一 forge 时
// semi-dynamic 段 byte 稳定可命中前缀缓存)。
//
// 跨同一 forge 多次调用时该字段必须保持 byte 一致 (相同 schema / 相同规则),
// 才能让 semi-dynamic 段 hash 稳定。任何动态拼接 (例如附加用户 query / 当次
// 任务 ID) 必须改用 WithLiteForge_Prompt。
//
// 关键词: aicache, PROMPT_SECTION_semi-dynamic, StaticInstruction,
//
//	WithLiteForge_StaticInstruction
func WithLiteForge_StaticInstruction(i string) LiteForgeOption {
	return func(forge *LiteForge) error {
		forge.StaticInstruction = i
		return nil
	}
}

// WithLiteForge_ExtraRequestOpts carries additional AIRequestOption values
// that are appended to reqOpts during Execute, allowing callers to inject
// parameters (e.g. aispec.WithThinkingLevel("none")) through the LiteForge
// option chain. Used by the auxiliary task scheduler for LiteCall degradation.
func WithLiteForge_ExtraRequestOpts(opts ...aicommon.AIRequestOption) LiteForgeOption {
	return func(forge *LiteForge) error {
		forge.extraRequestOpts = append(forge.extraRequestOpts, opts...)
		return nil
	}
}

// WithLiteForge_DynamicInstruction 是 WithLiteForge_Prompt 的语义别名,
// 显式表达"该 instruction 是 dynamic 段, 进 dynamic 而非 semi-dynamic"。
// 调用方在重构时 (P0-B4) 把 prompt 字符串拆成静态 + 动态两部分时,
// 推荐用 WithLiteForge_StaticInstruction + WithLiteForge_DynamicInstruction
// 这一对来代替单一 WithLiteForge_Prompt, 让调用点读起来更清晰。
//
// 关键词: WithLiteForge_DynamicInstruction, dynamic 段语义别名,
//
//	WithLiteForge_Prompt 同义
func WithLiteForge_DynamicInstruction(i string) LiteForgeOption {
	return WithLiteForge_Prompt(i)
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
	var call bytes.Buffer
	if len(params) == 1 {
		call.WriteString(params[0].Value)
	} else {
		for _, item := range params {
			if strings.Contains(item.Value, "\n") {
				call.WriteString(item.Key + ": \n")
				call.WriteString(utils.PrefixLines(item.Value, "  "))
			} else {
				fmt.Fprintf(&call, "%v: %v\n", item.Key, item.Value)
			}
		}
	}
	request := aidliteforge.Request{Name: l.ForgeName, ActionName: l.OutputActionName, Schema: l.OutputSchema,
		Prompt: l.Prompt, Params: call.String(), StaticInstruction: l.StaticInstruction,
		PreferSpeed: l.PreferSpeedPriority, DisableTimeline: l.DisableTimeline,
		MaxPromptTokens: l.maxPromptTokens, Images: imageData, Emitter: l.emitter,
		JSONHooks: l.OutputJsonHook, Validate: l.OutputValidator, ResponseHandler: l.responseHandler,
		ExtraOptions: l.extraRequestOpts}
	for _, field := range l.streamFields.Values() {
		request.StreamFields = append(request.StreamFields, aidliteforge.StreamField{NodeID: field.AINodeId, Key: field.FieldKey})
	}
	for _, field := range l.fieldStreamCallbacks {
		request.FieldCallbacks = append(request.FieldCallbacks, aidliteforge.FieldCallback{
			Keys: field.FieldKeys, Callback: field.Callback, Response: field.ResponseCallback})
	}
	// Use Config's default native protocol; callers can explicitly select text.
	configOptions := append([]aicommon.ConfigOption{}, l.ExtendAIDOptions...)
	configOptions = append(configOptions, opts...)
	result, err := aidliteforge.Execute(ctx, request, configOptions...)
	if err != nil {
		return nil, err
	}
	return &ForgeResult{Action: result.Action}, nil
}
