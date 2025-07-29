package yakgrpc

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//   // Knowledge Base
//   rpc GetKnowledgeBaseNameList(Empty) returns(GetKnowledgeBaseNameListResponse);
//   rpc DeleteKnowledgeBase(DeleteKnowledgeBaseRequest) returns(DbOperateMessage);
//   rpc CreateKnowledgeBase(CreateKnowledgeBaseRequest) returns(DbOperateMessage);
//   rpc UpdateKnowledgeBase(UpdateKnowledgeBaseRequest) returns(DbOperateMessage);

// rpc DeleteKnowledgeBaseEntry(DeleteKnowledgeBaseEntryRequest) returns(DbOperateMessage);
// rpc CreateKnowledgeBaseEntry(CreateKnowledgeBaseEntryRequest) returns(DbOperateMessage);
// rpc UpdateKnowledgeBaseEntry(UpdateKnowledgeBaseEntryRequest) returns(DbOperateMessage);
// rpc SearchKnowledgeBaseEntry(SearchKnowledgeBaseEntryRequest) returns(SearchKnowledgeBaseEntryResponse);
func (s *Server) GetKnowledgeBaseNameList(ctx context.Context, req *ypb.Empty) (*ypb.GetKnowledgeBaseNameListResponse, error) {
	db := consts.GetGormProfileDatabase()
	knowledgeBaseNameList, err := yakit.GetKnowledgeBaseNameList(db)
	if err != nil {
		return nil, err
	}
	response := &ypb.GetKnowledgeBaseNameListResponse{
		KnowledgeBaseNames: knowledgeBaseNameList,
	}
	return response, nil
}

func (s *Server) DeleteKnowledgeBase(ctx context.Context, req *ypb.DeleteKnowledgeBaseRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()
	err := yakit.DeleteKnowledgeBase(db, req.KnowledgeBaseId)
	if err != nil {
		return nil, err
	}
	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) CreateKnowledgeBase(ctx context.Context, req *ypb.CreateKnowledgeBaseRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()
	err := yakit.CreateKnowledgeBase(db, &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        req.GetKnowledgeBaseName(),
		KnowledgeBaseDescription: req.GetKnowledgeBaseDescription(),
		KnowledgeBaseType:        req.GetKnowledgeBaseType(),
	})
	if err != nil {
		return nil, err
	}
	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) UpdateKnowledgeBase(ctx context.Context, req *ypb.UpdateKnowledgeBaseRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()
	err := yakit.UpdateKnowledgeBaseInfo(db, req.GetKnowledgeBaseId(), &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        req.GetKnowledgeBaseName(),
		KnowledgeBaseDescription: req.GetKnowledgeBaseDescription(),
		KnowledgeBaseType:        req.GetKnowledgeBaseType(),
	})
	if err != nil {
		return nil, err
	}
	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) DeleteKnowledgeBaseEntry(ctx context.Context, req *ypb.DeleteKnowledgeBaseEntryRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()
	err := yakit.DeleteKnowledgeBaseEntry(db, req.GetKnowledgeBaseEntryId())
	if err != nil {
		return nil, err
	}
	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) CreateKnowledgeBaseEntry(ctx context.Context, req *ypb.CreateKnowledgeBaseEntryRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()
	err := yakit.CreateKnowledgeBaseEntry(db, &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:          req.GetKnowledgeBaseID(),
		KnowledgeTitle:           req.GetKnowledgeTitle(),
		KnowledgeType:            req.GetKnowledgeType(),
		ImportanceScore:          int(req.GetImportanceScore()),
		Keywords:                 req.GetKeywords(),
		KnowledgeDetails:         req.GetKnowledgeDetails(),
		Summary:                  req.GetSummary(),
		SourcePage:               int(req.GetSourcePage()),
		PotentialQuestions:       req.GetPotentialQuestions(),
		PotentialQuestionsVector: req.GetPotentialQuestionsVector(),
	})
	if err != nil {
		return nil, err
	}
	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) UpdateKnowledgeBaseEntry(ctx context.Context, req *ypb.UpdateKnowledgeBaseEntryRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()
	err := yakit.UpdateKnowledgeBaseEntry(db, &schema.KnowledgeBaseEntry{
		Model:              gorm.Model{ID: uint(req.GetKnowledgeBaseEntryID())},
		KnowledgeTitle:     req.GetKnowledgeTitle(),
		KnowledgeType:      req.GetKnowledgeType(),
		ImportanceScore:    int(req.GetImportanceScore()),
		Keywords:           req.GetKeywords(),
		KnowledgeDetails:   req.GetKnowledgeDetails(),
		Summary:            req.GetSummary(),
		SourcePage:         int(req.GetSourcePage()),
		PotentialQuestions: req.GetPotentialQuestions(),
	})
	if err != nil {
		return nil, err
	}
	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) SearchKnowledgeBaseEntry(ctx context.Context, req *ypb.SearchKnowledgeBaseEntryRequest) (*ypb.SearchKnowledgeBaseEntryResponse, error) {
	reqPagination := req.GetPagination()
	db := consts.GetGormProfileDatabase()
	_, entries, err := yakit.GetKnowledgeBaseEntryByFilter(db, req.GetKnowledgeBaseId(), req.GetKeyword(), reqPagination)
	if err != nil {
		return nil, err
	}

	return &ypb.SearchKnowledgeBaseEntryResponse{
		Total:                int64(len(entries)),
		Pagination:           reqPagination,
		KnowledgeBaseEntries: KnowledgeBaseEntryListToGrpcModel(entries),
	}, nil
}

func KnowledgeBaseEntryListToGrpcModel(entries []*schema.KnowledgeBaseEntry) []*ypb.KnowledgeBaseEntry {
	pbEntries := make([]*ypb.KnowledgeBaseEntry, len(entries))
	for i, entry := range entries {
		pbEntries[i] = KnowledgeBaseEntryToGrpcModel(entry)
	}
	return pbEntries
}

func KnowledgeBaseEntryToGrpcModel(entry *schema.KnowledgeBaseEntry) *ypb.KnowledgeBaseEntry {
	return &ypb.KnowledgeBaseEntry{
		ID:                       int64(entry.ID),
		KnowledgeBaseId:          entry.KnowledgeBaseID,
		KnowledgeTitle:           entry.KnowledgeTitle,
		KnowledgeType:            entry.KnowledgeType,
		ImportanceScore:          int32(entry.ImportanceScore),
		Keywords:                 entry.Keywords,
		KnowledgeDetails:         entry.KnowledgeDetails,
		Summary:                  entry.Summary,
		SourcePage:               int32(entry.SourcePage),
		PotentialQuestions:       entry.PotentialQuestions,
		PotentialQuestionsVector: entry.PotentialQuestionsVector,
	}
}

func (s *Server) BuildVectorIndexForKnowledgeBase(ctx context.Context, req *ypb.BuildVectorIndexForKnowledgeBaseRequest) (*ypb.GeneralResponse, error) {
	opts := []any{}
	aiOptions := []aispec.AIConfigOption{
		aispec.WithBaseURL(req.GetBaseUrl()),
		aispec.WithAPIKey(req.GetApiKey()),
		aispec.WithModel(req.GetModelName()),
		aispec.WithProxy(req.GetProxy()),
	}
	ragOpts := []rag.RAGOption{
		rag.WithEmbeddingModel(req.GetModelName()),
		rag.WithModelDimension(int(req.GetDimension())),
		rag.WithHNSWParameters(int(req.GetM()), float64(req.GetMl()), int(req.GetEfSearch()), int(req.GetEfConstruct())),
	}

	switch req.GetDistanceFuncType() {
	case "cosine":
		ragOpts = append(ragOpts, rag.WithCosineDistance())
	default:
		return nil, utils.Errorf("invalid distance function type: %s", req.GetDistanceFuncType())
	}
	for _, opt := range aiOptions {
		opts = append(opts, opt)
	}
	for _, opt := range ragOpts {
		opts = append(opts, opt)
	}
	err := rag.BuildVectorIndexForKnowledgeBase(consts.GetGormProfileDatabase(), req.GetKnowledgeBaseId(), opts...)
	if err != nil {
		return nil, err
	}
	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}
