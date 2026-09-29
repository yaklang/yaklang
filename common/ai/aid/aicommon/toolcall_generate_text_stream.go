package aicommon

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func (t *ToolCaller) textStreamGenerateParams(tool *aitool.Tool, handleError func(i any)) (*GenerateParamsResult, error) {
	emitter := t.emitter

	t.emitter.EmitToolCallStatus(t.callToolId, schema.TOOL_CALL_STATUS_PROCESSING_PARAMS)
	defer t.emitter.EmitToolCallStatus(t.callToolId, schema.TOOL_CALL_STATUS_RUNNING)

	if t.generateToolParamsBuilderWithMeta == nil {
		err := fmt.Errorf("text-stream parameter prompt builder with metadata is nil")
		emitter.EmitError("error generate tool[%v] params in task: %v: %v", tool.Name, t.task.GetName(), err)
		handleError(err)
		return nil, err
	}
	promptMeta, err := t.generateToolParamsBuilderWithMeta(tool, tool.Name)
	if err != nil {
		emitter.EmitError("error generate tool[%v] params in task: %v: %v", tool.Name, t.task.GetName(), err)
		handleError(err)
		return nil, err
	}
	if promptMeta == nil {
		err := fmt.Errorf("text-stream parameter prompt builder returned nil metadata")
		emitter.EmitError("error generate tool[%v] params in task: %v: %v", tool.Name, t.task.GetName(), err)
		handleError(err)
		return nil, err
	}
	paramsPrompt := promptMeta.Prompt

	// Batch children share the task/timeline prompt, but each generates params
	// for a different invocation. Carry the selected call's intent into R2 so
	// repeated uses of the same tool do not all target the first task item.
	if t.reason != "" || t.destinationIdentifier != "" || t.callExpectations != "" {
		intent, _ := json.Marshal(map[string]string{
			"tool":              tool.Name,
			"reason":            t.reason,
			"identifier":        t.destinationIdentifier,
			"call_expectations": t.callExpectations,
		})
		paramsPrompt += "\n\nGenerate parameters for this specific tool invocation. Use its reason, identifier and expectations to distinguish it from other calls in the task:\n" + string(intent)
	}

	invokeParams := aitool.InvokeParams{}
	var identifier string
	var callExpectations string
	var paramDuration time.Duration
	var rawAIResponse string
	releaseParamGeneration := func() {}
	if t.paramGenerationGate != nil {
		releaseParamGeneration, err = t.paramGenerationGate(t.ctx)
		releaseParamGeneration = idempotentToolCallerRelease(releaseParamGeneration)
		if err != nil {
			handleError(err)
			return nil, err
		}
		if err = toolCallerContextErr(t.ctx); err != nil {
			releaseParamGeneration()
			handleError(err)
			return nil, err
		}
	}
	defer releaseParamGeneration()
	if err = toolCallerContextErr(t.ctx); err != nil {
		handleError(err)
		return nil, err
	}
	// A reserved batch sequence belongs only to the first parameter transaction.
	// Review-driven recursive CallTool calls must allocate a fresh sequence
	// instead of replaying the outer transaction checkpoint.
	paramTransactionSeq := t.paramTransactionSeq
	t.paramTransactionSeq = 0

	err = CallAITransaction(t.config, paramsPrompt, func(request *AIRequest) (*AIResponse, error) {
		request.SetTaskIndex(t.task.GetIndex())
		return t.ai.CallAI(request)
	}, func(rsp *AIResponse) error {
		boundEmitter := rsp.BindEmitter(emitter)
		// Stream the model's thinking as the tool-call reason (updating the card)
		// during param generation — but only when no specific reason has already
		// been finalized (via preset, LiteForge, or directly_call_reason). This
		// prevents raw thinking fragments from overwriting a contextualized reason.
		rsp.SetOnReasonChunk(func(b []byte) {
			if len(b) == 0 || t.reasonFinalized {
				return
			}
			boundEmitter.EmitToolCallReason(t.callToolId, string(b))
		})
		pr, pw := utils.NewPipe()

		stream := rsp.GetOutputStreamReader("call-tools", true, emitter)

		var paramNames []string

		// Build action maker options for AITAG support
		var actionOpts []ActionMakerOption
		if promptMeta != nil && promptMeta.Nonce != "" && len(promptMeta.ParamNames) > 0 {
			actionOpts = append(actionOpts, WithActionNonce(promptMeta.Nonce))
			// Register AITAG handlers for each parameter
			for _, paramName := range promptMeta.ParamNames {
				paramNames = append(paramNames, paramName)
				tagName := fmt.Sprintf("TOOL_PARAM_%s", paramName)
				// Map the tag to a special key in params that we'll merge later
				tagParamName := fmt.Sprintf("__aitag__%s", paramName)
				paramNames = append(paramNames, tagParamName)
				actionOpts = append(actionOpts, WithActionTagToKey(tagName, tagParamName))
			}
			log.Debugf("registered AITAG handlers for tool[%s] params: %v with nonce: %s", tool.Name, promptMeta.ParamNames, promptMeta.Nonce)
		}

		_, err := boundEmitter.EmitDefaultSystemStreamEvent("generating-tool-call-params", pr, t.task.GetIndex())
		if err != nil {
			boundEmitter.EmitError("error emit default stream event for tool[%s] params: %v", tool.Name, err)
		}

		pw.WriteString("[开始处理参数] → ")

		start := time.Now()
		actionOpts = append(
			actionOpts,
			WithActionFieldStreamHandler(paramNames, func(key string, r io.Reader) {
				if !strings.HasPrefix(key, "__aitag__") {
					pw.WriteString(key + ": ")
					io.Copy(pw, r)
				} else {
					actKey := strings.TrimPrefix(key, "__aitag__")
					pw.WriteString(actKey + "(BLOCK)")
					io.Copy(pw, r)
				}
				pw.WriteString(" → ")
			}),
			WithActionFieldStreamHandler([]string{
				"call_expectations",
			}, func(key string, r io.Reader) {
				peekedR := utils.NewPeekableReader(r)
				_, err := peekedR.Peek(1)
				if err != nil {
					return
				}
				pw.WriteString(" [note] -> ")
				io.Copy(pw, peekedR)
			}),
		)

		parsed, err := extractFixedToolParamResponse(t.ctx, stream, tool, actionOpts...)
		if err != nil {
			boundEmitter.EmitError("error extract tool params: %v", err)
			pw.Close()
			return utils.Errorf("error extracting action params: %v", err)
		}
		// Parsing joins the source and field streams before exposing parameters.
		cost := time.Since(start)
		pw.WriteString(" [done] 耗时(Cost): " + fmt.Sprintf("%.2f", cost.Seconds()) + "s")
		pw.Close()

		// Extract identifier from action (destination identifier for this tool call)
		callToolParams := parsed.Envelope
		attemptIdentifier := sanitizeIdentifier(callToolParams.GetString("identifier"))
		if attemptIdentifier != "" {
			log.Debugf("extracted identifier[%s] for tool[%s]", attemptIdentifier, tool.Name)
		}

		attemptExpectations := callToolParams.GetString("call_expectations")
		if attemptExpectations != "" {
			log.Debugf("extracted call_expectations for tool[%s]: %s", tool.Name, attemptExpectations)
		}

		// Each attempt owns a fresh candidate. Commit only after the complete
		// JSON + AITAG payload passes the selected tool schema.
		attemptParams := make(aitool.InvokeParams)
		// First, get params from JSON
		for k, v := range callToolParams.GetObject("params") {
			attemptParams.Set(k, v)
		}

		if promptMeta != nil {
			for _, block := range parsed.Blocks {
				if block.Nonce != promptMeta.Nonce {
					message := fmt.Sprintf("tool[%s] generated mismatched AITAG nonce for param[%s], expected=%s observed=%s; applying bounded nonce recovery", tool.Name, block.ParamName, promptMeta.Nonce, block.Nonce)
					log.Warn(message)
					boundEmitter.EmitWarning(message)
				}
			}
		}
		if err := mergeFixedToolParamBlocks(attemptParams, parsed.Blocks, promptMeta); err != nil {
			return err
		}
		normalizeToolParamBooleans(tool, attemptParams)
		if valid, errors := tool.ValidateParams(attemptParams); !valid {
			return utils.Errorf("generated parameters for fixed tool %q failed schema validation: %s; regenerate the complete params object and all required AITAG blocks in this response", tool.Name, strings.Join(errors, "; "))
		}
		invokeParams = attemptParams
		identifier = attemptIdentifier
		callExpectations = attemptExpectations
		paramDuration = cost
		rawAIResponse = parsed.Raw

		return nil
	}, WithAIRequest_CallerLabel("toolcall-params"), WithAIRequest_Context(t.ctx), WithAIRequest_SeqId(paramTransactionSeq))
	releaseParamGeneration()
	if err != nil {
		emitter.EmitError("error calling AI for tool[%v] params: %v", tool.Name, err)
		handleError(fmt.Sprintf("error calling AI for tool[%v] params: %v", tool.Name, err))
		return nil, err
	}
	return &GenerateParamsResult{
		Params:           invokeParams,
		Identifier:       identifier,
		Duration:         paramDuration,
		RawAIResponse:    rawAIResponse,
		CallExpectations: callExpectations,
	}, nil
}
