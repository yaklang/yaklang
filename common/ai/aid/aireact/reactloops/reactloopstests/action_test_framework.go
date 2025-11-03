package reactloopstests

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"

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
	framework := &ActionTestFramework{
		t:              t,
		aiCallCount:    0,
		actionHandlers: make(map[string]*ActionHandlerCapture),
	}

	// Create a test ReAct instance with a callback that handles action execution
	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			framework.mu.Lock()
			framework.aiCallCount++
			callNum := framework.aiCallCount
			framework.mu.Unlock()

			rsp := i.NewAIResponse()

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
	)
	if err != nil {
		t.Fatalf("Failed to create test ReAct instance: %v", err)
	}

	framework.reactInstance = reactIns

	// Add task created callback
	options = append(options, reactloops.WithOnTaskCreated(func(task aicommon.AIStatefulTask) {
		framework.mu.Lock()
		framework.capturedTask = task
		framework.mu.Unlock()
	}))

	// Create the loop with provided options
	loop, err := reactloops.NewReActLoop(loopName, reactIns, options...)
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
	// Build action JSON
	actionJSON := fmt.Sprintf(`{"@action": "%s"`, actionName)
	for key, value := range params {
		switch v := value.(type) {
		case string:
			actionJSON += fmt.Sprintf(`, "%s": "%s"`, key, v)
		case int:
			actionJSON += fmt.Sprintf(`, "%s": %d`, key, v)
		case bool:
			actionJSON += fmt.Sprintf(`, "%s": %t`, key, v)
		default:
			actionJSON += fmt.Sprintf(`, "%s": %v`, key, v)
		}
	}
	actionJSON += "}"

	// Set the action JSON for the first AI call
	f.setFirstActionJSON(actionJSON)

	// Execute the loop
	return f.loop.Execute("test-task-"+actionName, context.Background(), "Testing action: "+actionName)
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
