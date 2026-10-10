package reactloopstests

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// TestActionFromTool_WithFramework tests AITool-generated actions using ActionTestFramework
func TestActionFromTool_WithFramework(t *testing.T) {
	// Track if the tool callback was called
	var toolCallbackCalled bool
	var receivedMessage string

	// Create a test tool
	echoTool, err := aitool.New(
		"echo_message",
		aitool.WithDescription("Echo back a message"),
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, config *aitool.ToolRuntimeConfig, stdout, stderr io.Writer) (any, error) {
			toolCallbackCalled = true
			message, ok := params["message"]
			if !ok {
				return nil, fmt.Errorf("missing required parameter 'message'")
			}
			receivedMessage = utils.InterfaceToString(message)
			return map[string]any{
				"echoed":  receivedMessage,
				"success": true,
			}, nil
		}),
		aitool.WithStringParam("message",
			aitool.WithParam_Description("The message to echo"),
			aitool.WithParam_Required(true),
		),
	)
	if err != nil {
		t.Fatalf("Failed to create echo tool: %v", err)
	}

	// Create test framework with AI config options to add the tool
	framework := NewActionTestFrameworkEx(
		t,
		"tool-test",
		nil, // No loop options
		[]aicommon.ConfigOption{
			aicommon.WithTools(echoTool),
			aicommon.WithAIAutoRetry(1), // Reduce retry for faster tests
		},
	)

	// Convert the tool to a LoopAction and register it
	loopAction := reactloops.ConvertAIToolToLoopAction(echoTool)

	// Register the action with the framework's loop
	registerOption := reactloops.WithRegisterLoopAction(
		loopAction.ActionType,
		loopAction.Description,
		loopAction.Options,
		loopAction.ActionVerifier,
		loopAction.ActionHandler,
	)
	registerOption(framework.GetLoop())

	// Execute the action with simplified format: {@action: "echo_message", message: "Hello World"}
	testMessage := "Hello World from AITool!"
	err = framework.ExecuteAction("echo_message", map[string]interface{}{
		"message": testMessage,
	})

	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}

	// Verify the tool callback was called
	if !toolCallbackCalled {
		t.Error("Tool callback was not called")
	}

	// Verify the correct message was received
	if receivedMessage != testMessage {
		t.Errorf("Expected message '%s', got '%s'", testMessage, receivedMessage)
	}

	t.Logf("Tool callback successfully called with message: %s", receivedMessage)
}

// Keep the verifier's existing best-effort extraction policy at the real callback boundary.
func TestActionFromTool_HandlerUsesVerifierExtraction(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{"flat", map[string]interface{}{"message": "flat"}, "flat"},
		{"wrapped", map[string]interface{}{"params": map[string]interface{}{"message": "wrapped"}}, "wrapped"},
		{"mixed_prefers_inner", map[string]interface{}{"message": "outer", "params": map[string]interface{}{"message": "inner"}}, "inner"},
		{"invalid_wrapper_uses_flat", map[string]interface{}{"message": "flat", "params": "not an object"}, "flat"},
		{"null_wrapper_uses_flat", map[string]interface{}{"message": "flat", "params": nil}, "flat"},
		{"wrapped_with_metadata", map[string]interface{}{"tool": "tolerant_echo", "identifier": "echo_test", "params": map[string]interface{}{"message": "wrapped"}}, "wrapped"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var received string
			tool, err := aitool.New("tolerant_echo",
				aitool.WithStringParam("message", aitool.WithParam_Required(true)),
				aitool.WithDangerousNoNeedUserReview(true),
				aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
					calls++
					received = params.GetString("message")
					return received, nil
				}))
			if err != nil {
				t.Fatal(err)
			}
			framework := NewActionTestFrameworkEx(t, "tolerant-tool",
				[]reactloops.ReActLoopOption{reactloops.WithRegisterLoopActionFromTool(tool)},
				[]aicommon.ConfigOption{aicommon.WithTools(tool), aicommon.WithWorkdir(t.TempDir())})
			if err := framework.ExecuteActionWithTimeout(tool.GetName(), tc.params, 10*time.Second); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || received != tc.want {
				t.Fatalf("callback calls=%d, message=%q; want one call with %q", calls, received, tc.want)
			}
		})
	}
}

// Validation feedback must reach both Timeline and the same transaction's retry error.
func TestActionFromTool_InvalidParamsLeaveCorrectionInTimeline(t *testing.T) {
	for _, input := range []map[string]interface{}{
		{},
		{"message": []interface{}{"not a string"}},
	} {
		t.Run(fmt.Sprint(input), func(t *testing.T) {
			calls := 0
			tool, err := aitool.New("timeline_echo",
				aitool.WithStringParam("message", aitool.WithParam_Required(true)),
				aitool.WithDangerousNoNeedUserReview(true),
				aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
					calls++
					return "unexpected", nil
				}))
			if err != nil {
				t.Fatal(err)
			}
			framework := NewActionTestFrameworkEx(t, "parameter-feedback",
				[]reactloops.ReActLoopOption{reactloops.WithRegisterLoopActionFromTool(tool)},
				[]aicommon.ConfigOption{aicommon.WithTools(tool), aicommon.WithWorkdir(t.TempDir())})
			handler, err := framework.GetLoop().GetActionHandler(tool.GetName())
			if err != nil {
				t.Fatal(err)
			}
			err = handler.ActionVerifier(framework.GetLoop(), aicommon.NewSimpleAction(tool.GetName(), input))
			if err == nil {
				t.Fatal("expected parameter validation failure")
			}
			timeline := framework.GetLoop().GetConfig().(*aicommon.Config).Timeline.String()
			for _, text := range []string{tool.GetName(), "message", "parameter validation failed", "参数修正提示", "本次未执行工具", "当前可用的 action", "不要原样重试或委托其他模型生成参数"} {
				if !strings.Contains(timeline, text) || !strings.Contains(err.Error(), text) {
					t.Errorf("missing %q in parameter feedback: error=%v, timeline=%s", text, err, timeline)
				}
			}
			if strings.Contains(err.Error(), "directly_call_tool") || calls != 0 {
				t.Fatalf("feedback must stay in this focus action without executing a tool: error=%v, calls=%d", err, calls)
			}
		})
	}
}

// TestActionFromTool_MultipleParameters tests a tool with multiple parameters
func TestActionFromTool_MultipleParameters(t *testing.T) {
	var calculationResult float64
	var operationPerformed string

	// Create a calculator tool
	calcTool, err := aitool.New(
		"calculate",
		aitool.WithDescription("Perform arithmetic operations"),
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, config *aitool.ToolRuntimeConfig, stdout, stderr io.Writer) (any, error) {
			a := utils.InterfaceToFloat64(params["a"])
			b := utils.InterfaceToFloat64(params["b"])
			operation := utils.InterfaceToString(params["operation"])

			operationPerformed = operation

			switch operation {
			case "add":
				calculationResult = a + b
			case "subtract":
				calculationResult = a - b
			case "multiply":
				calculationResult = a * b
			case "divide":
				if b == 0 {
					return nil, fmt.Errorf("division by zero")
				}
				calculationResult = a / b
			default:
				return nil, fmt.Errorf("unsupported operation: %s", operation)
			}

			return map[string]any{
				"result":    calculationResult,
				"operation": operation,
			}, nil
		}),
		aitool.WithNumberParam("a",
			aitool.WithParam_Description("First number"),
			aitool.WithParam_Required(true),
		),
		aitool.WithNumberParam("b",
			aitool.WithParam_Description("Second number"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam("operation",
			aitool.WithParam_Description("Operation to perform: add, subtract, multiply, divide"),
			aitool.WithParam_Required(true),
		),
	)
	if err != nil {
		t.Fatalf("Failed to create calculator tool: %v", err)
	}

	// Create test framework with AI config options to add the tool
	framework := NewActionTestFrameworkEx(
		t,
		"calc-test",
		nil, // No loop options
		[]aicommon.ConfigOption{
			aicommon.WithTools(calcTool),
			aicommon.WithAIAutoRetry(1), // Reduce retry for faster tests
		},
	)

	// Convert the tool to a LoopAction and register it
	loopAction := reactloops.ConvertAIToolToLoopAction(calcTool)

	// Register the action with the framework's loop
	registerOption := reactloops.WithRegisterLoopAction(
		loopAction.ActionType,
		loopAction.Description,
		loopAction.Options,
		loopAction.ActionVerifier,
		loopAction.ActionHandler,
	)
	registerOption(framework.GetLoop())

	// Execute: {@action: "calculate", a: 10, b: 5, operation: "multiply"}
	err = framework.ExecuteAction("calculate", map[string]interface{}{
		"a":         10.0,
		"b":         5.0,
		"operation": "multiply",
	})

	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}

	// Verify results
	if operationPerformed != "multiply" {
		t.Errorf("Expected operation 'multiply', got '%s'", operationPerformed)
	}

	expectedResult := 50.0
	if calculationResult != expectedResult {
		t.Errorf("Expected result %.2f, got %.2f", expectedResult, calculationResult)
	}

	t.Logf("Calculator tool executed: %s -> %.2f", operationPerformed, calculationResult)
}
