package taskstack

import (
	"bytes"
	"io"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

func DefaultSummaryAICallback(ctx *TaskSystemContext, details ...aispec.ChatDetail) (io.Reader, error) {
	t := ctx.CurrentTask
	summaryPrompt, err := GenerateSummaryPrompt(aispec.DetailsToString(details))
	if err != nil {
		return nil, err
	}
	req := NewAIRequest(summaryPrompt, WithAIRequest_TaskContext(ctx))
	summaryReader, err := t.AICallback(req)
	if err != nil {
		return nil, err
	}
	summaryBytes, err := io.ReadAll(summaryReader.Reader())
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(summaryBytes), nil
}
