// Package liteforge runs one structured model request without constructing a
// coordinator session, a plan or a task loop. Both output protocols share the
// same validation, streaming callbacks and request lifetime.
package liteforge

import (
	"context"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/jsonextractor"
)

type StreamField struct {
	NodeID string
	Key    string
}

type FieldCallback struct {
	Keys     []string
	Callback func(string, io.Reader, *aicommon.Emitter)
	Response aicommon.StreamableFieldResponseCallback
}

type Request struct {
	Name                         string
	ActionName                   string
	Schema                       string
	Prompt                       string
	Params                       string
	StaticInstruction            string
	PreferSpeed, DisableTimeline bool
	MaxPromptTokens              int
	Images                       []*aicommon.ImageData
	Emitter                      *aicommon.Emitter
	StreamFields                 []StreamField
	FieldCallbacks               []FieldCallback
	JSONHooks                    []jsonextractor.CallbackOption
	Validate                     func(*aicommon.Action) error
	ResponseHandler              aicommon.AuxiliaryResponseHandler
	ExtraOptions                 []aicommon.AIRequestOption
	ModelOptions                 []aispec.AIConfigOption
}

// ExecuteTyped adapts aicommon's typed invocation. Public Yak/Go LiteForge
// options are translated by aiforge into the same Request and Execute path.
func ExecuteTyped(prompt string, opts ...any) (*aicommon.ForgeResult, error) {
	var typed *aicommon.LiteForgeInvokeRequest
	var configOptions []aicommon.ConfigOption
	var static string
	for _, opt := range opts {
		switch value := opt.(type) {
		case *aicommon.LiteForgeInvokeRequest:
			typed = value
		case aicommon.ConfigOption:
			configOptions = append(configOptions, value)
		case aicommon.LiteForgeStaticInstruction:
			static = string(value)
		default:
			return nil, fmt.Errorf("unsupported liteforge option %T", opt)
		}
	}
	if typed == nil {
		return nil, fmt.Errorf("liteforge requires a typed request")
	}
	name := typed.OutputActionName
	if name == "" {
		name = typed.ActionName
	}
	if name == "" {
		name = "output"
	}
	schema := typed.OutputSchema
	if schema == "" {
		outputs := make([]any, 0, len(typed.Outputs))
		for _, output := range typed.Outputs {
			outputs = append(outputs, output)
		}
		schema = aitool.NewObjectSchemaWithActionName(name, outputs...)
	}
	g := aicommon.NewGeneralKVConfig(typed.Options...)
	if instruction := g.GetLiteForgeStaticInstruction(); instruction != "" {
		if static != "" {
			static += "\n"
		}
		static += instruction
	}
	r := Request{Name: typed.ActionName, ActionName: name, Schema: schema, Prompt: prompt,
		StaticInstruction: static, Emitter: typed.Emitter, ResponseHandler: typed.ResponseHandler,
		Validate: g.GetLiteForgeOutputValidator(), DisableTimeline: g.GetLiteForgeDisableTimeline(),
		MaxPromptTokens: g.GetLiteForgeMaxPromptTokens(), ExtraOptions: g.GetExtraRequestOpts()}
	for _, field := range g.GetStreamableFields() {
		r.StreamFields = append(r.StreamFields, StreamField{field.AINodeId(), field.FieldKey()})
	}
	for _, field := range g.GetStreamableFieldCallbacks() {
		if field != nil {
			r.FieldCallbacks = append(r.FieldCallbacks, FieldCallback{field.FieldKeys, field.Callback, field.ResponseCallback})
		}
	}
	ctx := typed.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return Execute(ctx, r, configOptions...)
}
