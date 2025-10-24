package aiforge

import (
	"bytes"
	"context"
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"strings"
	"text/template"
)

// LiteForge 被设计只允许提取数据，生成结构化（单步），如果需要多步拆解，不能使用 LiteForge
type LiteForge struct {
	ForgeName        string
	Prompt           string
	RequireSchema    string
	OutputSchema     string
	OutputActionName string
	ExtendAIDOptions []aid.Option

	OutputJsonHook []jsonextractor.CallbackOption
}

type LiteForgeOption func(*LiteForge) error

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

func WithExtendLiteForge_AIDOption(opts ...aid.Option) LiteForgeOption {
	return func(l *LiteForge) error {
		if l.ExtendAIDOptions == nil {
			l.ExtendAIDOptions = make([]aid.Option, 0)
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

func NewLiteForge(i string, opts ...LiteForgeOption) (*LiteForge, error) {
	lf := &LiteForge{
		ForgeName: i,
	}
	for _, o := range opts {
		err := o(lf)
		if err != nil {
			return nil, err
		}
	}
	return lf, nil
}

func (l *LiteForge) Execute(ctx context.Context, params []*ypb.ExecParamItem, opts ...aid.Option) (*ForgeResult, error) {
	return l.ExecuteEx(ctx, params, nil, opts...)
}

func (l *LiteForge) ExecuteEx(ctx context.Context, params []*ypb.ExecParamItem, imageData []*aicommon.ImageData, opts ...aid.Option) (*ForgeResult, error) {
	if l.OutputSchema == "" {
		return nil, fmt.Errorf("liteforge output schema is required")
	}

	cod, err := aid.NewCoordinatorContext(ctx, l.Prompt, append(l.ExtendAIDOptions, opts...)...)
	if err != nil {
		return nil, utils.Errorf("cannot create coordinator: %v", err)
	}

	nonce := strings.ToLower(utils.RandStringBytes(6))
	var callBuffer bytes.Buffer
	for _, i := range params {
		if strings.Contains(i.Value, "\n") {
			callBuffer.WriteString(i.Key + ": \\\n")
			for _, line := range utils.ParseStringToLines(i.Value) {
				callBuffer.WriteString("  ")
				callBuffer.WriteString(line + " \\\n")
			}
		} else {
			callBuffer.WriteString(fmt.Sprintf("%v: %v\n", i.Key, i.Value))
		}
	}
	call := callBuffer.String()

	temp := `# Preset
你现在在一个任务引擎中，是一个输出JSON的数据处理和总结提示小助手，我会为你提供一些基本信息和输入材料，你需要按照我的Schema生成一个JSON数据直接返回。

作为系统的一部分你应该直接返回JSON，避免多余的解释。

{{ if .PROMPT }}<background_{{ .NONCE }}>
{{ .PROMPT }}
</background_{{ .NONCE }}>{{end}}
{{ if .PARAMS }}<params_{{ .NONCE }}>
{{ .PARAMS }}
</params_{{ .NONCE }}>{{end}}

{{ if .MEMORY.PersistentMemory }}# 牢记
{{ .MEMORY.PersistentMemory}}{{end}}

# timeline
<timeline_{{ .NONCE }}>
{{ .MEMORY.Timeline }}
</timeline_{{ .NONCE }}>

# Output Formatter

请你根据下面 SCHEMA 构建数据

<schema_{{ .NONCE }}>
{{ .SCHEMA }}
</schema_{{ .NONCE }}>
`
	var promptParam = map[string]interface{}{
		"NONCE":  nonce,
		"PROMPT": string(l.Prompt),
		"PARAMS": string(call),
		"SCHEMA": string(l.OutputSchema),
		"MEMORY": cod.GetConfig().GetMemory(),
	}
	tmp, err := template.New("liteforge").Parse(temp)
	if err != nil {
		return nil, utils.Errorf("template parse failed: %v", err)
	}
	var buf bytes.Buffer
	err = tmp.Execute(&buf, promptParam)
	if err != nil {
		return nil, utils.Errorf("template execute failed: %v", err)
	}
	var action *aicommon.Action
	transactionErr := cod.CallAITransaction(buf.String(),
		func(response *aicommon.AIResponse) error {
			if l.ForgeName == "" {
				l.ForgeName = "LiteForge"
			}
			result := response.GetOutputStreamReader(fmt.Sprintf(`liteforge[%v]`, l.ForgeName), true, cod.GetConfig().GetEmitter())
			var mirrored bytes.Buffer
			action, err = aicommon.ExtractValidActionFormStream(ctx, io.TeeReader(result, &mirrored), l.OutputActionName, aicommon.WithActionJSONCallback(l.OutputJsonHook...))
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
