package liteforge

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
)

func Execute(ctx context.Context, r Request, opts ...aicommon.ConfigOption) (*aicommon.ForgeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Explicitly establish the native default even when a custom model callback
	// activates Config's legacy text default. Caller options still take priority.
	opts = append([]aicommon.ConfigOption{aicommon.WithEnableFunctionCallMode(true)}, opts...)
	// One-shot calls have no event loop, skills, task lifecycle or memory triage.
	opts = append(append([]aicommon.ConfigOption{}, opts...), aicommon.WithContext(ctx),
		aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithNoOpMemoryTriage())
	if r.Emitter != nil {
		opts = append(opts, aicommon.WithEmitter(r.Emitter))
	}
	cfg := aicommon.NewConfig(ctx, opts...)
	if r.Schema == "" {
		r.Schema, r.ActionName = cfg.LiteForgeOutputSchema, cfg.LiteForgeActionName
	}
	if r.ActionName == "" {
		r.ActionName = "object"
	}
	if r.Name == "" {
		r.Name = "LiteForge"
	}
	if r.Schema == "" && r.ResponseHandler == nil {
		return nil, fmt.Errorf("liteforge output schema is required")
	}
	// A custom response handler owns its prompt and transport (e.g. AI tags).
	// Ordinary structured requests inherit the caller's selected action protocol.
	native := cfg.EnableFunctionCallMode && r.ResponseHandler == nil
	var protocol *nativeOutput
	parse := r.ResponseHandler
	prompt := r.Prompt
	if parse == nil {
		parameters, validator, err := outputSchema(r.Schema, r.ActionName, native)
		if err != nil {
			return nil, err
		}
		promptSchema, functionCallSchema := r.Schema, ""
		if native {
			protocol = newNativeOutput(ctx, r, validator)
			defer protocol.close()
			parse = protocol.parse
			// Like mainloop, declare the output function in the trusted schema
			// slot. aiprojection removes it from messages and injects provider tools.
			functionCallSchema, err = aiprojection.CreateActionSchema(aispec.Tool{Type: "function", Function: aispec.ToolFunction{
				Name: protocol.wireName, Description: "提交本次单步结构化结果；字段遵循参数 schema，不执行外部操作。", Parameters: parameters}})
			if err != nil {
				return nil, err
			}
			promptSchema = ""
		} else {
			parse = func(resp *aicommon.AIResponse) (*aicommon.Action, error) {
				reader := resp.GetOutputStreamReader("liteforge["+r.Name+"]", true, cfg.GetEmitter())
				a := aicommon.NewActionMaker(r.ActionName, r.actionOptions(resp)...).ReadFromReader(ctx, reader)
				err := a.WaitParseResult(ctx)
				a.WaitStream(ctx)
				if err != nil {
					return nil, err
				}
				return admitOutput(a.GetParams(), r.ActionName, validator, false)
			}
		}
		var timeline string
		if !r.DisableTimeline {
			timeline = RecentTimeline(cfg.Timeline)
		}
		memory := "<persistent_memory>\n" + strings.Join(cfg.PersistentMemory, "\n") + "\n</persistent_memory>\n"
		prompt, err = RenderPrompt(PromptParams{Nonce: strings.ToLower(utils.RandStringBytes(6)),
			Prompt: r.Prompt, Params: r.Params, StaticInstruction: r.StaticInstruction,
			Schema: promptSchema, FunctionCallSchema: functionCallSchema, PersistentMemory: memory, TimelineOpen: timeline}, native)
		if err != nil {
			return nil, err
		}
	}
	if r.MaxPromptTokens > 0 {
		if tokens := aicommon.MeasureTokens(prompt); tokens > r.MaxPromptTokens {
			return nil, fmt.Errorf("liteforge prompt exceeds %d-token hard limit: %d", r.MaxPromptTokens, tokens)
		}
	}
	reqOptions := make([]aicommon.AIRequestOption, 0, len(r.Images)+len(r.ExtraOptions)+3)
	for _, image := range r.Images {
		reqOptions = append(reqOptions, aicommon.WithAIRequest_ImageData(image))
	}
	reqOptions = append(reqOptions, aicommon.WithAIRequest_Context(ctx), aicommon.WithAIRequest_CallerLabel("liteforge["+r.Name+"]"))
	reqOptions = append(reqOptions, r.ExtraOptions...)
	if protocol != nil {
		reqOptions = append(reqOptions, protocol.requestOption())
	}
	if protocol != nil {
		// Install the binder inside the ordinary model wrappers: those wrappers
		// can wait for the provider before returning their tee response.
		callbacks := cfg.GetRawAICallbacks()
		bind := func(cb aicommon.AICallbackType) aicommon.AICallbackType {
			if cb == nil {
				return nil
			}
			return func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				resp, err := cb(c, req)
				if resp != nil && err == nil {
					resp.SetTaskIndex(req.GetTaskIndex())
					protocol.bind(resp)
				}
				return resp, err
			}
		}
		_ = aicommon.WithFastAICallback(bind(callbacks.Original))(cfg)
		if callbacks.QualityPriorityRaw != nil {
			_ = aicommon.WithQualityPriorityAICallback(bind(callbacks.QualityPriorityRaw))(cfg)
		}
		if callbacks.SpeedPriorityRaw != nil {
			_ = aicommon.WithSpeedPriorityAICallback(bind(callbacks.SpeedPriorityRaw))(cfg)
		}
		if callbacks.VisionPriorityRaw != nil {
			_ = aicommon.WithVisionPriorityAICallback(bind(callbacks.VisionPriorityRaw))(cfg)
		}
	}
	call := cfg.CallAI
	if r.PreferSpeed {
		call = cfg.CallSpeedPriorityAI
	}
	var action *aicommon.Action
	err := aicommon.CallAITransactionWithFailureExtra(cfg, prompt, call, func(resp *aicommon.AIResponse) error {
		value, err := parse(resp)
		if err != nil {
			return err
		}
		if value == nil {
			return fmt.Errorf("liteforge response handler returned no action")
		}
		if r.Validate != nil {
			if err := r.Validate(value); err != nil {
				return fmt.Errorf("auxiliary output validation failed: %w", err)
			}
		}
		action = value
		return nil
	}, map[string]any{"liteforge_action": r.Name}, reqOptions...)
	if err != nil {
		return nil, fmt.Errorf("liteforge execute failed: %w", err)
	}
	return &aicommon.ForgeResult{Name: r.Name, Action: action}, nil
}

func (r Request) actionOptions(resp *aicommon.AIResponse) []aicommon.ActionMakerOption {
	emitter := resp.BindEmitter(r.Emitter)
	opts := []aicommon.ActionMakerOption{aicommon.WithActionJSONCallback(r.JSONHooks...)}
	for _, field := range r.StreamFields {
		field := field
		opts = append(opts, aicommon.WithActionFieldStreamHandler([]string{field.Key}, func(_ string, reader io.Reader) {
			reader = utils.JSONStringReader(reader)
			if r.Emitter == nil {
				_, _ = io.Copy(io.Discard, reader)
				return
			}
			emitter.EmitDefaultStreamEvent(field.NodeID, reader, resp.GetTaskIndex())
		}))
	}
	for _, field := range r.FieldCallbacks {
		field := field
		opts = append(opts, aicommon.WithActionFieldStreamHandler(field.Keys, func(key string, reader io.Reader) {
			if field.Callback != nil {
				field.Callback(key, reader, emitter)
			} else if field.Response != nil {
				field.Response(key, reader, resp, emitter)
			} else {
				_, _ = io.Copy(io.Discard, reader)
			}
		}))
	}
	return opts
}
