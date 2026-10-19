package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func fixtureServer() *server.MCPServer {
	s := server.NewMCPServer("yak-client-fixture", "1.0", server.WithResourceCapabilities(false, false), server.WithPromptCapabilities(false))
	s.AddTool(mcp.NewTool("echo", mcp.WithString("message", mcp.Required())), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(fmt.Sprint(req.Params.Arguments["message"]) + os.Getenv("YAK_MCP_FIXTURE_VALUE")), nil
	})
	s.AddTool(mcp.NewTool("failure"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultError("fixture tool failed"), nil
	})
	s.AddResource(mcp.NewResource("fixture://text", "text"), func(context.Context, mcp.ReadResourceRequest) ([]any, error) {
		return []any{mcp.TextResourceContents{ResourceContents: mcp.ResourceContents{URI: "fixture://text", MIMEType: "text/plain"}, Text: "resource text"}}, nil
	})
	s.AddPrompt(mcp.NewPrompt("greet"), func(context.Context, mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{Description: "greeting"}, nil
	})
	return s
}

func TestMCPStdioHelper(t *testing.T) {
	if os.Getenv("YAK_MCP_FIXTURE_CHILD") != "true" {
		return
	}
	if err := server.NewStdioServer(fixtureServer()).Listen(context.Background(), os.Stdin, os.Stdout); err != nil {
		os.Exit(1)
	}
	// The Go test harness must not print PASS into the JSON-RPC stream.
	os.Exit(0)
}

func TestClientTransports(t *testing.T) {
	for _, transport := range []string{"stdio", "sse", "streamable_http"} {
		t.Run(transport, func(t *testing.T) {
			var endpoint string
			opts := []Option{WithTimeout(2)}
			if transport == "stdio" {
				executable, err := os.Executable()
				require.NoError(t, err)
				// A quoted path containing spaces exercises real argv parsing.
				link := filepath.Join(t.TempDir(), "mcp helper with spaces")
				require.NoError(t, os.Symlink(executable, link))
				endpoint = strconv.Quote(link) + " -test.run=^TestMCPStdioHelper$"
				// The race runtime normally sleeps for one second on process exit;
				// omit that artificial delay when checking graceful stdio shutdown.
				opts = append(opts, WithEnv(map[string]any{"YAK_MCP_FIXTURE_CHILD": "true", "YAK_MCP_FIXTURE_VALUE": "-from-env", "GORACE": "atexit_sleep_ms=0"}))
			} else {
				s := fixtureServer()
				mux := http.NewServeMux()
				var shutdown func(context.Context) error
				if transport == "sse" {
					ss := server.NewSSEServer(s, "")
					ss.RegisterHandlers(mux)
					shutdown = ss.Shutdown
				} else {
					ss := server.NewStreamableHTTPServer(s, "")
					ss.RegisterHandlers(mux)
					shutdown = ss.Shutdown
				}
				httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer fixture" {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					mux.ServeHTTP(w, r)
				}))
				t.Cleanup(func() {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					_ = shutdown(ctx)
					httpServer.Close()
				})
				endpoint = httpServer.URL + "/mcp"
				if transport == "sse" {
					endpoint = httpServer.URL + "/sse"
				}
				opts = append(opts, WithHeaders(map[string]any{"Authorization": "Bearer fixture"}))
			}
			c, err := NewClient(transport, endpoint, opts...)
			require.NoError(t, err)
			t.Cleanup(func() { _ = c.Close() })
			// The constructor's initialization context has already been cancelled.
			require.NoError(t, c.Ping())
			page, err := c.ListTools()
			require.NoError(t, err)
			require.Len(t, page.Tools, 2)
			result, err := c.CallTool("echo", map[string]any{"message": "hello\n引号\""})
			require.NoError(t, err)
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			require.Contains(t, string(encoded), `hello\n引号\"`)
			if transport == "stdio" {
				require.Contains(t, string(encoded), "-from-env")
			}
			result, err = c.CallTool("failure", nil)
			require.ErrorContains(t, err, "isError=true")
			require.True(t, result.IsError)
			resources, err := c.ListResources()
			require.NoError(t, err)
			require.Len(t, resources.Resources, 1)
			resource, err := c.ReadResource("fixture://text")
			require.NoError(t, err)
			require.Len(t, resource.Contents, 1)
			prompts, err := c.ListPrompts()
			require.NoError(t, err)
			require.Len(t, prompts.Prompts, 1)
			prompt, err := c.GetPrompt("greet", nil)
			require.NoError(t, err)
			require.Equal(t, "greeting", prompt.Description)
			require.NoError(t, c.Close())
			require.NoError(t, c.Close())
			require.Error(t, c.Ping())
		})
	}
}

func TestClientRequestTimeoutAndCancellation(t *testing.T) {
	s := fixtureServer()
	s.AddTool(mcp.NewTool("slow"), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(200 * time.Millisecond)
		return mcp.NewToolResultText("late"), nil
	})
	httpServer := server.NewStreamableHTTPTestServer(s)
	defer httpServer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, err := NewClient("streamable_http", httpServer.URL+"/mcp", WithContext(ctx), WithTimeout(0.05))
	require.NoError(t, err)
	defer c.Close()
	_, err = c.CallTool("slow", nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	// A request timeout leaves the connection usable.
	require.NoError(t, c.Ping())
	cancel()
	require.Error(t, c.Ping())
	_, err = NewClient("stdio", "/path/that/does/not/exist")
	require.Error(t, err, "failed construction must clean up without a typed-nil panic")
}

func newRegistryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "profile.db"))
	require.NoError(t, err)
	db.DB().SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&schema.MCPServer{}, &schema.MCPServerToolConfig{}, &schema.MCPClientToolConfig{}).Error)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestRegistrySharedConfiguration(t *testing.T) {
	db := newRegistryDB(t)
	opts := []Option{WithDatabase(db)}
	entry, err := AddServer("a-local", "stdio", `python3 "/tmp/test server.py" --token secret-command`, append(opts,
		WithEnv(map[string]any{"TOKEN": "secret-env"}), WithEnabled(false))...)
	require.NoError(t, err)
	require.False(t, entry.Enable)
	stored, err := yakit.GetMCPServerByName(db, entry.Name)
	require.NoError(t, err)
	require.False(t, stored.Enable)
	require.Equal(t, "secret-env", stored.Envs["TOKEN"])
	_, err = Connect(entry.Name, opts...)
	require.ErrorContains(t, err, "disabled")
	_, err = AddServer(entry.Name, "stdio", "changed-command", opts...)
	require.ErrorContains(t, err, "already exists")
	_, err = AddServer("b-http", "streamable_http", "https://user:pass@example.test/mcp?token=secret-query#secret-fragment", append(opts, WithHeaders(map[string]any{"Authorization": "secret-header"}))...)
	require.NoError(t, err)
	page, err := ListServers("", append(opts, WithLimit(1))...)
	require.NoError(t, err)
	require.Equal(t, 2, page.Total)
	require.True(t, page.HasMore)
	require.Equal(t, 1, page.NextOffset)
	require.Equal(t, "a-local", page.Servers[0].Name)
	page, err = ListServers("", append(opts, WithLimit(1), WithOffset(page.NextOffset))...)
	require.NoError(t, err)
	require.False(t, page.HasMore)
	require.Equal(t, "https://example.test/mcp", page.Servers[0].URL)
	all, err := ListServers("", opts...)
	require.NoError(t, err)
	raw, err := json.Marshal(all)
	require.NoError(t, err)
	for _, secret := range []string{"secret-env", "secret-command", "secret-header", "secret-query", "secret-fragment", "user:pass"} {
		require.NotContains(t, string(raw), secret)
	}
	require.NoError(t, yakit.UpsertMCPServerToolMetadata(db, entry.Name, "echo", "description", "[]"))
	_, err = yakit.GetOrCreateMCPClientToolConfig(db, "bridge-fixture", schema.MCPClientToolSourceBridge, entry.Name, "description")
	require.NoError(t, err)
	require.NoError(t, DeleteServer(entry.Name, opts...))
	var count int
	require.NoError(t, db.Model(&schema.MCPServerToolConfig{}).Where("server_name = ?", entry.Name).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Model(&schema.MCPClientToolConfig{}).Where("server_name = ?", entry.Name).Count(&count).Error)
	require.Zero(t, count)
	require.Error(t, DeleteServer(entry.Name, opts...))
	_, err = ListServers("", append(opts, WithOffset(-1))...)
	require.Error(t, err)
	_, err = ListServers("", append(opts, WithLimit(101))...)
	require.Error(t, err)
	for _, tc := range []struct{ transport, endpoint string }{{"unknown", "value"}, {"stdio", "\"unclosed"}, {"sse", "file:///tmp/x"}} {
		_, err := AddServer("invalid", tc.transport, tc.endpoint, opts...)
		require.Error(t, err)
	}
	_, err = AddServer("invalid-env", "stdio", "python3", append(opts, WithEnv(map[string]any{"TOKEN": 123}))...)
	require.Error(t, err)
}

func TestConnectHonorsPerToolEnableFlags(t *testing.T) {
	db := newRegistryDB(t)
	httpServer := server.NewStreamableHTTPTestServer(fixtureServer())
	defer httpServer.Close()
	_, err := AddServer("stored", "streamable_http", httpServer.URL+"/mcp", WithDatabase(db))
	require.NoError(t, err)
	require.NoError(t, yakit.UpsertMCPServerToolConfig(db, "stored", "echo", false))
	c, err := Connect("stored", WithDatabase(db))
	require.NoError(t, err)
	defer c.Close()
	tools, err := c.ListTools()
	require.NoError(t, err)
	require.Len(t, tools.Tools, 1)
	require.Equal(t, "failure", tools.Tools[0].Name)
	_, err = c.CallTool("echo", nil)
	require.ErrorContains(t, err, "disabled")
	require.NoError(t, yakit.UpsertMCPServerToolConfig(db, "stored", "echo", true))
	_, err = c.CallTool("echo", map[string]any{"message": "enabled"})
	require.NoError(t, err)
}

func TestClientPassesOpaquePaginationCursor(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID     any            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		if strings.HasPrefix(req.Method, "notifications/") {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": mcp.LATEST_PROTOCOL_VERSION, "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "pages", "version": "1"}}
		case "tools/list":
			if req.Params["cursor"] == nil {
				result = map[string]any{"tools": []*mcp.Tool{mcp.NewTool("first")}, "nextCursor": "opaque+/=token"}
			} else {
				require.Equal(t, "opaque+/=token", req.Params["cursor"])
				result = map[string]any{"tools": []*mcp.Tool{mcp.NewTool("second")}}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	defer httpServer.Close()
	c, err := NewClient("streamable_http", httpServer.URL)
	require.NoError(t, err)
	defer c.Close()
	first, err := c.ListTools()
	require.NoError(t, err)
	require.Equal(t, mcp.Cursor("opaque+/=token"), first.NextCursor)
	second, err := c.ListTools(string(first.NextCursor))
	require.NoError(t, err)
	require.Empty(t, second.NextCursor)
	require.Equal(t, "second", second.Tools[0].Name)
}
