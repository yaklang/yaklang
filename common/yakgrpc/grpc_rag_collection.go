package yakgrpc

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) RAGCollectionSearch(req *ypb.RAGCollectionSearchRequest, stream ypb.Yak_RAGCollectionSearchServer) (err error) {
	ctx := stream.Context()
	options := parseOption(req)
	options = append(options, rag.WithRAGCtx(ctx))
	db := s.GetProfileDatabase()

	feedbackSearchResult := func(result aicommon.EnhanceKnowledge) {
		docType := schema.RAGDocumentType(result.GetType())
		uuid := result.GetUUID()

		var feedMessage = &ypb.RAGCollectionSearchResponse{
			Type:        result.GetType(),
			EnhancePlan: result.GetScoreMethod(),
			Similarity:  float32(result.GetScore()),
		}
		switch docType {
		case schema.RAGDocumentType_Knowledge:
			if uuid == "" {
				log.Errorf("Document missing UUID")
				return
			}
			knowledge, err := yakit.GetKnowledgeBaseEntryByUUID(db, uuid)
			if err != nil {
				log.Errorf("GetKnowledgeEntry failed: %s", err)
				return
			}

			feedMessage.Knowledge = KnowledgeBaseEntryToGrpcModel(knowledge)
		case schema.RAGDocumentType_Entity:
			if uuid == "" {
				log.Errorf("Document missing UUID")
				return
			}
			entity, err := yakit.GetEntityByIndex(db, uuid)
			if err != nil {
				log.Errorf("GetKnowledgeEntry failed: %s", err)
				return
			}

			feedMessage.Entity = entity.ToGRPC()
		case schema.RAGDocumentType_Relationship:
			if uuid == "" {
				log.Errorf("Document missing UUID")
				return
			}
			relationship, err := yakit.GetRelationshipByUUID(db, uuid)
			if err != nil {
				log.Errorf("GetKnowledgeEntry failed: %s", err)
				return
			}

			feedMessage.Relationship = relationship.ToGRPC()
		case schema.RAGDocumentType_KHop:
			feedMessage.KhopPath = result.GetContent()
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

	enhanceKnowledgeManager := rag.NewRagEnhanceKnowledgeManagerWithOptions(options...)

	resultCh, err := enhanceKnowledgeManager.FetchKnowledge(ctx, req.Query)
	if err != nil {
		return err
	}
	for result := range resultCh {
		feedbackSearchResult(result)
	}
	return nil
}

func parseOption(req *ypb.RAGCollectionSearchRequest) []rag.RAGSystemConfigOption {
	var options []rag.RAGSystemConfigOption
	if req.GetCollectionName() != "" {
		options = append(options, rag.WithRAGCollectionName(req.GetCollectionName()))
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
