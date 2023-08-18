package dap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-dap"
	"github.com/yaklang/yaklang/common/netx"
)

type Client struct {
	seq    int
	conn   net.Conn
	reader *bufio.Reader
}

func (c *Client) Close() {
	c.conn.Close()
}

func (c *Client) send(request dap.Message) {
	dap.WriteProtocolMessage(c.conn, request)
}

func (c *Client) ReadMessage() (dap.Message, error) {
	return dap.ReadProtocolMessage(c.reader)
}

func (c *Client) newRequest(command string) *dap.Request {
	request := &dap.Request{}
	request.Type = "request"
	request.Command = command
	request.Seq = c.seq
	c.seq++
	return request
}

func (c *Client) InitializeRequest() {
	request := &dap.InitializeRequest{Request: *c.newRequest("initialize")}
	request.Arguments = dap.InitializeRequestArguments{
		AdapterID:                    "yak",
		PathFormat:                   "path",
		LinesStartAt1:                true,
		ColumnsStartAt1:              true,
		SupportsVariableType:         true,
		SupportsVariablePaging:       true,
		SupportsRunInTerminalRequest: true,
		Locale:                       "en-us",
	}
	c.send(request)
}

func (c *Client) NextRequest(thread int) {
	request := &dap.NextRequest{Request: *c.newRequest("next")}
	request.Arguments.ThreadId = thread
	c.send(request)
}

func (c *Client) NextInstructionRequest(thread int) {
	request := &dap.NextRequest{Request: *c.newRequest("next")}
	request.Arguments.ThreadId = thread
	request.Arguments.Granularity = "instruction"
	c.send(request)
}

func (c *Client) StepInRequest(thread int) {
	request := &dap.StepInRequest{Request: *c.newRequest("stepIn")}
	request.Arguments.ThreadId = thread
	c.send(request)
}

func (c *Client) StepInInstructionRequest(thread int) {
	request := &dap.StepInRequest{Request: *c.newRequest("stepIn")}
	request.Arguments.ThreadId = thread
	request.Arguments.Granularity = "instruction"
	c.send(request)
}

func (c *Client) StepOutRequest(thread int) {
	request := &dap.StepOutRequest{Request: *c.newRequest("stepOut")}
	request.Arguments.ThreadId = thread
	c.send(request)
}

func (c *Client) StepOutInstructionRequest(thread int) {
	request := &dap.StepOutRequest{Request: *c.newRequest("stepOut")}
	request.Arguments.ThreadId = thread
	request.Arguments.Granularity = "instruction"
	c.send(request)
}

func (c *Client) PauseRequest(threadId int) {
	request := &dap.PauseRequest{Request: *c.newRequest("pause")}
	request.Arguments.ThreadId = threadId
	c.send(request)
}

func (c *Client) LaunchRequest(mode, program string, stopOnEntry bool) {
	request := &dap.LaunchRequest{Request: *c.newRequest("launch")}
	request.Arguments = toRawMessage(map[string]interface{}{
		"request":     "launch",
		"mode":        mode,
		"program":     program,
		"stopOnEntry": stopOnEntry,
	})
	c.send(request)
}

func (c *Client) ContinueRequest(thread int) {
	request := &dap.ContinueRequest{Request: *c.newRequest("continue")}
	request.Arguments.ThreadId = thread
	c.send(request)
}

func (c *Client) DisconnectRequest() {
	request := &dap.DisconnectRequest{Request: *c.newRequest("disconnect")}
	c.send(request)
}

func (c *Client) ConfigurationDoneRequest() {
	request := &dap.ConfigurationDoneRequest{Request: *c.newRequest("configurationDone")}
	c.send(request)
}

func (c *Client) ThreadsRequest() {
	request := &dap.ThreadsRequest{Request: *c.newRequest("threads")}
	c.send(request)
}

func (c *Client) EvaluateRequest(expr string, fid int, context string) {
	request := &dap.EvaluateRequest{Request: *c.newRequest("evaluate")}
	request.Arguments.Expression = expr
	request.Arguments.FrameId = fid
	request.Arguments.Context = context
	c.send(request)
}

func (c *Client) StackTraceRequest(threadID, startFrame, levels int) {
	request := &dap.StackTraceRequest{Request: *c.newRequest("stackTrace")}
	request.Arguments.ThreadId = threadID
	request.Arguments.StartFrame = startFrame
	request.Arguments.Levels = levels
	c.send(request)
}

func (c *Client) ScopesRequest(frameID int) {
	request := &dap.ScopesRequest{Request: *c.newRequest("scopes")}
	request.Arguments.FrameId = frameID
	c.send(request)
}

func (c *Client) VariablesRequest(variablesReference int) {
	request := &dap.VariablesRequest{Request: *c.newRequest("variables")}
	request.Arguments.VariablesReference = variablesReference
	c.send(request)
}

func (c *Client) SetVariableRequest(variablesRef int, name, value string) {
	request := &dap.SetVariableRequest{Request: *c.newRequest("setVariable")}
	request.Arguments.VariablesReference = variablesRef
	request.Arguments.Name = name
	request.Arguments.Value = value
	c.send(request)
}

func (c *Client) SetExpressionRequest() {
	c.send(&dap.SetExpressionRequest{Request: *c.newRequest("setExpression")})
}

func (c *Client) SetBreakpointsRequest(file string, lines []int) {
	c.SetBreakpointsRequestWithArgs(file, lines, nil, nil, nil)
}

func (c *Client) SetExceptionBreakpointsRequest() {
	request := &dap.SetBreakpointsRequest{Request: *c.newRequest("setExceptionBreakpoints")}
	c.send(request)
}

func (c *Client) SetBreakpointsRequestWithArgs(file string, lines []int, conditions, hitConditions, logMessages map[int]string) {
	request := &dap.SetBreakpointsRequest{Request: *c.newRequest("setBreakpoints")}
	request.Arguments = dap.SetBreakpointsArguments{
		Source: dap.Source{
			Name: filepath.Base(file),
			Path: file,
		},
		Breakpoints: make([]dap.SourceBreakpoint, len(lines)),
	}
	for i, l := range lines {
		request.Arguments.Breakpoints[i].Line = l
		if cond, ok := conditions[l]; ok {
			request.Arguments.Breakpoints[i].Condition = cond
		}
		if hitCond, ok := hitConditions[l]; ok {
			request.Arguments.Breakpoints[i].HitCondition = hitCond
		}
		if logMessage, ok := logMessages[l]; ok {
			request.Arguments.Breakpoints[i].LogMessage = logMessage
		}
	}
	c.send(request)
}

func (c *Client) ExpectOutputEventDetaching(t *testing.T) *dap.OutputEvent {
	t.Helper()
	return c.ExpectOutputEventRegex(t, `Detaching\n`)
}

func (c *Client) ExpectOutputEventHelpInfo(t *testing.T) *dap.OutputEvent {
	t.Helper()
	return c.ExpectOutputEventRegex(t, `Type 'dbg help' for help info.`)
}

func (c *Client) ExpectInvisibleErrorResponse(t *testing.T) *dap.ErrorResponse {
	t.Helper()
	er := c.ExpectErrorResponse(t)
	if er.Body.Error != nil && er.Body.Error.ShowUser {
		t.Errorf("\ngot %#v\nwant ShowUser=false", er)
	}
	return er
}

func pretty(v interface{}) string {
	s, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		return fmt.Sprintf("%#v", s)
	}
	return string(s)
}

func toRawMessage(in interface{}) json.RawMessage {
	out, _ := json.Marshal(in)
	return out
}

func NewTestClient(addr string) *Client {
	conn, err := netx.DialTCPTimeout(time.Duration(5)*time.Second, addr)
	if err != nil {
		log.Fatalf("dail error: %v", err)
	}
	return NewTestClientFromConn(conn)
}

func NewTestClientFromConn(conn net.Conn) *Client {
	c := &Client{
		conn:   conn,
		seq:    1,
		reader: bufio.NewReader(conn),
	}
	return c
}
