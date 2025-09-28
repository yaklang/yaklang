package aiforge

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/entityrepos"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/utils/chanx"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed liteforge_schema/liteforge_refine.schema.json
var refineSchema string

//go:embed liteforge_prompt/liteforge_refine_prompt.txt
var refinePrompt string

//go:embed liteforge_prompt/liteforge_refine_erm.txt
var refineERMPrompt string

func Action2RagKnowledgeEntries(
	action *aicommon.Action,
	knowledgeBaseID int64,
) ([]*schema.KnowledgeBaseEntry, error) {
	if action == nil {
		return nil, utils.Errorf("action is nil")
	}

	collection := action.GetInvokeParamsArray("rag_source_list")
	if len(collection) == 0 {
		return nil, utils.Errorf("no knowledge-collection found in action")
	}

	entries := make([]*schema.KnowledgeBaseEntry, 0, len(collection))
	for _, item := range collection {
		metadata := item.GetObject("structured_metadata")
		if metadata == nil {
			continue
		}
		entry := &schema.KnowledgeBaseEntry{
			KnowledgeBaseID:  knowledgeBaseID,
			KnowledgeTitle:   item.GetString("title"),
			KnowledgeType:    item.GetString("content_type"),
			KnowledgeDetails: item.GetString("embedding_text"), // 核心文本作为详细信息
			Summary:          metadata.GetString("summary"),
			Keywords:         schema.StringArray(metadata.GetStringSlice("keywords")),
		}
		entries = append(entries, entry)
	}

	if len(entries) == 0 {
		return nil, utils.Errorf("no entries could be converted from the provided action")
	}

	return entries, nil
}

func BuildKnowledgeFromFile(kbName string, path string, option ...any) (<-chan *schema.KnowledgeBaseEntry, error) {
	analyzeResult, err := AnalyzeFile(path, option...)
	if err != nil {
		return nil, utils.Errorf("failed to start analyze file: %v", err)
	}
	option = append(option, RefineWithKnowledgeBaseName(kbName))
	return _buildKnowledge(analyzeResult, option...)
}

func BuildKnowledgeFromBytes(kbName string, content []byte, option ...any) (<-chan *schema.KnowledgeBaseEntry, error) {
	return BuildKnowledgeFromReader(kbName, bytes.NewReader(content), option...)
}

func BuildKnowledgeFromReader(kbName string, reader io.Reader, option ...any) (<-chan *schema.KnowledgeBaseEntry, error) {
	analyzeResult, err := AnalyzeReader(reader, option...)
	if err != nil {
		return nil, utils.Errorf("failed to start analyze reader: %v", err)
	}
	option = append(option, RefineWithKnowledgeBaseName(kbName))
	return _buildKnowledge(analyzeResult, option...)
}

func _buildKnowledge(analyzeChannel <-chan AnalysisResult, option ...any) (<-chan *schema.KnowledgeBaseEntry, error) {
	refineConfig := NewRefineConfig(option...)
	knowledgeDatabaseName := refineConfig.KnowledgeBaseName
	kb, err := knowledgebase.NewKnowledgeBase(refineConfig.Database, knowledgeDatabaseName, refineConfig.KnowledgeBaseDesc, refineConfig.KnowledgeBaseType)
	if err != nil {
		return nil, utils.Errorf("fial to create knowledgDatabase: %v", err)
	}

	er, err := AnalyzeERMFromAnalysisResult(analyzeChannel, option...)
	if err != nil {
		return nil, utils.Errorf("failed to start build erm from input: %v", err)
	}

	return BuildKnowledgeFromEntityRepository(er, kb, option...)
}

func BuildKnowledgeFromEntityRepository(er *entityrepos.EntityRepository, kb *knowledgebase.KnowledgeBase, option ...any) (<-chan *schema.KnowledgeBaseEntry, error) {
	refineConfig := NewRefineConfig(option...)
	refineConfig.AnalyzeLog("start build knowledge from entity repository use default qc")

	output := chanx.NewUnlimitedChan[*schema.KnowledgeBaseEntry](refineConfig.Ctx, 100)

	go func() {
		hopAnalyzeWg := utils.NewSizedWaitGroup(refineConfig.AnalyzeConcurrency)
		defer output.Close()
		defer hopAnalyzeWg.Wait()

		var total int64 = 0
		var done int64 = 0
		var processingStartTime = time.Now()
		var lastLogTime = time.Now()
		var vectorIndexDuration int64 = 0
		var knowledgeEntryDuration int64 = 0
		var saveDuration int64 = 0

		for hop := range er.YieldKHop(refineConfig.Ctx, append(refineConfig.KHopOption(), entityrepos.WithRuntimeBuildOnly(true))...) {
			currentTotal := atomic.AddInt64(&total, 1)

			// 性能监控：只在性能表现差时才打印详细日志
			shouldLogPerformance := false
			var logReason string

			// 检查是否需要记录性能日志的条件
			if currentTotal%1000 == 0 || time.Since(lastLogTime) > 60*time.Second {
				elapsed := time.Since(processingStartTime)
				rate := float64(currentTotal) / elapsed.Seconds()
				avgVectorTime := time.Duration(atomic.LoadInt64(&vectorIndexDuration)) / time.Duration(currentTotal)
				avgKnowledgeTime := time.Duration(atomic.LoadInt64(&knowledgeEntryDuration)) / time.Duration(int(math.Max(1, float64(done))))
				avgSaveTime := time.Duration(atomic.LoadInt64(&saveDuration)) / time.Duration(int(math.Max(1, float64(done))))

				// 性能阈值判断：只在性能差时才打印
				if rate < 10.0 { // 处理速度低于10 hops/s
					shouldLogPerformance = true
					logReason = "slow_rate"
				} else if avgVectorTime > 200*time.Millisecond { // 平均向量索引时间超过200ms
					shouldLogPerformance = true
					logReason = "slow_vector"
				} else if avgKnowledgeTime > 1*time.Second { // 平均知识构建时间超过1s
					shouldLogPerformance = true
					logReason = "slow_knowledge"
				} else if avgSaveTime > 500*time.Millisecond { // 平均保存时间超过500ms
					shouldLogPerformance = true
					logReason = "slow_save"
				} else if elapsed > 5*time.Minute && currentTotal < 100 { // 运行超过5分钟但处理数量少
					shouldLogPerformance = true
					logReason = "inefficient"
				}

				if shouldLogPerformance {
					refineConfig.AnalyzeLog("MULTI-HOP PERFORMANCE [%s]: total=%d, elapsed=%v, rate=%.1f hops/s, avg_vector=%v, avg_knowledge=%v, avg_save=%v",
						logReason, currentTotal, elapsed, rate, avgVectorTime, avgKnowledgeTime, avgSaveTime)
				} else {
					// 性能良好时，只记录简单的进度信息
					refineConfig.AnalyzeLog("Multi-hop processing: %d hops completed, rate=%.1f hops/s", currentTotal, rate)
				}
				lastLogTime = time.Now()
			}

			refineConfig.AnalyzeStatusCard("多跳知识构建(Multi-Hops Knowledge)", currentTotal)
			hopAnalyzeWg.Add(1)
			if refineConfig.Ctx != nil && refineConfig.Ctx.Err() != nil {
				break
			}
			go func(currentHop *entityrepos.KHopPath) {
				defer hopAnalyzeWg.Done()

				// 向量索引处理（异步）
				go func() {
					vectorStart := time.Now()
					err := er.AddKHopToVectorIndex(currentHop)
					vectorTime := time.Since(vectorStart)
					atomic.AddInt64(&vectorIndexDuration, int64(vectorTime))

					if err != nil {
						refineConfig.AnalyzeLog("failed to add khop to vector index: %v", err)
					}
					// 只在向量索引明显慢时才警告
					if vectorTime > 2*time.Second {
						refineConfig.AnalyzeLog("SLOW VECTOR INDEX: hop=%s took %v", currentHop.String()[:min(50, len(currentHop.String()))], vectorTime)
					}
				}()

				if refineConfig.Ctx != nil {
					select {
					case <-refineConfig.Ctx.Done():
						return
					default:
					}
				}

				if !refineConfig.AllowMultiHopAIRefine {
					return
				}

				// 知识条目构建
				knowledgeStart := time.Now()
				entries, err := BuildKnowledgeEntryFromKHop(currentHop, kb, option...)
				knowledgeTime := time.Since(knowledgeStart)
				atomic.AddInt64(&knowledgeEntryDuration, int64(knowledgeTime))

				if err != nil {
					refineConfig.AnalyzeLog("failed to build knowledge entry: %v", err)
					return
				}

				// 只在知识构建明显慢时才警告
				if knowledgeTime > 10*time.Second {
					refineConfig.AnalyzeLog("SLOW KNOWLEDGE BUILD: hop=%s took %v, entries=%d",
						currentHop.String()[:min(50, len(currentHop.String()))], knowledgeTime, len(entries))
				}

				for _, entry := range entries {
					output.SafeFeed(entry)
				}

				count := atomic.AddInt64(&done, 1)
				refineConfig.AnalyzeStatusCard("[multi-hops]: knowledge", fmt.Sprintf("%v/%v", count, currentTotal))

				// 保存知识条目
				saveStart := time.Now()
				err = SaveKnowledgeEntries(kb, entries, currentHop.GetRelatedEntityUUIDs(), option...)
				saveTime := time.Since(saveStart)
				atomic.AddInt64(&saveDuration, int64(saveTime))

				if err != nil {
					refineConfig.AnalyzeLog("failed to save knowledge entries: %v", err)
					return
				}

				// 只在保存明显慢时才警告
				if saveTime > 3*time.Second {
					refineConfig.AnalyzeLog("SLOW KNOWLEDGE SAVE: hop=%s took %v, entries=%d",
						currentHop.String()[:min(50, len(currentHop.String()))], saveTime, len(entries))
				}
			}(hop)
		}

		// 最终统计：只在性能异常时详细输出
		finalElapsed := time.Since(processingStartTime)
		finalTotal := atomic.LoadInt64(&total)
		finalDone := atomic.LoadInt64(&done)
		finalRate := float64(finalTotal) / finalElapsed.Seconds()

		// 判断是否需要详细的最终统计
		if finalRate < 5.0 || finalElapsed > 10*time.Minute || finalTotal > 1000 {
			if finalTotal == 0 {
				finalTotal = 1
			}
			// 性能差或处理数量大时输出详细统计
			avgVectorTime := time.Duration(atomic.LoadInt64(&vectorIndexDuration)) / time.Duration(finalTotal)
			avgKnowledgeTime := time.Duration(atomic.LoadInt64(&knowledgeEntryDuration)) / time.Duration(int(math.Max(1, float64(finalDone))))
			avgSaveTime := time.Duration(atomic.LoadInt64(&saveDuration)) / time.Duration(int(math.Max(1, float64(finalDone))))

			refineConfig.AnalyzeLog("MULTI-HOP COMPLETED [detailed]: total=%d, completed=%d, elapsed=%v, rate=%.1f hops/s, avg_vector=%v, avg_knowledge=%v, avg_save=%v",
				finalTotal, finalDone, finalElapsed, finalRate, avgVectorTime, avgKnowledgeTime, avgSaveTime)
		} else {
			// 性能良好时简单输出
			refineConfig.AnalyzeLog("Multi-hop completed: %d hops processed in %v (%.1f hops/s)",
				finalTotal, finalElapsed, finalRate)
		}
	}()

	return output.OutputChannel(), nil
}

func BuildKnowledgeEntryFromKHop(hop *entityrepos.KHopPath, kb *knowledgebase.KnowledgeBase, option ...any) ([]*schema.KnowledgeBaseEntry, error) {
	refineConfig := NewRefineConfig(option...)

	input := hop.String()
	query, err := LiteForgeQueryFromChunk(refineERMPrompt, refineConfig.ExtraPrompt, chunkmaker.NewBufferChunk([]byte(input)), 200)
	if err != nil {
		return nil, err
	}

	refineResult, err := _executeLiteForgeTemp(query, refineConfig.ForgeExecOption(refineSchema)...)
	if err != nil {
		return nil, err
	}

	entries, err := Action2RagKnowledgeEntries(refineResult.Action, 0)
	if err != nil {
		log.Errorf("failed to convert action to knowledge base entries: %v", err)
		return nil, err
	}
	return entries, nil
}

func SaveKnowledgeEntries(kb *knowledgebase.KnowledgeBase, entries []*schema.KnowledgeBaseEntry, relationalEntityUUID []string, options ...any) error { //todo return knowledge uuid
	documentOption := []rag.DocumentOption{rag.WithDocumentRelatedEntities(relationalEntityUUID...)}

	refineConfig := NewRefineConfig(options...)
	for _, entry := range entries {
		entry.KnowledgeBaseID = kb.GetID()
		entry.RelatedEntityUUIDS = strings.Join(relationalEntityUUID, ",")
		if len(entry.KnowledgeDetails) <= 0 {
			continue
		}
		if len(entry.KnowledgeDetails) > refineConfig.KnowledgeEntryLength {
			detailList, err := SplitTextSafe(entry.KnowledgeDetails, refineConfig.KnowledgeEntryLength, options...)
			if err != nil {
				return utils.Errorf("fail to split knowledge details: %v", err)
			}
			for _, detail := range detailList {
				err := kb.AddKnowledgeEntry(&schema.KnowledgeBaseEntry{
					KnowledgeBaseID:    entry.KnowledgeBaseID,
					KnowledgeTitle:     entry.KnowledgeTitle,
					KnowledgeType:      entry.KnowledgeType,
					KnowledgeDetails:   detail,
					Summary:            entry.Summary,
					Keywords:           entry.Keywords,
					ImportanceScore:    entry.ImportanceScore,
					RelatedEntityUUIDS: entry.RelatedEntityUUIDS,
				}, documentOption...)
				if err != nil {
					return utils.Errorf("failed to create knowledge base entry: %v", err)
				}
			}
		} else {
			err := kb.AddKnowledgeEntry(entry, documentOption...)
			if err != nil {
				return utils.Errorf("failed to create knowledge base entry: %v", err)
			}
		}
	}
	return nil
}

func BuildKnowledgeEntryFromEntityRepos(name string, option ...any) (<-chan *schema.KnowledgeBaseEntry, error) {
	refineConfig := NewRefineConfig(option...)
	refineConfig.AnalyzeLog("start build knowledge from entity repository use default qc")

	er, err := entityrepos.GetEntityRepositoryByName(refineConfig.Database, name, option)
	if err != nil {
		return nil, utils.Errorf("failed to load entity repository: %v", err)
	}
	kb, err := knowledgebase.NewKnowledgeBase(refineConfig.Database, name, refineConfig.KnowledgeBaseDesc, refineConfig.KnowledgeBaseType)
	if err != nil {
		return nil, utils.Errorf("failed to create knowledge base: %v", err)
	}

	var knowledgeCount *int64 = new(int64)
	var kHopCount *int64 = new(int64)
	var finishedKHopCount *int64 = new(int64)
	throttle := utils.NewThrottle(1)
	updateEntityGraphStatus := func() {
		throttle(func() {
			refineConfig.AnalyzeStatusCard(
				"知识条目/多跳知识片(KnowledgeEntries/KHop)",
				fmt.Sprintf("%d/%d",
					atomic.LoadInt64(knowledgeCount),
					atomic.LoadInt64(kHopCount),
				))
		})
	}

	output := chanx.NewUnlimitedChan[*schema.KnowledgeBaseEntry](refineConfig.Ctx, 100)

	go func() {
		hopAnalyzeWg := utils.NewSizedWaitGroup(refineConfig.AnalyzeConcurrency)
		defer output.Close()
		defer hopAnalyzeWg.Wait()

		for hop := range er.YieldKHop(refineConfig.Ctx, refineConfig.KHopOption()...) {
			atomic.AddInt64(kHopCount, 1)
			updateEntityGraphStatus()
			hopAnalyzeWg.Add(1)
			if refineConfig.Ctx != nil && refineConfig.Ctx.Err() != nil {
				break
			}
			go func() {
				defer hopAnalyzeWg.Done()
				go func() {
					err := er.AddKHopToVectorIndex(hop)
					if err != nil {
						refineConfig.AnalyzeLog("failed to add khop to vector index: %v", err)
					}
				}()

				if refineConfig.Ctx != nil {
					select {
					case <-refineConfig.Ctx.Done():
						return
					default:
					}
				}

				entries, err := BuildKnowledgeEntryFromKHop(hop, kb, option...)
				if err != nil {
					refineConfig.AnalyzeLog("failed to build knowledge entry: %v", err)
					return
				}

				for _, entry := range entries {
					output.SafeFeed(entry)
					atomic.AddInt64(knowledgeCount, 1)
					updateEntityGraphStatus()
				}
				atomic.AddInt64(finishedKHopCount, 1)
				refineConfig.AnalyzeStatusCard("多跳知识进度(finished/total)", fmt.Sprintf("%d/%d", atomic.LoadInt64(finishedKHopCount), atomic.LoadInt64(kHopCount)))

				err = SaveKnowledgeEntries(kb, entries, hop.GetRelatedEntityUUIDs(), option...)
				if err != nil {
					refineConfig.AnalyzeLog("failed to save knowledge entries: %v", err)
					return
				}
			}()
		}
	}()

	return output.OutputChannel(), nil
}
