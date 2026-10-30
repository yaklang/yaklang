// Package yakscript provides a generic, trusted Yak implementation of AIClient.
// The interpreter is registered by common/yak to avoid an import cycle.
// README.md defines the script ABI and the HTTP/SSE/business/Agent boundaries.
package yakscript

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httptrace"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

const ProviderType = "yak-script"

// Request is a per-invocation snapshot. Adapters must pass Context to network
// calls. RequestID can be used as an isolated upstream conversation ID.
type Request struct {
	Context     context.Context
	RequestID   string
	Prompt      string
	RawMessages []aispec.ChatDetail
	Tools       []aispec.Tool
	ToolChoice  any
	failureMu   sync.Mutex
	failure     *aispec.ModelCallFailure
}

// Executor runs chat(request, emit). A script either returns a complete string,
// or emits text fragments and returns nil. With native calling enabled it can
// return a normalized {content, tool_calls, finish_reason} object. Returning an error fails the attempt;
// errors must not become model text.
// Both return and emit carry normalized content, never an HTTP/SSE envelope or
// reasoning text. The current script ABI has no separate reasoning emitter.
type Executor func(context.Context, string, *Request, func(string) error) (any, error)

var executor atomic.Pointer[Executor]

func RegisterExecutor(run Executor) { executor.Store(&run) }

type GatewayClient struct{ config *aispec.AIConfig }

var _ aispec.AIClient = (*GatewayClient)(nil)

func (g *GatewayClient) LoadOption(opts ...aispec.AIConfigOption) {
	g.config = aispec.NewDefaultAIConfig(opts...)
}
func (g *GatewayClient) GetConfig() *aispec.AIConfig { return g.config }

func (g *GatewayClient) CheckValid() error {
	if g.config == nil || strings.TrimSpace(g.config.Script) == "" {
		return errors.New("yak-script requires script source defining chat(request, emit)")
	}
	if run := executor.Load(); run == nil || *run == nil {
		return errors.New("Yak model script runtime is not registered; import common/yak")
	}
	return nil
}

func (g *GatewayClient) Chat(prompt string, functions ...any) (string, error) {
	if err := g.CheckValid(); err != nil {
		return "", &aispec.ModelCallFailure{Kind: "configuration", Cause: err}
	}
	c := g.config
	if len(functions) > 0 {
		return "", &aispec.ModelCallFailure{Kind: "configuration", Cause: errors.New("yak-script does not accept legacy functions; use tools")}
	}
	if len(c.Tools) > 0 && !c.EnableNativeToolCalling {
		return "", &aispec.ModelCallFailure{Kind: "configuration", Cause: errors.New("yak-script native tool calling is disabled in provider configuration")}
	}
	if len(c.Images) > 0 || len(c.Videos) > 0 {
		return "", &aispec.ModelCallFailure{Kind: "configuration", Cause: errors.New("yak-script currently supports text input only")}
	}
	ctx := c.Context
	if ctx == nil {
		ctx = context.Background()
	}
	scope := httptrace.GetScope(ctx)
	req := &Request{Context: ctx, RequestID: scope.CallID, Prompt: prompt, RawMessages: c.RawMessages, Tools: c.Tools, ToolChoice: c.ToolChoice}
	return g.attempt(ctx, req)
}

// attempt only executes and normalizes one script call. aispec owns recovery.
func (g *GatewayClient) attempt(ctx context.Context, req *Request) (resultText string, resultErr error) {
	defer func() {
		var failure *aispec.ModelCallFailure
		if resultErr != nil && ctx.Err() == nil && !errors.As(resultErr, &failure) {
			resultErr = &aispec.ModelCallFailure{Kind: "script", Cause: resultErr}
		}
	}()
	c := g.config
	bufferOutput := len(req.Tools) > 0
	var mu sync.Mutex
	var output strings.Builder
	closed := false

	emit := func(text string) error {
		mu.Lock()
		defer mu.Unlock()
		if err := ctx.Err(); err != nil {
			return err
		}
		if closed {
			return errors.New("yak-script output is already closed")
		}
		if text != "" {
			output.WriteString(text)
			if c.StreamHandler != nil && !bufferOutput {
				// A finite reader per fragment avoids an idle pipe while an
				// upstream agent is still working toward its first answer.
				c.StreamHandler(strings.NewReader(text))
			}
		}
		return nil
	}
	run := executor.Load()
	value, err := (*run)(ctx, c.Script, req, emit)
	if returnedErr, ok := value.(error); ok && err == nil {
		err = returnedErr
		value = nil
	}
	mu.Lock()
	closed = true
	text := output.String()
	mu.Unlock()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", fmt.Errorf("yak-script execution failed: %w", err)
	}
	finishReason := "stop"
	var calls []*aispec.ToolCall
	if value != nil {
		if text != "" {
			return "", errors.New("yak-script chat must either emit text or return a result, not both")
		}
		if returned, ok := value.(string); ok {
			text = returned
		} else {
			if !c.EnableNativeToolCalling {
				return "", errors.New("yak-script structured results require native tool calling to be enabled")
			}
			result, err := normalizeNativeResult(value, req.Tools)
			if err != nil {
				return "", err
			}
			text, calls, finishReason = result.Content, result.ToolCalls, result.FinishReason
		}
	}
	if strings.TrimSpace(text) == "" && len(calls) == 0 {
		return "", &aispec.ModelCallFailure{Kind: "protocol", Cause: errors.New("yak-script returned an empty model response")}
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	// Native results are committed only after every call has been validated.
	if (value != nil || bufferOutput) && c.StreamHandler != nil && text != "" {
		c.StreamHandler(strings.NewReader(text))
	}
	if len(calls) > 0 {
		if c.ToolCallArgumentsStreamHandler != nil {
			for _, call := range calls {
				c.ToolCallArgumentsStreamHandler(strings.NewReader(call.Function.Arguments))
			}
		}
		if c.ToolCallCallback != nil {
			c.ToolCallCallback(calls)
		}
	}
	if c.FinishReasonCallback != nil {
		c.FinishReasonCallback(finishReason, nil)
	}
	return text, nil
}

func (g *GatewayClient) ChatStream(prompt string) (io.Reader, error) {
	text, err := aispec.ChatWithGlobalRetry(g, prompt)
	if err != nil {
		return nil, err
	}
	return strings.NewReader(text), nil
}

func (g *GatewayClient) ExtractData(input, description string, fields map[string]any) (map[string]any, error) {
	if err := g.CheckValid(); err != nil {
		return nil, err
	}
	return aispec.ExecuteStructuredOutput(description+"\n"+input, fields, func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
		// Preserve the adapter while applying the structured-output callbacks.
		clone := *g.config
		for _, opt := range opts {
			opt(&clone)
		}
		return aispec.ChatWithGlobalRetry(&GatewayClient{config: &clone}, prompt)
	}, aispec.WithFunctionCallMode(false), aispec.WithContext(g.config.Context))
}

func (g *GatewayClient) GetModelList() ([]*aispec.ModelMeta, error) {
	if g.config == nil || g.config.Model == "" {
		return nil, nil
	}
	return []*aispec.ModelMeta{{Id: g.config.Model, Object: "model", OwnedBy: ProviderType}}, nil
}
func (*GatewayClient) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	return nil, errors.New("yak-script HTTP requests are owned by the adapter script")
}
func (*GatewayClient) SupportedStructuredStream() bool { return false }
func (*GatewayClient) StructuredStream(string, ...any) (chan *aispec.StructuredData, error) {
	return nil, errors.New("yak-script structured event streaming is not supported")
}
