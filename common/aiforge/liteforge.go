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

	timelineFrozen, timelineOpen := cod.ContextProvider.TimelineDumpFrozenOpen()
	rendered, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:               nonce,
		Prompt:              string(l.Prompt),
		StaticInstruction:   string(l.StaticInstruction),
		Params:              call,
		Schema:              string(l.OutputSchema),
		PersistentMemory:    cod.ContextProvider.PersistentMemory(),
		TimelineFrozenBlock: timelineFrozen,
		TimelineOpen:        timelineOpen,
	})
	if err != nil {
		return nil, err
	}
	var action *aicommon.Action
	aiCallback := cod.CallAI
	if l.PreferSpeedPriority {
		aiCallback = cod.CallSpeedPriorityAI
	}
	transactionErr := aicommon.CallAITransactionWithFailureExtra(cod, rendered, aiCallback,
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
		map[string]any{
			"liteforge_action": l.ForgeName,
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

// liteForgePromptParams 是 LiteForge prompt 渲染时传入模板的字段集合。
//
// aicache 5 段稳定性分层 (P0-B1 / P0-B3) 必填字段:
//   - TimelineFrozenBlock: timeline 的 reducer + 非末 interval (frozen 前缀, 字节稳定)
//   - TimelineOpen: timeline 的最末 interval + midterm (易变尾段)
//   - TimelineDump: 兼容字段, 仅当 TimelineFrozenBlock + TimelineOpen 全为空
//     时回退到 legacy 渲染路径
//
// 关键词: aicache, PROMPT_SECTION, LiteForge 模板, liteForgePromptParams,
//
//	TimelineFrozenBlock, TimelineOpen
type liteForgePromptParams struct {
	Nonce               string
	Prompt              string
	StaticInstruction   string
	Params              string
	Schema              string
	PersistentMemory    string
	TimelineDump        string // legacy 兼容字段
	TimelineFrozenBlock string // frozen 前缀, 进 AI_CACHE_FROZEN 块
	TimelineOpen        string // 易变尾段, 进 PROMPT_SECTION_timeline-open
}

// liteForgePromptTemplate 是 LiteForge 的 prompt 模板，按 aicache 5 段稳定性
// 分层框架包装：
//   - high-static 段：# Preset / # Output Formatter（仅放真正系统级、跨 forge 字节稳定的内容）
//   - semi-dynamic 段：# SCHEMA / # Instruction / # 牢记 (PersistentMemory)
//     这三个字段都是"按 forge 维度稳定但跨 forge 不同"，不放高静态段，避免污染
//     不同 forge 调用之间的 high-static hash
//   - frozen-block 段：TimelineFrozenBlock (RenderWithFrozenBoundary 的 frozen 前缀)
//   - timeline-open 段：TimelineOpen (RenderWithFrozenBoundary 的 open 尾段)
//   - dynamic 段：<context_NONCE>...</context_NONCE>（调用方动态上下文）+ <params_NONCE>...</params_NONCE>（用户参数）；
//     外层 PROMPT_SECTION_dynamic_NONCE 屏蔽 prompt-injection
//
// 之前的实现把 .Schema 与 .StaticInstruction 放进 high-static 段, 导致每个 forge
// 都创建一份新的 high-static hash (cachebench 实测 16 个不同 hash, 应当为 1)，
// P0-B1 把它们下移到 semi-dynamic, 让真正系统级的 # Preset / # Output Formatter
// 段 hash 在所有 forge / 跨调用中保持完全一致。
//
// 关键词: aicache, P0-B1, AI_CACHE_SYSTEM, PROMPT_SECTION, LiteForge 模板,
//
//	liteForgePromptTemplate, schema 下移 semi-dynamic, instruction 下移
const liteForgePromptTemplate = `<|AI_CACHE_SYSTEM_high-static|>
# Preset
你现在在一个任务引擎中，是一个输出 JSON 的数据处理和总结提示小助手。系统会为你提供基本信息与输入材料，你需要按照我提供的 Schema 生成一个 JSON 数据直接返回。作为系统的一部分你应该直接返回 JSON，避免多余的解释。

# Role Boundary (角色边界)

- LiteForge 是单步结构化抽取器，不是多步规划器；任何调用都在一次模型响应内完成。
- 不调度子任务、不发起工具调用、不读写文件、不访问网络、不调用浏览器，也不会等到"下一步"。
- 不输出"我先思考一下 / 让我们一步步来"这类铺垫段落；看到 schema 即直接生成对应 JSON。
- 你不是对话伙伴，不要询问用户、不要请求补充材料、不要给出可执行计划，只在 schema 的字段维度内做判断。
- 即便输入信息高度残缺，仍按 schema 输出最完整的合法 JSON，缺失字段以"合法空值"或"标注 unknown"的方式表达，不去拒答或反向追问。

# Reasoning Discipline (推理纪律)

你的推理空间被三件事严格约束：schema 的字段定义、当前输入材料、持久记忆 (PersistentMemory) 与时间线 (Timeline) 中已沉淀的事实。除此之外的内容都属于"猜测"。

- schema 没声明的字段一律不要新增，更不要把"我觉得有用"的额外字段塞进根对象。
- 输入材料没给的事实不要凭直觉补全；不知道就把字段留空、置 null、或选 unknown。
- 涉及枚举 (enum) / 正则 (pattern) / 数值边界 (minimum / maximum) 的字段，严格遵循 schema 取值；与材料对齐不上时优先选枚举里语义最弱的那个 (例如 unknown / other)。
- 数组类字段优先按"输入材料中证据出现的顺序"组织，不要重新排序，也不要去重到信息丢失。
- 涉及总结 / 摘要类字段时，先复述材料中的事实证据，再做归纳；禁止脑补人物、动机、时间、地名、数字、URL。
- 同一信息在 schema 多个字段中出现时，保持一致；不要在一个字段说 A，另一个字段说 not A。

# Output Style (输出风格)

- 风格保持严肃、精确、中性，等同一个有经验的工程师在写结构化日志。
- 全程禁用 emoji；不要使用装饰性 unicode 符号 (例如 ✓ ✗ → ★ ⚠ ➜ 等)，也不要使用全角标点修饰技术信息。
- 不写"以下是..."/"希望对你有帮助"/"如有疑问请告诉我"这类前后缀；不要复述用户的请求作为开头。
- JSON 之外不要包 markdown 代码块围栏 (例如三个反引号配 json 这种 fenced code block 形式)，也不要附带"返回结果如下"之类说明。
- 字符串值的语言遵循材料语言：中文输入对应字段中文摘要，英文输入对应英文摘要；字段键名严格保持 schema 中的英文 key、原本的大小写与下划线写法不变。
- 涉及代码 / 命令 / 标识符等技术片段时，按字面值原样复刻，不做美化、不做翻译。

# Output Formatter (形式准则)

请你根据下面 SCHEMA 构建数据，遵守以下硬约束：

1. 严格按 SCHEMA 生成 JSON：不能缺少 required 字段，不能新增 schema 之外的字段。
2. 字符串类型必须使用双引号包裹；数字类型不要被引号包裹；布尔类型必须是 true 或 false (不允许 "true" / "yes" / 1)。
3. 可选字段允许省略；一旦输出则必须满足该字段的类型 / 枚举 / 长度等约束，不允许"输出但不合规"的中间状态。
4. 不在 JSON 之外追加任何解释、提示、警告或注释；不要使用 // 行注释或 /* */ 块注释 (JSON 标准不允许)。
5. 输出保持可读缩进 (2 空格或 4 空格均可，但同一份 JSON 内保持一致)，不要把 JSON 压缩成单行。
6. 数组与对象内不允许 trailing comma；不允许同一对象重复 key；不允许在 JSON 之后追加额外内容 (例如多余的右花括号或日志行)。
7. 必填字段即便信息缺失也要给出合法默认：空字符串、空数组、空对象、或 null (取决于 schema 允许的最弱合法值)，不要因为"没有信息"而整字段省略。
8. 涉及二进制 / 图像 / 文件路径时，优先用 schema 提供的字段表达；不允许把原始 base64 直接塞进 summary 类字段污染下游索引。

# Common Failure Modes (常见错误规避)

下列模式是 LiteForge 历史上频繁出错的地方，输出前请逐项自检：

- 不要把 JSON 整体包进 markdown 代码块；输出第一个非空白字符必须是左花括号或左方括号，最后一个非空白字符必须是右花括号或右方括号。
- 不要在 JSON 末尾追加诸如 "Done." / "希望以上结果对你有帮助" / "let me know if ..." 类话术。
- 不要把 schema 中的字段名翻译成其他语言或改写大小写 (例如 schema 是 embedding_text 就不能输出 embeddingText / EmbeddingText)。
- 不要把 schema 之外的辅助计算 (例如 token 数、置信度) 当成额外字段输出，除非 schema 明确包含它。
- 不要在数组里塞 null 占位 (例如长度为三、前两个元素是 null 这种)；如果某个槽位无内容，整个数组应缩短而不是占位。
- 不要因为输入材料过长而擅自截断；如材料超长，先在材料范围内做摘要再填字段，但不允许在 JSON 内插入"[已截断]"等提示。
- 不要把不确定的事实标注成确定值；优先用 schema 的弱值 (空 / null / unknown) 而不是编造。

# 工作循环约定 (Working Loop Convention)

- 步骤 1: 通读输入材料 + 持久记忆 + 时间线，识别 schema 中每个 required 字段对应的事实证据。
- 步骤 2: 对每个字段先在心里写出"证据来源 + 字段值"两栏，再决定输出。
- 步骤 3: 按 schema 顺序构建 JSON 对象，可选字段视证据强度决定保留或省略。
- 步骤 4: 自检——逐条对照本文件的 Output Formatter 与 Common Failure Modes 清单，确认无违反项。
- 步骤 5: 直接返回 JSON，不附前缀也不附后缀。

记住：你输出的 JSON 会被下游程序按字节逐位解析与索引；任何"对人友好但对解析器不友好"的修饰都会让整条管道失败。
<|AI_CACHE_SYSTEM_END_high-static|>

<|PROMPT_SECTION_semi-dynamic|>
{{ if .Schema }}# SCHEMA

<schema>
{{ .Schema }}
</schema>
{{ end }}{{ if .StaticInstruction }}# Instruction

<instruction>
{{ .StaticInstruction }}
</instruction>
{{ end }}{{ if .PersistentMemory }}# 牢记
{{ .PersistentMemory }}{{ end }}
<|PROMPT_SECTION_END_semi-dynamic|>
{{ if .TimelineFrozenBlock }}
<|AI_CACHE_FROZEN_semi-dynamic|>
{{ .TimelineFrozenBlock }}
<|AI_CACHE_FROZEN_END_semi-dynamic|>
{{ end }}{{ if .TimelineOpen }}
<|PROMPT_SECTION_timeline-open|>
<timeline_{{ .Nonce }}>
{{ .TimelineOpen }}
</timeline_{{ .Nonce }}>
<|PROMPT_SECTION_END_timeline-open|>
{{ else if and (not .TimelineFrozenBlock) .TimelineDump }}
<|PROMPT_SECTION_timeline|>
<timeline_{{ .Nonce }}>
{{ .TimelineDump }}
</timeline_{{ .Nonce }}>
<|PROMPT_SECTION_END_timeline|>
{{ else }}
<|PROMPT_SECTION_timeline-open|>
<|PROMPT_SECTION_END_timeline-open|>
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
