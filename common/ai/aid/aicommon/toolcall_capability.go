package aicommon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
)

// ToolCallCapability is deliberately three-valued: a text response or a
// transient error is not evidence that the provider lacks tool calls.
type ToolCallCapability string

const (
	ToolCallUnknown     ToolCallCapability = "unknown"
	ToolCallSupported   ToolCallCapability = "supported"
	ToolCallUnsupported ToolCallCapability = "unsupported"
)

const toolCallProbeName = "yak_probe_echo"
const toolCallProbeValue = "yak_tool_call_probe_ok"

type toolCallProbeKey struct {
	service, model, tier string
}

type toolCallProbeEntry struct {
	state   ToolCallCapability
	err     error
	expires time.Time
	done    chan struct{}
}

type toolCallCapabilityCache struct {
	mu      sync.Mutex
	entries map[toolCallProbeKey]*toolCallProbeEntry
}

// WithCheckToolCall controls the internal first-use probe. Disabling it keeps
// the explicit EnableFunctionCallMode choice; it is not a Yak script option.
func WithCheckToolCall(enabled bool) ConfigOption {
	return func(c *Config) error {
		c.CheckToolCall = enabled
		return nil
	}
}

func WithToolCallProbeTimeout(timeout time.Duration) ConfigOption {
	return func(c *Config) error {
		if timeout > 0 {
			c.ToolCallProbeTimeout = timeout
		}
		return nil
	}
}

func WithToolCallProbeCacheTTL(ttl time.Duration) ConfigOption {
	return func(c *Config) error {
		if ttl > 0 {
			c.ToolCallProbeCacheTTL = ttl
		}
		return nil
	}
}

// CheckToolCallCapability probes the same callback tier as the upcoming loop
// request. The echo function is declared to the model but never executed.
// Results are isolated to this Config, so credentials and opaque callback
// routing cannot accidentally share a cache entry across sessions.
func (c *Config) CheckToolCallCapability(ctx context.Context, useSpeedPriority bool) (ToolCallCapability, error) {
	if c == nil {
		return ToolCallUnknown, errors.New("tool-call probe requires a config")
	}
	if !c.EnableFunctionCallMode {
		return ToolCallUnsupported, nil
	}
	if !c.CheckToolCall {
		return ToolCallSupported, nil
	}
	if ctx == nil {
		ctx = c.GetContext()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ToolCallUnknown, err
	}

	var callback AICallbackType
	tier := string(consts.TierIntelligent)
	if useSpeedPriority {
		callback = c.GetSpeedPriorityRawAICallback()
		tier = string(consts.TierLightweight)
	} else {
		callback = c.GetQualityPriorityRawAICallback()
	}
	if callback == nil {
		callback = c.GetOriginalAICallback()
	}
	if callback == nil {
		return ToolCallUnknown, errors.New("tool-call probe has no AI callback")
	}
	key := toolCallProbeKey{service: c.AiServerName, model: c.AiModelName, tier: tier}
	cache := c.getToolCallProbeCache()
	for {
		cache.mu.Lock()
		if entry := cache.entries[key]; entry != nil {
			if entry.done != nil {
				done := entry.done
				cache.mu.Unlock()
				select {
				case <-done:
					continue
				case <-ctx.Done():
					return ToolCallUnknown, ctx.Err()
				}
			}
			if time.Now().Before(entry.expires) {
				state, err := entry.state, entry.err
				cache.mu.Unlock()
				return state, err
			}
		}
		entry := &toolCallProbeEntry{done: make(chan struct{})}
		cache.entries[key] = entry
		cache.mu.Unlock()

		state, err := c.probeToolCall(ctx, callback, tier)
		ttl := c.ToolCallProbeCacheTTL
		if ttl <= 0 {
			ttl = 30 * time.Minute
		}
		if state == ToolCallUnknown && ttl > time.Minute {
			ttl = time.Minute
		}
		cache.mu.Lock()
		if ctx.Err() != nil {
			delete(cache.entries, key)
		} else {
			entry.state, entry.err, entry.expires = state, err, time.Now().Add(ttl)
		}
		close(entry.done)
		entry.done = nil
		cache.mu.Unlock()
		return state, err
	}
}

func (c *Config) getToolCallProbeCache() *toolCallCapabilityCache {
	c.m.Lock()
	defer c.m.Unlock()
	if c.toolCallProbeCache == nil {
		c.toolCallProbeCache = &toolCallCapabilityCache{entries: make(map[toolCallProbeKey]*toolCallProbeEntry)}
	}
	return c.toolCallProbeCache
}

func (c *Config) probeToolCall(parent context.Context, callback AICallbackType, tier string) (ToolCallCapability, error) {
	timeout := c.ToolCallProbeTimeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	var callsMu sync.Mutex
	parts := make(map[int]*aispec.ToolCall)
	request := NewAIRequest(
		"Call the yak_probe_echo tool exactly once with message yak_tool_call_probe_ok. Do not answer in text.",
		WithAIRequest_Context(ctx),
		WithAIRequest_CallerLabel("toolcall-capability-probe"),
		WithAIRequest_DetachCheckpoint(),
		WithAIRequest_ExtraSpecOpts(
			aispec.WithTools([]aispec.Tool{{Type: "function", Function: aispec.ToolFunction{
				Name: toolCallProbeName, Description: "Echo a short capability probe value.",
				Parameters: map[string]any{"type": "object", "properties": map[string]any{
					"message": map[string]any{"type": "string"},
				}, "required": []string{"message"}},
			}}}),
			aispec.WithToolChoice("auto"),
			aispec.WithToolCallCallback(func(deltas []*aispec.ToolCall) {
				callsMu.Lock()
				defer callsMu.Unlock()
				for _, delta := range deltas {
					if delta == nil {
						continue
					}
					part := parts[delta.Index]
					if part == nil {
						part = &aispec.ToolCall{Index: delta.Index}
						parts[delta.Index] = part
					}
					if delta.ID != "" {
						part.ID = delta.ID
					}
					if delta.Type != "" {
						part.Type = delta.Type
					}
					if delta.Function.Name != "" {
						part.Function.Name = delta.Function.Name
					}
					part.Function.Arguments += delta.Function.Arguments
				}
			}),
		),
	)
	request.SetModelTier(tier)
	response, err := callback(c, request)
	if err != nil {
		return classifyToolCallProbeFailure(err, response)
	}
	if response == nil {
		return ToolCallUnknown, errors.New("tool-call probe returned no response")
	}
	// Drain both streams without an emitter. A tool-only response may have no
	// content; reading must not wait for a first text byte.
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		_, _ = io.Copy(io.Discard, response.GetUnboundStreamReader(false))
	}()
	if !response.WaitForCallbackDone(ctx) {
		return ToolCallUnknown, ctx.Err()
	}
	select {
	case <-drained:
	case <-ctx.Done():
		return ToolCallUnknown, ctx.Err()
	}
	if err := response.GetError(); err != nil {
		return classifyToolCallProbeFailure(err, response)
	}
	if status := response.GetHTTPStatusCode(); status >= 400 {
		return classifyToolCallProbeFailure(fmt.Errorf("HTTP %d", status), response)
	}
	callsMu.Lock()
	defer callsMu.Unlock()
	for _, call := range parts {
		if call.ID == "" || call.Function.Name != toolCallProbeName || (call.Type != "" && call.Type != "function") {
			continue
		}
		var args struct {
			Message string `json:"message"`
		}
		if json.Unmarshal([]byte(call.Function.Arguments), &args) == nil && args.Message == toolCallProbeValue {
			return ToolCallSupported, nil
		}
	}
	return ToolCallUnknown, errors.New("tool-call probe received no valid echo tool call")
}

func classifyToolCallProbeFailure(err error, response *AIResponse) (ToolCallCapability, error) {
	if err == nil {
		return ToolCallUnknown, nil
	}
	status := 0
	message := strings.ToLower(err.Error())
	if response != nil {
		status = response.GetHTTPStatusCode()
		// Inspect provider details for classification, but never return or log
		// the response dump: it may contain request-specific private data.
		message += strings.ToLower(response.GetRawHTTPResponseDump())
	}
	if (status == 400 || status == 422 || status == 501) &&
		(strings.Contains(message, "tool") || strings.Contains(message, "function")) &&
		(strings.Contains(message, "not support") || strings.Contains(message, "unsupported") ||
			strings.Contains(message, "unknown field") || strings.Contains(message, "unrecognized")) {
		return ToolCallUnsupported, fmt.Errorf("provider rejected tool calls (HTTP %d)", status)
	}
	if status > 0 {
		return ToolCallUnknown, fmt.Errorf("tool-call probe request failed (HTTP %d)", status)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ToolCallUnknown, err
	}
	return ToolCallUnknown, fmt.Errorf("tool-call probe request failed (%T)", err)
}
