package aicommon

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestGenerateParams_ActionTolerance(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		ok   bool
	}{
		{"canonical", `{"@action":"call-tool","tool":"read_file","identifier":"read_config","params":{"path":"/config","action":"read","type":"A"}}`, true},
		{"alias", `{"action":"call-tool","tool":"read_file","identifier":"read_config","params":{"path":"/config","action":"read","type":"A"}}`, true},
		{"missing_marker", `{"tool":"read_file","identifier":"read_config","params":{"path":"/config","action":"read","type":"A"}}`, true},
		{"nested_business_marker", `{"tool":"read_file","identifier":"read_config","params":{"path":"/config","action":"read","type":"A","@action":"finish"}}`, true},
		{"wrong_tool", `{"tool":"other_tool","params":{"path":"/config"}}`, false},
		{"missing_tool", `{"params":{"path":"/config","action":"read","type":"A"},"identifier":"read_config"}`, true},
		{"missing_params", `{"tool":"read_file"}`, false},
		{"null_params", `{"tool":"read_file","params":null}`, false},
		{"array_params", `{"tool":"read_file","params":[]}`, false},
		{"string_params", `{"tool":"read_file","params":"/config"}`, false},
		{"wrong_action", `{"action":"finish","tool":"read_file","params":{"path":"/config"}}`, false},
		{"empty_action", `{"@action":"","tool":"read_file","params":{"path":"/config"}}`, false},
		{"conflicting_action", `{"action":"call-tool","@action":"finish","tool":"read_file","params":{"path":"/config"}}`, false},
		{"nested_envelope", `{"wrapper":{"tool":"read_file","params":{"path":"/config"}}}`, false},
		{"truncated_envelope", `{"tool":"read_file","params":{"path":"/config"},`, false},
	} {
		for _, functionCall := range []bool{false, true} {
			mode := "text"
			if functionCall {
				mode = "functioncall"
			}
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				var calls atomic.Int32
				cfg := NewTestConfig(context.Background(), WithAITransactionAutoRetry(1), WithEnableFunctionCallMode(functionCall),
					WithAICallback(func(config AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
						calls.Add(1)
						rsp := config.NewAIResponse()
						rsp.EmitOutputStream(strings.NewReader(test.raw))
						rsp.Close()
						return rsp, nil
					}))
				tool, err := aitool.New("read_file", aitool.WithStringParam("path", aitool.WithParam_Required(true)),
					aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
						return "unused", nil
					}))
				require.NoError(t, err)
				caller, err := NewToolCaller(context.Background(),
					WithToolCaller_AICallerConfig(cfg), WithToolCaller_AICaller(cfg),
					WithToolCaller_Emitter(cfg.GetEmitter()), WithToolCaller_Task(cfg.DefaultTask),
					WithToolCaller_CallToolID("action-tolerance"),
					WithToolCaller_GenerateToolParamsBuilder(func(*aitool.Tool, string) (string, error) {
						return "generate params", nil
					}))
				require.NoError(t, err)
				result, err := caller.generateParams(tool, func(any) {})
				require.EqualValues(t, 1, calls.Load(), "tolerance must not require a second model response")
				if !test.ok {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
				require.Equal(t, "/config", result.Params.GetString("path"))
				require.Equal(t, "read", result.Params.GetString("action"))
				require.Equal(t, "A", result.Params.GetString("type"))
				require.Equal(t, "read_config", result.Identifier)
				require.Equal(t, test.raw, result.RawAIResponse)
			})
		}
	}
}

func TestExtractToolCallAction_EmptyParams(t *testing.T) {
	parsed, err := extractFixedToolParamResponse(context.Background(), strings.NewReader(`{"tool":"clock","params":{}}`), aitool.NewWithoutCallback("clock"))
	require.NoError(t, err)
	require.Equal(t, "call-tool", parsed.Envelope.GetString(ActionMagicKey))
	require.Equal(t, "clock", parsed.Envelope.GetString("tool"))
	require.Empty(t, parsed.Envelope.GetObject("params"))
}
