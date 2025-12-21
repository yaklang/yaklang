package rag

import (
	"context"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
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

				// Declare event pointer to capture in closure
				var event *schema.AiOutputEvent
				var err error

				// Use finishCallback to emit reference material after stream is fully consumed
				// At that point, ResultBuffer will be filled (before logWriter.Close())
				event, err = e.EmitDefaultStreamEvent(
					"enhance-query",
					reader,
					"",
					func() {
						// This callback is called after reader EOF (after logWriter.Close())
						// At this point, ResultBuffer should be filled
						if info.ResultBuffer != nil && info.ResultBuffer.Len() > 0 {
							streamId := ""
							if event != nil {
								streamId = event.GetContentJSONPath(`$.event_writer_id`)
							}
							if streamId != "" {
								_, emitErr := e.EmitTextReferenceMaterial(streamId, info.ResultBuffer.String())
								if emitErr != nil {
									log.Warnf("failed to emit reference material: %v", emitErr)
								}
							} else {
								log.Warnf("failed to get stream id for reference material, method: %s", info.Method)
							}
						} else {
							log.Debugf("no result buffer content for method: %s", info.Method)
						}
					},
				)
				if err != nil {
					log.Warnf("failed to emit enhance-query stream event: %v", err)
					return
				}
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

				// Declare event pointer to capture in closure
				var event *schema.AiOutputEvent
				var err error

				// Use finishCallback to emit reference material after stream is fully consumed
				// At that point, ResultBuffer will be filled (before logWriter.Close())
				event, err = e.EmitDefaultStreamEvent(
					"enhance-query",
					reader,
					"",
					func() {
						// This callback is called after reader EOF (after logWriter.Close())
						// At this point, ResultBuffer should be filled
						if info.ResultBuffer != nil && info.ResultBuffer.Len() > 0 {
							streamId := ""
							if event != nil {
								streamId = event.GetContentJSONPath(`$.event_writer_id`)
							}
							if streamId != "" {
								_, emitErr := e.EmitTextReferenceMaterial(streamId, info.ResultBuffer.String())
								if emitErr != nil {
									log.Warnf("failed to emit reference material: %v", emitErr)
								}
							} else {
								log.Warnf("failed to get stream id for reference material, method: %s", info.Method)
							}
						} else {
							log.Debugf("no result buffer content for method: %s", info.Method)
						}
					},
				)
				if err != nil {
					log.Warnf("failed to emit enhance-query stream event: %v", err)
					return
				}
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
