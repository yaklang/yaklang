package liteforge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/bufpipe"
)

type nativeAttempt struct {
	function, id, finish string
	arguments            strings.Builder
	index                int
	seen, multiple       bool
}

type argumentStream struct {
	reader *bufpipe.PipeReader
	writer *bufpipe.PipeWriter
	action *aicommon.Action
	data   strings.Builder
	done   chan struct{}
	err    error
}

type nativeOutput struct {
	mu        sync.Mutex
	ctx       context.Context
	request   Request
	wireName  string
	validator *jsonschema.Schema
	current   *nativeAttempt
	streams   []*argumentStream
	response  *aicommon.AIResponse
	stop      func() bool
}

var functionName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func newNativeOutput(ctx context.Context, r Request, validator *jsonschema.Schema) *nativeOutput {
	name := r.ActionName
	if !functionName.MatchString(name) || strings.HasPrefix(name, "END_") {
		name = "liteforge_output"
	}
	n := &nativeOutput{ctx: ctx, request: r, wireName: name, validator: validator}
	n.stop = context.AfterFunc(ctx, n.closePipes)
	return n
}

func (n *nativeOutput) closePipes() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, stream := range n.streams {
		_ = stream.writer.Close()
	}
}

func (n *nativeOutput) close() { n.stop(); n.closePipes() }

func (n *nativeOutput) startLocked(stream *argumentStream) {
	if stream.action == nil && n.response != nil {
		stream.action = aicommon.NewActionMaker(n.request.ActionName, n.request.actionOptions(n.response)...).ReadFromReader(n.ctx, stream.reader)
	}
}

// argumentsStream consumes the provider's dedicated raw arguments reader.
// Tool-call deltas identify the function and verify the complete transport;
// they are not used as a second source of field-stream notifications.
func (n *nativeOutput) argumentsStream(reader io.Reader) {
	pr, pw := bufpipe.NewPipe()
	stream := &argumentStream{reader: pr, writer: pw, done: make(chan struct{})}
	n.mu.Lock()
	n.streams = append(n.streams, stream)
	n.startLocked(stream)
	n.mu.Unlock()
	_, stream.err = io.Copy(io.MultiWriter(pw, &stream.data), reader)
	_ = pw.Close()
	close(stream.done)
}

func (n *nativeOutput) requestOption() aicommon.AIRequestOption {
	return func(req *aicommon.AIRequest) {
		n.closePipes()
		n.mu.Lock()
		n.current, n.streams, n.response = nil, nil, nil
		n.mu.Unlock()
		aicommon.WithAIRequest_ExtraSpecOpts(
			aispec.WithToolChoice("auto"),
			// Consume arguments separately from ordinary content/reason streams. Do
			// not enable the flag that merges native arguments into response text.
			aispec.WithToolCallArgumentsStreamHandler(n.argumentsStream),
			aispec.AIConfigOption(func(cfg *aispec.AIConfig) {
				previous := cfg.RawHTTPResponseHeaderCallback
				cfg.RawHTTPResponseHeaderCallback = func(header []byte) {
					if previous != nil {
						previous(header)
					}
					n.mu.Lock()
					n.current = nil
					n.mu.Unlock()
				}
			}),
			aispec.WithToolCallCallback(func(calls []*aispec.ToolCall) {
				n.mu.Lock()
				defer n.mu.Unlock()
				for _, call := range calls {
					if call == nil {
						continue
					}
					if n.current == nil {
						n.current = &nativeAttempt{}
					}
					a := n.current
					if a.seen && (a.index != call.Index || (a.id != "" && call.ID != "" && a.id != call.ID)) {
						a.multiple = true
					}
					a.seen, a.index = true, call.Index
					if call.ID != "" {
						a.id = call.ID
					}
					if part := call.Function.Name; part != "" {
						if part == n.wireName && strings.HasPrefix(part, a.function) {
							a.function = part
						} else {
							a.function += part
						}
					}
					// Keep the delta copy solely for transport consistency and providers
					// that only expose the historical callback. Never feed it into the
					// dedicated arguments stream a second time.
					a.arguments.WriteString(call.Function.Arguments)
				}
			}),
			aispec.WithFinishReasonCallback(func(reason string, _ []byte) {
				n.mu.Lock()
				if n.current != nil {
					n.current.finish = reason
				}
				n.mu.Unlock()
			}),
		)(req)
	}
}

func (n *nativeOutput) bind(resp *aicommon.AIResponse) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.response = resp
	for _, stream := range n.streams {
		n.startLocked(stream)
	}
}

func (n *nativeOutput) parse(resp *aicommon.AIResponse) (*aicommon.Action, error) {
	n.bind(resp)
	_, readErr := io.Copy(io.Discard, resp.GetUnboundStreamReader(false))
	n.mu.Lock()
	streams := append([]*argumentStream{}, n.streams...)
	current := n.current
	n.mu.Unlock()
	for _, stream := range streams {
		select {
		case <-stream.done:
		case <-n.ctx.Done():
			return nil, n.ctx.Err()
		}
		if stream.action != nil {
			_ = stream.action.WaitParseResult(n.ctx)
			stream.action.WaitStream(n.ctx)
		}
	}
	if readErr != nil {
		return nil, readErr
	}
	if current == nil || !current.seen || current.multiple || current.function != n.wireName || current.finish != "tool_calls" {
		return nil, fmt.Errorf("%s requires exactly one complete native function call", n.request.ActionName)
	}
	arguments := current.arguments.String()
	if len(streams) > 0 {
		stream := streams[len(streams)-1]
		if stream.err != nil {
			return nil, stream.err
		}
		// A superseded stream cannot supply the final result. Merged HTTP retry
		// streams are rejected and retried by the bounded transaction budget.
		if stream.data.String() != arguments {
			return nil, fmt.Errorf("native arguments stream does not match the final function call")
		}
		arguments = stream.data.String()
	} else {
		// Compatibility for providers that implement only ToolCallCallback. The
		// normal provider path always consumes ToolCallArgumentsStreamHandler.
		a := aicommon.NewActionMaker(n.request.ActionName, n.request.actionOptions(resp)...).ReadFromReader(n.ctx, strings.NewReader(arguments))
		_ = a.WaitParseResult(n.ctx)
		a.WaitStream(n.ctx)
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(arguments), &value); err != nil {
		return nil, fmt.Errorf("incomplete or invalid native arguments: %w", err)
	}
	return admitOutput(value, n.request.ActionName, n.validator, true)
}
