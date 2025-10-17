package yakgrpc

import (
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
)

func (s *Server) RAGCollectionSearch(req *ypb.RAGCollectionSearchRequest, stream ypb.Yak_RAGCollectionSearchServer) (err error) {
	ctx := stream.Context()
	options := parseOption(req)
	options = append(options, rag.WithRAGCtx(ctx))
	db := s.GetProfileDatabase()

	wg := sync.WaitGroup{}

	feedbackSearchResult := func(result *rag.ScoredResult) {
		document := result.Document
		if document == nil {
			return
		}
		docType := document.Type

		var feedMessage = &ypb.RAGCollectionSearchResponse{
			Type:        string(docType),
			EnhancePlan: result.QueryMethod,
			Similarity:  float32(result.Score),
		}
		switch docType {
		case schema.RAGDocumentType_Knowledge:
			uuid, ok := document.Metadata.GetDataUUID()
			if !ok {
				log.Errorf("Document missing DataUUID in metadata")
				return
			}
			knowledge, err := yakit.GetKnowledgeBaseEntryByUUID(db, uuid)
			if err != nil {
				log.Errorf("GetKnowledgeEntry failed: %s", err)
				return
			}

			feedMessage.Knowledge = KnowledgeBaseEntryToGrpcModel(knowledge)
		case schema.RAGDocumentType_Entity:
			uuid, ok := document.Metadata.GetDataUUID()
			if !ok {
				log.Errorf("Document missing DataUUID in metadata")
				return
			}
			entity, err := yakit.GetEntityByIndex(db, uuid)
			if err != nil {
				log.Errorf("GetKnowledgeEntry failed: %s", err)
				return
			}

			feedMessage.Entity = entity.ToGRPC()
		case schema.RAGDocumentType_Relationship:
			uuid, ok := document.Metadata.GetDataUUID()
			if !ok {
				log.Errorf("Document missing DataUUID in metadata")
				return
			}
			relationship, err := yakit.GetRelationshipByUUID(db, uuid)
			if err != nil {
				log.Errorf("GetKnowledgeEntry failed: %s", err)
				return
			}

			feedMessage.Relationship = relationship.ToGRPC()
		case schema.RAGDocumentType_KHop:
			feedMessage.KhopPath = document.Content
		default:
			log.Errorf("Unknown document type %s", docType)
			return
		}
		err := stream.Send(feedMessage)
		if err != nil {
			log.Errorf("stream.Send failed: %s", err)
			return
		}
	}

	options = append(options, rag.WithEveryQueryResultCallback(func(result *rag.ScoredResult) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			feedbackSearchResult(result)
		}()
	}))

	result, err := rag.Query(s.GetProfileDatabase(), req.Query, options...)
	if err != nil {
		return err
	}
	for r := range result {
		log.Debugf("Query result: %v", r)
	}
	wg.Wait()
	return nil
}

func parseOption(req *ypb.RAGCollectionSearchRequest) []rag.RAGQueryOption {
	var options []rag.RAGQueryOption
	if req.GetCollectionName() != "" {
		options = append(options, rag.WithRAGCollectionNames(req.GetCollectionName()))
	}

	if req.GetLimit() > 0 {
		options = append(options, rag.WithRAGLimit(int(req.GetLimit())))
	}

	if len(req.GetEnhancePlan()) > 0 {
		options = append(options, rag.WithRAGEnhance(req.GetEnhancePlan()...))
	}

	if len(req.GetDocumentType()) > 0 {
		options = append(options, rag.WithRAGDocumentType(req.GetDocumentType()...))
	}

	if req.GetSimilarityThreshold() > 0 {
		options = append(options, rag.WithRAGSimilarityThreshold(float64(req.GetSimilarityThreshold())))
	}

	if req.GetConcurrency() > 0 {
		options = append(options, rag.WithRAGConcurrent(int(req.GetConcurrency())))
	}

	return options
}
