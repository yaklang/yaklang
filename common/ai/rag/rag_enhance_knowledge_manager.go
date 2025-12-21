package rag

import (
	"context"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/log"
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
			WithRAGLogReaderWithInfo(func(reader io.Reader, info *vectorstore.SubQueryLogInfo, referenceMaterialCallback func(content string)) {
				if e == nil {
					io.Copy(io.Discard, reader)
					return
				}
				event, err := e.EmitTextMarkdownStreamEvent(
					"enhance-query",
					reader,
					"",
				)
				if err != nil {
					log.Warnf("failed to emit enhance-query stream event: %v", err)
					return
				}

				// After stream is consumed, emit reference material with search results
				go func() {
					// Wait for reader to be fully consumed
					<-info.ReaderDone
					if info.ResultBuffer != nil && info.ResultBuffer.Len() > 0 {
						streamId := ""
						if event != nil {
							streamId = event.GetContentJSONPath(`$.event_writer_id`)
						}
						if streamId != "" {
							_, err := e.EmitTextReferenceMaterial(streamId, info.ResultBuffer.String())
							if err != nil {
								log.Warnf("failed to emit reference material: %v", err)
							}
						}
					}
				}()
			}),
		)
		if err != nil {
			return nil, err
		}
		return result.OutputChannel(), nil
	})
}

func NewRagEnhanceKnowledgeManagerWithOptions(opts ...RAGSystemConfigOption) *aicommon.EnhanceKnowledgeManager {
	return aicommon.NewEnhanceKnowledgeManager(func(ctx context.Context, e *aicommon.Emitter, query string) (<-chan aicommon.EnhanceKnowledge, error) {
		result := chanx.NewUnlimitedChan[aicommon.EnhanceKnowledge](ctx, 10)
		allOpts := append(
			opts,
			WithRAGCtx(ctx),
			WithEveryQueryResultCallback(func(data *ScoredResult) {
				result.SafeFeed(data)
			}),
			WithRAGOnQueryFinish(func(_ []*ScoredResult) {
				result.Close()
			}),
			WithRAGLogReaderWithInfo(func(reader io.Reader, info *vectorstore.SubQueryLogInfo, referenceMaterialCallback func(content string)) {
				if e == nil {
					io.Copy(io.Discard, reader)
					return
				}
				event, err := e.EmitTextMarkdownStreamEvent(
					"enhance-query",
					reader,
					"",
				)
				if err != nil {
					log.Warnf("failed to emit enhance-query stream event: %v", err)
					return
				}

				// After stream is consumed, emit reference material with search results
				go func() {
					// Wait for reader to be fully consumed
					<-info.ReaderDone
					if info.ResultBuffer != nil && info.ResultBuffer.Len() > 0 {
						streamId := ""
						if event != nil {
							streamId = event.GetContentJSONPath(`$.event_writer_id`)
						}
						if streamId != "" {
							_, err := e.EmitTextReferenceMaterial(streamId, info.ResultBuffer.String())
							if err != nil {
								log.Warnf("failed to emit reference material: %v", err)
							}
						}
					}
				}()
			}),
		)
		_, err := QueryYakitProfile(query,
			allOpts...,
		)
		if err != nil {
			return nil, err
		}
		return result.OutputChannel(), nil
	})
}
