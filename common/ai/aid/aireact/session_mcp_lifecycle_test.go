package aireact

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/schema"
)

const sessionMCPStdioClosedEnv = "YAK_TEST_SESSION_MCP_STDIO_CLOSED"

type existingSessionMCPClient struct {
	closeCalls atomic.Int32
}

func (c *existingSessionMCPClient) Close() error {
	c.closeCalls.Add(1)
	return nil
}

func newLifecycleMCPServer() *server.MCPServer {
	mcpServer := server.NewMCPServer("session-lifecycle", "1.0.0")
	for _, name := range []string{"first", "second"} {
		mcpServer.AddTool(mcp.NewTool(name), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []any{mcp.TextContent{Type: "text", Text: "ok"}}}, nil
		})
	}
	return mcpServer
}

// TestSessionMCPStdioHelper runs only in the child test process. The marker is
// written after the server observes stdin EOF, not merely a client-side flag.
func TestSessionMCPStdioHelper(t *testing.T) {
	closedPath := os.Getenv(sessionMCPStdioClosedEnv)
	if closedPath == "" {
		return
	}
	require.NoError(t, server.NewStdioServer(newLifecycleMCPServer()).Listen(context.Background(), os.Stdin, os.Stdout))
	require.NoError(t, os.WriteFile(closedPath, []byte("closed"), 0o600))
}

func newSessionMCPLifecycleFixture(t *testing.T, transport string) (*schema.MCPServer, func()) {
	t.Helper()
	config := &schema.MCPServer{Name: "session-lifecycle", Type: transport, Enable: true}
	if transport == "stdio" {
		executable, err := os.Executable()
		require.NoError(t, err)
		if strings.ContainsAny(executable, " \t\r\n") {
			t.Skip("stdio command parser requires an executable path without whitespace")
		}
		closedPath := filepath.Join(t.TempDir(), "stdio-closed")
		t.Setenv(sessionMCPStdioClosedEnv, closedPath)
		config.Command = executable + " -test.run=^TestSessionMCPStdioHelper$"
		return config, func() {
			t.Helper()
			require.Eventually(t, func() bool {
				_, err := os.Stat(closedPath)
				return err == nil
			}, 5*time.Second, 10*time.Millisecond, "stdio server did not receive EOF")
		}
	}

	disconnected := make(chan struct{})
	sseServer := server.NewSSEServer(newLifecycleMCPServer(), "")
	mux := http.NewServeMux()
	sseServer.RegisterHandlers(mux)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sse" {
			defer close(disconnected)
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = sseServer.Shutdown(ctx)
		httpServer.CloseClientConnections()
		httpServer.Close()
	})
	config.URL = httpServer.URL + "/sse"
	return config, func() {
		t.Helper()
		select {
		case <-disconnected:
		case <-time.After(5 * time.Second):
			t.Fatal("SSE request stayed open after client cleanup")
		}
	}
}

func sessionMCPLifecycleOptions(t *testing.T, ctx context.Context, servers ...*schema.MCPServer) []aicommon.ConfigOption {
	t.Helper()
	extra := make([]*aicommon.ExtraMCPServer, 0, len(servers))
	for _, config := range servers {
		extra = append(extra, &aicommon.ExtraMCPServer{Server: config})
	}
	return []aicommon.ConfigOption{
		aicommon.WithContext(ctx),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithDisallowMCPServers(false),
		aicommon.WithRestrictToolsToExtraMCPServers(true),
		aicommon.WithExtraMCPServers(extra...),
		aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, errors.New("unexpected AI invocation in MCP lifecycle test")
		}),
	}
}

func TestNewReActRequiredMCPFailureClosesMountedClients(t *testing.T) {
	for _, transport := range []string{"sse", "stdio"} {
		t.Run(transport, func(t *testing.T) {
			config, waitClosed := newSessionMCPLifecycleFixture(t, transport)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			var loadedConfig *aicommon.Config
			opts := sessionMCPLifecycleOptions(t, ctx, config,
				&schema.MCPServer{Name: "unavailable", Type: "unsupported"},
			)
			opts = append(opts, func(cfg *aicommon.Config) error {
				loadedConfig = cfg
				return nil
			})

			react, err := NewTestReAct(opts...)
			require.Nil(t, react)
			require.ErrorContains(t, err, "required session-scoped MCP capabilities are unavailable")
			// Both tools share one client. Loading must have succeeded before the
			// later required server failed, and cleanup must not double-close it.
			for _, name := range []string{"first", "second"} {
				tool, err := loadedConfig.GetAiToolManager().GetToolByName("mcp_session-lifecycle_" + name)
				require.NoError(t, err)
				require.NotNil(t, tool.BridgeMCPClient)
			}
			waitClosed()
			require.NoError(t, ctx.Err(), "construction rollback must not cancel the caller context")
		})
	}
}

func TestNewReActRequiredMCPSuccessKeepsClientsUntilSessionEnds(t *testing.T) {
	for _, transport := range []string{"sse", "stdio"} {
		t.Run(transport, func(t *testing.T) {
			config, waitClosed := newSessionMCPLifecycleFixture(t, transport)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			react, err := NewTestReAct(sessionMCPLifecycleOptions(t, ctx, config)...)
			require.NoError(t, err)
			var tools []*aitool.Tool
			for _, name := range []string{"first", "second"} {
				tool, err := react.config.GetAiToolManager().GetToolByName("mcp_session-lifecycle_" + name)
				require.NoError(t, err)
				result, err := tool.Callback(ctx, aitool.InvokeParams{}, nil, io.Discard, io.Discard)
				require.NoError(t, err, "constructor and initialization cleanup must leave successful clients usable")
				require.Equal(t, "ok", result)
				tools = append(tools, tool)
			}
			cancel()
			waitClosed()
			for _, tool := range tools {
				require.NoError(t, tool.BridgeMCPClient.Close(), "shared-client cleanup must be idempotent")
			}
		})
	}
}

func TestNewReActRequiredMCPFailurePreservesExistingClients(t *testing.T) {
	config, waitClosed := newSessionMCPLifecycleFixture(t, "sse")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	existingClient := &existingSessionMCPClient{}
	var existingTools []*aitool.Tool
	for _, name := range []string{"first", "second"} {
		tool := aitool.NewWithoutCallback("mcp_session-lifecycle_" + name)
		tool.BridgeMCPClient = existingClient
		existingTools = append(existingTools, tool)
	}
	opts := sessionMCPLifecycleOptions(t, ctx, config,
		&schema.MCPServer{Name: "unavailable", Type: "unsupported"},
	)
	opts = append(opts, aicommon.WithAiToolManagerOptions(buildinaitools.WithExtendTools(existingTools)))
	react, err := NewTestReAct(opts...)
	require.Nil(t, react)
	require.ErrorContains(t, err, "required session-scoped MCP capabilities are unavailable")
	// All newly loaded names already exist, so their new client is not reachable
	// through the tool manager. Rollback must still close it without closing the
	// caller's existing client that owns those names.
	waitClosed()
	require.Zero(t, existingClient.closeCalls.Load())
	require.NoError(t, ctx.Err())
}

func newCallerMCPLifecycleManager(t *testing.T) (*buildinaitools.AiToolManager, *aitool.Tool) {
	t.Helper()
	allowed, err := aitool.New("caller-allowed", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
		return "caller-tool-ok", nil
	}))
	require.NoError(t, err)
	denied := aitool.NewWithoutCallback("caller-denied")
	manager := buildinaitools.NewToolManagerByToolGetter(func() []*aitool.Tool {
		return []*aitool.Tool{allowed, denied}
	})
	manager.RestrictToTools(allowed.Name)
	manager.DisableTool(denied.Name)
	manager.SetDisallowMCPServers(true)
	return manager, allowed
}

func TestNewReActExtraMCPFailureAllowsCallerManagerRetry(t *testing.T) {
	for _, managerFirst := range []bool{false, true} {
		name := "manager_last"
		if managerFirst {
			name = "manager_first"
		}
		t.Run(name, func(t *testing.T) {
			testNewReActExtraMCPFailureAllowsCallerManagerRetry(t, managerFirst)
		})
	}
}

func testNewReActExtraMCPFailureAllowsCallerManagerRetry(t *testing.T, managerFirst bool) {
	manager, callerTool := newCallerMCPLifecycleManager(t)
	withManager := func(opts []aicommon.ConfigOption) []aicommon.ConfigOption {
		if managerFirst {
			return append([]aicommon.ConfigOption{aicommon.WithAiToolManager(manager)}, opts...)
		}
		return append(opts, aicommon.WithAiToolManager(manager))
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	firstConfig, firstClosed := newSessionMCPLifecycleFixture(t, "sse")
	firstOpts := sessionMCPLifecycleOptions(t, ctx, firstConfig,
		&schema.MCPServer{Name: "recovered-lifecycle", Type: "unsupported"},
	)
	firstOpts = withManager(firstOpts)
	failed, err := NewTestReAct(firstOpts...)
	require.Nil(t, failed)
	require.ErrorContains(t, err, "required session-scoped MCP capabilities are unavailable")
	firstClosed()

	visible, err := manager.GetEnableTools()
	require.NoError(t, err)
	var visibleNames []string
	for _, tool := range visible {
		visibleNames = append(visibleNames, tool.Name)
	}
	assert.Equal(t, []string{callerTool.Name}, visibleNames, "failed construction must not append session tools or replace the caller's restriction")
	assert.True(t, manager.DisallowMCPServers(), "NewConfig must apply session policy to a private manager, not the caller")

	// Retry with the exact same caller manager after all required MCP servers
	// become available. The original successful names must resolve to live new
	// callbacks, not the closed clients from the failed construction.
	retryConfig, retryClosed := newSessionMCPLifecycleFixture(t, "sse")
	recoveredConfig, recoveredClosed := newSessionMCPLifecycleFixture(t, "sse")
	recoveredConfig.Name = "recovered-lifecycle"
	retryOpts := sessionMCPLifecycleOptions(t, ctx, retryConfig, recoveredConfig)
	retryOpts = withManager(retryOpts)
	react, err := NewTestReAct(retryOpts...)
	require.NoError(t, err)
	for _, serverName := range []string{retryConfig.Name, recoveredConfig.Name} {
		for _, name := range []string{"first", "second"} {
			tool, err := react.config.GetAiToolManager().GetToolByName("mcp_" + serverName + "_" + name)
			require.NoError(t, err)
			result, err := tool.Callback(ctx, aitool.InvokeParams{}, nil, io.Discard, io.Discard)
			require.NoError(t, err, "same-manager retry must use a live client")
			require.Equal(t, "ok", result)
		}
	}
	require.NotSame(t, manager, react.config.GetAiToolManager())
	_, err = react.config.GetAiToolManager().GetToolByName(callerTool.Name)
	require.ErrorContains(t, err, "outside the restricted tool set")

	visible, err = manager.GetEnableTools()
	require.NoError(t, err)
	require.Len(t, visible, 1, "successful sessions must keep private tools and restrictions out of the caller")
	require.Same(t, callerTool, visible[0])
	require.True(t, manager.DisallowMCPServers())
	_, err = manager.GetToolByName("caller-denied")
	require.ErrorContains(t, err, "outside the restricted tool set")
	result, err := callerTool.Callback(ctx, aitool.InvokeParams{}, nil, io.Discard, io.Discard)
	require.NoError(t, err)
	require.Equal(t, "caller-tool-ok", result)

	cancel()
	retryClosed()
	recoveredClosed()
}

func TestNewReActWithoutExtraMCPKeepsCallerManager(t *testing.T) {
	manager, callerTool := newCallerMCPLifecycleManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	opts := sessionMCPLifecycleOptions(t, ctx)
	opts = append(opts,
		aicommon.WithAiToolManager(manager),
		aicommon.WithRestrictToolsToExtraMCPServers(false),
		aicommon.WithDisallowMCPServers(true),
	)
	react, err := NewTestReAct(opts...)
	require.NoError(t, err)
	require.Same(t, manager, react.config.GetAiToolManager(), "ordinary ReAct construction must retain caller-owned manager sharing")
	tool, err := react.config.GetAiToolManager().GetToolByName(callerTool.Name)
	require.NoError(t, err)
	require.Same(t, callerTool, tool)
	result, err := tool.Callback(ctx, aitool.InvokeParams{}, nil, io.Discard, io.Discard)
	require.NoError(t, err)
	require.Equal(t, "caller-tool-ok", result)
}
