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
	summaryReader, err := t.AICallback(ctx, aispec.NewUserChatDetail(summaryPrompt))
	if err != nil {
		return nil, err
	}
	summaryBytes, err := io.ReadAll(summaryReader)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(summaryBytes), nil
}
