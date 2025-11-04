package reactloopstests

import (
	"context"
	"fmt"
	"io"
	"testing"

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

	t.Logf("✅ Tool callback successfully called with message: %s", receivedMessage)
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

	t.Logf("✅ Calculator tool executed: %s -> %.2f", operationPerformed, calculationResult)
}
