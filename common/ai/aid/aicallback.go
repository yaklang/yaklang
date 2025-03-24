package aid

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

type AIRequest struct {
	prompt string
	ctx    *taskContext
}

func (r *AIRequest) GetPrompt() string {
	return r.prompt
}

type AIResponse struct {
	ch          *chanx.UnlimitedChan[*OutputStream]
	enableDebug bool
}

func (a *AIResponse) Debug(i ...bool) {
	if len(i) <= 0 {
		a.enableDebug = true
		return
	}

	a.enableDebug = i[0]
}

func (a *AIResponse) GetOutputStreamReader(nodeId string, system bool, config *Config) io.Reader {
	pr, pw := utils.NewBufPipe(nil)
	go func() {
		defer pw.Close()
		for i := range a.ch.Out {
			if i == nil {
				continue
			}

			targetStream := i.out
			if a.enableDebug {
				targetStream = io.TeeReader(i.out, os.Stdout)
			}
			if i.IsReason {
				if system {
					config.EmitSystemReasonStreamEvent(nodeId, targetStream)
				} else {
					config.EmitReasonStreamEvent(nodeId, targetStream)
				}
				continue
			}

			targetStream = io.TeeReader(targetStream, pw)
			if system {
				config.EmitSystemStreamEvent(nodeId, targetStream)
			} else {
				config.EmitStreamEvent(nodeId, targetStream)
			}
		}
		config.WaitForStream()
	}()
	return pr
}

type OutputStream struct {
	NodeType string
	IsReason bool
	out      io.Reader
}

type AIRequestOption func(req *AIRequest)

func NewAIRequest(prompt string, opt ...AIRequestOption) *AIRequest {
	req := &AIRequest{
		prompt: prompt,
	}
	for _, i := range opt {
		i(req)
	}
	return req
}

func WithAIRequest_TaskContext(ctx *taskContext) AIRequestOption {
	return func(req *AIRequest) {
		req.ctx = ctx
	}
}

type AICallbackType func(req *AIRequest) (*AIResponse, error)

func NewAIResponse() *AIResponse {
	return &AIResponse{
		ch: chanx.NewUnlimitedChan[*OutputStream](context.TODO(), 2),
	}
}

func (r *AIResponse) EmitOutputStream(reader io.Reader) {
	r.ch.SafeFeed(&OutputStream{
		out: reader,
	})
}

func (r *AIResponse) EmitReasonStream(reader io.Reader) {
	r.ch.SafeFeed(&OutputStream{
		IsReason: true,
		out:      reader,
	})
}

func (r *AIResponse) Close() {
	if r.ch == nil {
		return
	}
	r.ch.Close()
}

func AIChatToAICallbackType(cb func(prompt string, opts ...aispec.AIConfigOption) (string, error)) AICallbackType {
	return func(req *AIRequest) (*AIResponse, error) {
		resp := NewAIResponse()
		go func() {
			defer resp.Close()

			isStream := false
			output, err := cb(
				req.GetPrompt(),
				aispec.WithStreamHandler(func(reader io.Reader) {
					isStream = true
					resp.EmitOutputStream(reader)
				}),
				aispec.WithReasonStreamHandler(func(reader io.Reader) {
					isStream = true
					resp.EmitReasonStream(reader)
				}),
			)
			if err != nil {
				log.Errorf("chat error: %v", err)
			}
			if !isStream {
				resp.EmitOutputStream(strings.NewReader(output))
			}
		}()
		return resp, nil
	}
}
