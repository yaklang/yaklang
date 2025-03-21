package aid

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"io"
)

type AIRequest struct {
	prompt           string
	shouldHaveAction bool
	ctx              *taskContext
}

func (r *AIRequest) GetPrompt() string {
	return r.prompt
}

type AIResponse struct {
	ch *chanx.UnlimitedChan[*OutputStream]
}

func (a *AIResponse) Reader() io.Reader {
	pr, pw := utils.NewBufPipe(nil)
	go func() {
		defer pw.Close()
		for i := range a.ch.Out {
			if i == nil {
				continue
			}
			if !i.IsReason {
				io.Copy(pw, i.out)
			}
		}
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
