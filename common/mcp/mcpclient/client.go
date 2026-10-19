package mcpclient

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/client"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils/shlex"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// Client is an initialized MCP connection. Each request has its own timeout;
// expiry of an initialization/request timeout does not end the connection.
// Call Close when finished, or bind its lifetime with mcp.context(ctx).
type Client struct {
	client     client.MCPClient
	ctx        context.Context
	cancel     context.CancelFunc
	timeout    time.Duration
	closeOnce  sync.Once
	closeErr   error
	db         *gorm.DB
	serverName string
}

// NewClient connects to stdio, sse or streamable_http and performs initialize.
// For stdio, endpoint is a quoted command line, parsed without invoking a shell.
//
// Example:
// ```
// c = mcp.NewClient("streamable_http", "http://127.0.0.1:8080/mcp", mcp.timeout(10))~
// defer c.Close()
// page = c.ListTools()~
// dump(page.Tools)
// result = c.CallTool("echo", {"message": "hello"})~
// dump(result.Content)
// ```
func NewClient(transport, endpoint string, opts ...Option) (*Client, error) {
	o, err := newOptions(opts...)
	if err != nil {
		return nil, err
	}
	if err := validateEndpoint(transport, endpoint); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(o.ctx)
	c := &Client{ctx: ctx, cancel: cancel, timeout: o.timeout}
	initialized := false
	defer func() {
		if !initialized {
			_ = c.Close()
		}
	}()
	initCtx, initCancel := context.WithTimeout(ctx, o.timeout)
	defer initCancel()
	switch transport {
	case "stdio":
		parts, _ := shlex.Split(endpoint) // validated above
		env := make([]string, 0, len(o.env))
		for k, v := range o.env {
			env = append(env, k+"="+v.(string))
		}
		var stdio *client.StdioMCPClient
		stdio, err = client.NewStdioMCPClientWithContext(ctx, parts[0], env, parts[1:]...)
		if err == nil {
			c.client = stdio
		}
	case "sse":
		var sse *client.SSEMCPClient
		sse, err = client.NewSSEMCPClient(endpoint, stringMap(o.headers))
		if err == nil {
			c.client = sse
			// Start uses the connection lifetime. The timer only bounds startup;
			// cancelling initCtx after success must leave the SSE stream alive.
			timer := time.AfterFunc(o.timeout, cancel)
			err = sse.Start(ctx)
			timer.Stop()
		}
	case "streamable_http":
		var httpClient *client.StreamableHTTPMCPClient
		httpClient, err = client.NewStreamableHTTPMCPClient(endpoint, stringMap(o.headers))
		if err == nil {
			c.client = httpClient
		}
	}
	if err != nil {
		return nil, fmt.Errorf("connect MCP client: %w", err)
	}
	req := mcp.InitializeRequest{}
	req.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	req.Params.ClientInfo = mcp.Implementation{Name: "yaklang-mcp-client", Version: consts.GetYakVersion()}
	if _, err := c.client.Initialize(initCtx, req); err != nil {
		return nil, fmt.Errorf("initialize MCP client: %w", err)
	}
	initialized = true
	context.AfterFunc(ctx, func() { _ = c.Close() })
	return c, nil
}

func validateEndpoint(transport, endpoint string) error {
	switch transport {
	case "stdio":
		parts, err := shlex.Split(endpoint)
		if err != nil || len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
			return fmt.Errorf("MCP stdio command must contain an executable and balanced quotes")
		}
	case "sse", "streamable_http":
		u, err := url.Parse(endpoint)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("MCP URL must be an absolute http or https URL")
		}
	default:
		return fmt.Errorf("unsupported MCP server type %q; use stdio, sse or streamable_http", transport)
	}
	return nil
}

// Connect opens an enabled server from the same profile registry used by Yakit.
func Connect(name string, opts ...Option) (*Client, error) {
	o, err := registryOptions(opts...)
	if err != nil {
		return nil, err
	}
	srv, err := yakit.GetMCPServerByName(o.db, name)
	if err != nil {
		return nil, err
	}
	if !srv.Enable {
		return nil, fmt.Errorf("MCP server %q is disabled", name)
	}
	endpoint := srv.URL
	if srv.Type == "stdio" {
		endpoint = srv.Command
	}
	defaults := []Option{WithEnv(map[string]any(srv.Envs)), WithHeaders(map[string]any(srv.Headers))}
	c, err := NewClient(srv.Type, endpoint, append(defaults, opts...)...)
	if err != nil {
		return nil, err
	}
	c.db, c.serverName = o.db, srv.Name
	return c, nil
}

// Close is idempotent and cancels outstanding requests and idle SSE streams.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		// Give stdio servers a chance to exit on stdin EOF, then interrupt a
		// child that ignores it. The subprocess is bound to c.ctx.
		timer := time.AfterFunc(time.Second, c.cancel)
		if c.client != nil {
			c.closeErr = c.client.Close()
		}
		timer.Stop()
		c.cancel()
	})
	return c.closeErr
}

func (c *Client) requestContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.ctx, c.timeout)
}

// ListTools returns one remote page, including NextCursor. Pass that cursor to
// the next call until it is empty; do not assume the first page is complete.
func (c *Client) ListTools(cursor ...string) (*mcp.ListToolsResult, error) {
	ctx, cancel := c.requestContext()
	defer cancel()
	req := mcp.ListToolsRequest{}
	if len(cursor) > 0 {
		req.Params.Cursor = mcp.Cursor(cursor[0])
	}
	result, err := c.client.ListTools(ctx, req)
	if err != nil || c.db == nil {
		return result, err
	}
	configs, err := yakit.BatchGetMCPServerToolConfigs(c.db, c.serverName)
	if err != nil {
		return nil, err
	}
	tools := make([]*mcp.Tool, 0, len(result.Tools))
	for _, tool := range result.Tools {
		if cfg := configs[tool.Name]; cfg == nil || cfg.Enable {
			tools = append(tools, tool)
		}
	}
	result.Tools = tools
	return result, nil
}

// CallTool preserves structured MCP content and IsError. A remote tool error
// is also returned as a Yak error, so the ~ operator cannot mistake it for success.
func (c *Client) CallTool(name string, arguments map[string]any) (*mcp.CallToolResult, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("MCP tool name is required")
	}
	if c.db != nil {
		cfg, err := yakit.GetMCPServerToolConfig(c.db, c.serverName, name)
		if err != nil {
			return nil, err
		}
		if !cfg.Enable {
			return nil, fmt.Errorf("MCP tool %q is disabled for server %q", name, c.serverName)
		}
	}
	ctx, cancel := c.requestContext()
	defer cancel()
	req := mcp.CallToolRequest{}
	req.Params.Name, req.Params.Arguments = name, arguments
	result, err := c.client.CallTool(ctx, req)
	if err == nil && result != nil && result.IsError {
		return result, fmt.Errorf("MCP tool %q returned isError=true", name)
	}
	return result, err
}

func (c *Client) Ping() error {
	ctx, cancel := c.requestContext()
	defer cancel()
	return c.client.Ping(ctx)
}

func (c *Client) ListResources(cursor ...string) (*mcp.ListResourcesResult, error) {
	ctx, cancel := c.requestContext()
	defer cancel()
	req := mcp.ListResourcesRequest{}
	if len(cursor) > 0 {
		req.Params.Cursor = mcp.Cursor(cursor[0])
	}
	return c.client.ListResources(ctx, req)
}

func (c *Client) ReadResource(uri string) (*mcp.ReadResourceResult, error) {
	ctx, cancel := c.requestContext()
	defer cancel()
	req := mcp.ReadResourceRequest{}
	req.Params.URI = uri
	return c.client.ReadResource(ctx, req)
}

func (c *Client) ListPrompts(cursor ...string) (*mcp.ListPromptsResult, error) {
	ctx, cancel := c.requestContext()
	defer cancel()
	req := mcp.ListPromptsRequest{}
	if len(cursor) > 0 {
		req.Params.Cursor = mcp.Cursor(cursor[0])
	}
	return c.client.ListPrompts(ctx, req)
}

func (c *Client) GetPrompt(name string, arguments map[string]string) (*mcp.GetPromptResult, error) {
	ctx, cancel := c.requestContext()
	defer cancel()
	req := mcp.GetPromptRequest{}
	req.Params.Name, req.Params.Arguments = name, arguments
	return c.client.GetPrompt(ctx, req)
}
