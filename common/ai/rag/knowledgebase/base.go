package knowledgebase

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/aiforge/contracts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var Simpleliteforge contracts.LiteForge

// KnowledgeBase 知识库结构体，提供对知识库的操作接口
type KnowledgeBase struct {
	db        *gorm.DB
	name      string
	ragSystem *rag.RAGSystem
	id        int64
}

func (kb *KnowledgeBase) GetID() int64 {
	return kb.id
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{}, &schema.VectorStoreDocument{}, &schema.VectorStoreCollection{}).Error
}

// NewKnowledgeBase 创建新的知识库实例（先获取，获取不到则创建）
func NewKnowledgeBase(db *gorm.DB, name, description, kbType string, opts ...any) (*KnowledgeBase, error) {
	if err := AutoMigrate(db); err != nil {
		return nil, utils.Errorf("自动迁移知识库表失败: %v", err)
	}
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
		ragSystem, err := rag.CreateCollection(db, name, description, append(opts, rag.WithLazyLoadEmbeddingClient())...)
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
			id:        int64(knowledgeBaseInfo.ID),
		}, nil
	}

	// 如果知识库信息存在但 RAG Collection 不存在，创建 RAG Collection
	if !needCreateInfo && !collectionExists {
		ragSystem, err := rag.CreateCollection(db, name, knowledgeBaseInfo.KnowledgeBaseDescription, append(opts, rag.WithLazyLoadEmbeddingClient())...)
		if err != nil {
			return nil, utils.Errorf("创建RAG集合失败: %v", err)
		}

		return &KnowledgeBase{
			db:        db,
			name:      name,
			ragSystem: ragSystem,
			id:        int64(knowledgeBaseInfo.ID),
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
		id:        int64(knowledgeBaseInfo.ID),
	}, nil
}

// CreateKnowledgeBase 创建全新的知识库（如果已存在会返回错误）
func CreateKnowledgeBase(db *gorm.DB, name, description, kbType string, opts ...any) (*KnowledgeBase, error) {
	if err := AutoMigrate(db); err != nil {
		return nil, utils.Errorf("自动迁移知识库表失败: %v", err)
	}
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
		id:        int64(knowledgeBaseInfo.ID),
	}, nil
}

// LoadKnowledgeBase 加载已存在的知识库
func LoadKnowledgeBase(db *gorm.DB, name string, opts ...any) (*KnowledgeBase, error) {
	if err := AutoMigrate(db); err != nil {
		return nil, utils.Errorf("自动迁移知识库表失败: %v", err)
	}
	var knowledgeBaseInfo schema.KnowledgeBaseInfo
	err := db.Model(&schema.KnowledgeBaseInfo{}).Where("knowledge_base_name = ?", name).First(&knowledgeBaseInfo).Error
	if err != nil {
		return nil, utils.Errorf("获取知识库信息失败: %v", err)
	}

	// 加载 RAG 系统
	ragSystem, err := rag.CreateOrLoadCollection(db, name, "")
	if err != nil {
		return nil, utils.Errorf("加载知识库失败: %v", err)
	}

	return &KnowledgeBase{
		db:        db,
		name:      name,
		ragSystem: ragSystem,
		id:        int64(knowledgeBaseInfo.ID),
	}, nil
}
func LoadKnowledgeBaseByID(db *gorm.DB, id int64, opts ...any) (*KnowledgeBase, error) {
	if err := AutoMigrate(db); err != nil {
		return nil, utils.Errorf("自动迁移知识库表失败: %v", err)
	}
	var knowledgeBaseInfo schema.KnowledgeBaseInfo
	err := db.Model(&schema.KnowledgeBaseInfo{}).Where("id = ?", id).First(&knowledgeBaseInfo).Error
	if err != nil {
		return nil, utils.Errorf("获取知识库信息失败: %v", err)
	}

	return LoadKnowledgeBase(db, knowledgeBaseInfo.KnowledgeBaseName, opts...)
}

// AddKnowledgeEntry 添加知识条目到知识库（使用事务）
func (kb *KnowledgeBase) AddKnowledgeEntry(entry *schema.KnowledgeBaseEntry, options ...rag.DocumentOption) error {
	err := yakit.CreateKnowledgeBaseEntry(kb.db, entry)
	if err != nil {
		return utils.Errorf("创建知识库条目失败: %v", err)
	}

	if err := kb.addEntryToVectorIndex(entry, options...); err != nil {
		return utils.Errorf("添加向量索引失败: %v", err)
	}

	return nil
}

func (kb *KnowledgeBase) AddKnowledgeEntryQuestion(entry *schema.KnowledgeBaseEntry, options ...rag.DocumentOption) error {
	entry.KnowledgeBaseID = kb.id
	err := yakit.CreateKnowledgeBaseEntry(kb.db, entry)
	if err != nil {
		return utils.Errorf("创建知识库条目失败: %v", err)
	}

	if err := kb.addQuestionToVectorIndex(entry, options...); err != nil {
		return utils.Errorf("添加知识问题索引失败: %v", err)
	}

	return nil
}

func (kb *KnowledgeBase) UpdateKnowledgeBaseInfo(name, description, kbType string) error {
	err := yakit.UpdateKnowledgeBaseInfo(kb.db, kb.id, &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        name,
		KnowledgeBaseDescription: description,
		KnowledgeBaseType:        kbType,
	})
	if err != nil {
		return utils.Errorf("更新知识库信息失败: %v", err)
	}
	return nil
}

// Drop 删除当前知识库
func (kb *KnowledgeBase) Drop() error {
	err := kb.ClearDocuments()
	if err != nil {
		return utils.Errorf("清空知识库文档失败: %v", err)
	}
	err = kb.db.Model(&schema.KnowledgeBaseInfo{}).Where("id = ?", kb.id).Unscoped().Delete(&schema.KnowledgeBaseInfo{}).Error
	if err != nil {
		return utils.Errorf("删除知识库信息失败: %v", err)
	}
	rag.DeleteCollection(kb.db, kb.name)
	return nil
}

// UpdateKnowledgeEntry 更新知识条目（使用事务）
func (kb *KnowledgeBase) UpdateKnowledgeEntry(id string, entry *schema.KnowledgeBaseEntry) error {
	entry.HiddenIndex = id
	err := yakit.UpdateKnowledgeBaseEntryByHiddenIndex(kb.db, id, entry)
	if err != nil {
		return utils.Errorf("更新知识库条目失败: %v", err)
	}

	// 然后更新向量索引（事务外进行）
	documentID := utils.InterfaceToString(entry.HiddenIndex)

	// 删除旧的向量索引
	if err := kb.ragSystem.DeleteDocuments(documentID); err != nil {
		return utils.Errorf("删除旧向量索引失败: %v", err)
	}

	// 添加新的向量索引
	if err := kb.addEntryToVectorIndex(entry); err != nil {
		return utils.Errorf("更新向量索引失败: %v", err)
	}
	return nil
}

// DeleteKnowledgeEntry 删除知识条目（使用事务）
func (kb *KnowledgeBase) DeleteKnowledgeEntry(entryID string) error {
	if err := kb.ragSystem.DeleteDocuments(entryID); err != nil {
		return utils.Errorf("删除向量索引失败: %v", err)
	}

	err := yakit.DeleteKnowledgeBaseEntryByHiddenIndex(kb.db, entryID)
	if err != nil {
		return utils.Errorf("删除数据库条目失败: %v", err)
	}
	return nil
}

// KnowledgeEntryWithScore 带相似度分数的知识库条目
type KnowledgeEntryWithScore struct {
	Entry *schema.KnowledgeBaseEntry `json:"entry"`
	Score float64                    `json:"score"`
}

// GetKnowledgeEntry 根据ID获取知识条目
func (kb *KnowledgeBase) GetKnowledgeEntry(entryID string) (*schema.KnowledgeBaseEntry, error) {
	return yakit.GetKnowledgeBaseEntryByHiddenIndex(kb.db, entryID)
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
	kb.ragSystem.ClearDocuments()
	kb.db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", kb.id).Unscoped().Delete(&schema.KnowledgeBaseEntry{})
	return nil
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
func (kb *KnowledgeBase) addEntryToVectorIndex(entry *schema.KnowledgeBaseEntry, options ...rag.DocumentOption) error {
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
		"knowledge_base_id":    entry.KnowledgeBaseID,
		"knowledge_title":      entry.KnowledgeTitle,
		"knowledge_type":       entry.KnowledgeType,
		"importance_score":     entry.ImportanceScore,
		"keywords":             entry.Keywords,
		"source_page":          entry.SourcePage,
		"potential_questions":  entry.PotentialQuestions,
		schema.META_Data_Title: entry.KnowledgeTitle,
		schema.META_Data_UUID:  entry.HiddenIndex,
	}

	// 使用条目ID作为文档ID
	documentID := utils.InterfaceToString(entry.HiddenIndex)
	options = append(options, rag.WithDocumentRawMetadata(metadata), rag.WithDocumentType(schema.RAGDocumentType_Knowledge))

	// 添加文档到RAG系统
	return kb.ragSystem.Add(documentID, content, options...)
}

func (kb *KnowledgeBase) addQuestionToVectorIndex(entry *schema.KnowledgeBaseEntry, options ...rag.DocumentOption) error {
	// 构建元数据
	metadata := map[string]any{
		"knowledge_base_id":    entry.KnowledgeBaseID,
		"knowledge_title":      entry.KnowledgeTitle,
		"knowledge_type":       entry.KnowledgeType,
		"importance_score":     entry.ImportanceScore,
		"keywords":             entry.Keywords,
		"source_page":          entry.SourcePage,
		"potential_questions":  entry.PotentialQuestions,
		schema.META_Data_Title: entry.KnowledgeTitle,
		schema.META_Data_UUID:  entry.HiddenIndex,
	}

	// 使用条目ID作为文档ID
	baseDocumentID := utils.InterfaceToString(entry.HiddenIndex)
	options = append(options, rag.WithDocumentRawMetadata(metadata), rag.WithDocumentType(schema.RAGDocumentType_QuestionIndex))

	for _, question := range entry.PotentialQuestions {
		documentID := fmt.Sprintf("%s_question_%s", baseDocumentID, utils.CalcSha1(question))
		err := kb.ragSystem.Add(documentID, question, options...)
		if err != nil {
			return err
		}
	}
	return nil
}

// SyncKnowledgeBaseWithRAG 同步知识库和RAG，以知识库为准
func (kb *KnowledgeBase) SyncKnowledgeBaseWithRAG() (*SyncResult, error) {
	result := &SyncResult{
		AddedToRAG:     []string{},
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
		dbEntryIDs[entry.HiddenIndex] = true
		dbEntryMap[entry.HiddenIndex] = entry
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
	for entryID, entry := range dbEntryMap {
		if !ragDocumentIDs[entryID] {
			if err := kb.addEntryToVectorIndex(entry); err != nil {
				result.SyncErrors = append(result.SyncErrors,
					fmt.Sprintf("添加条目 %s 到RAG失败: %v", entryID, err))
			} else {
				result.AddedToRAG = append(result.AddedToRAG, entryID)
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
	AddedToRAG        []string `json:"added_to_rag"`        // 添加到RAG的条目ID列表
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
func (kb *KnowledgeBase) BatchSyncEntries(entryIDs []string) (*SyncResult, error) {
	result := &SyncResult{
		AddedToRAG:     []string{},
		DeletedFromRAG: []string{},
		SyncErrors:     []string{},
	}

	for _, entryID := range entryIDs {
		// 获取知识条目
		entry, err := yakit.GetKnowledgeBaseEntryByHiddenIndex(kb.db, entryID)
		if err != nil {
			result.SyncErrors = append(result.SyncErrors,
				fmt.Sprintf("获取条目 %s 失败: %v", entryID, err))
			continue
		}

		// 检查RAG中是否已存在
		documentID := entryID
		_, exists, err := kb.ragSystem.GetDocument(documentID)
		if err != nil {
			result.SyncErrors = append(result.SyncErrors,
				fmt.Sprintf("检查RAG文档 %s 失败: %v", entryID, err))
			continue
		}

		if exists {
			// 如果存在，先删除再添加（更新）
			if err := kb.ragSystem.DeleteDocuments(documentID); err != nil {
				result.SyncErrors = append(result.SyncErrors,
					fmt.Sprintf("删除RAG文档 %s 失败: %v", entryID, err))
				continue
			}
		}

		// 添加到RAG
		if err := kb.addEntryToVectorIndex(entry); err != nil {
			result.SyncErrors = append(result.SyncErrors,
				fmt.Sprintf("添加条目 %s 到RAG失败: %v", entryID, err))
		} else {
			result.AddedToRAG = append(result.AddedToRAG, entryID)
		}
	}

	return result, nil
}

func (kb *KnowledgeBase) EmbedKnowledgeBaseEntry(id string) error {
	entry, err := yakit.GetKnowledgeBaseEntryByHiddenIndex(kb.db, id)
	if err != nil {
		return utils.Errorf("获取知识库条目失败: %v", err)
	}
	return kb.addEntryToVectorIndex(entry)
}

func (kb *KnowledgeBase) EmbedKnowledgeBase() error {
	_, err := rag.BuildVectorIndexForKnowledgeBase(kb.db, kb.id)
	return err
}
