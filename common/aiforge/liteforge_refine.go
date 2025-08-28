package aiforge

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed liteforge_schema/liteforge_refine.schema.json
var refineSchema string

//go:embed liteforge_prompt/liteforge_refine_prompt.txt
var refinePrompt string

func Refine(path string, option ...any) (*knowledgebase.KnowledgeBase, error) {
	refineConfig := NewAnalysisConfig(option...)

	refineConfig.AnalyzeLog("analyze video: %s", path)
	analyzeResult, err := AnalyzeFile(path, option...)
	if err != nil {
		return nil, utils.Errorf("failed to start analyze video: %v", err)
	}

	return RefineEx(analyzeResult, consts.GetGormProfileDatabase(), option...)
}

func RefineEx(input <-chan AnalysisResult, db *gorm.DB, options ...any) (*knowledgebase.KnowledgeBase, error) {
	refineConfig := NewRefineConfig(options...)
	knowledgeDatabaseName := refineConfig.KnowledgeBaseName

	refineConfig.AnalyzeStatusCard("Refine", "creating knowledge base")
	kb, err := knowledgebase.NewKnowledgeBase(db, knowledgeDatabaseName, refineConfig.KnowledgeBaseDesc, refineConfig.KnowledgeBaseType)
	if err != nil {
		return nil, utils.Errorf("fial to create knowledgDatabase: %v", err)
	}
	baseInfo, err := kb.GetInfo()
	if err != nil {
		return nil, utils.Errorf("failed to get knowledge base info: %v", err)
	}

	refineConfig.AnalyzeLog("knowledge base created: %s", knowledgeDatabaseName)
	refineConfig.AnalyzeStatusCard("Refine", "creating knowledge base collection")

	count := 0
	wg := sync.WaitGroup{}
	startOnce := &sync.Once{}

	for v := range input {
		startOnce.Do(func() {
			refineConfig.AnalyzeStatusCard("Refine", "refining chunks")
		})
		count++
		knowledgeRawData := v.Dump()
		if len(knowledgeRawData) <= 0 {
			log.Errorf("no knowledge data could be converted")
			refineConfig.AnalyzeLog("skip refine chunk [%d]: no knowledge data could be converted", count)
			continue
		}
		query := fmt.Sprintf("%s\n ```main_analysis\n%s\n``` \nextract prompt:\n%s ", refinePrompt, knowledgeRawData, refineConfig.RefinePrompt)
		refineResult, err := _executeLiteForgeTemp(query, append(refineConfig.fallbackOptions, _withOutputJSONSchema(refineSchema))...)
		if err != nil {
			if refineConfig.Strict {
				return nil, utils.Errorf("failed to execute liteforge: %v", err)
			}
			refineConfig.AnalyzeLog("refine chunk [%d] failed: %v", count, err)
			log.Errorf("refine chunk [%d] failed: %v", count, err)
			continue
		}
		refineConfig.AnalyzeStatusCard("refine chunk count", count)
		wg.Add(1)
		go func() {
			defer wg.Done()
			entries, err := Action2RagKnowledgeEntries(refineResult.Action, int64(baseInfo.ID))
			if err != nil {
				log.Errorf("failed to convert action to knowledge base entries: %v", err)
				return
			}
			for _, entry := range entries {
				if len(entry.KnowledgeDetails) <= 0 {
					continue
				}
				if len(entry.KnowledgeDetails) > refineConfig.KnowledgeEntryLength {
					detailList, err := SplitTextSafe(entry.KnowledgeDetails, refineConfig.KnowledgeEntryLength, options...)
					if err != nil {
						return
					}
					for _, detail := range detailList {
						err := kb.AddKnowledgeEntry(&schema.KnowledgeBaseEntry{
							KnowledgeBaseID:  entry.KnowledgeBaseID,
							KnowledgeTitle:   entry.KnowledgeTitle,
							KnowledgeType:    entry.KnowledgeType,
							KnowledgeDetails: detail,
							Summary:          entry.Summary,
							Keywords:         entry.Keywords,
						})
						if err != nil {
							log.Errorf("failed to create knowledge base entry: %v", err)
							return
						}
					}
				} else {
					err := kb.AddKnowledgeEntry(entry)
					if err != nil {
						log.Errorf("failed to create knowledge base entry: %v", err)
						return
					}
				}
			}
		}()
	}
	wg.Wait()
	return kb, nil
}

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
