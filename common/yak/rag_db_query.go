package yak

import (
	"context"
	"path/filepath"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// =============================================================================
// DBQuery - 数据库直接查询接口
// 用于快速的数据库模糊搜索，不使用语义搜索/向量搜索
// 适合去重检查、快速验证等场景
// =============================================================================

// DBQueryConfig 数据库查询配置
type DBQueryConfig struct {
	db              *gorm.DB
	collectionNames []string
	ragFilename     string // RAG 文件路径，自动导入后查询
	limit           int
	offset          int
	ctx             context.Context
}

// DBQueryOption 数据库查询选项函数类型
type DBQueryOption func(*DBQueryConfig)

// NewDBQueryConfig 创建默认配置
func NewDBQueryConfig(opts ...DBQueryOption) *DBQueryConfig {
	config := &DBQueryConfig{
		db:     consts.GetGormProfileDatabase(),
		limit:  20,
		offset: 0,
		ctx:    context.Background(),
	}
	for _, opt := range opts {
		opt(config)
	}
	return config
}

// =============================================================================
// DBQuery 选项函数
// =============================================================================

// _dbQueryCollection 指定查询的集合名称（单个）
// Example:
// ```
//
//	results = rag.DBQueryKnowledge("关键词", rag.dbQueryCollection("my-collection"))
//
// ```
func _dbQueryCollection(name string) DBQueryOption {
	return func(config *DBQueryConfig) {
		config.collectionNames = []string{name}
	}
}

// _dbQueryCollections 指定查询的多个集合名称
// Example:
// ```
//
//	results = rag.DBQueryKnowledge("关键词", rag.dbQueryCollections("col1", "col2"))
//
// ```
func _dbQueryCollections(names ...string) DBQueryOption {
	return func(config *DBQueryConfig) {
		config.collectionNames = names
	}
}

// _dbQueryLimit 设置查询结果数量限制
// Example:
// ```
//
//	results = rag.DBQueryKnowledge("关键词", rag.dbQueryLimit(10))
//
// ```
func _dbQueryLimit(limit int) DBQueryOption {
	return func(config *DBQueryConfig) {
		config.limit = limit
	}
}

// _dbQueryOffset 设置查询偏移量（用于分页）
// Example:
// ```
//
//	results = rag.DBQueryKnowledge("关键词", rag.dbQueryOffset(20), rag.dbQueryLimit(10))
//
// ```
func _dbQueryOffset(offset int) DBQueryOption {
	return func(config *DBQueryConfig) {
		config.offset = offset
	}
}

// _dbQueryRAGFilename 从 RAG 文件导入后查询
// 自动导入 RAG 文件到临时集合，然后在该集合上执行查询
// Example:
// ```
//
//	results = rag.DBQueryKnowledge("关键词", rag.dbQueryRAGFilename("/path/to/my.rag"))
//
// ```
func _dbQueryRAGFilename(filename string) DBQueryOption {
	return func(config *DBQueryConfig) {
		config.ragFilename = filename
	}
}

// _dbQueryDB 指定数据库连接
// Example:
// ```
//
//	db = rag.NewRagDatabase("/path/to/db")
//	results = rag.DBQueryKnowledge("关键词", rag.dbQueryDB(db))
//
// ```
func _dbQueryDB(db *gorm.DB) DBQueryOption {
	return func(config *DBQueryConfig) {
		config.db = db
	}
}

// _dbQueryCtx 设置查询上下文
// Example:
// ```
//
//	ctx = context.WithTimeout(context.Background(), 10*time.Second)
//	results = rag.DBQueryKnowledge("关键词", rag.dbQueryCtx(ctx))
//
// ```
func _dbQueryCtx(ctx context.Context) DBQueryOption {
	return func(config *DBQueryConfig) {
		config.ctx = ctx
	}
}

// =============================================================================
// 辅助函数
// =============================================================================

// ensureRAGImported 确保 RAG 文件已导入
// 返回导入后的集合名称
func ensureRAGImported(config *DBQueryConfig) (string, error) {
	if config.ragFilename == "" {
		return "", nil
	}

	// 使用文件名生成集合名
	baseName := filepath.Base(config.ragFilename)
	tempRagName := "dbquery_" + utils.CalcSha256(config.ragFilename)[:8] + "_" + baseName

	db := config.db
	if db == nil {
		db = consts.GetGormProfileDatabase()
	}

	// 检查是否已导入
	if !rag.HasRagSystem(db, tempRagName) {
		// 导入 RAG 文件
		err := rag.ImportRAG(config.ragFilename,
			rag.WithDB(db),
			rag.WithRAGCollectionName(tempRagName),
			rag.WithExportOverwriteExisting(false),
		)
		if err != nil {
			return "", utils.Errorf("failed to import RAG file %s: %v", config.ragFilename, err)
		}
		log.Infof("Imported RAG file %s as collection %s for DB query", config.ragFilename, tempRagName)
	}

	return tempRagName, nil
}

// getKnowledgeBaseIDs 获取集合对应的知识库 ID 列表
func getKnowledgeBaseIDs(db *gorm.DB, collectionNames []string) ([]int64, error) {
	if len(collectionNames) == 0 {
		// 如果没有指定集合，查询所有知识库
		var allKBs []*schema.KnowledgeBaseInfo
		if err := db.Model(&schema.KnowledgeBaseInfo{}).Find(&allKBs).Error; err != nil {
			return nil, err
		}
		ids := make([]int64, len(allKBs))
		for i, kb := range allKBs {
			ids[i] = int64(kb.ID)
		}
		return ids, nil
	}

	// 根据集合名称查找知识库
	var ids []int64
	for _, name := range collectionNames {
		var kb schema.KnowledgeBaseInfo
		if err := db.Model(&schema.KnowledgeBaseInfo{}).Where("knowledge_base_name = ?", name).First(&kb).Error; err == nil {
			ids = append(ids, int64(kb.ID))
		}
	}
	return ids, nil
}

// getEntityRepositoryIDs 获取集合对应的实体仓库 ID 列表
func getEntityRepositoryIDs(db *gorm.DB, collectionNames []string) ([]int64, error) {
	if len(collectionNames) == 0 {
		// 如果没有指定集合，查询所有实体仓库
		var allRepos []*schema.EntityRepository
		if err := db.Model(&schema.EntityRepository{}).Find(&allRepos).Error; err != nil {
			return nil, err
		}
		ids := make([]int64, len(allRepos))
		for i, repo := range allRepos {
			ids[i] = int64(repo.ID)
		}
		return ids, nil
	}

	// 根据集合名称查找实体仓库
	var ids []int64
	for _, name := range collectionNames {
		var repo schema.EntityRepository
		if err := db.Model(&schema.EntityRepository{}).Where("entity_repository_name = ?", name).First(&repo).Error; err == nil {
			ids = append(ids, int64(repo.ID))
		}
	}
	return ids, nil
}

// getCollectionUUIDs 获取集合对应的 UUID 列表
func getCollectionUUIDs(db *gorm.DB, collectionNames []string) ([]string, error) {
	if len(collectionNames) == 0 {
		// 如果没有指定集合，查询所有集合
		var allCollections []*schema.VectorStoreCollection
		if err := db.Model(&schema.VectorStoreCollection{}).Find(&allCollections).Error; err != nil {
			return nil, err
		}
		uuids := make([]string, len(allCollections))
		for i, col := range allCollections {
			uuids[i] = col.UUID
		}
		return uuids, nil
	}

	// 根据集合名称查找 UUID
	var uuids []string
	for _, name := range collectionNames {
		var col schema.VectorStoreCollection
		if err := db.Model(&schema.VectorStoreCollection{}).Where("name = ?", name).First(&col).Error; err == nil {
			uuids = append(uuids, col.UUID)
		}
	}
	return uuids, nil
}

// =============================================================================
// DBQuery 检查函数 - 用于验证知识条目及其向量索引是否完整
// =============================================================================

// DBQueryKnowledgeExistsResult 检查结果
type DBQueryKnowledgeExistsResult struct {
	Exists           bool                       // 是否存在完整的索引（知识+向量）
	KnowledgeEntry   *schema.KnowledgeBaseEntry // 知识条目
	VectorDocCount   int                        // 向量文档数量
	HasKnowledge     bool                       // 是否有知识条目
	HasVectorIndexes bool                       // 是否有向量索引
}

// _dbQueryKnowledgeExists 检查知识条目是否存在且有对应的向量索引
// 这个函数用于增量更新时的去重检查
// 只有当知识条目存在且有对应的向量文档时，才认为该条目已被完整索引
//
// Parameters:
//   - keyword: 搜索关键词（通常是工具/插件名称）
//   - opts: 查询选项
//
// Returns:
//   - *DBQueryKnowledgeExistsResult: 检查结果，包含是否存在、知识条目、向量数量等
//
// Example:
// ```yak
//
//	result = rag.DBQueryKnowledgeExists("get_location", rag.dbQueryRAGFilename("/tmp/caps.rag"))~
//	if result.Exists {
//	    println("Already indexed with", result.VectorDocCount, "vectors")
//	}
//
// ```
func _dbQueryKnowledgeExists(keyword string, opts ...DBQueryOption) (*DBQueryKnowledgeExistsResult, error) {
	config := NewDBQueryConfig(opts...)

	result := &DBQueryKnowledgeExistsResult{
		Exists:           false,
		HasKnowledge:     false,
		HasVectorIndexes: false,
		VectorDocCount:   0,
	}

	// 处理 RAG 文件导入
	if config.ragFilename != "" {
		importedName, err := ensureRAGImported(config)
		if err != nil {
			return result, err
		}
		config.collectionNames = []string{importedName}
	}

	db := config.db
	if db == nil {
		db = consts.GetGormProfileDatabase()
	}

	// 1. 首先查询知识条目
	kbIDs, err := getKnowledgeBaseIDs(db, config.collectionNames)
	if err != nil {
		return result, err
	}

	var matchedEntry *schema.KnowledgeBaseEntry

	if len(kbIDs) == 0 {
		// 查询所有知识库
		paging := &ypb.Paging{Page: 1, Limit: 20}
		_, entries, err := yakit.GetKnowledgeBaseEntryByFilter(db, 0, keyword, paging)
		if err != nil {
			return result, err
		}
		// 查找精确匹配
		for _, entry := range entries {
			if entry.KnowledgeTitle == keyword {
				matchedEntry = entry
				break
			}
		}
	} else {
		// 查询指定的知识库
		for _, kbID := range kbIDs {
			paging := &ypb.Paging{Page: 1, Limit: 20}
			_, entries, err := yakit.GetKnowledgeBaseEntryByFilter(db, kbID, keyword, paging)
			if err != nil {
				continue
			}
			// 查找精确匹配
			for _, entry := range entries {
				if entry.KnowledgeTitle == keyword {
					matchedEntry = entry
					break
				}
			}
			if matchedEntry != nil {
				break
			}
		}
	}

	if matchedEntry == nil {
		// 没有找到知识条目
		return result, nil
	}

	result.HasKnowledge = true
	result.KnowledgeEntry = matchedEntry

	// 2. 查询对应的向量文档
	// 使用知识条目的 HiddenIndex 作为 entry_id 来查找向量文档
	entryID := matchedEntry.HiddenIndex
	if entryID == "" {
		// 没有 entry_id，无法查找向量
		return result, nil
	}

	// 获取集合 UUID
	collectionUUIDs, err := getCollectionUUIDs(db, config.collectionNames)
	if err != nil {
		return result, err
	}

	// 查询向量文档，使用 entry_id 作为搜索关键词
	filter := &yakit.VectorDocumentFilter{
		Keywords: []string{entryID},
	}
	if len(collectionUUIDs) > 0 {
		filter.CollectionUUID = collectionUUIDs[0]
	}

	// 计数向量文档
	vectorCount := 0
	ctx, cancel := context.WithCancel(config.ctx)
	defer cancel()

	for range yakit.SearchVectorStoreDocumentBM25Yield(ctx, db, filter) {
		vectorCount++
		// 限制最大计数，避免过多遍历
		if vectorCount >= 100 {
			break
		}
	}

	result.VectorDocCount = vectorCount
	result.HasVectorIndexes = vectorCount > 0
	result.Exists = result.HasKnowledge && result.HasVectorIndexes

	return result, nil
}

// _dbQueryCountVectorsByEntryID 根据 entry_id 计算向量文档数量
// 用于检查某个知识条目有多少向量索引
//
// Parameters:
//   - entryID: 知识条目的 HiddenIndex
//   - opts: 查询选项
//
// Example:
// ```yak
//
//	count = rag.DBQueryCountVectorsByEntryID("abc123", rag.dbQueryRAGFilename("/tmp/caps.rag"))~
//	println("This entry has", count, "vector indexes")
//
// ```
func _dbQueryCountVectorsByEntryID(entryID string, opts ...DBQueryOption) (int, error) {
	config := NewDBQueryConfig(opts...)

	// 处理 RAG 文件导入
	if config.ragFilename != "" {
		importedName, err := ensureRAGImported(config)
		if err != nil {
			return 0, err
		}
		config.collectionNames = []string{importedName}
	}

	db := config.db
	if db == nil {
		db = consts.GetGormProfileDatabase()
	}

	// 获取集合 UUID
	collectionUUIDs, err := getCollectionUUIDs(db, config.collectionNames)
	if err != nil {
		return 0, err
	}

	// 查询向量文档
	filter := &yakit.VectorDocumentFilter{
		Keywords: []string{entryID},
	}
	if len(collectionUUIDs) > 0 {
		filter.CollectionUUID = collectionUUIDs[0]
	}

	// 计数
	count := 0
	ctx, cancel := context.WithCancel(config.ctx)
	defer cancel()

	for range yakit.SearchVectorStoreDocumentBM25Yield(ctx, db, filter) {
		count++
		if count >= 1000 {
			break
		}
	}

	return count, nil
}

// =============================================================================
// DBQuery 主要查询函数
// =============================================================================

// _dbQueryKnowledge 数据库直接查询知识库条目
// 使用 SQL 模糊搜索，不使用语义搜索，速度非常快（~2ms）
// 适合去重检查、快速验证等场景
//
// Parameters:
//   - keyword: 搜索关键词
//   - opts: 查询选项（集合、限制、偏移等）
//
// Example:
// ```yak
//
//	// 基本用法
//	entries = rag.DBQueryKnowledge("get_location")~
//	for _, entry := range entries {
//	    println(entry.KnowledgeTitle)
//	}
//
//	// 指定集合
//	entries = rag.DBQueryKnowledge("关键词", rag.dbQueryCollection("my-collection"))~
//
//	// 从 RAG 文件查询
//	entries = rag.DBQueryKnowledge("关键词", rag.dbQueryRAGFilename("/tmp/my.rag"))~
//
//	// 分页查询
//	entries = rag.DBQueryKnowledge("关键词", rag.dbQueryLimit(10), rag.dbQueryOffset(20))~
//
// ```
func _dbQueryKnowledge(keyword string, opts ...DBQueryOption) ([]*schema.KnowledgeBaseEntry, error) {
	config := NewDBQueryConfig(opts...)

	// 处理 RAG 文件导入
	if config.ragFilename != "" {
		importedName, err := ensureRAGImported(config)
		if err != nil {
			return nil, err
		}
		config.collectionNames = []string{importedName}
	}

	db := config.db
	if db == nil {
		db = consts.GetGormProfileDatabase()
	}

	// 获取知识库 ID 列表
	kbIDs, err := getKnowledgeBaseIDs(db, config.collectionNames)
	if err != nil {
		return nil, err
	}

	// 如果指定了集合但找不到对应的知识库，返回空结果
	if len(config.collectionNames) > 0 && len(kbIDs) == 0 {
		return []*schema.KnowledgeBaseEntry{}, nil
	}

	// 执行查询
	var allEntries []*schema.KnowledgeBaseEntry

	if len(kbIDs) == 0 {
		// 查询所有知识库
		paging := &ypb.Paging{
			Page:  int64(config.offset/config.limit + 1),
			Limit: int64(config.limit),
		}
		_, entries, err := yakit.GetKnowledgeBaseEntryByFilter(db, 0, keyword, paging)
		if err != nil {
			return nil, err
		}
		allEntries = entries
	} else {
		// 查询指定的知识库
		for _, kbID := range kbIDs {
			paging := &ypb.Paging{
				Page:  int64(config.offset/config.limit + 1),
				Limit: int64(config.limit),
			}
			_, entries, err := yakit.GetKnowledgeBaseEntryByFilter(db, kbID, keyword, paging)
			if err != nil {
				continue
			}
			allEntries = append(allEntries, entries...)
			if len(allEntries) >= config.limit {
				break
			}
		}
	}

	// 限制结果数量
	if len(allEntries) > config.limit {
		allEntries = allEntries[:config.limit]
	}

	return allEntries, nil
}

// _dbQueryUniqueKnowledgeTitles 获取唯一的知识标题列表
// 使用 SQL DISTINCT 查询，返回不重复的 KnowledgeTitle 列表
// 适合增量更新时的快速去重检查
//
// Parameters:
//   - opts: 查询选项（集合、限制等）
//
// Returns:
//   - []string: 唯一的知识标题列表
//
// Example:
// ```yak
//
//	// 获取所有唯一的知识标题
//	titles = rag.DBQueryUniqueKnowledgeTitles(rag.dbQueryCollection("my-collection"))~
//	for _, title := range titles {
//	    println(title)
//	}
//
//	// 从 RAG 文件查询
//	titles = rag.DBQueryUniqueKnowledgeTitles(rag.dbQueryRAGFilename("/tmp/my.rag"))~
//
// ```
func _dbQueryUniqueKnowledgeTitles(opts ...DBQueryOption) ([]string, error) {
	config := NewDBQueryConfig(opts...)

	// 处理 RAG 文件导入
	if config.ragFilename != "" {
		importedName, err := ensureRAGImported(config)
		if err != nil {
			return nil, err
		}
		config.collectionNames = []string{importedName}
	}

	db := config.db
	if db == nil {
		db = consts.GetGormProfileDatabase()
	}

	// 获取知识库 ID 列表
	kbIDs, err := getKnowledgeBaseIDs(db, config.collectionNames)
	if err != nil {
		return nil, err
	}

	// 如果指定了集合但找不到对应的知识库，返回空结果
	if len(config.collectionNames) > 0 && len(kbIDs) == 0 {
		return []string{}, nil
	}

	// 使用 DISTINCT 查询唯一的标题
	var titles []string

	query := db.Model(&schema.KnowledgeBaseEntry{}).Select("DISTINCT knowledge_title")

	// 过滤知识库 ID
	if len(kbIDs) > 0 {
		query = query.Where("knowledge_base_id IN (?)", kbIDs)
	}

	// 只获取非空标题
	query = query.Where("knowledge_title != '' AND knowledge_title IS NOT NULL")

	// 限制结果数量
	if config.limit > 0 {
		query = query.Limit(config.limit)
	}

	// 执行查询
	if err := query.Pluck("knowledge_title", &titles).Error; err != nil {
		return nil, utils.Errorf("failed to query unique knowledge titles: %v", err)
	}

	return titles, nil
}

// _dbQueryEntity 数据库直接查询实体
// 使用 SQL 模糊搜索，不使用语义搜索，速度非常快
// 适合去重检查、快速验证等场景
//
// Parameters:
//   - keyword: 搜索关键词
//   - opts: 查询选项（集合、限制、偏移等）
//
// Example:
// ```yak
//
//	// 基本用法
//	entities = rag.DBQueryEntity("用户")~
//	for _, entity := range entities {
//	    println(entity.EntityName, entity.EntityType)
//	}
//
//	// 指定集合
//	entities = rag.DBQueryEntity("关键词", rag.dbQueryCollection("my-repo"))~
//
//	// 从 RAG 文件查询
//	entities = rag.DBQueryEntity("关键词", rag.dbQueryRAGFilename("/tmp/my.rag"))~
//
// ```
func _dbQueryEntity(keyword string, opts ...DBQueryOption) ([]*schema.ERModelEntity, error) {
	config := NewDBQueryConfig(opts...)

	// 处理 RAG 文件导入
	if config.ragFilename != "" {
		importedName, err := ensureRAGImported(config)
		if err != nil {
			return nil, err
		}
		config.collectionNames = []string{importedName}
	}

	db := config.db
	if db == nil {
		db = consts.GetGormProfileDatabase()
	}

	// 获取实体仓库 ID 列表
	repoIDs, err := getEntityRepositoryIDs(db, config.collectionNames)
	if err != nil {
		return nil, err
	}

	// 构建查询
	query := db.Model(&schema.ERModelEntity{})

	// 如果指定了仓库，添加过滤条件
	if len(repoIDs) > 0 {
		query = query.Where("entity_repository_id IN (?)", repoIDs)
	}

	// 添加关键词模糊搜索
	if keyword != "" {
		searchPattern := "%" + keyword + "%"
		query = query.Where("entity_name LIKE ? OR entity_description LIKE ? OR entity_type LIKE ?",
			searchPattern, searchPattern, searchPattern)
	}

	// 添加分页
	query = query.Offset(config.offset).Limit(config.limit)

	var entities []*schema.ERModelEntity
	if err := query.Find(&entities).Error; err != nil {
		return nil, err
	}

	return entities, nil
}

// _dbQueryVectorDocument 数据库直接查询向量文档
// 使用 SQL 模糊搜索，不使用语义搜索，速度非常快
// 适合去重检查、快速验证等场景
//
// Parameters:
//   - keyword: 搜索关键词
//   - opts: 查询选项（集合、限制、偏移等）
//
// Example:
// ```yak
//
//	// 基本用法
//	docs = rag.DBQueryVectorDocument("关键词")~
//	for _, doc := range docs {
//	    println(doc.DocumentID, doc.Content[:100])
//	}
//
//	// 指定集合
//	docs = rag.DBQueryVectorDocument("关键词", rag.dbQueryCollection("my-collection"))~
//
//	// 从 RAG 文件查询
//	docs = rag.DBQueryVectorDocument("关键词", rag.dbQueryRAGFilename("/tmp/my.rag"))~
//
// ```
func _dbQueryVectorDocument(keyword string, opts ...DBQueryOption) ([]*schema.VectorStoreDocument, error) {
	config := NewDBQueryConfig(opts...)

	// 处理 RAG 文件导入
	if config.ragFilename != "" {
		importedName, err := ensureRAGImported(config)
		if err != nil {
			return nil, err
		}
		config.collectionNames = []string{importedName}
	}

	db := config.db
	if db == nil {
		db = consts.GetGormProfileDatabase()
	}

	// 获取集合 UUID 列表
	collectionUUIDs, err := getCollectionUUIDs(db, config.collectionNames)
	if err != nil {
		return nil, err
	}

	// 使用 yakit 的过滤器和 yield 函数
	filter := &yakit.VectorDocumentFilter{
		Keywords: []string{keyword},
	}

	// 如果指定了集合，只查询第一个集合（后续可以扩展支持多个）
	if len(collectionUUIDs) > 0 {
		filter.CollectionUUID = collectionUUIDs[0]
	}

	// 收集结果
	var documents []*schema.VectorStoreDocument
	ctx, cancel := context.WithCancel(config.ctx)
	defer cancel()

	count := 0
	for doc := range yakit.SearchVectorStoreDocumentBM25Yield(ctx, db, filter) {
		if count >= config.offset {
			documents = append(documents, doc)
		}
		count++
		if len(documents) >= config.limit {
			cancel()
			break
		}
	}

	return documents, nil
}
