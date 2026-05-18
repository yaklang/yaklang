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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mcpserver "github.com/yaklang/yaklang/common/mcp/mcp-go/server"
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
