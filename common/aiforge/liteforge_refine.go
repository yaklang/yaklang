package aiforge

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"strings"
	"sync/atomic"

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
		for hop := range er.YieldKHop(refineConfig.Ctx, append(refineConfig.KHopOption(), entityrepos.WithRuntimeBuildOnly(true))...) {
			atomic.AddInt64(&total, 1)
			refineConfig.AnalyzeStatusCard("多跳知识构建(Multi-Hops Knowledge)", total)
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

				if !refineConfig.AllowMultiHopAIRefine {
					return
				}

				entries, err := BuildKnowledgeEntryFromKHop(hop, kb, option...)
				if err != nil {
					refineConfig.AnalyzeLog("failed to build knowledge entry: %v", err)
					return
				}

				for _, entry := range entries {
					output.SafeFeed(entry)
				}

				count := atomic.AddInt64(&done, 1)
				refineConfig.AnalyzeStatusCard("[multi-hops]: knowledge", fmt.Sprintf("%v/%v", count, total))

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

func BuildKnowledgeEntryFromKHop(hop *entityrepos.KHopPath, kb *knowledgebase.KnowledgeBase, option ...any) ([]*schema.KnowledgeBaseEntry, error) {
	refineConfig := NewRefineConfig(option...)

	input := hop.String()
	query, err := LiteForgeQueryFromChunk(refineERMPrompt, refineConfig.ExtraPrompt, chunkmaker.NewBufferChunk([]byte(input)), 200)
	if err != nil {
		return nil, err
	}

	refineResult, err := _executeLiteForgeTemp(query, append(refineConfig.fallbackOptions, WithOutputJSONSchema(refineSchema))...)
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
