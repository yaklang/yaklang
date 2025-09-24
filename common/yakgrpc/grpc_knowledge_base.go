package yakgrpc

import (
	"context"
	"encoding/json"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

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

// GetKnowledgeBase 获取知识库信息和条目列表
func (s *Server) GetKnowledgeBase(ctx context.Context, req *ypb.GetKnowledgeBaseRequest) (*ypb.GetKnowledgeBaseResponse, error) {
	db := consts.GetGormProfileDatabase()
	var knowledgeBases []*schema.KnowledgeBaseInfo
	query := db.Model(&schema.KnowledgeBaseInfo{})

	// 实现关键词和ID的二选一逻辑
	if req.GetKeyword() != "" {
		// 如果关键词不为空，忽略ID，使用关键词搜索
		query = bizhelper.FuzzSearchEx(query, []string{"knowledge_base_name", "knowledge_base_description"}, req.GetKeyword(), false)
	} else if req.GetKnowledgeBaseId() > 0 {
		// 如果ID不为空，搜索指定ID
		query = query.Where("id = ?", req.GetKnowledgeBaseId())
	}
	// 如果都为空，进行分页搜索（无过滤条件）

	// 使用 bizhelper.Paging 实现分页功能
	pagination := req.GetPagination()
	page := 1
	limit := 10
	if pagination != nil {
		page = int(pagination.GetPage())
		limit = int(pagination.GetLimit())
	}
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}

	p, db := bizhelper.Paging(query, page, limit, &knowledgeBases)
	if db.Error != nil {
		return nil, utils.Errorf("获取知识库列表失败: %v", db.Error)
	}

	// 转换为protobuf格式
	pbKnowledgeBases := make([]*ypb.KnowledgeBaseInfo, len(knowledgeBases))
	for i, kb := range knowledgeBases {
		pbKnowledgeBases[i] = &ypb.KnowledgeBaseInfo{
			ID:                       int64(kb.ID),
			KnowledgeBaseName:        kb.KnowledgeBaseName,
			KnowledgeBaseDescription: kb.KnowledgeBaseDescription,
			KnowledgeBaseType:        kb.KnowledgeBaseType,
		}
	}

	return &ypb.GetKnowledgeBaseResponse{
		KnowledgeBases: pbKnowledgeBases,
		Pagination:     pagination,
		Total:          int64(p.TotalRecord),
	}, nil
}

func (s *Server) DeleteKnowledgeBase(ctx context.Context, req *ypb.DeleteKnowledgeBaseRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()
	kb, err := knowledgebase.LoadKnowledgeBaseByID(db, req.GetKnowledgeBaseId())
	if err != nil {
		return nil, utils.Errorf("获取知识库信息失败: %v", err)
	}
	err = kb.Drop()
	if err != nil {
		return nil, utils.Errorf("删除知识库失败: %v", err)
	}

	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) CreateKnowledgeBase(ctx context.Context, req *ypb.CreateKnowledgeBaseRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()

	// 使用knowledgebase包创建知识库
	_, err := knowledgebase.CreateKnowledgeBase(db,
		req.GetKnowledgeBaseName(),
		req.GetKnowledgeBaseDescription(),
		req.GetKnowledgeBaseType())
	if err != nil {
		return nil, err
	}

	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) UpdateKnowledgeBase(ctx context.Context, req *ypb.UpdateKnowledgeBaseRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()

	kb, err := knowledgebase.LoadKnowledgeBaseByID(db, req.GetKnowledgeBaseId())
	if err != nil {
		return nil, utils.Errorf("获取知识库信息失败: %v", err)
	}
	err = kb.UpdateKnowledgeBaseInfo(req.GetKnowledgeBaseName(), req.GetKnowledgeBaseDescription(), req.GetKnowledgeBaseType())
	if err != nil {
		return nil, utils.Errorf("更新知识库信息失败: %v", err)
	}

	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) DeleteKnowledgeBaseEntry(ctx context.Context, req *ypb.DeleteKnowledgeBaseEntryRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()

	kb, err := knowledgebase.LoadKnowledgeBaseByID(db, req.GetKnowledgeBaseId())
	if err != nil {
		return nil, utils.Errorf("获取知识库信息失败: %v", err)
	}
	err = kb.DeleteKnowledgeEntry(req.GetKnowledgeBaseEntryHiddenIndex())
	if err != nil {
		return nil, utils.Errorf("删除知识库条目失败: %v", err)
	}
	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) CreateKnowledgeBaseEntry(ctx context.Context, req *ypb.CreateKnowledgeBaseEntryRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()

	// 加载知识库
	kb, err := knowledgebase.LoadKnowledgeBaseByID(db, req.GetKnowledgeBaseID())
	if err != nil {
		return nil, utils.Errorf("加载知识库失败: %v", err)
	}

	// 创建知识库条目
	entry := &schema.KnowledgeBaseEntry{
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
	}

	// 使用知识库实例添加条目
	err = kb.AddKnowledgeEntry(entry)
	if err != nil {
		return nil, err
	}

	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) UpdateKnowledgeBaseEntry(ctx context.Context, req *ypb.UpdateKnowledgeBaseEntryRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()

	kb, err := knowledgebase.LoadKnowledgeBaseByID(db, req.GetKnowledgeBaseID())
	if err != nil {
		return nil, utils.Errorf("获取知识库信息失败: %v", err)
	}

	err = kb.UpdateKnowledgeEntry(req.GetKnowledgeBaseEntryHiddenIndex(), &schema.KnowledgeBaseEntry{
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
		return nil, utils.Errorf("更新知识库条目失败: %v", err)
	}

	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) SearchKnowledgeBaseEntry(ctx context.Context, req *ypb.SearchKnowledgeBaseEntryRequest) (*ypb.SearchKnowledgeBaseEntryResponse, error) {
	reqPagination := req.GetPagination()
	db := consts.GetGormProfileDatabase()

	// 如果有关键字搜索，尝试使用知识库的向量搜索功能
	// if req.GetKeyword() != "" {
	// 	kb, err := knowledgebase.LoadKnowledgeBaseByID(db, req.GetKnowledgeBaseId())
	// 	if err == nil {
	// 		// 使用知识库的搜索功能
	// 		limit := 50 // 默认限制
	// 		if reqPagination != nil && reqPagination.Limit > 0 {
	// 			limit = int(reqPagination.Limit)
	// 		}

	// 		entries, err := kb.SearchKnowledgeEntries(req.GetKeyword(), limit)
	// 		if err == nil {
	// 			return &ypb.SearchKnowledgeBaseEntryResponse{
	// 				Total:                int64(len(entries)),
	// 				Pagination:           reqPagination,
	// 				KnowledgeBaseEntries: KnowledgeBaseEntryListToGrpcModel(entries),
	// 			}, nil
	// 		}
	// 		// 如果向量搜索失败，回退到传统搜索
	// 	}
	// }

	// 回退到传统的数据库搜索
	p, entries, err := yakit.GetKnowledgeBaseEntryByFilter(db, req.GetKnowledgeBaseId(), req.GetKeyword(), reqPagination)
	if err != nil {
		return nil, err
	}

	return &ypb.SearchKnowledgeBaseEntryResponse{
		Total:                int64(p.TotalRecord),
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
		HiddenIndex:              entry.HiddenIndex,
		Summary:                  entry.Summary,
		SourcePage:               int32(entry.SourcePage),
		PotentialQuestions:       entry.PotentialQuestions,
		PotentialQuestionsVector: entry.PotentialQuestionsVector,
		RelatedEntityUUIDS:       entry.RelatedEntityUUIDS,
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
	_, err := rag.BuildVectorIndexForKnowledgeBase(consts.GetGormProfileDatabase(), req.GetKnowledgeBaseId(), opts...)
	if err != nil {
		return nil, err
	}
	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) BuildVectorIndexForKnowledgeBaseEntry(ctx context.Context, req *ypb.BuildVectorIndexForKnowledgeBaseEntryRequest) (*ypb.GeneralResponse, error) {
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
	_, err := rag.BuildVectorIndexForKnowledgeBaseEntry(consts.GetGormProfileDatabase(), req.GetKnowledgeBaseId(), req.GetKnowledgeBaseEntryHiddenIndex(), opts...)
	if err != nil {
		return nil, err
	}
	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

// rpc QueryKnowledgeBaseByAI(QueryKnowledgeBaseByAIRequest) returns(stream QueryKnowledgeBaseByAIResponse);
func (s *Server) QueryKnowledgeBaseByAI(req *ypb.QueryKnowledgeBaseByAIRequest, stream ypb.Yak_QueryKnowledgeBaseByAIServer) error {
	db := consts.GetGormProfileDatabase()
	if !req.GetQueryAllCollections() {
		kb, err := knowledgebase.LoadKnowledgeBaseByID(db, req.GetKnowledgeBaseID())
		if err != nil {
			return err
		}
		res, err := kb.SearchKnowledgeEntriesWithEnhance(req.GetQuery(), knowledgebase.WithEnhancePlan(req.GetEnhancePlan()), knowledgebase.WithEnableAISummary(true))
		if err != nil {
			return err
		}
		for result := range res {
			jsonData, err := json.Marshal(result.Data)
			if err != nil {
				log.Errorf("marshal query result data failed: %v", err)
				continue
			}
			stream.Send(&ypb.QueryKnowledgeBaseByAIResponse{
				Message:     utils.EscapeInvalidUTF8Byte([]byte(result.Message)),
				Data:        utils.EscapeInvalidUTF8Byte(jsonData),
				MessageType: result.Type,
			})
		}
		return nil
	}

	res, err := knowledgebase.Query(db, req.GetQuery(), knowledgebase.WithEnhancePlan(req.GetEnhancePlan()), knowledgebase.WithEnableAISummary(true))
	if err != nil {
		return err
	}
	for result := range res {
		jsonData, err := json.Marshal(result.Data)
		if err != nil {
			log.Errorf("marshal query result data failed: %v", err)
			continue
		}
		stream.Send(&ypb.QueryKnowledgeBaseByAIResponse{
			Message:     utils.EscapeInvalidUTF8Byte([]byte(result.Message)),
			Data:        utils.EscapeInvalidUTF8Byte(jsonData),
			MessageType: result.Type,
		})
	}
	return nil
}

func (s *Server) GetKnowledgeBaseTypeList(ctx context.Context, req *ypb.Empty) (*ypb.GetKnowledgeBaseTypeListResponse, error) {
	types := []*ypb.KnowledgeBaseType{
		{
			Name:        "默认",
			Description: "默认知识库类型",
			Value:       "",
		},
		{
			Name:        "AI",
			Description: "AI 知识库类型， 用于 AI 上下文增强",
			Value:       "ai",
		},
		{
			Name:        "数据清洗",
			Description: "用于储存清洗文档、视频、图片等数据的知识库，可以用于 AI 上下文增强或用于AI向量查询",
			Value:       "data_cleaning",
		},
	}
	return &ypb.GetKnowledgeBaseTypeListResponse{
		KnowledgeBaseTypes: types,
	}, nil
}
