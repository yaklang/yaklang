package dap

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/google/go-dap"
)

func (c *Client) ExpectMessage(t *testing.T) dap.Message {
	t.Helper()
	m, err := dap.ReadProtocolMessage(c.reader)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func (c *Client) ExpectOutputEventRegex(t *testing.T, want string) *dap.OutputEvent {
	t.Helper()
	e := c.ExpectOutputEvent(t)
	if matched, _ := regexp.MatchString(want, e.Body.Output); !matched {
		t.Errorf("\ngot %#v\nwant Output=%q", e, want)
	}
	return e
}

func (c *Client) ExpectOutputEvent(t *testing.T) *dap.OutputEvent {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckOutputEvent(t, m)
}

func (c *Client) CheckOutputEvent(t *testing.T, m dap.Message) *dap.OutputEvent {
	t.Helper()
	r, ok := m.(*dap.OutputEvent)
	if !ok {
		t.Fatalf("got %#v, want *dap.OutputEvent", m)
	}
	return r
}

func (c *Client) ExpectInitializeResponseAndCapabilities(t *testing.T) *dap.InitializeResponse {
	t.Helper()
	initResp := c.ExpectInitializeResponse(t)
	wantCapabilities := dap.Capabilities{
		SupportsStepInTargetsRequest:     true,
		SupportsEvaluateForHovers:        true,
		SupportsConditionalBreakpoints:   true,
		SupportsConfigurationDoneRequest: true,
		SupportsDataBreakpoints:          true,
		SupportsDisassembleRequest:       true,
		SupportTerminateDebuggee:         true,
		SupportsSetVariable:              true,
		SupportsSetExpression:            true,
	}
	if !reflect.DeepEqual(initResp.Body, wantCapabilities) {
		t.Errorf("capabilities in initializeResponse: got %+v, want %v", pretty(initResp.Body), pretty(wantCapabilities))
	}
	return initResp
}

func (c *Client) ExpectInitializeResponse(t *testing.T) *dap.InitializeResponse {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckInitializeResponse(t, m)
}

func (c *Client) CheckInitializeResponse(t *testing.T, m dap.Message) *dap.InitializeResponse {
	t.Helper()
	r, ok := m.(*dap.InitializeResponse)
	if !ok {
		t.Fatalf("got %#v, want *dap.InitializeResponse", m)
	}
	return r
}

func (c *Client) ExpectInitializedEvent(t *testing.T) *dap.InitializedEvent {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckInitializedEvent(t, m)
}

func (c *Client) CheckInitializedEvent(t *testing.T, m dap.Message) *dap.InitializedEvent {
	t.Helper()
	r, ok := m.(*dap.InitializedEvent)
	if !ok {
		t.Fatalf("got %#v, want *dap.InitializedEvent", m)
	}
	return r
}

func (c *Client) ExpectLaunchResponse(t *testing.T) *dap.LaunchResponse {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckLaunchResponse(t, m)
}

func (c *Client) CheckLaunchResponse(t *testing.T, m dap.Message) *dap.LaunchResponse {
	t.Helper()
	r, ok := m.(*dap.LaunchResponse)
	if !ok {
		t.Fatalf("got %#v, want *dap.LaunchResponse", m)
	}
	return r
}

func (c *Client) ExpectContinueResponse(t *testing.T) *dap.ContinueResponse {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckContinueResponse(t, m)
}

func (c *Client) CheckContinueResponse(t *testing.T, m dap.Message) *dap.ContinueResponse {
	t.Helper()
	r, ok := m.(*dap.ContinueResponse)
	if !ok {
		t.Fatalf("got %#v, want *dap.ContinueResponse", m)
	}
	return r
}

func (c *Client) ExpectDisconnectResponse(t *testing.T) *dap.DisconnectResponse {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckDisconnectResponse(t, m)
}

func (c *Client) CheckDisconnectResponse(t *testing.T, m dap.Message) *dap.DisconnectResponse {
	t.Helper()
	r, ok := m.(*dap.DisconnectResponse)
	if !ok {
		t.Fatalf("got %#v, want *dap.DisconnectResponse", m)
	}
	return r
}

func (c *Client) ExpectTerminatedEvent(t *testing.T) *dap.TerminatedEvent {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckTerminatedEvent(t, m)
}

func (c *Client) CheckTerminatedEvent(t *testing.T, m dap.Message) *dap.TerminatedEvent {
	t.Helper()
	r, ok := m.(*dap.TerminatedEvent)
	if !ok {
		t.Fatalf("got %#v, want *dap.TerminatedEvent", m)
	}
	return r
}

func (c *Client) ExpectSetBreakpointsResponse(t *testing.T) *dap.SetBreakpointsResponse {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckSetBreakpointsResponse(t, m)
}

func (c *Client) CheckSetBreakpointsResponse(t *testing.T, m dap.Message) *dap.SetBreakpointsResponse {
	t.Helper()
	r, ok := m.(*dap.SetBreakpointsResponse)
	if !ok {
		t.Fatalf("got %#v, want *dap.SetBreakpointsResponse", m)
	}
	return r
}

func (c *Client) ExpectSetExceptionBreakpointsResponse(t *testing.T) *dap.SetExceptionBreakpointsResponse {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckSetExceptionBreakpointsResponse(t, m)
}

func (c *Client) CheckSetExceptionBreakpointsResponse(t *testing.T, m dap.Message) *dap.SetExceptionBreakpointsResponse {
	t.Helper()
	r, ok := m.(*dap.SetExceptionBreakpointsResponse)
	if !ok {
		t.Fatalf("got %#v, want *dap.SetExceptionBreakpointsResponse", m)
	}
	return r
}

func (c *Client) ExpectStoppedEvent(t *testing.T) *dap.StoppedEvent {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckStoppedEvent(t, m)
}

func (c *Client) CheckStoppedEvent(t *testing.T, m dap.Message) *dap.StoppedEvent {
	t.Helper()
	r, ok := m.(*dap.StoppedEvent)
	if !ok {
		t.Fatalf("got %#v, want *dap.StoppedEvent", m)
	}
	return r
}

func (c *Client) ExpectConfigurationDoneResponse(t *testing.T) *dap.ConfigurationDoneResponse {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckConfigurationDoneResponse(t, m)
}

func (c *Client) CheckConfigurationDoneResponse(t *testing.T, m dap.Message) *dap.ConfigurationDoneResponse {
	t.Helper()
	r, ok := m.(*dap.ConfigurationDoneResponse)
	if !ok {
		t.Fatalf("got %#v, want *dap.ConfigurationDoneResponse", m)
	}
	return r
}

func (c *Client) ExpectThreadsResponse(t *testing.T) *dap.ThreadsResponse {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckThreadsResponse(t, m)
}

func (c *Client) CheckThreadsResponse(t *testing.T, m dap.Message) *dap.ThreadsResponse {
	t.Helper()
	r, ok := m.(*dap.ThreadsResponse)
	if !ok {
		t.Fatalf("got %#v, want *dap.ThreadsResponse", m)
	}
	return r
}

func (c *Client) ExpectStackTraceResponse(t *testing.T) *dap.StackTraceResponse {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckStackTraceResponse(t, m)
}

func (c *Client) CheckStackTraceResponse(t *testing.T, m dap.Message) *dap.StackTraceResponse {
	t.Helper()
	r, ok := m.(*dap.StackTraceResponse)
	if !ok {
		t.Fatalf("got %#v, want *dap.StackTraceResponse", m)
	}
	return r
}

func (c *Client) ExpectEvaluateResponse(t *testing.T) *dap.EvaluateResponse {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckEvaluateResponse(t, m)
}

func (c *Client) CheckEvaluateResponse(t *testing.T, m dap.Message) *dap.EvaluateResponse {
	t.Helper()
	r, ok := m.(*dap.EvaluateResponse)
	if !ok {
		t.Fatalf("got %#v, want *dap.EvaluateResponse", m)
	}
	return r
}

func (c *Client) ExpectErrorResponse(t *testing.T) *dap.ErrorResponse {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckErrorResponse(t, m)
}

func (c *Client) CheckErrorResponse(t *testing.T, m dap.Message) *dap.ErrorResponse {
	t.Helper()
	r, ok := m.(*dap.ErrorResponse)
	if !ok {
		t.Fatalf("got %#v, want *dap.ErrorResponse", m)
	}
	return r
}

func (c *Client) ExpectScopesResponse(t *testing.T) *dap.ScopesResponse {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckScopesResponse(t, m)
}

func (c *Client) CheckScopesResponse(t *testing.T, m dap.Message) *dap.ScopesResponse {
	t.Helper()
	r, ok := m.(*dap.ScopesResponse)
	if !ok {
		t.Fatalf("got %#v, want *dap.ScopesResponse", m)
	}
	return r
}

func (c *Client) ExpectVariablesResponse(t *testing.T) *dap.VariablesResponse {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckVariablesResponse(t, m)
}

func (c *Client) CheckVariablesResponse(t *testing.T, m dap.Message) *dap.VariablesResponse {
	t.Helper()
	r, ok := m.(*dap.VariablesResponse)
	if !ok {
		t.Fatalf("got %#v, want *dap.VariablesResponse", m)
	}
	return r
}

func (c *Client) ExpectSetVariableResponse(t *testing.T) *dap.SetVariableResponse {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckSetVariableResponse(t, m)
}

func (c *Client) CheckSetVariableResponse(t *testing.T, m dap.Message) *dap.SetVariableResponse {
	t.Helper()
	r, ok := m.(*dap.SetVariableResponse)
	if !ok {
		t.Fatalf("got %#v, want *dap.SetVariableResponse", m)
	}
	return r
}

func (c *Client) ExpectSetExpressionResponse(t *testing.T) *dap.SetExpressionResponse {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckSetExpressionResponse(t, m)
}

func (c *Client) CheckSetExpressionResponse(t *testing.T, m dap.Message) *dap.SetExpressionResponse {
	t.Helper()
	r, ok := m.(*dap.SetExpressionResponse)
	if !ok {
		t.Fatalf("got %#v, want *dap.SetExpressionResponse", m)
	}
	return r
}

func (c *Client) CheckStopLocation(t *testing.T, thread int, name string, line int) {
	t.Helper()
	c.StackTraceRequest(thread, 0, 20)
	st := c.ExpectStackTraceResponse(t)
	if len(st.Body.StackFrames) < 1 {
		t.Errorf("\ngot  %#v\nwant len(stackframes) => 1", st)
	} else {
		if line != -1 && st.Body.StackFrames[0].Line != line {
			t.Errorf("\ngot  %#v\nwant Line=%d", st, line)
		}
		if st.Body.StackFrames[0].Name != name {
			t.Errorf("\ngot  %#v\nwant Name=%q", st, name)
		}
	}
}
