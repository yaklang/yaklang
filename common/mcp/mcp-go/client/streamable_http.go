package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
)

type StreamableHTTPMCPClient struct {
	endpoint        *url.URL
	httpClient      *http.Client
	headers         map[string]string
	requestID       atomic.Int64
	initialized     bool
	protocolVersion string
	sessionID       string

	notifications []func(mcp.JSONRPCNotification)
	notifyMu      sync.RWMutex

	streamMu          sync.Mutex
	eventStreamCancel context.CancelFunc
	done              chan struct{}
}

func NewStreamableHTTPMCPClient(
	endpoint string,
	headers ...map[string]string,
) (*StreamableHTTPMCPClient, error) {
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	customHeaders := make(map[string]string)
	if len(headers) > 0 {
		for k, v := range headers[0] {
			customHeaders[k] = v
		}
	}

	return &StreamableHTTPMCPClient{
		endpoint: parsedURL,
		httpClient: &http.Client{
			Transport: &http.Transport{
				ResponseHeaderTimeout: 60 * time.Minute,
				IdleConnTimeout:       60 * time.Minute,
				TLSHandshakeTimeout:   10 * time.Second,
			},
			Timeout: 60 * time.Minute,
		},
		headers: customHeaders,
		done:    make(chan struct{}),
	}, nil
}

func (c *StreamableHTTPMCPClient) OnNotification(
	handler func(notification mcp.JSONRPCNotification),
) {
	c.notifyMu.Lock()
	defer c.notifyMu.Unlock()
	c.notifications = append(c.notifications, handler)
}

func (c *StreamableHTTPMCPClient) applyHeaders(req *http.Request) {
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}
	if c.sessionID != "" {
		req.Header.Set(mcp.HeaderSessionID, c.sessionID)
	}
	if c.protocolVersion != "" {
		req.Header.Set(mcp.HeaderProtocolVersion, c.protocolVersion)
	}
}

func (c *StreamableHTTPMCPClient) sendRequest(
	ctx context.Context,
	method string,
	params interface{},
) (*json.RawMessage, error) {
	if !c.initialized && method != "initialize" {
		return nil, fmt.Errorf("client not initialized")
	}

	id := c.requestID.Add(1)
	request := mcp.JSONRPCRequest{
		JSONRPC: mcp.JSONRPC_VERSION,
		ID:      id,
		Request: mcp.Request{
			Method: method,
		},
		Params: params,
	}

	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.endpoint.String(),
		bytes.NewReader(requestBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	c.applyHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	c.updateSessionFromHeaders(resp.Header)

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusAccepted:
		return nil, fmt.Errorf("request accepted without response")
	default:
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf(
			"request failed with status %d: %s",
			resp.StatusCode,
			body,
		)
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") {
		return c.readResponseFromSSE(resp.Body, id)
	}

	return decodeJSONRPCResponseBody(resp.Body)
}

func (c *StreamableHTTPMCPClient) sendNotification(
	ctx context.Context,
	method string,
	params interface{},
) error {
	notification := mcp.JSONRPCNotification{
		JSONRPC: mcp.JSONRPC_VERSION,
		Notification: mcp.Notification{
			Method: method,
		},
	}
	if params != nil {
		notification.Params = mcp.NotificationParams{
			AdditionalFields: map[string]interface{}{},
		}
		if p, ok := params.(map[string]interface{}); ok {
			notification.Params.AdditionalFields = p
		}
	}

	requestBytes, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.endpoint.String(),
		bytes.NewReader(requestBytes),
	)
	if err != nil {
		return fmt.Errorf("failed to create notification request: %w", err)
	}

	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	c.applyHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send notification: %w", err)
	}
	defer resp.Body.Close()

	c.updateSessionFromHeaders(resp.Header)

	if resp.StatusCode != http.StatusAccepted &&
		resp.StatusCode != http.StatusOK &&
		resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf(
			"notification failed with status %d: %s",
			resp.StatusCode,
			body,
		)
	}
	return nil
}

func (c *StreamableHTTPMCPClient) Initialize(
	ctx context.Context,
	request mcp.InitializeRequest,
) (*mcp.InitializeResult, error) {
	params := struct {
		ProtocolVersion string                 `json:"protocolVersion"`
		ClientInfo      mcp.Implementation     `json:"clientInfo"`
		Capabilities    mcp.ClientCapabilities `json:"capabilities"`
	}{
		ProtocolVersion: request.Params.ProtocolVersion,
		ClientInfo:      request.Params.ClientInfo,
		Capabilities:    request.Params.Capabilities,
	}

	response, err := c.sendRequest(ctx, "initialize", params)
	if err != nil {
		return nil, err
	}

	var result mcp.InitializeResult
	if err := json.Unmarshal(*response, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.protocolVersion = result.ProtocolVersion
	c.initialized = true

	if err := c.sendNotification(ctx, "notifications/initialized", nil); err != nil {
		return nil, fmt.Errorf("failed to send initialized notification: %w", err)
	}

	c.ensureEventStream()
	return &result, nil
}

func (c *StreamableHTTPMCPClient) Ping(ctx context.Context) error {
	_, err := c.sendRequest(ctx, "ping", nil)
	return err
}

func (c *StreamableHTTPMCPClient) ListResources(
	ctx context.Context,
	request mcp.ListResourcesRequest,
) (*mcp.ListResourcesResult, error) {
	response, err := c.sendRequest(ctx, "resources/list", request.Params)
	if err != nil {
		return nil, err
	}

	var result mcp.ListResourcesResult
	if err := json.Unmarshal(*response, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &result, nil
}

func (c *StreamableHTTPMCPClient) ListResourceTemplates(
	ctx context.Context,
	request mcp.ListResourceTemplatesRequest,
) (*mcp.ListResourceTemplatesResult, error) {
	response, err := c.sendRequest(
		ctx,
		"resources/templates/list",
		request.Params,
	)
	if err != nil {
		return nil, err
	}

	var result mcp.ListResourceTemplatesResult
	if err := json.Unmarshal(*response, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &result, nil
}

func (c *StreamableHTTPMCPClient) ReadResource(
	ctx context.Context,
	request mcp.ReadResourceRequest,
) (*mcp.ReadResourceResult, error) {
	response, err := c.sendRequest(ctx, "resources/read", request.Params)
	if err != nil {
		return nil, err
	}

	var result mcp.ReadResourceResult
	if err := json.Unmarshal(*response, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &result, nil
}

func (c *StreamableHTTPMCPClient) Subscribe(
	ctx context.Context,
	request mcp.SubscribeRequest,
) error {
	_, err := c.sendRequest(ctx, "resources/subscribe", request.Params)
	return err
}

func (c *StreamableHTTPMCPClient) Unsubscribe(
	ctx context.Context,
	request mcp.UnsubscribeRequest,
) error {
	_, err := c.sendRequest(ctx, "resources/unsubscribe", request.Params)
	return err
}

func (c *StreamableHTTPMCPClient) ListPrompts(
	ctx context.Context,
	request mcp.ListPromptsRequest,
) (*mcp.ListPromptsResult, error) {
	response, err := c.sendRequest(ctx, "prompts/list", request.Params)
	if err != nil {
		return nil, err
	}

	var result mcp.ListPromptsResult
	if err := json.Unmarshal(*response, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &result, nil
}

func (c *StreamableHTTPMCPClient) GetPrompt(
	ctx context.Context,
	request mcp.GetPromptRequest,
) (*mcp.GetPromptResult, error) {
	response, err := c.sendRequest(ctx, "prompts/get", request.Params)
	if err != nil {
		return nil, err
	}

	var result mcp.GetPromptResult
	if err := json.Unmarshal(*response, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &result, nil
}

func (c *StreamableHTTPMCPClient) ListTools(
	ctx context.Context,
	request mcp.ListToolsRequest,
) (*mcp.ListToolsResult, error) {
	response, err := c.sendRequest(ctx, "tools/list", request.Params)
	if err != nil {
		return nil, err
	}

	var result mcp.ListToolsResult
	if err := json.Unmarshal(*response, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &result, nil
}

func (c *StreamableHTTPMCPClient) CallTool(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	response, err := c.sendRequest(ctx, "tools/call", request.Params)
	if err != nil {
		return nil, err
	}

	var result mcp.CallToolResult
	if err := json.Unmarshal(*response, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &result, nil
}

func (c *StreamableHTTPMCPClient) SetLevel(
	ctx context.Context,
	request mcp.SetLevelRequest,
) error {
	_, err := c.sendRequest(ctx, "logging/setLevel", request.Params)
	return err
}

func (c *StreamableHTTPMCPClient) Complete(
	ctx context.Context,
	request mcp.CompleteRequest,
) (*mcp.CompleteResult, error) {
	response, err := c.sendRequest(ctx, "completion/complete", request.Params)
	if err != nil {
		return nil, err
	}

	var result mcp.CompleteResult
	if err := json.Unmarshal(*response, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &result, nil
}

func (c *StreamableHTTPMCPClient) Close() error {
	select {
	case <-c.done:
		return nil
	default:
		close(c.done)
	}

	c.streamMu.Lock()
	cancel := c.eventStreamCancel
	c.eventStreamCancel = nil
	c.streamMu.Unlock()
	if cancel != nil {
		cancel()
	}

	if c.sessionID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodDelete,
			c.endpoint.String(),
			nil,
		)
		if err == nil {
			req.Header.Set("Accept", "application/json, text/event-stream")
			c.applyHeaders(req)
			resp, err := c.httpClient.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}
	}

	return nil
}

func (c *StreamableHTTPMCPClient) ensureEventStream() {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()

	if c.sessionID == "" || c.eventStreamCancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.eventStreamCancel = cancel

	go c.readEventStream(ctx)
}

func (c *StreamableHTTPMCPClient) readEventStream(ctx context.Context) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.endpoint.String(),
		nil,
	)
	if err != nil {
		return
	}

	req.Header.Set("Accept", "text/event-stream")
	c.applyHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Debugf("streamable http event stream connect failed: %v", err)
		return
	}
	defer resp.Body.Close()

	c.updateSessionFromHeaders(resp.Header)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Debugf(
			"streamable http event stream failed with status %d: %s",
			resp.StatusCode,
			body,
		)
		return
	}

	_ = consumeSSE(resp.Body, func(payload json.RawMessage) (bool, error) {
		c.dispatchSSEPayload(payload)
		return false, nil
	})
}

func (c *StreamableHTTPMCPClient) readResponseFromSSE(
	reader io.Reader,
	expectedID int64,
) (*json.RawMessage, error) {
	var result *json.RawMessage

	err := consumeSSE(reader, func(payload json.RawMessage) (bool, error) {
		var base struct {
			ID     *int64          `json:"id,omitempty"`
			Method string          `json:"method,omitempty"`
			Result json.RawMessage `json:"result,omitempty"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal(payload, &base); err != nil {
			return false, nil
		}

		if base.ID == nil {
			c.dispatchSSEPayload(payload)
			return false, nil
		}

		if *base.ID != expectedID {
			return false, nil
		}

		if base.Error != nil {
			return true, errors.New(base.Error.Message)
		}

		result = &base.Result
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("no response received for request %d", expectedID)
	}
	return result, nil
}

func (c *StreamableHTTPMCPClient) dispatchSSEPayload(payload json.RawMessage) {
	var base struct {
		ID *int64 `json:"id,omitempty"`
	}
	if err := json.Unmarshal(payload, &base); err != nil {
		return
	}
	if base.ID != nil {
		return
	}

	var notification mcp.JSONRPCNotification
	if err := json.Unmarshal(payload, &notification); err != nil {
		return
	}

	c.notifyMu.RLock()
	defer c.notifyMu.RUnlock()

	for _, handler := range c.notifications {
		handler(notification)
	}
}

func (c *StreamableHTTPMCPClient) updateSessionFromHeaders(header http.Header) {
	if sessionID := header.Get(mcp.HeaderSessionID); sessionID != "" {
		c.sessionID = sessionID
		return
	}
	if sessionID := header.Get(mcp.LegacyHeaderSessionID); sessionID != "" {
		c.sessionID = sessionID
	}
}

func decodeJSONRPCResponseBody(reader io.Reader) (*json.RawMessage, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var response struct {
		Result json.RawMessage `json:"result,omitempty"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if response.Error != nil {
		return nil, errors.New(response.Error.Message)
	}

	return &response.Result, nil
}

func consumeSSE(
	reader io.Reader,
	onMessage func(json.RawMessage) (bool, error),
) error {
	bufReader := bufio.NewReader(reader)
	var dataLines []string

	flushEvent := func() (bool, error) {
		if len(dataLines) == 0 {
			return false, nil
		}
		payload := json.RawMessage(strings.Join(dataLines, "\n"))
		dataLines = nil
		return onMessage(payload)
	}

	for {
		content, err := utils.BufioReadLine(bufReader)
		if err != nil {
			if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
				_, flushErr := flushEvent()
				return flushErr
			}
			return err
		}

		line := string(content)
		if line == "" {
			done, err := flushEvent()
			if done || err != nil {
				return err
			}
			continue
		}

		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(
				dataLines,
				strings.TrimSpace(strings.TrimPrefix(line, "data:")),
			)
		}
	}
}
