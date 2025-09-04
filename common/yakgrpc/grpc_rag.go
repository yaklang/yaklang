package yakgrpc

import (
	"context"
	"encoding/json"

	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// GetAllVectorStoreCollectionsWithFilter 获取所有向量存储集合（带过滤和分页）
func (s *Server) GetAllVectorStoreCollectionsWithFilter(ctx context.Context, req *ypb.GetAllVectorStoreCollectionsWithFilterRequest) (*ypb.GetAllVectorStoreCollectionsWithFilterResponse, error) {
	db := consts.GetGormProfileDatabase()

	// 构建查询
	query := db.Model(&schema.VectorStoreCollection{})

	// 关键词搜索
	if req.GetKeyword() != "" {
		query = bizhelper.FuzzSearchEx(query, []string{"name", "description", "model_name"}, req.GetKeyword(), false)
	}

	// ID过滤
	if req.GetID() > 0 {
		query = query.Where("id = ?", req.GetID())
	}

	// 分页
	var collections []*schema.VectorStoreCollection
	pagination := req.GetPagination()
	page := 1
	limit := 10
	if pagination != nil {
		page = int(pagination.GetPage())
		if page <= 0 {
			page = 1
		}
		limit = int(pagination.GetLimit())
		if limit <= 0 {
			limit = 10
		}
	}

	p, db := bizhelper.Paging(query, page, limit, &collections)
	if db.Error != nil {
		return nil, utils.Errorf("查询向量存储集合失败: %v", db.Error)
	}

	// 转换为 protobuf 格式
	pbCollections := make([]*ypb.VectorStoreCollection, 0, len(collections))
	for _, collection := range collections {
		pbCollection := &ypb.VectorStoreCollection{
			ID:               int64(collection.ID),
			Name:             collection.Name,
			Description:      collection.Description,
			ModelName:        collection.ModelName,
			Dimension:        int32(collection.Dimension),
			M:                int32(collection.M),
			Ml:               float32(collection.Ml),
			EfSearch:         int32(collection.EfSearch),
			EfConstruct:      int32(collection.EfConstruct),
			DistanceFuncType: collection.DistanceFuncType,
		}
		pbCollections = append(pbCollections, pbCollection)
	}

	return &ypb.GetAllVectorStoreCollectionsWithFilterResponse{
		Collections: pbCollections,
		Pagination: &ypb.Paging{
			Page:  int64(p.Page),
			Limit: int64(p.Limit),
		},
		Total: int64(p.TotalRecord),
	}, nil
}

// UpdateVectorStoreCollection 更新向量存储集合信息
func (s *Server) UpdateVectorStoreCollection(ctx context.Context, req *ypb.UpdateVectorStoreCollectionRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()

	// 验证请求参数
	if req.GetID() <= 0 {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: "集合ID不能为空",
		}, nil
	}

	// 查找集合
	var collection schema.VectorStoreCollection
	err := db.Model(&schema.VectorStoreCollection{}).Where("id = ?", req.GetID()).First(&collection).Error
	if err != nil {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: utils.Errorf("找不到指定的向量存储集合: %v", err).Error(),
		}, nil
	}

	// 更新字段
	updates := map[string]interface{}{}
	if req.GetName() != "" {
		// 检查名称是否已存在（除了当前集合）
		var count int64
		db.Model(&schema.VectorStoreCollection{}).Where("name = ? AND id != ?", req.GetName(), req.GetID()).Count(&count)
		if count > 0 {
			return &ypb.GeneralResponse{
				Ok:     false,
				Reason: "集合名称已存在",
			}, nil
		}
		updates["name"] = req.GetName()
	}
	if req.GetDescription() != "" {
		updates["description"] = req.GetDescription()
	}

	// 执行更新
	if len(updates) > 0 {
		err = db.Model(&collection).Updates(updates).Error
		if err != nil {
			return &ypb.GeneralResponse{
				Ok:     false,
				Reason: utils.Errorf("更新向量存储集合失败: %v", err).Error(),
			}, nil
		}
	}

	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

// ListVectorStoreEntries 列出向量存储条目
func (s *Server) ListVectorStoreEntries(ctx context.Context, req *ypb.ListVectorStoreEntriesRequest) (*ypb.ListVectorStoreEntriesResponse, error) {
	db := consts.GetGormProfileDatabase()

	// 验证集合ID
	if req.GetCollectionID() <= 0 {
		return nil, utils.Errorf("集合ID不能为空")
	}

	// 验证集合是否存在
	var collection schema.VectorStoreCollection
	err := db.Model(&schema.VectorStoreCollection{}).Where("id = ?", req.GetCollectionID()).First(&collection).Error
	if err != nil {
		return nil, utils.Errorf("找不到指定的向量存储集合: %v", err)
	}

	// 构建查询
	query := db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", req.GetCollectionID())

	// 关键词搜索
	if req.GetKeyword() != "" {
		query = bizhelper.FuzzSearchEx(query, []string{"document_id", "content"}, req.GetKeyword(), false)
	}

	// 分页
	var documents []*schema.VectorStoreDocument
	pagination := req.GetPagination()
	page := 1
	limit := 10
	if pagination != nil {
		page = int(pagination.GetPage())
		if page <= 0 {
			page = 1
		}
		limit = int(pagination.GetLimit())
		if limit <= 0 {
			limit = 10
		}
	}

	p, db := bizhelper.Paging(query, page, limit, &documents)
	if db.Error != nil {
		return nil, utils.Errorf("查询向量存储条目失败: %v", db.Error)
	}

	// 转换为 protobuf 格式
	pbEntries := make([]*ypb.VectorStoreEntry, 0, len(documents))
	for _, doc := range documents {
		// 序列化元数据
		metadataBytes, _ := json.Marshal(doc.Metadata)
		metadataStr := string(metadataBytes)

		// 转换嵌入向量
		embedding := make([]float32, len(doc.Embedding))
		for i, v := range doc.Embedding {
			embedding[i] = v
		}

		pbEntry := &ypb.VectorStoreEntry{
			ID:        int64(doc.ID),
			UID:       doc.DocumentID,
			Content:   doc.Content,
			Metadata:  metadataStr,
			Embedding: embedding,
		}
		pbEntries = append(pbEntries, pbEntry)
	}

	return &ypb.ListVectorStoreEntriesResponse{
		Entries: pbEntries,
		Pagination: &ypb.Paging{
			Page:  int64(p.Page),
			Limit: int64(p.Limit),
		},
		Total: int64(p.TotalRecord),
	}, nil
}

// CreateVectorStoreEntry 创建向量存储条目
func (s *Server) CreateVectorStoreEntry(ctx context.Context, req *ypb.CreateVectorStoreEntryRequest) (*ypb.GeneralResponse, error) {
	db := consts.GetGormProfileDatabase()

	// 验证请求参数
	if req.GetUID() == "" {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: "文档ID不能为空",
		}, nil
	}
	if req.GetContent() == "" {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: "文档内容不能为空",
		}, nil
	}

	// 解析元数据
	var metadata schema.MetadataMap
	if req.GetMetadata() != "" {
		err := json.Unmarshal([]byte(req.GetMetadata()), &metadata)
		if err != nil {
			return &ypb.GeneralResponse{
				Ok:     false,
				Reason: utils.Errorf("元数据格式错误: %v", err).Error(),
			}, nil
		}
	}

	// 这里需要调用RAG系统来添加文档
	// 由于需要集合信息，我们需要从元数据中获取集合名称或者通过其他方式确定
	collectionName := ""
	if metadata != nil && metadata["collection_name"] != nil {
		if name, ok := metadata["collection_name"].(string); ok {
			collectionName = name
		}
	}

	if collectionName == "" {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: "元数据中必须包含collection_name字段",
		}, nil
	}

	// 检查集合是否存在
	if !rag.CollectionIsExists(db, collectionName) {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: utils.Errorf("集合 %s 不存在", collectionName).Error(),
		}, nil
	}

	// 加载RAG系统
	ragSystem, err := rag.LoadCollection(db, collectionName)
	if err != nil {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: utils.Errorf("加载RAG集合失败: %v", err).Error(),
		}, nil
	}

	// 添加文档到RAG系统
	err = ragSystem.Add(req.GetUID(), req.GetContent(), rag.WithDocumentRawMetadata(metadata))
	if err != nil {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: utils.Errorf("添加文档到RAG系统失败: %v", err).Error(),
		}, nil
	}

	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

// GetDocumentByVectorStoreEntryID 根据向量存储条目ID获取对应的知识库文档
func (s *Server) GetDocumentByVectorStoreEntryID(ctx context.Context, req *ypb.GetDocumentByVectorStoreEntryIDRequest) (*ypb.GetDocumentByVectorStoreEntryIDResponse, error) {
	db := consts.GetGormProfileDatabase()

	// 验证参数
	if req.GetID() <= 0 {
		return nil, utils.Errorf("文档ID不能为空")
	}

	// 查找向量存储文档
	var doc schema.VectorStoreDocument
	err := db.Model(&schema.VectorStoreDocument{}).Where("id = ?", req.GetID()).First(&doc).Error
	if err != nil {
		return nil, utils.Errorf("找不到指定的向量存储文档: %v", err)
	}

	// 从文档ID解析知识库条目ID
	// 如果文档ID是知识库条目ID的字符串形式，尝试解析
	entryID := doc.DocumentID

	// 获取知识库条目
	entry, err := yakit.GetKnowledgeBaseEntryByHiddenIndex(db, entryID)
	if err != nil {
		return nil, utils.Errorf("获取知识库条目失败: %v", err)
	}

	// 转换为 protobuf 格式
	pbEntry := &ypb.KnowledgeBaseEntry{
		ID:                 int64(entry.ID),
		KnowledgeBaseId:    entry.KnowledgeBaseID,
		KnowledgeTitle:     entry.KnowledgeTitle,
		KnowledgeType:      entry.KnowledgeType,
		ImportanceScore:    int32(entry.ImportanceScore),
		Keywords:           []string(entry.Keywords),
		KnowledgeDetails:   entry.KnowledgeDetails,
		Summary:            entry.Summary,
		SourcePage:         int32(entry.SourcePage),
		PotentialQuestions: []string(entry.PotentialQuestions),
	}

	return &ypb.GetDocumentByVectorStoreEntryIDResponse{
		Document: pbEntry,
	}, nil
}
