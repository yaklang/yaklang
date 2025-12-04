package yakgrpc

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
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

	// 删除名称为空的rag
	var emptyRagCollection schema.VectorStoreCollection
	if err := db.Model(&schema.VectorStoreCollection{}).Where("name = ''").First(&emptyRagCollection).Error; err == nil {
		db.Model(&schema.VectorStoreCollection{}).Where("name = ''").Unscoped().Delete(&schema.VectorStoreCollection{})
		db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", emptyRagCollection.ID).Unscoped().Delete(&schema.VectorStoreDocument{})
	}

	var emptyKnowledgeBase schema.KnowledgeBaseInfo
	if err := db.Model(&schema.KnowledgeBaseInfo{}).Where("knowledge_base_name = ''").First(&emptyKnowledgeBase).Error; err == nil {
		db.Model(&schema.KnowledgeBaseInfo{}).Where("knowledge_base_name = ''").Unscoped().Delete(&schema.KnowledgeBaseInfo{})
		db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", emptyKnowledgeBase.ID).Unscoped().Delete(&schema.KnowledgeBaseEntry{})
	}

	var emptyEntityRepository schema.EntityRepository
	if err := db.Model(&schema.EntityRepository{}).Where("entity_base_name = ''").First(&emptyEntityRepository).Error; err == nil {
		db.Model(&schema.EntityRepository{}).Where("entity_base_name = ''").Unscoped().Delete(&schema.EntityRepository{})
		db.Model(&schema.ERModelEntity{}).Where("repository_uuid = ?", emptyEntityRepository.Uuid).Unscoped().Delete(&schema.ERModelEntity{})
		db.Model(&schema.ERModelRelationship{}).Where("repository_uuid = ?", emptyEntityRepository.Uuid).Unscoped().Delete(&schema.ERModelRelationship{})
	}

	paging := req.GetPagination()
	if paging == nil {
		paging = &ypb.Paging{
			Page:  1,
			Limit: 10,
		}
	}
	p, knowledgeBases, err := yakit.QueryKnowledgeBasePaging(db, req.GetKnowledgeBaseId(), req.GetKeyword(), paging)
	if err != nil {
		return nil, utils.Errorf("获取知识库列表失败: %v", err)
	}

	// 转换为protobuf格式
	pbKnowledgeBases := make([]*ypb.KnowledgeBaseInfo, len(knowledgeBases))
	for i, kb := range knowledgeBases {
		ragSystem, err := rag.Get(kb.KnowledgeBaseName, rag.WithDB(db))
		if err != nil {
			return nil, utils.Errorf("获取知识库失败: %v", err)
		}
		isImported := ragSystem.VectorStore.GetCollectionInfo().SerialVersionUID != ""
		pbKnowledgeBases[i] = &ypb.KnowledgeBaseInfo{
			ID:                       int64(kb.ID),
			KnowledgeBaseName:        kb.KnowledgeBaseName,
			IsImported:               isImported,
			KnowledgeBaseDescription: kb.KnowledgeBaseDescription,
			KnowledgeBaseType:        kb.KnowledgeBaseType,
			Tags:                     utils.StringSplitAndStrip(kb.Tags, ","),
		}
	}

	return &ypb.GetKnowledgeBaseResponse{
		KnowledgeBases: pbKnowledgeBases,
		Pagination:     req.GetPagination(),
		Total:          int64(p.TotalRecord),
	}, nil
}

func (s *Server) DeleteKnowledgeBase(ctx context.Context, req *ypb.DeleteKnowledgeBaseRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()

	name := req.GetName()
	if name == "" {
		var info schema.KnowledgeBaseInfo
		err := db.Where("id = ?", req.GetKnowledgeBaseId()).First(&info).Error
		if err != nil {
			return nil, utils.Errorf("get KnowledgeBaseInfo failed: %s", err)
		}

		name = info.KnowledgeBaseName
	}

	err := rag.DeleteRAG(db, name)
	if err != nil {
		return nil, utils.Errorf("删除知识库失败: %v", err)
	}

	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) CreateKnowledgeBase(ctx context.Context, req *ypb.CreateKnowledgeBaseRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()

	_, err := rag.Get(req.GetKnowledgeBaseName(), rag.WithDB(db))
	if err != nil {
		return nil, utils.Wrap(err, "创建知识库失败")
	}

	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) CreateKnowledgeBaseV2(ctx context.Context, req *ypb.CreateKnowledgeBaseV2Request) (*ypb.CreateKnowledgeBaseV2Response, error) {
	db := consts.GetGormProfileDatabase()
	ragSystem, err := rag.Get(req.GetName(), rag.WithDB(db), rag.WithDescription(req.GetDescription()), rag.WithTags(req.GetTags()...))
	if err != nil {
		return nil, utils.Wrap(err, "创建知识库失败")
	}

	kbInfo := ragSystem.KnowledgeBase.GetKnowledgeBaseInfo()
	if kbInfo == nil {
		return nil, utils.Errorf("获取知识库信息失败")
	}
	return &ypb.CreateKnowledgeBaseV2Response{
		KnowledgeBase: &ypb.KnowledgeBaseInfo{
			ID:                       int64(kbInfo.ID),
			KnowledgeBaseName:        kbInfo.KnowledgeBaseName,
			KnowledgeBaseDescription: kbInfo.KnowledgeBaseDescription,
			KnowledgeBaseType:        kbInfo.KnowledgeBaseType,
			Tags:                     utils.StringSplitAndStrip(kbInfo.Tags, ","),
		},
		IsSuccess:   true,
		Message:     "创建知识库成功",
		MessageType: "info",
	}, nil
}

func (s *Server) UpdateKnowledgeBase(ctx context.Context, req *ypb.UpdateKnowledgeBaseRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()

	kb, err := knowledgebase.LoadKnowledgeBaseByID(db, req.GetKnowledgeBaseId())
	if err != nil {
		return nil, utils.Errorf("获取知识库信息失败: %v", err)
	}
	err = yakit.UpdateKnowledgeBaseInfo(db, kb.GetID(), &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        req.GetKnowledgeBaseName(),
		KnowledgeBaseDescription: req.GetKnowledgeBaseDescription(),
		KnowledgeBaseType:        req.GetKnowledgeBaseType(),
		Tags:                     strings.Join(req.GetTags(), ","),
	})
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

	// 回退到传统的数据库搜索
	p, entries, err := yakit.QueryKnowledgeBaseEntryPaging(db, req.GetFilter(), reqPagination)
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
	aiOptions := []aispec.AIConfigOption{
		aispec.WithBaseURL(req.GetBaseUrl()),
		aispec.WithAPIKey(req.GetApiKey()),
		aispec.WithModel(req.GetModelName()),
		aispec.WithProxy(req.GetProxy()),
	}
	ragOpts := []rag.RAGSystemConfigOption{
		rag.WithEmbeddingModel(req.GetModelName()),
		rag.WithModelDimension(int(req.GetDimension())),
		rag.WithHNSWParameters(int(req.GetM()), float64(req.GetMl()), int(req.GetEfSearch()), int(req.GetEfConstruct())),
		rag.WithAIOptions(aiOptions...),
	}

	switch req.GetDistanceFuncType() {
	case "cosine":
		ragOpts = append(ragOpts, rag.WithCosineDistance())
	default:
		return nil, utils.Errorf("invalid distance function type: %s", req.GetDistanceFuncType())
	}

	_, err := rag.BuildVectorIndexForKnowledgeBase(consts.GetGormProfileDatabase(), req.GetKnowledgeBaseId(), ragOpts...)
	if err != nil {
		return nil, err
	}
	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) BuildVectorIndexForKnowledgeBaseEntry(ctx context.Context, req *ypb.BuildVectorIndexForKnowledgeBaseEntryRequest) (*ypb.GeneralResponse, error) {
	aiOptions := []aispec.AIConfigOption{
		aispec.WithBaseURL(req.GetBaseUrl()),
		aispec.WithAPIKey(req.GetApiKey()),
		aispec.WithModel(req.GetModelName()),
		aispec.WithProxy(req.GetProxy()),
	}
	ragOpts := []rag.RAGSystemConfigOption{
		rag.WithEmbeddingModel(req.GetModelName()),
		rag.WithModelDimension(int(req.GetDimension())),
		rag.WithHNSWParameters(int(req.GetM()), float64(req.GetMl()), int(req.GetEfSearch()), int(req.GetEfConstruct())),
		rag.WithAIOptions(aiOptions...),
	}

	switch req.GetDistanceFuncType() {
	case "cosine":
		ragOpts = append(ragOpts, rag.WithCosineDistance())
	default:
		return nil, utils.Errorf("invalid distance function type: %s", req.GetDistanceFuncType())
	}
	_, err := rag.BuildVectorIndexForKnowledgeBaseEntry(consts.GetGormProfileDatabase(), req.GetKnowledgeBaseId(), req.GetKnowledgeBaseEntryHiddenIndex(), ragOpts...)
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
