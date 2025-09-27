package rag

import (
	"context"
	"io"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

func NewRagEnhanceKnowledgeManager() *aicommon.EnhanceKnowledgeManager {
	return aicommon.NewEnhanceKnowledgeManager(func(ctx context.Context, e *aicommon.Emitter, query string) (<-chan aicommon.EnhanceKnowledge, error) {
		result := chanx.NewUnlimitedChan[aicommon.EnhanceKnowledge](ctx, 10)
		_, err := QueryYakitProfile(query,
			WithRAGCtx(ctx),
			WithEveryQueryResultCallback(func(data *ScoredResult) {
				result.SafeFeed(data)
			}),
			WithRAGOnQueryFinish(func(_ []*ScoredResult) {
				result.Close()
			}),
			WithRAGLogReader(func(reader io.Reader) {
				if e == nil {
					io.Copy(io.Discard, reader)
					return
				}
				e.EmitStreamEvent(
					"enhance-query",
					time.Now(),
					reader,
					"",
				)
			}),
		)
		if err != nil {
			return nil, err
		}
		return result.OutputChannel(), nil
	})
}
