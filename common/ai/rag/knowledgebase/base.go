package knowledgebase

import (
	"fmt"
	"strconv"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// KnowledgeBase 知识库结构体，提供对知识库的操作接口
type KnowledgeBase struct {
	db        *gorm.DB
	name      string
	ragSystem *rag.RAGSystem
}

// NewKnowledgeBase 创建新的知识库实例（先获取，获取不到则创建）
func NewKnowledgeBase(db *gorm.DB, name, description, kbType string, opts ...any) (*KnowledgeBase, error) {
	// 先检查 KnowledgeBaseInfo 是否存在
	var knowledgeBaseInfo schema.KnowledgeBaseInfo
	err := db.Model(&schema.KnowledgeBaseInfo{}).Where("knowledge_base_name = ?", name).First(&knowledgeBaseInfo).Error

	var needCreateInfo bool
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			needCreateInfo = true
		} else {
			return nil, utils.Errorf("查询知识库信息失败: %v", err)
		}
	}

	// 检查 RAG Collection 是否存在
	collectionExists := rag.CollectionIsExists(db, name)

	// 如果都不存在，创建新的知识库
	if needCreateInfo && !collectionExists {
		// 使用事务创建 KnowledgeBaseInfo
		err = utils.GormTransaction(db, func(tx *gorm.DB) error {
			knowledgeBaseInfo = schema.KnowledgeBaseInfo{
				KnowledgeBaseName:        name,
				KnowledgeBaseDescription: description,
				KnowledgeBaseType:        kbType,
			}
			return yakit.CreateKnowledgeBase(tx, &knowledgeBaseInfo)
		})
		if err != nil {
			return nil, utils.Errorf("创建知识库信息失败: %v", err)
		}

		// 创建 RAG Collection
		ragSystem, err := rag.CreateCollection(db, name, description, opts...)
		if err != nil {
			// 如果 RAG 创建失败，删除已创建的 KnowledgeBaseInfo
			_ = utils.GormTransaction(db, func(tx *gorm.DB) error {
				return yakit.DeleteKnowledgeBase(tx, int64(knowledgeBaseInfo.ID))
			})
			return nil, utils.Errorf("创建RAG集合失败: %v", err)
		}

		return &KnowledgeBase{
			db:        db,
			name:      name,
			ragSystem: ragSystem,
		}, nil
	}

	// 如果知识库信息存在但 RAG Collection 不存在，创建 RAG Collection
	if !needCreateInfo && !collectionExists {
		ragSystem, err := rag.CreateCollection(db, name, knowledgeBaseInfo.KnowledgeBaseDescription, opts...)
		if err != nil {
			return nil, utils.Errorf("创建RAG集合失败: %v", err)
		}

		return &KnowledgeBase{
			db:        db,
			name:      name,
			ragSystem: ragSystem,
		}, nil
	}

	// 如果知识库信息不存在但 RAG Collection 存在，创建知识库信息
	if needCreateInfo && collectionExists {
		err = utils.GormTransaction(db, func(tx *gorm.DB) error {
			knowledgeBaseInfo = schema.KnowledgeBaseInfo{
				KnowledgeBaseName:        name,
				KnowledgeBaseDescription: description,
				KnowledgeBaseType:        kbType,
			}
			return yakit.CreateKnowledgeBase(tx, &knowledgeBaseInfo)
		})
		if err != nil {
			return nil, utils.Errorf("创建知识库信息失败: %v", err)
		}
	}

	// 如果都存在，直接加载
	ragSystem, err := rag.LoadCollection(db, name)
	if err != nil {
		return nil, utils.Errorf("加载RAG集合失败: %v", err)
	}

	return &KnowledgeBase{
		db:        db,
		name:      name,
		ragSystem: ragSystem,
	}, nil
}

// CreateKnowledgeBase 创建全新的知识库（如果已存在会返回错误）
func CreateKnowledgeBase(db *gorm.DB, name, description, kbType string, opts ...any) (*KnowledgeBase, error) {
	// 检查是否已存在
	var existingInfo schema.KnowledgeBaseInfo
	err := db.Model(&schema.KnowledgeBaseInfo{}).Where("knowledge_base_name = ?", name).First(&existingInfo).Error
	if err == nil {
		return nil, utils.Errorf("知识库 %s 已存在", name)
	}
	if err != gorm.ErrRecordNotFound {
		return nil, utils.Errorf("检查知识库是否存在失败: %v", err)
	}

	// 检查 RAG Collection 是否存在
	if rag.CollectionIsExists(db, name) {
		return nil, utils.Errorf("RAG集合 %s 已存在", name)
	}

	// 使用事务创建 KnowledgeBaseInfo
	var knowledgeBaseInfo schema.KnowledgeBaseInfo
	err = utils.GormTransaction(db, func(tx *gorm.DB) error {
		knowledgeBaseInfo = schema.KnowledgeBaseInfo{
			KnowledgeBaseName:        name,
			KnowledgeBaseDescription: description,
			KnowledgeBaseType:        kbType,
		}
		return yakit.CreateKnowledgeBase(tx, &knowledgeBaseInfo)
	})
	if err != nil {
		return nil, utils.Errorf("创建知识库信息失败: %v", err)
	}

	// 创建 RAG Collection
	ragSystem, err := rag.CreateCollection(db, name, description, opts...)
	if err != nil {
		// 如果 RAG 创建失败，删除已创建的 KnowledgeBaseInfo
		_ = utils.GormTransaction(db, func(tx *gorm.DB) error {
			return yakit.DeleteKnowledgeBase(tx, int64(knowledgeBaseInfo.ID))
		})
		return nil, utils.Errorf("创建RAG集合失败: %v", err)
	}

	return &KnowledgeBase{
		db:        db,
		name:      name,
		ragSystem: ragSystem,
	}, nil
}

// LoadKnowledgeBase 加载已存在的知识库
func LoadKnowledgeBase(db *gorm.DB, name string, opts ...any) (*KnowledgeBase, error) {
	// 检查知识库是否存在
	if !rag.CollectionIsExists(db, name) {
		return nil, utils.Errorf("知识库 %s 不存在", name)
	}

	// 加载 RAG 系统
	ragSystem, err := rag.LoadCollection(db, name)
	if err != nil {
		return nil, utils.Errorf("加载知识库失败: %v", err)
	}

	return &KnowledgeBase{
		db:        db,
		name:      name,
		ragSystem: ragSystem,
	}, nil
}

// AddKnowledgeEntry 添加知识条目到知识库（使用事务）
func (kb *KnowledgeBase) AddKnowledgeEntry(entry *schema.KnowledgeBaseEntry) error {
	// 先添加到数据库（使用事务）
	err := utils.GormTransaction(kb.db, func(tx *gorm.DB) error {
		return yakit.CreateKnowledgeBaseEntry(tx, entry)
	})
	if err != nil {
		return utils.Errorf("创建知识库条目失败: %v", err)
	}

	// 然后添加到向量索引（事务外进行，避免数据库锁定）
	if err := kb.addEntryToVectorIndex(entry); err != nil {
		// 如果向量索引添加失败，回滚数据库操作
		_ = utils.GormTransaction(kb.db, func(tx *gorm.DB) error {
			return yakit.DeleteKnowledgeBaseEntry(tx, int64(entry.ID))
		})
		return utils.Errorf("添加向量索引失败: %v", err)
	}

	return nil
}

// UpdateKnowledgeEntry 更新知识条目（使用事务）
func (kb *KnowledgeBase) UpdateKnowledgeEntry(entry *schema.KnowledgeBaseEntry) error {
	// 先更新数据库（使用事务）
	var oldEntry *schema.KnowledgeBaseEntry
	err := utils.GormTransaction(kb.db, func(tx *gorm.DB) error {
		// 获取旧的条目信息用于回滚
		var err error
		oldEntry, err = yakit.GetKnowledgeBaseEntryById(tx, int64(entry.ID))
		if err != nil {
			return err
		}

		return yakit.UpdateKnowledgeBaseEntry(tx, entry)
	})
	if err != nil {
		return utils.Errorf("更新知识库条目失败: %v", err)
	}

	// 然后更新向量索引（事务外进行）
	documentID := utils.InterfaceToString(entry.ID)

	// 删除旧的向量索引
	if err := kb.ragSystem.DeleteDocuments(documentID); err != nil {
		// 如果删除失败，回滚数据库操作
		_ = utils.GormTransaction(kb.db, func(tx *gorm.DB) error {
			return yakit.UpdateKnowledgeBaseEntry(tx, oldEntry)
		})
		return utils.Errorf("删除旧向量索引失败: %v", err)
	}

	// 添加新的向量索引
	if err := kb.addEntryToVectorIndex(entry); err != nil {
		// 如果添加失败，回滚数据库操作
		_ = utils.GormTransaction(kb.db, func(tx *gorm.DB) error {
			return yakit.UpdateKnowledgeBaseEntry(tx, oldEntry)
		})
		return utils.Errorf("更新向量索引失败: %v", err)
	}

	return nil
}

// DeleteKnowledgeEntry 删除知识条目（使用事务）
func (kb *KnowledgeBase) DeleteKnowledgeEntry(entryID int64) error {
	// 先获取条目信息用于回滚
	entry, err := yakit.GetKnowledgeBaseEntryById(kb.db, entryID)
	if err != nil {
		return utils.Errorf("获取知识库条目失败: %v", err)
	}

	// 先从向量索引中删除（事务外进行）
	documentID := utils.InterfaceToString(entryID)
	if err := kb.ragSystem.DeleteDocuments(documentID); err != nil {
		return utils.Errorf("删除向量索引失败: %v", err)
	}

	// 然后从数据库中删除（使用事务）
	err = utils.GormTransaction(kb.db, func(tx *gorm.DB) error {
		return yakit.DeleteKnowledgeBaseEntry(tx, entryID)
	})
	if err != nil {
		// 如果数据库删除失败，恢复向量索引
		_ = kb.addEntryToVectorIndex(entry)
		return utils.Errorf("删除数据库条目失败: %v", err)
	}

	return nil
}

// SearchKnowledgeEntries 搜索知识条目，返回知识库条目对象
func (kb *KnowledgeBase) SearchKnowledgeEntries(query string, limit int) ([]*schema.KnowledgeBaseEntry, error) {
	// 先通过RAG系统进行向量搜索
	searchResults, err := kb.ragSystem.QueryWithPage(query, 1, limit)
	if err != nil {
		return nil, utils.Errorf("RAG搜索失败: %v", err)
	}

	// 通过搜索结果中的文档ID查询对应的知识库条目
	var entries []*schema.KnowledgeBaseEntry
	for _, result := range searchResults {
		// 文档ID就是知识库条目的ID
		entryID, err := strconv.ParseInt(result.Document.ID, 10, 64)
		if err != nil {
			// 如果ID解析失败，跳过这个结果
			continue
		}

		entry, err := yakit.GetKnowledgeBaseEntryById(kb.db, entryID)
		if err != nil {
			// 如果查询失败，跳过这个结果
			continue
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// SearchKnowledgeEntriesWithScore 搜索知识条目并返回相似度分数
func (kb *KnowledgeBase) SearchKnowledgeEntriesWithScore(query string, limit int) ([]*KnowledgeEntryWithScore, error) {
	// 先通过RAG系统进行向量搜索
	searchResults, err := kb.ragSystem.QueryWithPage(query, 1, limit)
	if err != nil {
		return nil, utils.Errorf("RAG搜索失败: %v", err)
	}

	// 通过搜索结果中的文档ID查询对应的知识库条目，并保留分数
	var entriesWithScore []*KnowledgeEntryWithScore
	for _, result := range searchResults {
		// 文档ID就是知识库条目的ID
		entryID, err := strconv.ParseInt(result.Document.ID, 10, 64)
		if err != nil {
			// 如果ID解析失败，跳过这个结果
			continue
		}

		entry, err := yakit.GetKnowledgeBaseEntryById(kb.db, entryID)
		if err != nil {
			// 如果查询失败，跳过这个结果
			continue
		}

		entriesWithScore = append(entriesWithScore, &KnowledgeEntryWithScore{
			Entry: entry,
			Score: float64(result.Score),
		})
	}

	return entriesWithScore, nil
}

// KnowledgeEntryWithScore 带相似度分数的知识库条目
type KnowledgeEntryWithScore struct {
	Entry *schema.KnowledgeBaseEntry `json:"entry"`
	Score float64                    `json:"score"`
}

// GetKnowledgeEntry 根据ID获取知识条目
func (kb *KnowledgeBase) GetKnowledgeEntry(entryID int64) (*schema.KnowledgeBaseEntry, error) {
	return yakit.GetKnowledgeBaseEntryById(kb.db, entryID)
}

// ListKnowledgeEntries 分页获取知识条目列表
func (kb *KnowledgeBase) ListKnowledgeEntries(keyword string, page, limit int) ([]*schema.KnowledgeBaseEntry, error) {
	// 需要先找到对应的 KnowledgeBaseInfo
	knowledgeBaseInfo, err := kb.GetInfo()
	if err != nil {
		return nil, err
	}

	filter := &ypb.Paging{
		Page:  int64(page),
		Limit: int64(limit),
	}

	_, entries, err := yakit.GetKnowledgeBaseEntryByFilter(kb.db, int64(knowledgeBaseInfo.ID), keyword, filter)
	if err != nil {
		return nil, utils.Errorf("获取知识库条目列表失败: %v", err)
	}

	return entries, nil
}

// BuildVectorIndex 为知识库构建向量索引
func (kb *KnowledgeBase) BuildVectorIndex() error {
	// 需要先找到对应的 KnowledgeBaseInfo
	knowledgeBaseInfo, err := kb.GetInfo()
	if err != nil {
		return err
	}

	_, err = rag.BuildVectorIndexForKnowledgeBase(kb.db, int64(knowledgeBaseInfo.ID))
	return err
}

// GetInfo 获取知识库信息
func (kb *KnowledgeBase) GetInfo() (*schema.KnowledgeBaseInfo, error) {
	var knowledgeBaseInfo schema.KnowledgeBaseInfo
	err := kb.db.Model(&schema.KnowledgeBaseInfo{}).Where("knowledge_base_name = ?", kb.name).First(&knowledgeBaseInfo).Error
	if err != nil {
		return nil, utils.Errorf("获取知识库信息失败: %v", err)
	}
	return &knowledgeBaseInfo, nil
}

// CountDocuments 获取文档总数
func (kb *KnowledgeBase) CountDocuments() (int, error) {
	return kb.ragSystem.CountDocuments()
}

// ClearDocuments 清空所有文档
func (kb *KnowledgeBase) ClearDocuments() error {
	return kb.ragSystem.ClearDocuments()
}

// GetName 获取知识库名称
func (kb *KnowledgeBase) GetName() string {
	return kb.name
}

// GetRAGSystem 获取底层的 RAG 系统
func (kb *KnowledgeBase) GetRAGSystem() *rag.RAGSystem {
	return kb.ragSystem
}

// addEntryToVectorIndex 将知识条目添加到向量索引
func (kb *KnowledgeBase) addEntryToVectorIndex(entry *schema.KnowledgeBaseEntry) error {
	// 构建文档内容
	content := entry.KnowledgeTitle
	if entry.Summary != "" {
		content += "\n\n" + entry.Summary
	}
	if entry.KnowledgeDetails != "" {
		content += "\n\n" + entry.KnowledgeDetails
	}

	// 构建元数据
	metadata := map[string]any{
		"knowledge_base_id":   entry.KnowledgeBaseID,
		"knowledge_title":     entry.KnowledgeTitle,
		"knowledge_type":      entry.KnowledgeType,
		"importance_score":    entry.ImportanceScore,
		"keywords":            entry.Keywords,
		"source_page":         entry.SourcePage,
		"potential_questions": entry.PotentialQuestions,
	}

	// 使用条目ID作为文档ID
	documentID := utils.InterfaceToString(entry.ID)

	// 添加文档到RAG系统
	return kb.ragSystem.Add(documentID, content, rag.WithDocumentRawMetadata(metadata))
}

// SyncKnowledgeBaseWithRAG 同步知识库和RAG，以知识库为准
func (kb *KnowledgeBase) SyncKnowledgeBaseWithRAG() (*SyncResult, error) {
	result := &SyncResult{
		AddedToRAG:     []int64{},
		DeletedFromRAG: []string{},
		SyncErrors:     []string{},
	}

	// 获取知识库信息
	knowledgeBaseInfo, err := kb.GetInfo()
	if err != nil {
		return result, err
	}

	// 获取知识库中的所有条目ID
	var dbEntries []*schema.KnowledgeBaseEntry
	err = kb.db.Model(&schema.KnowledgeBaseEntry{}).
		Where("knowledge_base_id = ?", knowledgeBaseInfo.ID).
		Find(&dbEntries).Error
	if err != nil {
		return result, utils.Errorf("获取知识库条目失败: %v", err)
	}

	// 创建数据库中条目ID的映射
	dbEntryIDs := make(map[string]bool)
	dbEntryMap := make(map[string]*schema.KnowledgeBaseEntry)
	for _, entry := range dbEntries {
		entryIDStr := utils.InterfaceToString(entry.ID)
		dbEntryIDs[entryIDStr] = true
		dbEntryMap[entryIDStr] = entry
	}

	// 获取RAG中的所有文档
	ragDocuments, err := kb.ragSystem.ListDocuments()
	if err != nil {
		return result, utils.Errorf("获取RAG文档列表失败: %v", err)
	}

	// 创建RAG中文档ID的映射
	ragDocumentIDs := make(map[string]bool)
	for _, doc := range ragDocuments {
		ragDocumentIDs[doc.ID] = true
	}

	// 查找知识库中有但RAG中没有的条目，添加到RAG
	for entryIDStr, entry := range dbEntryMap {
		if !ragDocumentIDs[entryIDStr] {
			if err := kb.addEntryToVectorIndex(entry); err != nil {
				result.SyncErrors = append(result.SyncErrors,
					fmt.Sprintf("添加条目 %s 到RAG失败: %v", entryIDStr, err))
			} else {
				if entryID, err := strconv.ParseInt(entryIDStr, 10, 64); err == nil {
					result.AddedToRAG = append(result.AddedToRAG, entryID)
				}
			}
		}
	}

	// 查找RAG中有但知识库中没有的文档，从RAG中删除
	for _, doc := range ragDocuments {
		if !dbEntryIDs[doc.ID] {
			if err := kb.ragSystem.DeleteDocuments(doc.ID); err != nil {
				result.SyncErrors = append(result.SyncErrors,
					fmt.Sprintf("从RAG删除文档 %s 失败: %v", doc.ID, err))
			} else {
				result.DeletedFromRAG = append(result.DeletedFromRAG, doc.ID)
			}
		}
	}

	result.TotalDBEntries = len(dbEntries)
	result.TotalRAGDocuments = len(ragDocuments)

	return result, nil
}

// SyncResult 同步操作的结果
type SyncResult struct {
	TotalDBEntries    int      `json:"total_db_entries"`    // 数据库中的总条目数
	TotalRAGDocuments int      `json:"total_rag_documents"` // RAG中的总文档数
	AddedToRAG        []int64  `json:"added_to_rag"`        // 添加到RAG的条目ID列表
	DeletedFromRAG    []string `json:"deleted_from_rag"`    // 从RAG删除的文档ID列表
	SyncErrors        []string `json:"sync_errors"`         // 同步过程中的错误列表
}

// GetSyncStatus 获取当前同步状态信息
func (kb *KnowledgeBase) GetSyncStatus() (*SyncStatus, error) {
	status := &SyncStatus{}

	// 获取知识库信息
	knowledgeBaseInfo, err := kb.GetInfo()
	if err != nil {
		return status, err
	}

	// 获取数据库中的条目总数
	var dbCount int64
	err = kb.db.Model(&schema.KnowledgeBaseEntry{}).
		Where("knowledge_base_id = ?", knowledgeBaseInfo.ID).
		Count(&dbCount).Error
	if err != nil {
		return status, utils.Errorf("获取数据库条目数量失败: %v", err)
	}

	// 获取RAG中的文档总数
	ragCount, err := kb.ragSystem.CountDocuments()
	if err != nil {
		return status, utils.Errorf("获取RAG文档数量失败: %v", err)
	}

	status.DatabaseEntries = int(dbCount)
	status.RAGDocuments = ragCount
	status.InSync = (status.DatabaseEntries == status.RAGDocuments)

	return status, nil
}

// SyncStatus 同步状态信息
type SyncStatus struct {
	DatabaseEntries int  `json:"database_entries"` // 数据库中的条目数
	RAGDocuments    int  `json:"rag_documents"`    // RAG中的文档数
	InSync          bool `json:"in_sync"`          // 是否同步
}

// BatchSyncEntries 批量同步指定的知识条目
func (kb *KnowledgeBase) BatchSyncEntries(entryIDs []int64) (*SyncResult, error) {
	result := &SyncResult{
		AddedToRAG:     []int64{},
		DeletedFromRAG: []string{},
		SyncErrors:     []string{},
	}

	for _, entryID := range entryIDs {
		// 获取知识条目
		entry, err := yakit.GetKnowledgeBaseEntryById(kb.db, entryID)
		if err != nil {
			result.SyncErrors = append(result.SyncErrors,
				fmt.Sprintf("获取条目 %d 失败: %v", entryID, err))
			continue
		}

		// 检查RAG中是否已存在
		documentID := utils.InterfaceToString(entryID)
		_, exists, err := kb.ragSystem.GetDocument(documentID)
		if err != nil {
			result.SyncErrors = append(result.SyncErrors,
				fmt.Sprintf("检查RAG文档 %d 失败: %v", entryID, err))
			continue
		}

		if exists {
			// 如果存在，先删除再添加（更新）
			if err := kb.ragSystem.DeleteDocuments(documentID); err != nil {
				result.SyncErrors = append(result.SyncErrors,
					fmt.Sprintf("删除RAG文档 %d 失败: %v", entryID, err))
				continue
			}
		}

		// 添加到RAG
		if err := kb.addEntryToVectorIndex(entry); err != nil {
			result.SyncErrors = append(result.SyncErrors,
				fmt.Sprintf("添加条目 %d 到RAG失败: %v", entryID, err))
		} else {
			result.AddedToRAG = append(result.AddedToRAG, entryID)
		}
	}

	return result, nil
}
