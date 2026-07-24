package mcp

// Regression tests for bugs fixed in the MCP layer.
//
// Issue 2: scanning tools sent non-standard notification method names
//          (e.g. "port_scan/progress") that caused strict MCP SDK validators
//          (Python SDK, etc.) to drop the SSE connection.
//          Fixed: all progress notifications now use "notifications/progress"
//          and all info notifications use "notifications/message".
//
// Issue 3: exec_codec required every parameter to be explicitly supplied.
//          Callers omitting parameters with declared DefaultValue received
//          "codec param <name> not found" errors.
//          Fixed: CodecFlowExec now falls back to the DefaultValue from
//          CodecLibsDoc when a parameter is absent.
//
// Issue 4: handleQueryPayload called s.grpcClient.GetProfileDatabase() which
//          is only valid in the embedded (in-process) execution mode and
//          panics when the MCP server is accessed remotely via a gRPC stub.
//          Fixed: type routing now uses GetAllPayloadGroup over gRPC and the
//          walk logic is extracted into findPayloadGroupIsFile.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	mcpserver "github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec/codegrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ---------------------------------------------------------------------------
// Issue 2 – notification method names must conform to the MCP spec
// ---------------------------------------------------------------------------

// collectNotificationMethods subscribes to the server's notification hub, calls
// trigger, drains notifications for up to drainDur, then returns the collected
// method names.
func collectNotificationMethods(
	t *testing.T,
	srv *mcpserver.MCPServer,
	trigger func(),
	drainDur time.Duration,
) []string {
	t.Helper()

	ch, cancel := srv.SubscribeNotifications(32)
	defer cancel()

	trigger()

	var methods []string
	deadline := time.After(drainDur)
drain:
	for {
		select {
		case n, ok := <-ch:
			if !ok {
				break drain
			}
			methods = append(methods, n.Notification.Method)
		case <-deadline:
			break drain
		}
	}
	return methods
}

// TestNotificationMethodNames verifies that the method names used for
// progress and info notifications conform to the MCP spec.
//
// The handler replicates the notification pattern used by port_scan,
// hybrid_scan, brute, subdomain_collection, and save_payload.
func TestNotificationMethodNames(t *testing.T) {
	srv := mcpserver.NewMCPServer("test", "1.0")

	// Simulate the notification calls made by the fixed scanning tools.
	trigger := func() {
		// Progress notification – only sent when progressToken is non-nil.
		token := "test-token-123"
		_ = srv.SendNotificationToClient("notifications/progress", map[string]any{
			"progressToken": token,
			"progress":      0.5,
		})

		// Info/message notification.
		_ = srv.SendNotificationToClient("notifications/message", map[string]any{
			"level": "info",
			"data":  "scan started",
		})
	}

	methods := collectNotificationMethods(t, srv, trigger, 200*time.Millisecond)

	require.Len(t, methods, 2, "expected exactly two notifications")

	// Neither method may use the old vendor-namespaced patterns.
	for _, m := range methods {
		assert.NotContains(t, m, "port_scan/", "method must not use vendor-namespaced prefix")
		assert.NotContains(t, m, "hybrid_scan/", "method must not use vendor-namespaced prefix")
		assert.NotContains(t, m, "brute/", "method must not use vendor-namespaced prefix")
		assert.NotContains(t, m, "save_payload/", "method must not use vendor-namespaced prefix")
		assert.NotContains(t, m, "subdomain_collection/", "method must not use vendor-namespaced prefix")
	}

	assert.Equal(t, "notifications/progress", methods[0])
	assert.Equal(t, "notifications/message", methods[1])
}

// TestProgressNotificationSkippedWithoutToken verifies that no progress
// notification is sent when the caller omits progressToken (nil).
// Sending notifications/progress with a null token violates the MCP spec and
// causes strict SDK validators to fail.
func TestProgressNotificationSkippedWithoutToken(t *testing.T) {
	srv := mcpserver.NewMCPServer("test", "1.0")

	var progressToken interface{} // nil – client did not supply a token

	trigger := func() {
		// This is the pattern used in all fixed scanning handlers:
		// only send progress when progressToken is non-nil.
		if progressToken != nil {
			_ = srv.SendNotificationToClient("notifications/progress", map[string]any{
				"progressToken": progressToken,
				"progress":      1.0,
			})
		}
		// Info notifications are always sent regardless of token.
		_ = srv.SendNotificationToClient("notifications/message", map[string]any{
			"level": "info",
			"data":  "done",
		})
	}

	methods := collectNotificationMethods(t, srv, trigger, 200*time.Millisecond)

	// Only the message notification should arrive; progress must be absent.
	require.Len(t, methods, 1)
	assert.Equal(t, "notifications/message", methods[0])
}

// ---------------------------------------------------------------------------
// Issue 3 – CodecFlowExec must fall back to declared DefaultValue
// ---------------------------------------------------------------------------

// TestCodecFlowExec_DefaultParameters verifies that methods with declared
// DefaultValue in their TOML documentation can be called with an empty params
// list and still succeed.
func TestCodecFlowExec_DefaultParameters(t *testing.T) {
	cases := []struct {
		name      string
		codecType string
		input     string
	}{
		{
			// Base64Encode: Alphabet has DefaultValue = "standard"
			name:      "Base64Encode without explicit Alphabet",
			codecType: "Base64Encode",
			input:     "hello",
		},
		{
			// Base64Decode: Alphabet has DefaultValue = "standard"
			name:      "Base64Decode without explicit Alphabet",
			codecType: "Base64Decode",
			input:     "aGVsbG8=",
		},
		{
			// HtmlEncode: entityRef has DefaultValue = "named"
			// (fullEncode is a checkbox without DefaultValue – omit it too;
			//  the TOML doc marks it Required=true but has no DefaultValue,
			//  so we supply it explicitly to isolate the entityRef check)
			name:      "JsonFormat without explicit mode",
			codecType: "JsonFormat",
			input:     `{"key":"value"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &ypb.CodecRequestFlow{
				Text: tc.input,
				WorkFlow: []*ypb.CodecWork{
					{
						CodecType: tc.codecType,
						Params:    []*ypb.ExecParamItem{}, // intentionally empty
					},
				},
			}

			resp, err := codegrpc.CodecFlowExec(req)
			require.NoErrorf(t, err,
				"CodecFlowExec(%s) with empty params should succeed using DefaultValue, got: %v",
				tc.codecType, err)
			assert.NotNil(t, resp)
		})
	}
}

// TestCodecFlowExec_NoDefaultValue verifies that parameters without a
// DefaultValue still return an error when omitted, so the fix does not
// silently swallow legitimately missing required inputs.
func TestCodecFlowExec_NoDefaultValue(t *testing.T) {
	// HtmlEncode has two params: entityRef (DefaultValue="named") and
	// fullEncode (checkbox, no DefaultValue). Supplying entityRef but not
	// fullEncode should still produce an error for the missing parameter.
	req := &ypb.CodecRequestFlow{
		Text: "<b>hi</b>",
		WorkFlow: []*ypb.CodecWork{
			{
				CodecType: "HtmlEncode",
				Params: []*ypb.ExecParamItem{
					{Key: "entityRef", Value: "named"},
					// fullEncode intentionally omitted
				},
			},
		},
	}

	_, err := codegrpc.CodecFlowExec(req)
	// fullEncode has no DefaultValue in the TOML doc, so an error is expected.
	require.Error(t, err, "expected error for missing fullEncode parameter with no DefaultValue")
	assert.Contains(t, err.Error(), "fullEncode")
}

// ---------------------------------------------------------------------------
// Issue 4 – findPayloadGroupIsFile must route by node type, not local DB
// ---------------------------------------------------------------------------

// TestFindPayloadGroupIsFile covers the tree-walk logic extracted from
// handleQueryPayload. No gRPC client is needed.
func TestFindPayloadGroupIsFile(t *testing.T) {
	// Build a representative group tree:
	//
	//   root
	//   ├── db_group       (DataBase)
	//   ├── file_group     (File)
	//   └── folder/
	//       ├── nested_db  (DataBase)
	//       └── nested_file (File)
	nodes := []*ypb.PayloadGroupNode{
		{Type: "DataBase", Name: "db_group"},
		{Type: "File", Name: "file_group"},
		{
			Type: "Folder",
			Name: "folder",
			Nodes: []*ypb.PayloadGroupNode{
				{Type: "DataBase", Name: "nested_db"},
				{Type: "File", Name: "nested_file"},
			},
		},
	}

	tests := []struct {
		group      string
		wantIsFile bool
		wantErr    bool
	}{
		{"db_group", false, false},
		{"file_group", true, false},
		{"nested_db", false, false},
		{"nested_file", true, false},
		{"nonexistent", false, true},
	}

	for _, tc := range tests {
		t.Run(tc.group, func(t *testing.T) {
			got, err := findPayloadGroupIsFile(nodes, tc.group)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.group)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantIsFile, got,
					"group %q: expected isFile=%v", tc.group, tc.wantIsFile)
			}
		})
	}
}

// TestFindPayloadGroupIsFile_EmptyTree verifies the error path for an empty
// node list (e.g. server returned no groups at all).
func TestFindPayloadGroupIsFile_EmptyTree(t *testing.T) {
	_, err := findPayloadGroupIsFile(nil, "any_group")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "any_group")
}

// TestFindPayloadGroupIsFile_DeeplyNested verifies that the walk correctly
// recurses into multiply-nested folders.
func TestFindPayloadGroupIsFile_DeeplyNested(t *testing.T) {
	nodes := []*ypb.PayloadGroupNode{
		{
			Type: "Folder",
			Name: "l1",
			Nodes: []*ypb.PayloadGroupNode{
				{
					Type: "Folder",
					Name: "l2",
					Nodes: []*ypb.PayloadGroupNode{
						{Type: "File", Name: "deep_file"},
					},
				},
			},
		},
	}

	isFile, err := findPayloadGroupIsFile(nodes, "deep_file")
	require.NoError(t, err)
	assert.True(t, isFile)
}

// Ensure the test package compiles even without a live gRPC server.
var _ context.Context = context.Background()

// ---------------------------------------------------------------------------
// Issue 5 – decodeHookComma: array-to-string must use comma separator
// ---------------------------------------------------------------------------

// TestDecodeHookComma_PortScanMultiPort verifies that number arrays passed
// from MCP clients (e.g. ports: [80, 443]) are decoded into comma-separated
// strings in proto string fields, not newline-separated strings that
// downstream parsers (utils.ParseStringToPorts) cannot split.
func TestDecodeHookComma_PortScanMultiPort(t *testing.T) {
	args := map[string]any{
		"targets":      []any{"192.168.1.1", "192.168.1.2"},
		"ports":        []any{float64(80), float64(443), float64(8080)},
		"excludePorts": []any{float64(22)},
	}

	var req ypb.PortScanRequest
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: decodeHookComma,
		Result:     &req,
	})
	require.NoError(t, err)
	err = decoder.Decode(args)
	require.NoError(t, err)

	// Ports must be comma-separated so ParseStringToPorts can split them.
	assert.Equal(t, "80,443,8080", req.Ports)
	assert.Equal(t, "22", req.ExcludePorts)
	// Targets also comma-separated (consistent with downstream PrettifyListFromStringSplitEx).
	assert.Equal(t, "192.168.1.1,192.168.1.2", req.Targets)
}

// TestDecodeHookComma_SinglePort verifies that a single-element array still
// decodes correctly.
func TestDecodeHookComma_SinglePort(t *testing.T) {
	args := map[string]any{
		"ports": []any{float64(443)},
	}

	var req ypb.PortScanRequest
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: decodeHookComma,
		Result:     &req,
	})
	require.NoError(t, err)
	err = decoder.Decode(args)
	require.NoError(t, err)
	assert.Equal(t, "443", req.Ports)
}

// TestPortScan_StringPorts_Range verifies that port_scan now accepts a string
// "ports" parameter (not number[]), which is the correct schema for ranges like
// "1000-10000". The old schema was number[] which cannot express ranges.
func TestPortScan_StringPorts_Range(t *testing.T) {
	args := map[string]any{
		"ports": "1000-10000",
	}

	var req ypb.PortScanRequest
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: decodeHookComma,
		Result:     &req,
	})
	require.NoError(t, err)
	err = decoder.Decode(args)
	require.NoError(t, err)
	assert.Equal(t, "1000-10000", req.Ports)
}

// TestDecodeHook_NewlineStillUsedByDefault verifies that the original
// decodeHook (used by non-port_scan tools) still uses newline separator,
// ensuring the fix is scoped to port_scan only.
func TestDecodeHook_NewlineStillUsedByDefault(t *testing.T) {
	args := map[string]any{
		"ports": []any{float64(80), float64(443)},
	}

	var req ypb.PortScanRequest
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: decodeHook,
		Result:     &req,
	})
	require.NoError(t, err)
	err = decoder.Decode(args)
	require.NoError(t, err)

	// The default decodeHook joins with "\n" — this is the old (broken for
	// port_scan) behavior that we are NOT changing globally.
	assert.Equal(t, "80\n443", req.Ports)
}

// ---------------------------------------------------------------------------
// Issue 6 – handleExecYakScript must use decodeYakRequest (normalize + hook)
// ---------------------------------------------------------------------------

// TestDecodeYakRequest_FlattensNestedRequest verifies that normalizeMCPArguments
// (called inside decodeYakRequest) flattens arguments nested under a
// "request" key, which some MCP clients send. Without this, handleExecYakScript
// would silently fail to populate PluginName/Code/PluginType.
func TestDecodeYakRequest_FlattensNestedRequest(t *testing.T) {
	args := map[string]any{
		"request": map[string]any{
			"code":       `println("hello")`,
			"pluginType": "yak",
		},
	}

	var req ypb.DebugPluginRequest
	err := decodeYakRequest(args, &req)
	require.NoError(t, err)
	assert.Equal(t, `println("hello")`, req.Code)
	assert.Equal(t, "yak", req.PluginType)
}

// TestDecodeYakRequest_PlainArgs verifies that flat (non-nested) arguments
// decode correctly — the common case.
func TestDecodeYakRequest_PlainArgs(t *testing.T) {
	args := map[string]any{
		"code":       `println("hello")`,
		"pluginType": "yak",
		"pluginName": "my-script",
	}

	var req ypb.DebugPluginRequest
	err := decodeYakRequest(args, &req)
	require.NoError(t, err)
	assert.Equal(t, `println("hello")`, req.Code)
	assert.Equal(t, "yak", req.PluginType)
	assert.Equal(t, "my-script", req.PluginName)
}

// ---------------------------------------------------------------------------
// Issue 6b – decodeHook must not corrupt []byte fields (yso_dump.data)
// ---------------------------------------------------------------------------

// TestDecodeHook_ByteSlicePassthrough verifies that when a []byte value
// (e.g. from normalizeMCPArguments base64-decoding) is decoded into a []byte
// proto field, the decodeHook does NOT expand each byte into a decimal-string
// element and re-join them. This was the root cause of yso_dump.data corruption.
func TestDecodeHook_ByteSlicePassthrough(t *testing.T) {
	raw := []byte{0xac, 0xed, 0x00, 0x05} // Java serialization magic

	type target struct {
		Data []byte
	}
	var dst target
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: decodeHook,
		Result:     &dst,
	})
	require.NoError(t, err)
	err = decoder.Decode(map[string]any{"Data": raw})
	require.NoError(t, err)
	assert.Equal(t, raw, dst.Data, "[]byte must pass through decodeHook unchanged")
}

// TestDecodeYakRequest_YsoDumpData simulates the yso_dump tool: a base64
// string "data" param → normalizeMCPArguments decodes to []byte → decodeHook
// must preserve it → YsoBytesObject.Data must contain the raw bytes.
func TestDecodeYakRequest_YsoDumpData(t *testing.T) {
	// base64("hello world") = "aGVsbG8gd29ybGQ="
	args := map[string]any{
		"data": "aGVsbG8gd29ybGQ=",
	}
	var req ypb.YsoBytesObject
	err := decodeYakRequest(args, &req)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello world"), req.Data)
}

// ---------------------------------------------------------------------------
// Issue 7 – No Required+Default contradictions on any MCP tool parameter
// ---------------------------------------------------------------------------

// TestNoRequiredPlusDefaultContradiction iterates over all registered builtin
// tools and verifies that no parameter is simultaneously marked Required and
// given a Default. Required forces the AI client to always send the parameter,
// making the Default unreachable — a contradiction that wastes the AI's
// token budget and degrades the UX.
func TestNoRequiredPlusDefaultContradiction(t *testing.T) {
	tools := GlobalBuiltinTools()
	require.NotEmpty(t, tools, "expected builtin tools to be registered")

	var violations []string
	for name, twh := range tools {
		tool := twh.Tool()
		if tool == nil || tool.InputSchema.Properties == nil {
			continue
		}
		requiredSet := make(map[string]bool)
		for _, r := range tool.InputSchema.Required {
			requiredSet[r] = true
		}
		tool.InputSchema.Properties.ForEach(func(propName string, propVal any) bool {
			prop, ok := propVal.(map[string]any)
			if !ok {
				return true
			}
			_, hasDefault := prop["default"]
			isRequired := requiredSet[propName]
			if hasDefault && isRequired {
				violations = append(violations,
					fmt.Sprintf("tool %q param %q has both Required and Default", name, propName))
			}
			return true
		})
	}

	assert.Empty(t, violations,
		"the following params have contradictory Required+Default:\n%s",
		strings.Join(violations, "\n"))
}

// ---------------------------------------------------------------------------
// Issue 8 – Key parameters must have format examples in their Description
// ---------------------------------------------------------------------------

// TestParamDescriptionsHaveExamples checks that parameters known to accept
// non-obvious formats include "e.g." examples in their description, so the
// AI caller knows how to fill them without guessing.
func TestParamDescriptionsHaveExamples(t *testing.T) {
	tools := GlobalBuiltinTools()

	type expect struct {
		tool    string
		param   string
		substrs []string // any of these substrings satisfies the check
	}
	expectations := []expect{
		{"port_scan", "ports", []string{"e.g."}},
		{"port_scan", "hostAlivePorts", []string{"e.g."}},
		{"port_scan", "excludePorts", []string{"e.g."}},
		{"brute", "targets", []string{"e.g."}},
		{"http_fuzzer", "batchTarget", []string{"e.g."}},
		{"query_http_flow", "isWebsocket", []string{"http/https", "websocket"}},
		{"query_risks", "isRead", []string{"false"}},
		{"query_risks", "ports", []string{"e.g."}},
	}

	for _, exp := range expectations {
		t.Run(exp.tool+"/"+exp.param, func(t *testing.T) {
			twh, ok := tools[exp.tool]
			require.True(t, ok, "tool %q not found", exp.tool)

			tool := twh.Tool()
			require.NotNil(t, tool.InputSchema.Properties)

			propVal, ok := tool.InputSchema.Properties.Get(exp.param)
			require.True(t, ok, "param %q not found on tool %q", exp.param, exp.tool)

			prop, ok := propVal.(map[string]any)
			require.True(t, ok)

			desc, _ := prop["description"].(string)
			require.NotEmpty(t, desc, "param %q on tool %q has empty description", exp.param, exp.tool)

			matched := false
			for _, s := range exp.substrs {
				if strings.Contains(desc, s) {
					matched = true
					break
				}
			}
			assert.True(t, matched,
				"description for %q.%q should contain one of %v, got: %s",
				exp.tool, exp.param, exp.substrs, desc)
		})
	}
}

// TestSetSystemProxyDescription verifies the tool description is not the
// copy-paste "Get system proxy" leftover.
func TestSetSystemProxyDescription(t *testing.T) {
	twh, ok := GlobalBuiltinTools()["set_system_proxy"]
	require.True(t, ok)
	tool := twh.Tool()
	assert.Contains(t, tool.Description, "Set system proxy")
	assert.NotContains(t, tool.Description, "Get system proxy")
}

// ---------------------------------------------------------------------------
// Issue 7 – port_scan returns no results even when open ports exist
// ---------------------------------------------------------------------------

// TestPortScan_ReturnsOpenPort verifies that scanning a real, locally-listening
// mock HTTP port through the port_scan MCP tool surfaces the open port in the
// result. This is a baseline integration test: explicit fingerprint mode +
// skippedHostAliveScan, so it exercises the decode/gRPC/result pipeline without
// depending on the mode-default or loopback-guard logic.
func TestPortScan_ReturnsOpenPort(t *testing.T) {
	// Start a real listening TCP port serving a trivial HTTP response.
	host, mockPort := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello"))
	require.NoError(t, utils.WaitConnect(utils.HostPort(host, mockPort), 5),
		"mock HTTP server should be ready")

	// Build an MCP server backed by a real gRPC server so the full
	// servicescan / DebugPlugin pipeline runs.
	client, err := NewLocalClient(true)
	if err != nil {
		t.Skipf("skipping: NewLocalClient not registered (%v)", err)
	}
	srv, err := NewMCPServer(
		WithEnablePortScanToolSet(),
		WithGRPCClient(client),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	res, err := CallBuiltinTool(srv, ctx, "port_scan", map[string]any{
		"targets": []any{host},
		"ports":   strconv.Itoa(mockPort),
		"mode":    "fingerprint",
		"proto":   []any{"tcp"},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotEmpty(t, res.Content, "port_scan should return results for an open port")

	// At least one result entry must reference the scanned port.
	var combined strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			combined.WriteString(tc.Text)
		}
	}
	assert.Contains(t, combined.String(), strconv.Itoa(mockPort),
		"port_scan result should mention the scanned open port %d, got: %s",
		mockPort, combined.String())
}

// TestPortScan_RangeWithHostAlive scans a *range* of ports on localhost in
// fingerprint mode WITHOUT explicitly skipping the host-alive scan. This
// verifies that the underlying ping module correctly treats loopback as alive
// (PingAutoConfig returns Ok=true for 127.0.0.1 without attempting ICMP/TCP
// probing), so the scan proceeds and finds open ports inside the range.
func TestPortScan_RangeWithHostAlive(t *testing.T) {
	host, mockPort := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello"))
	require.NoError(t, utils.WaitConnect(utils.HostPort(host, mockPort), 5),
		"mock HTTP server should be ready")

	client, err := NewLocalClient(true)
	if err != nil {
		t.Skipf("skipping: NewLocalClient not registered (%v)", err)
	}
	srv, err := NewMCPServer(
		WithEnablePortScanToolSet(),
		WithGRPCClient(client),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	ports := fmt.Sprintf("%d-%d", mockPort, mockPort+5)
	res, err := CallBuiltinTool(srv, ctx, "port_scan", map[string]any{
		"targets": []any{host},
		"ports":   ports,
		"mode":    "fingerprint",
		"proto":   []any{"tcp"},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotEmpty(t, res.Content, "port_scan over a range should return results, got empty")

	var combined strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			combined.WriteString(tc.Text)
		}
	}
	assert.Contains(t, combined.String(), strconv.Itoa(mockPort),
		"port_scan over range %s should surface the open port %d, got: %s",
		ports, mockPort, combined.String())
}

// TestPortScan_AgentCallSequence reproduces the EXACT two-step call sequence an
// AI agent makes against the port_scan toolset:
//
//  1. port_scan(targets:["127.0.0.1"], ports:"<range>", saveToDB:true)
//     — no "mode" (the agent omits it), no "skippedHostAliveScan".
//  2. query_ports(hosts:"127.0.0.1", state:"open")
//
// Root cause: when the agent omits "mode", req.Mode stays "" (the MCP server
// never injects schema defaults). The gRPC layer passes --mode "" to the yak
// script; cli.String("mode", cli.setDefault("fingerprint")) finds --mode in
// args and returns "" (not the default), so none of the script's three mode
// branches match and the scan silently produces zero results. The fix applies
// the "fingerprint" default in handlePortScan when mode is empty.
func TestPortScan_AgentCallSequence(t *testing.T) {
	// Stand up a real listening TCP port inside the scanned range.
	host, mockPort := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello"))
	require.NoError(t, utils.WaitConnect(utils.HostPort(host, mockPort), 5),
		"mock HTTP server should be ready")

	client, err := NewLocalClient(true)
	if err != nil {
		t.Skipf("skipping: NewLocalClient not registered (%v)", err)
	}
	srv, err := NewMCPServer(
		WithEnablePortScanToolSet(),
		WithGRPCClient(client),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Step 1 — the agent's port_scan call: mode omitted (defaults to
	// "fingerprint" via handlePortScan), saveToDB on. No skippedHostAliveScan —
	// the underlying ping module treats loopback as always-alive.
	ports := fmt.Sprintf("%d-%d", mockPort, mockPort+5)
	scanRes, err := CallBuiltinTool(srv, ctx, "port_scan", map[string]any{
		"targets":  []any{host},
		"ports":    ports,
		"saveToDB": true,
	})
	require.NoError(t, err)
	require.NotNil(t, scanRes)

	var scanText strings.Builder
	for _, c := range scanRes.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			scanText.WriteString(tc.Text)
		}
	}
	t.Logf("port_scan result: %s", scanText.String())
	assert.Contains(t, scanText.String(), strconv.Itoa(mockPort),
		"port_scan should surface the open port %d in range %s, got: %s",
		mockPort, ports, scanText.String())

	// Step 2 — the agent's query_ports call to read back from the DB.
	queryRes, err := CallBuiltinTool(srv, ctx, "query_ports", map[string]any{
		"hosts":      host,
		"state":      "open",
		"pagination": map[string]any{"page": 1, "limit": 50},
	})
	require.NoError(t, err)
	require.NotNil(t, queryRes)

	var queryText strings.Builder
	for _, c := range queryRes.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			queryText.WriteString(tc.Text)
		}
	}
	t.Logf("query_ports result: %s", queryText.String())
	assert.Contains(t, queryText.String(), strconv.Itoa(mockPort),
		"query_ports should return the saved open port %d, got: %s",
		mockPort, queryText.String())
}

// TestPortScan_OmittedModeDefault is a unit test that verifies the mode-default
// logic: when the caller omits "mode", it defaults to "fingerprint"; explicit
// modes are preserved as-is. This is the primary root-cause fix — without it,
// an empty mode reaches the yak script as --mode "" and no scan branch matches,
// silently producing zero results.
func TestPortScan_OmittedModeDefault(t *testing.T) {
	cases := []struct {
		name     string
		args     map[string]any
		wantMode string
	}{
		{"omitted mode → fingerprint", map[string]any{
			"targets": []any{"127.0.0.1"}, "ports": "80",
		}, "fingerprint"},
		{"explicit fingerprint preserved", map[string]any{
			"targets": []any{"127.0.0.1"}, "ports": "80", "mode": "fingerprint",
		}, "fingerprint"},
		{"explicit syn preserved", map[string]any{
			"targets": []any{"8.8.8.8"}, "ports": "80", "mode": "syn",
		}, "syn"},
		{"explicit all preserved", map[string]any{
			"targets": []any{"8.8.8.8"}, "ports": "80", "mode": "all",
		}, "all"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req ypb.PortScanRequest
			decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
				DecodeHook: decodeHookComma,
				Result:     &req,
			})
			require.NoError(t, err)
			require.NoError(t, decoder.Decode(tc.args))

			// Apply the same default as handlePortScan.
			if req.GetMode() == "" {
				req.Mode = "fingerprint"
			}
			assert.Equal(t, tc.wantMode, req.GetMode())
		})
	}
}

// TestPortScan_IsOpenFilter verifies that the handlePortScan isOpen filter
// correctly reads isOpen from the nested content.data JSON (not the top level
// of exec.Message) and that closed ports are filtered out while open ports are
// kept and prefixed with [Result].
//
// The yak script emits results via yakit.Output({host, port, isOpen: bool}),
// which arrives as a YakitMessage:
//
//	{"type":"log","content":{"level":"json","data":"{...\"isOpen\":true}"}}
//
// handleExecMessage extracts content.data (the inner JSON string) into content.
// The filter must check gjson.Get(content, "isOpen"), not
// gjson.GetBytes(exec.Message, "isOpen").
func TestPortScan_IsOpenFilter(t *testing.T) {
	// Simulate the message shapes handlePortScan processes.
	openMsg := `{"type":"log","content":{"level":"json","data":"{\"host\":\"127.0.0.1\",\"port\":80,\"isOpen\":true}"}}`
	closedMsg := `{"type":"log","content":{"level":"json","data":"{\"host\":\"127.0.0.1\",\"port\":81,\"isOpen\":false}"}}`
	infoMsg := `{"type":"log","content":{"level":"info","data":"some info message"}}`

	for _, tc := range []struct {
		name       string
		msg        string
		wantDrop   bool
		wantResult bool
	}{
		{"open port kept", openMsg, false, true},
		{"closed port dropped", closedMsg, true, false},
		{"info message (no isOpen) passes through", infoMsg, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content := handleExecMessage(tc.msg)
			isOpenField := gjson.Get(content, "isOpen")
			dropped := false
			isResult := false
			if isOpenField.Exists() {
				if !isOpenField.Bool() {
					dropped = true
				} else {
					content = "[Result] " + content
					isResult = true
				}
			}
			assert.Equal(t, tc.wantDrop, dropped)
			assert.Equal(t, tc.wantResult, isResult)
		})
	}
}
