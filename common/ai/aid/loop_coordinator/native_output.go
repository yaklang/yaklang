package loop_coordinator

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// nativeLiteForgeProtocol adapts one-shot infrastructure output (compression,
// attachment observations, etc.) to a native call in the coordinator engine.
// It never creates a ReAct loop or accepts ordinary JSON response text.
func nativeLiteForgeProtocol(name, outputSchema string) (aicommon.AIRequestOption, aicommon.AuxiliaryResponseHandler, error) {
	var parameters map[string]any
	if err := json.Unmarshal([]byte(outputSchema), &parameters); err != nil {
		return nil, nil, err
	}
	delete(parameters, "$schema")
	properties, _ := parameters["properties"].(map[string]any)
	delete(properties, "@action")
	if required, ok := parameters["required"].([]any); ok {
		filtered := make([]any, 0, len(required))
		for _, key := range required {
			if key != "@action" {
				filtered = append(filtered, key)
			}
		}
		parameters["required"] = filtered
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("native-output.json", parameters); err != nil {
		return nil, nil, err
	}
	validator, err := compiler.Compile("native-output.json")
	if err != nil {
		return nil, nil, err
	}
	var mu sync.Mutex
	var function, arguments, id, finish string
	var index int
	var seen bool
	var invalid bool
	reset := func() {
		mu.Lock()
		function, arguments, id, finish = "", "", "", ""
		seen, invalid = false, false
		mu.Unlock()
	}
	request := aicommon.AIRequestOption(func(req *aicommon.AIRequest) {
		reset()
		aicommon.WithAIRequest_ExtraSpecOpts(
			aispec.WithTools([]aispec.Tool{{Type: "function", Function: aispec.ToolFunction{Name: name, Description: "Submit the required one-shot result using this native function.", Parameters: parameters}}}),
			// This helper has exactly one output function. Name it explicitly:
			// some providers still emit response text for the generic "required" choice.
			aispec.WithToolChoice(map[string]any{"type": "function", "function": map[string]any{"name": name}}),
			aispec.AIConfigOption(func(cfg *aispec.AIConfig) {
				previous := cfg.RawHTTPResponseHeaderCallback
				cfg.RawHTTPResponseHeaderCallback = func(header []byte) {
					if previous != nil {
						previous(header)
					}
					reset()
				}
			}),
			aispec.WithToolCallCallback(func(calls []*aispec.ToolCall) {
				mu.Lock()
				defer mu.Unlock()
				for _, call := range calls {
					if call == nil {
						continue
					}
					if seen && (index != call.Index || (id != "" && call.ID != "" && id != call.ID)) {
						invalid = true
					}
					seen, index = true, call.Index
					if call.ID != "" {
						id = call.ID
					}
					if part := call.Function.Name; part != "" {
						if part == name && strings.HasPrefix(part, function) {
							function = part
						} else {
							function += part
						}
					}
					arguments += call.Function.Arguments
				}
			}),
			aispec.WithFinishReasonCallback(func(reason string, _ []byte) { mu.Lock(); finish = reason; mu.Unlock() }),
		)(req)
	})
	parse := func(resp *aicommon.AIResponse) (*aicommon.Action, error) {
		if _, err := io.Copy(io.Discard, resp.GetUnboundStreamReader(false)); err != nil {
			return nil, err
		}
		mu.Lock()
		defer mu.Unlock()
		if invalid || !seen || function != name || finish != "tool_calls" {
			return nil, fmt.Errorf("%s requires exactly one native function call (seen=%t, multiple=%t, function=%q, finish_reason=%q)", name, seen, invalid, function, finish)
		}
		var value map[string]any
		if err := json.Unmarshal([]byte(arguments), &value); err != nil {
			return nil, err
		}
		if err := validator.Validate(value); err != nil {
			return nil, err
		}
		value["@action"] = name // Internal Action adapter; never read from model text.
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return aicommon.ExtractAction(string(data), name)
	}
	return request, parse, nil
}
