package reactloopstests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

// ActionTestFramework provides utilities for testing ReAct loop actions
type ActionTestFramework struct {
	t              *testing.T
	loop           *reactloops.ReActLoop
	reactInstance  aicommon.AIInvokeRuntime
	capturedTask   aicommon.AIStatefulTask
	mu             sync.Mutex
	aiCallCount    int
	actionHandlers map[string]*ActionHandlerCapture
}

// ActionHandlerCapture captures information about action handler execution
type ActionHandlerCapture struct {
	Called      bool
	Action      *aicommon.Action
	Operator    *reactloops.LoopActionHandlerOperator
	FeedbackMsg string
	Failed      bool
	FailedMsg   string
	Continued   bool
	Terminated  bool
	CalledCount int
}

// NewActionTestFramework creates a new action testing framework
func NewActionTestFramework(t *testing.T, loopName string, options ...reactloops.ReActLoopOption) *ActionTestFramework {
	return NewActionTestFrameworkEx(t, loopName, options, nil)
}

// NewActionTestFrameworkEx creates a new action testing framework with extended options
// It supports both ReActLoop options and AI config options
func NewActionTestFrameworkEx(
	t *testing.T,
	loopName string,
	loopOptions []reactloops.ReActLoopOption,
	aiConfigOptions []aicommon.ConfigOption,
) *ActionTestFramework {
	framework := &ActionTestFramework{
		t:              t,
		aiCallCount:    0,
		actionHandlers: make(map[string]*ActionHandlerCapture),
	}

	// Prepare AI config options with the callback
	allAIOptions := []aicommon.ConfigOption{
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			framework.mu.Lock()
			framework.aiCallCount++
			callNum := framework.aiCallCount
			framework.mu.Unlock()

			rsp := i.NewAIResponse()

			// Check if this is a verification prompt
			prompt := req.GetPrompt()
			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				// Return verification response
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "Test completed successfully", "human_readable_result": "Test result"}`))
				rsp.Close()
				return rsp, nil
			}

			// For the first call, extract the action from the test request
			// This is set via framework's context
			if callNum == 1 {
				actionJSON := framework.getFirstActionJSON()
				if actionJSON != "" {
					rsp.EmitOutputStream(bytes.NewBufferString(actionJSON))
				} else {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "answer": "Test completed"}`))
				}
			} else {
				// Subsequent calls finish
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "answer": "Test completed"}`))
			}

			rsp.Close()
			return rsp, nil
		}),
		// Auto-approve all tool uses in tests (no manual review required)
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		// Reduce retry counts for faster tests
		aicommon.WithAIAutoRetry(1),            // Only retry once (default 5)
		aicommon.WithAITransactionAutoRetry(1), // Only 1 transaction retry (default 5)
	}

	// Append user-provided AI config options
	if len(aiConfigOptions) > 0 {
		allAIOptions = append(allAIOptions, aiConfigOptions...)
	}

	// Create a test ReAct instance with all AI config options
	reactIns, err := aireact.NewTestReAct(allAIOptions...)
	if err != nil {
		t.Fatalf("Failed to create test ReAct instance: %v", err)
	}

	framework.reactInstance = reactIns

	// Prepare loop options with task created callback
	allLoopOptions := make([]reactloops.ReActLoopOption, 0)
	if len(loopOptions) > 0 {
		allLoopOptions = append(allLoopOptions, loopOptions...)
	}

	// Add task created callback
	allLoopOptions = append(allLoopOptions, reactloops.WithOnTaskCreated(func(task aicommon.AIStatefulTask) {
		framework.mu.Lock()
		framework.capturedTask = task
		framework.mu.Unlock()
	}))

	// Create the loop with all options
	loop, err := reactloops.NewReActLoop(loopName, reactIns, allLoopOptions...)
	if err != nil {
		t.Fatalf("Failed to create ReActLoop: %v", err)
	}

	framework.loop = loop
	return framework
}

// firstActionJSON stores the action JSON for the first AI call
var firstActionJSON string
var firstActionMu sync.Mutex

func (f *ActionTestFramework) getFirstActionJSON() string {
	firstActionMu.Lock()
	defer firstActionMu.Unlock()
	result := firstActionJSON
	firstActionJSON = "" // Clear after reading
	return result
}

func (f *ActionTestFramework) setFirstActionJSON(json string) {
	firstActionMu.Lock()
	defer firstActionMu.Unlock()
	firstActionJSON = json
}

// RegisterTestAction registers an action for testing and captures its execution
func (f *ActionTestFramework) RegisterTestAction(
	actionName string,
	description string,
	verifier func(loop *reactloops.ReActLoop, action *aicommon.Action) error,
	handler func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator),
) {
	capture := &ActionHandlerCapture{
		Called: false,
	}
	f.actionHandlers[actionName] = capture

	// Wrap the handler to capture execution details
	wrappedHandler := func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		f.mu.Lock()
		capture.Called = true
		capture.Action = action
		capture.Operator = op
		capture.CalledCount++
		f.mu.Unlock()

		// Call the actual handler
		if handler != nil {
			handler(loop, action, op)
		}

		// Capture feedback and status
		f.mu.Lock()
		if op.GetFeedback() != nil {
			capture.FeedbackMsg = op.GetFeedback().String()
		}
		capture.Continued = op.IsContinued()
		terminated, err := op.IsTerminated()
		capture.Terminated = terminated
		if err != nil {
			capture.Failed = true
			capture.FailedMsg = err.Error()
		}
		f.mu.Unlock()
	}

	// Register the action with the loop using the ReActLoopOption
	option := reactloops.WithRegisterLoopAction(
		actionName,
		description,
		nil, // No additional parameters for testing
		verifier,
		wrappedHandler,
	)

	// Apply the option to the loop
	option(f.loop)
}

// ExecuteAction executes a specific action with given parameters
func (f *ActionTestFramework) ExecuteAction(actionName string, params map[string]interface{}) error {
	return f.ExecuteActionWithTimeout(actionName, params, 0)
}

// ExecuteActionWithTimeout executes a specific action with given parameters and timeout
// If timeout is 0 or negative, it uses context.Background() (no timeout)
// If timeout > 0, it creates a context with the specified timeout
func (f *ActionTestFramework) ExecuteActionWithTimeout(actionName string, params map[string]interface{}, timeout time.Duration) error {
	// Build action map
	actionMap := make(map[string]interface{})
	actionMap["@action"] = actionName
	for key, value := range params {
		actionMap[key] = value
	}

	// Marshal to JSON (this properly escapes strings, handles newlines, etc.)
	actionBytes, err := json.Marshal(actionMap)
	if err != nil {
		return fmt.Errorf("failed to marshal action JSON: %v", err)
	}
	actionJSON := string(actionBytes)

	// Set the action JSON for the first AI call
	f.setFirstActionJSON(actionJSON)

	// Create context with timeout if specified
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		defer cancel()
	}

	// Execute the loop
	return f.loop.Execute("test-task-"+actionName, ctx, "Testing action: "+actionName)
}

// GetLoop returns the underlying ReActLoop for custom operations
func (f *ActionTestFramework) GetLoop() *reactloops.ReActLoop {
	return f.loop
}

// GetTask returns the captured task
func (f *ActionTestFramework) GetTask() aicommon.AIStatefulTask {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.capturedTask
}

// GetActionCapture returns the capture information for a specific action
func (f *ActionTestFramework) GetActionCapture(actionName string) *ActionHandlerCapture {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.actionHandlers[actionName]
}

// AssertActionCalled asserts that an action was called
func (f *ActionTestFramework) AssertActionCalled(actionName string) {
	capture := f.GetActionCapture(actionName)
	if capture == nil {
		f.t.Errorf("Action '%s' was never registered", actionName)
		return
	}
	if !capture.Called {
		f.t.Errorf("Action '%s' was not called", actionName)
	}
}

// AssertActionNotCalled asserts that an action was not called
func (f *ActionTestFramework) AssertActionNotCalled(actionName string) {
	capture := f.GetActionCapture(actionName)
	if capture == nil {
		return // Action not registered, so it wasn't called
	}
	if capture.Called {
		f.t.Errorf("Action '%s' should not have been called", actionName)
	}
}

// AssertActionFailed asserts that an action failed
func (f *ActionTestFramework) AssertActionFailed(actionName string) {
	capture := f.GetActionCapture(actionName)
	if capture == nil {
		f.t.Errorf("Action '%s' was never registered", actionName)
		return
	}
	if !capture.Failed {
		f.t.Errorf("Action '%s' should have failed", actionName)
	}
}

// AssertActionSucceeded asserts that an action succeeded (continued or terminated without failure)
func (f *ActionTestFramework) AssertActionSucceeded(actionName string) {
	capture := f.GetActionCapture(actionName)
	if capture == nil {
		f.t.Errorf("Action '%s' was never registered", actionName)
		return
	}
	if capture.Failed {
		f.t.Errorf("Action '%s' should have succeeded but failed with: %s", actionName, capture.FailedMsg)
	}
}

// AssertFeedbackContains asserts that the feedback message contains a substring
func (f *ActionTestFramework) AssertFeedbackContains(actionName string, substring string) {
	capture := f.GetActionCapture(actionName)
	if capture == nil {
		f.t.Errorf("Action '%s' was never registered", actionName)
		return
	}
	if !utils.MatchAllOfSubString(capture.FeedbackMsg, substring) {
		f.t.Errorf("Action '%s' feedback should contain '%s', got: %s", actionName, substring, capture.FeedbackMsg)
	}
}

// AssertLoopContextValue asserts a value in the loop context
func (f *ActionTestFramework) AssertLoopContextValue(key string, expectedValue interface{}) {
	actualValue := f.loop.Get(key)
	if actualValue != expectedValue {
		f.t.Errorf("Loop context[%s] should be %v, got: %v", key, expectedValue, actualValue)
	}
}

// AssertLoopContextInt asserts an int value in the loop context
func (f *ActionTestFramework) AssertLoopContextInt(key string, expectedValue int) {
	actualValue := f.loop.GetInt(key)
	if actualValue != expectedValue {
		f.t.Errorf("Loop context[%s] should be %d, got: %d", key, expectedValue, actualValue)
	}
}

// GetAICallCount returns the number of AI calls made
func (f *ActionTestFramework) GetAICallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.aiCallCount
}

// Reset resets all captured state (useful for multiple test cases)
func (f *ActionTestFramework) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aiCallCount = 0
	f.capturedTask = nil
	for _, capture := range f.actionHandlers {
		capture.Called = false
		capture.Action = nil
		capture.Operator = nil
		capture.FeedbackMsg = ""
		capture.Failed = false
		capture.FailedMsg = ""
		capture.Continued = false
		capture.Terminated = false
		capture.CalledCount = 0
	}
}
