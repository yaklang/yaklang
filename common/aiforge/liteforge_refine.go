package aiforge

import (
	_ "embed"
	"fmt"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"sync"
)

//go:embed liteforge_refine.schema.json
var refineSchema string

//go:embed liteforge_refine_prompt.txt
var refinePrompt string

func RefineVideo(path string, option ...any) (*rag.RAGSystem, error) {
	refineConfig := NewAnalysisConfig(option...)

	knowledgeDatabaseName := path + uuid.New().String()
	newKnowledgeBase := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName: knowledgeDatabaseName,
	}

	db := consts.GetGormProfileDatabase()

	refineConfig.AnalyzeStatusCard("Video Refine", "creating knowledge base")
	err := yakit.CreateKnowledgeBase(db, newKnowledgeBase)
	if err != nil {
		return nil, utils.Errorf("fial to create knowledgDatabase: %v", err)
	}

	refineConfig.AnalyzeLog("knowledge base created: %s", knowledgeDatabaseName)
	refineConfig.AnalyzeStatusCard("Video Refine", "creating knowledge base collection")
	collection, err := rag.CreateCollection(db, newKnowledgeBase.KnowledgeBaseName, "video_refine")
	if err != nil {
		return nil, err
	}

	chunkChan := chanx.NewUnlimitedChan[chunkmaker.Chunk](refineConfig.Ctx, 100)
	cm, err := chunkmaker.NewSimpleChunkMaker(chunkChan, chunkmaker.WithCtx(refineConfig.Ctx))
	if err != nil {
		return nil, err
	}
	analyzeOption := append(option, WithAnalyzeStreamChunkCallback(func(chunk chunkmaker.Chunk) {
		chunkChan.SafeFeed(chunk)
	}))

	go func() {
		defer chunkChan.Close()
		refineConfig.AnalyzeLog("analyze video: %s", path)
		videoResult, err := AnalyzeVideo(path, analyzeOption...)
		if err != nil {
			log.Errorf(err.Error())
		}
		refineConfig.AnalyzeStatusCard("refine chunk total", len(videoResult.ImageSegments))
	}()

	wg := sync.WaitGroup{}

	count := 0

	ar, err := aireducer.NewReducerEx(cm,
		aireducer.WithContext(refineConfig.Ctx),
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.Memory, chunk chunkmaker.Chunk) error {
			count++
			refineConfig.AnalyzeStatusCard("refine chunk count", count)

			knowledgeRawData := chunk.Data()
			query := fmt.Sprintf("%s\n ```main_analysis\n%s\n``` ", refinePrompt, knowledgeRawData)

			refineResult, err := _executeLiteForgeTemp(query, append(refineConfig.fallbackOptions, _withOutputJSONSchema(refineSchema))...)
			if err != nil {
				return err
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				entries, err := Action2RagKnowledgeEntries(refineResult.Action, int64(newKnowledgeBase.ID))
				if err != nil {
					log.Errorf("failed to convert action to knowledge base entries: %v", err)
					return
				}
				for _, entry := range entries {
					if len(entry.KnowledgeDetails) <= 0 {
						continue
					}
					if len(entry.KnowledgeDetails) > 1200 {
						entry.KnowledgeDetails = utils.ShrinkString(entry.KnowledgeDetails, 1200)
					}
					err := yakit.CreateKnowledgeBaseEntry(db, entry)
					if err != nil {
						log.Errorf("failed to create knowledge base entry: %v", err)
						return
					}

					err = collection.Add(entry.KnowledgeTitle, entry.KnowledgeDetails)
					if err != nil {
						log.Errorf("failed to add knowledge title to knowledge: %v", err)
						return
					}
				}
				refineConfig.AnalyzeLog("analyze chunk [%d]", count)
			}()
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}
	refineConfig.AnalyzeStatusCard("Video Refine", "refining video chunks")
	err = ar.Run()
	if err != nil {
		return nil, err
	}

	wg.Wait()

	return collection, nil
}

func Action2RagKnowledgeEntries(
	action *aid.Action,
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
