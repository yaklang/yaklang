package dap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-dap"
	"github.com/yaklang/yaklang/common/netx"
)

type TestClient struct {
	seq    int
	conn   net.Conn
	reader *bufio.Reader
}

func (c *TestClient) Close() {
	c.conn.Close()
}

func (c *TestClient) send(request dap.Message) {
	dap.WriteProtocolMessage(c.conn, request)
}

func (c *TestClient) ReadMessage() (dap.Message, error) {
	return dap.ReadProtocolMessage(c.reader)
}

func (c *TestClient) newRequest(command string) *dap.Request {
	request := &dap.Request{}
	request.Type = "request"
	request.Command = command
	request.Seq = c.seq
	c.seq++
	return request
}

func (c *TestClient) ExpectMessage(t *testing.T) dap.Message {
	t.Helper()
	m, err := dap.ReadProtocolMessage(c.reader)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func (c *TestClient) InitializeRequest() {
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

func (c *TestClient) LaunchRequest(mode, program string) {
	request := &dap.LaunchRequest{Request: *c.newRequest("launch")}
	request.Arguments = toRawMessage(map[string]interface{}{
		"request": "launch",
		"mode":    mode,
		"program": program,
	})
	c.send(request)
}

func (c *TestClient) DisconnectRequest() {
	request := &dap.DisconnectRequest{Request: *c.newRequest("disconnect")}
	c.send(request)
}

func (c *TestClient) ExpectInitializeResponseAndCapabilities(t *testing.T) *dap.InitializeResponse {
	t.Helper()
	initResp := c.ExpectInitializeResponse(t)
	wantCapabilities := dap.Capabilities{
		SupportsStepInTargetsRequest:     true,
		SupportsEvaluateForHovers:        true,
		SupportsConditionalBreakpoints:   true,
		SupportsConfigurationDoneRequest: true,
		SupportsDataBreakpoints:          true,
		SupportsDisassembleRequest:       true,
	}
	if !reflect.DeepEqual(initResp.Body, wantCapabilities) {
		t.Errorf("capabilities in initializeResponse: got %+v, want %v", pretty(initResp.Body), pretty(wantCapabilities))
	}
	return initResp
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

func NewTestClient(addr string) *TestClient {
	conn, err := netx.DialTCPTimeout(time.Duration(5)*time.Second, addr)
	if err != nil {
		log.Fatalf("dail error: %v", err)
	}
	return NewTestClientFromConn(conn)
}

func NewTestClientFromConn(conn net.Conn) *TestClient {
	c := &TestClient{
		conn:   conn,
		seq:    1,
		reader: bufio.NewReader(conn),
	}
	return c
}
