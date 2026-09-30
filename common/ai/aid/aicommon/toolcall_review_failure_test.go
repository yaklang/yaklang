package aicommon

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestToolReviewRejectedProposalFailsClosed(t *testing.T) {
	tool := aitool.NewWithoutCallback("review_rejected")
	replacement := aitool.NewWithoutCallback("review_replacement")
	for _, scenario := range []string{"missing_tool_handler", "missing_params_handler", "nil_replacement", "params_failure", "wrong_params_handler_missing"} {
		t.Run(scenario, func(t *testing.T) {
			config := NewTestConfig(context.Background())
			caller := &ToolCaller{ctx: context.Background(), emitter: config.GetEmitter()}
			input := aitool.InvokeParams{"suggestion": "wrong_tool"}
			if scenario != "missing_tool_handler" {
				caller.reviewWrongToolHandler = func(context.Context, *aitool.Tool, string, string) (*aitool.Tool, bool, error) {
					if scenario == "nil_replacement" {
						return nil, false, nil
					}
					return replacement, false, nil
				}
			}
			if scenario == "params_failure" {
				caller.reviewWrongParamHandler = func(_ context.Context, selected *aitool.Tool, old aitool.InvokeParams, _ string) (aitool.InvokeParams, error) {
					require.Same(t, replacement, selected)
					require.Nil(t, old)
					return nil, errors.New("failed to construct replacement proposal")
				}
			}
			if scenario == "wrong_params_handler_missing" {
				input["suggestion"] = "wrong_params"
			}
			_, _, result, next, err := caller.review(tool, aitool.InvokeParams{"old": "value"}, input, func(any) {
				t.Error("failure must not turn into a direct-answer decision")
			})
			require.Error(t, err)
			require.Nil(t, result)
			require.Equal(t, HandleToolUseNext_Default, next)
		})
	}
}
