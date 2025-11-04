package aid

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// TestActionParamExtraction_VariousFormats tests that Action can correctly extract params
// from various AI response formats.
//
// IMPORTANT: All valid AI responses MUST contain the @action field. The @action field is
// a core requirement - without it, the ActionMaker cannot parse the response.
//
// Valid formats include:
// 1. Standard: {"@action": "call-tool", "tool": "echo", "params": {"input": "hello"}}
// 2. Simplified: {"@action": "echo", "input": "hello"}
// 3. Nested: {"@action": "call-tool", "params": {"nested": {"input": "hello"}}}
func TestActionParamExtraction_VariousFormats(t *testing.T) {
	testCases := []struct {
		name          string
		aiResponse    string
		expectedInput string
		actionName    string
		actionAlias   []string
	}{
		{
			name:          "Standard format with @action, tool, and params",
			aiResponse:    `{"@action": "call-tool", "tool": "echo", "params": {"input": "hello1"}}`,
			expectedInput: "hello1",
			actionName:    "call-tool",
		},
		{
			name:          "Echo as @action with params wrapper",
			aiResponse:    `{"@action": "echo", "params": {"input": "hello2"}}`,
			expectedInput: "hello2",
			actionName:    "echo",
		},
		{
			name:          "Simplified format - @action with direct params",
			aiResponse:    `{"@action": "echo", "input": "hello3"}`,
			expectedInput: "hello3",
			actionName:    "echo",
		},
		{
			name:          "Nested params with @action",
			aiResponse:    `{"@action": "call-tool", "tool": "echo", "params": {"nested": {"input": "hello4"}}}`,
			expectedInput: "hello4",
			actionName:    "call-tool",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			// Parse the JSON into an Action
			opts := []aicommon.ActionMakerOption{}
			if len(tc.actionAlias) > 0 {
				opts = append(opts, aicommon.WithActionAlias(tc.actionAlias...))
			}
			action, err := aicommon.ExtractActionFromStream(ctx, strings.NewReader(tc.aiResponse), tc.actionName, opts...)
			if err != nil {
				t.Fatalf("Failed to parse AI response: %v", err)
			}

			// Wait for parsing to complete
			action.WaitParse(ctx)
			action.WaitStream(ctx)

			// Debug: log the action structure
			t.Logf("Action type: %s", action.ActionType())

			// GetParams() returns the actual parameters parsed by the ActionMaker
			allParams := action.GetParams()
			t.Logf("GetParams(): %+v", allParams)

			var extractedInput string

			// Method 1: Check if params field exists (nested format)
			// Standard: {"@action": "call-tool", "tool": "echo", "params": {"input": "hello"}}
			if paramsField, hasParams := allParams["params"]; hasParams {
				if paramsMap, ok := paramsField.(map[string]any); ok {
					// Direct input in params
					if input, ok := paramsMap["input"].(string); ok {
						extractedInput = input
					}
					// Nested input in params.nested.input
					if extractedInput == "" {
						if nestedField, hasNested := paramsMap["nested"]; hasNested {
							if nestedMap, ok := nestedField.(map[string]any); ok {
								if input, ok := nestedMap["input"].(string); ok {
									extractedInput = input
								}
							}
						}
					}
				}
			}

			// Method 2: Direct params format (no "params" wrapper)
			// Simplified: {"@action": "echo", "input": "hello"}
			if extractedInput == "" {
				if input, ok := allParams["input"].(string); ok {
					extractedInput = input
				}
			}

			// Method 3: Check params.nested.input at root level (flattened keys)
			if extractedInput == "" {
				if input, ok := allParams["params.nested.input"].(string); ok {
					extractedInput = input
				}
			}

			// Verify we extracted the correct value
			if extractedInput != tc.expectedInput {
				t.Errorf("Expected input '%s', but got '%s'\nAllParams: %+v",
					tc.expectedInput, extractedInput, allParams)
			} else {
				t.Logf("✓ Successfully extracted input '%s' from format: %s", extractedInput, tc.name)
			}
		})
	}
}
