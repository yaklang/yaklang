package knowledgalchemist

import (
	"context"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/aiforge/aibp"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
)

type KnowledgeAlchemist struct {
	standards           string // standards for knowledge extraction.
	TimeTriggerInterval time.Duration
	ChunkSize           int64
	SeparatorTrigger    string

	ExtendAIDOption []aid.Option
	Concurrent      int
}

type Option func(*KnowledgeAlchemist)

func NewKnowledgeAlchemist(opts ...Option) *KnowledgeAlchemist {
	ka := &KnowledgeAlchemist{
		TimeTriggerInterval: 0,
		ChunkSize:           1024 * 10, // default chunk size
		Concurrent:          20,
	}
	for _, opt := range opts {
		opt(ka)
	}
	return ka
}

func (ka *KnowledgeAlchemist) Refine(ctx context.Context, db *gorm.DB, path string) (*rag.RAGSystem, error) {
	refineForge, err := aibp.NewKnowledgeRefineForge()
	if err != nil {
		return nil, err
	}
	knowledgeDatabaseName := path + uuid.New().String()
	newKnowledgeBase := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName: knowledgeDatabaseName,
	}
	swg := utils.NewSizedWaitGroup(ka.Concurrent)
	index := 0
	rd, err := aireducer.NewReducerFromFile(
		path,
		aireducer.WithTimeTriggerInterval(ka.TimeTriggerInterval),
		aireducer.WithChunkSize(ka.ChunkSize),
		aireducer.WithSeparatorTrigger(ka.SeparatorTrigger),
		aireducer.WithContext(ctx),
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.Memory, chunk chunkmaker.Chunk) error {
			currentIndex := index
			log.Infof("chunk index [%d]: start refine knowledge type: %s", currentIndex, chunk.MIMEType().String())
			swg.Add(1)
			go func() {
				defer swg.Done()
				var res *aiforge.ForgeResult
				if chunk.MIMEType().IsImage() {
					res, err = refineForge.ExecuteEx(ctx, nil, []*aid.ImageData{{
						Data:     chunk.Data(),
						IsBase64: false,
					}}, ka.ExtendAIDOption...)
					if err != nil {
						log.Errorf("chunk index [%d]: failed to execute refine for knowledge: %v with image type", currentIndex, err)
						return
					}
				} else {
					res, err = refineForge.ExecuteEx(ctx, []*ypb.ExecParamItem{{
						Key:   "data",
						Value: string(chunk.Data()),
					}}, nil, ka.ExtendAIDOption...)
					if err != nil {
						log.Errorf("chunk index [%d]: failed to execute refine for knowledge: %v with raw text type", currentIndex, err)
						return
					}
				}

				entries, err := ResultAction2KnowledgeBaseEntries(res.Action, int64(newKnowledgeBase.ID))
				if err != nil {
					log.Errorf("chunk index [%d]: failed to convert result action to knowledge base entries: %v", currentIndex, err)
					return
				}

				log.Infof("chunk index [%d]: successfully refined knowledge type: %s, entries count: %d", currentIndex, chunk.MIMEType().String(), len(entries))

				for _, entry := range entries {
					err := yakit.CreateKnowledgeBaseEntry(db, entry)
					if err != nil {
						log.Errorf("chunk index [%d]: failed to create knowledgeDatabase: %v", currentIndex, err)
					}
				}
			}()
			index++
			return nil
		}),
	)
	if err != nil {
		return nil, utils.Errorf("fial to create ai reducer: %v", err)
	}

	err = yakit.CreateKnowledgeBase(db, newKnowledgeBase)
	if err != nil {
		return nil, utils.Errorf("fial to create knowledgDatabase: %v", err)
	}

	err = rd.Run()
	if err != nil {
		return nil, err
	}

	yakit.CreateCollection
	return nil, nil
}

func ResultAction2KnowledgeBaseEntries(
	result *aid.Action,
	knowledgeBaseID int64,
) ([]*schema.KnowledgeBaseEntry, error) {
	if result == nil {
		return nil, utils.Errorf("result is nil")
	}

	// "knowledge-collection" 是一个数组，每个元素是一个知识条目
	collection := result.GetInvokeParamsArray("knowledge-collection")
	if len(collection) == 0 {
		return nil, utils.Errorf("no knowledge-collection found in action")
	}

	entries := make([]*schema.KnowledgeBaseEntry, 0, len(collection))
	for _, item := range collection {
		entry := &schema.KnowledgeBaseEntry{
			KnowledgeBaseID:    knowledgeBaseID,
			KnowledgeTitle:     item.GetString("knowledge_title"),
			KnowledgeType:      item.GetString("knowledge_type"),
			ImportanceScore:    item.GetInteger("importanceScore"),
			Keywords:           schema.StringArray(item.GetStringSlice("keywords")),
			KnowledgeDetails:   item.GetString("knowledge_details"),
			Summary:            item.GetString("summary"),
			SourcePage:         item.GetInteger("source_page"),
			PotentialQuestions: schema.StringArray(item.GetStringSlice("potential_questions")),
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
