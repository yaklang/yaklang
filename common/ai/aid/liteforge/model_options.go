package liteforge

import (
	"io"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/bufpipe"
)

// Forward provider options without replacing the parser's readers or transport.
// Script stream observers see the same incremental bytes on an independent pipe.
func modelRequestOption(native bool, opts []aispec.AIConfigOption) aicommon.AIRequestOption {
	return aicommon.WithAIRequest_ExtraSpecOpts(func(cfg *aispec.AIConfig) {
		core := *cfg
		user := aispec.NewDefaultAIConfig(opts...)
		for _, opt := range opts {
			opt(cfg)
		}
		cfg.Context = core.Context
		cfg.Tools, cfg.ToolChoice = core.Tools, core.ToolChoice
		cfg.StreamHandler = observeStream(core.StreamHandler, user.StreamHandler)
		cfg.ReasonStreamHandler = observeStream(core.ReasonStreamHandler, user.ReasonStreamHandler)
		cfg.ToolCallArgumentsStreamHandler = core.ToolCallArgumentsStreamHandler
		if native {
			cfg.ToolCallArgumentsStreamHandler = observeStream(core.ToolCallArgumentsStreamHandler,
				user.ToolCallArgumentsStreamHandler, user.StreamHandler)
		}
		cfg.ToolCallCallback = core.ToolCallCallback
		if user.ToolCallCallback != nil {
			cfg.ToolCallCallback = func(calls []*aispec.ToolCall) {
				if core.ToolCallCallback != nil {
					core.ToolCallCallback(calls)
				}
				user.ToolCallCallback(calls)
			}
		}
		cfg.FinishReasonCallback = core.FinishReasonCallback
		if user.FinishReasonCallback != nil {
			cfg.FinishReasonCallback = func(reason string, data []byte) {
				if core.FinishReasonCallback != nil {
					core.FinishReasonCallback(reason, data)
				}
				user.FinishReasonCallback(reason, data)
			}
		}
		cfg.ModelInfoCallback = core.ModelInfoCallback
		if user.ModelInfoCallback != nil {
			cfg.ModelInfoCallback = func(provider, model string, thinking ...string) {
				if core.ModelInfoCallback != nil {
					core.ModelInfoCallback(provider, model, thinking...)
				}
				user.ModelInfoCallback(provider, model, thinking...)
			}
		}
		cfg.ModelInfoConfirmCallback = core.ModelInfoConfirmCallback
		if user.ModelInfoConfirmCallback != nil {
			cfg.ModelInfoConfirmCallback = func(provider, model string, thinking ...string) {
				if core.ModelInfoConfirmCallback != nil {
					core.ModelInfoConfirmCallback(provider, model, thinking...)
				}
				user.ModelInfoConfirmCallback(provider, model, thinking...)
			}
		}
		cfg.RawHTTPResponseHeaderCallback = core.RawHTTPResponseHeaderCallback
		if user.RawHTTPResponseHeaderCallback != nil {
			cfg.RawHTTPResponseHeaderCallback = func(data []byte) {
				if core.RawHTTPResponseHeaderCallback != nil {
					core.RawHTTPResponseHeaderCallback(data)
				}
				user.RawHTTPResponseHeaderCallback(data)
			}
		}
		cfg.RawHTTPResponseCallback = core.RawHTTPResponseCallback
		if user.RawHTTPResponseCallback != nil {
			cfg.RawHTTPResponseCallback = func(header, body []byte) {
				if core.RawHTTPResponseCallback != nil {
					core.RawHTTPResponseCallback(header, body)
				}
				user.RawHTTPResponseCallback(header, body)
			}
		}
		cfg.RawHTTPRequestResponseCallback = core.RawHTTPRequestResponseCallback
		if user.RawHTTPRequestResponseCallback != nil {
			cfg.RawHTTPRequestResponseCallback = func(req, header, body []byte, usage *aispec.ChatUsage) {
				if core.RawHTTPRequestResponseCallback != nil {
					core.RawHTTPRequestResponseCallback(req, header, body, usage)
				}
				user.RawHTTPRequestResponseCallback(req, header, body, usage)
			}
		}
	})
}

func observeStream(core func(io.Reader), observers ...func(io.Reader)) func(io.Reader) {
	var active []func(io.Reader)
	for _, observer := range observers {
		if observer != nil {
			active = append(active, observer)
		}
	}
	if len(active) == 0 {
		return core
	}
	return func(reader io.Reader) {
		if reader == nil {
			return
		}
		var wg sync.WaitGroup
		var writers []io.Writer
		var pipes []*bufpipe.PipeWriter
		handlers := active
		if core != nil {
			handlers = append([]func(io.Reader){core}, active...)
		}
		for _, handler := range handlers {
			pr, pw := bufpipe.NewPipe()
			writers, pipes = append(writers, pw), append(pipes, pw)
			wg.Add(1)
			go func(handler func(io.Reader)) {
				defer wg.Done()
				handler(pr)
			}(handler)
		}
		_, err := io.Copy(io.MultiWriter(writers...), reader)
		for _, pipe := range pipes {
			_ = pipe.CloseWithError(err)
		}
		wg.Wait()
	}
}
