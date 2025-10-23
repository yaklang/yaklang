package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// CreateKnowledgeBase 创建知识库
func CreateKnowledgeBase(db *gorm.DB, knowledgeBase *schema.KnowledgeBaseInfo) error {
	db = db.Model(&schema.KnowledgeBaseInfo{})
	err := db.Create(knowledgeBase).Error
	if err != nil {
		return utils.Errorf("create KnowledgeBase failed: %s", err)
	}
	return nil
}

// UpdateKnowledgeBase 更新知识库信息
func UpdateKnowledgeBaseInfo(db *gorm.DB, id int64, knowledgeBase *schema.KnowledgeBaseInfo) error {
	db = db.Model(&schema.KnowledgeBaseInfo{})
	// 先判断是否存在
	count := 0
	db.Where("id = ?", id).Count(&count)
	if count == 0 {
		return utils.Errorf("knowledge base not found")
	} else {
		err := db.Where("id = ?", id).Updates(knowledgeBase).Error
		if err != nil {
			return utils.Errorf("update KnowledgeBase failed: %s", err)
		}
		return nil
	}
}

// DeleteKnowledgeBase 删除知识库和知识库条目
func DeleteKnowledgeBase(db *gorm.DB, id int64) error {
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		tx = tx.Model(&schema.KnowledgeBaseInfo{})
		err := tx.Where("id = ?", id).Unscoped().Delete(&schema.KnowledgeBaseInfo{}).Error
		if err != nil {
			return utils.Errorf("delete KnowledgeBase failed: %s", err)
		}
		err = tx.Where("knowledge_base_id = ?", id).Unscoped().Delete(&schema.KnowledgeBaseEntry{}).Error
		if err != nil {
			return utils.Errorf("delete KnowledgeBaseEntry failed: %s", err)
		}
		return nil
	})
}

// GetKnowledgeBase 获取知识库信息
func GetKnowledgeBase(db *gorm.DB, id int64) (*schema.KnowledgeBaseInfo, error) {
	db = db.Model(&schema.KnowledgeBaseInfo{})
	var knowledgeBase schema.KnowledgeBaseInfo
	err := db.Where("id = ?", id).First(&knowledgeBase).Error
	if err != nil {
		return nil, utils.Errorf("get KnowledgeBase failed: %s", err)
	}
	return &knowledgeBase, nil
}

// GetKnowledgeBase 获取知识库信息
func GetKnowledgeBaseByName(db *gorm.DB, name string) (*schema.KnowledgeBaseInfo, error) {
	db = db.Model(&schema.KnowledgeBaseInfo{})
	var knowledgeBase schema.KnowledgeBaseInfo
	err := db.Where("knowledge_base_name = ?", name).First(&knowledgeBase).Error
	if err != nil {
		return nil, utils.Errorf("get KnowledgeBase failed: %s", err)
	}
	return &knowledgeBase, nil
}

// GetKnowledgeBaseNameList 获取知识库名称列表
func GetKnowledgeBaseNameList(db *gorm.DB) ([]string, error) {
	db = db.Model(&schema.KnowledgeBaseInfo{}).Select("knowledge_base_name")
	var knowledgeBaseNames []string
	err := db.Pluck("knowledge_base_name", &knowledgeBaseNames).Error
	if err != nil {
		return nil, utils.Errorf("get KnowledgeBaseNameList failed: %s", err)
	}
	return knowledgeBaseNames, nil
}

func UpdateKnowledgeBaseEntryByHiddenIndex(db *gorm.DB, hiddenIndex string, knowledgeBase *schema.KnowledgeBaseEntry) error {
	db = db.Model(&schema.KnowledgeBaseEntry{})
	count := 0
	db.Where("hidden_index = ?", hiddenIndex).Count(&count)
	if count == 0 {
		return utils.Errorf("knowledge base entry not found")
	} else {
		err := db.Where("hidden_index = ?", hiddenIndex).Updates(knowledgeBase).Error
		if err != nil {
			return utils.Errorf("update KnowledgeBase failed: %s", err)
		}
		return nil
	}
}

// CreateKnowledgeBaseEntry 创建知识库条目
func CreateKnowledgeBaseEntry(db *gorm.DB, knowledgeBase *schema.KnowledgeBaseEntry) error {
	db = db.Model(&schema.KnowledgeBaseEntry{})
	err := db.Create(knowledgeBase).Error
	if err != nil {
		return utils.Errorf("create/update KnowledgeBase failed: %s", err)
	}
	return nil
}

func DeleteKnowledgeBaseEntryByHiddenIndex(db *gorm.DB, hiddenIndex string) error {
	db = db.Model(&schema.KnowledgeBaseEntry{})
	err := db.Where("hidden_index = ?", hiddenIndex).Unscoped().Delete(&schema.KnowledgeBaseEntry{}).Error
	if err != nil {
		return utils.Errorf("delete KnowledgeBase failed: %s", err)
	}
	return nil
}

// GetKnowledgeBaseEntryByHiddenIndex 根据ID获取知识库条目
func GetKnowledgeBaseEntryByHiddenIndex(db *gorm.DB, hiddenIndex string) (*schema.KnowledgeBaseEntry, error) {
	db = db.Model(&schema.KnowledgeBaseEntry{})
	var knowledgeBase schema.KnowledgeBaseEntry
	err := db.Where("hidden_index = ?", hiddenIndex).First(&knowledgeBase).Error
	if err != nil {
		return nil, utils.Errorf("get KnowledgeBase failed: %s", err)
	}
	return &knowledgeBase, nil
}

// SearchKnowledgeBaseEntry 搜索知识库条目
func SearchKnowledgeBaseEntry(db *gorm.DB, id int64, keyword string) ([]*schema.KnowledgeBaseEntry, error) {
	db = db.Model(&schema.KnowledgeBaseEntry{})
	db = db.Where("knowledge_base_id = ?", id)
	db = bizhelper.FuzzSearchEx(db, []string{"knowledge_title", "knowledge_details", "keywords"}, keyword, false)
	var knowledgeBases []*schema.KnowledgeBaseEntry
	err := db.Find(&knowledgeBases).Error
	if err != nil {
		return nil, utils.Errorf("search KnowledgeBase failed: %s", err)
	}
	return knowledgeBases, nil
}

// FilterKnowledgeBaseEntry 过滤知识库条目
func FilterKnowledgeBaseEntry(db *gorm.DB, entryFilter *ypb.SearchKnowledgeBaseEntryFilter) *gorm.DB {
	if entryFilter == nil {
		return db
	}
	db = db.Model(&schema.KnowledgeBaseEntry{})

	// 精确匹配知识库ID
	if entryFilter.KnowledgeBaseId > 0 {
		db = bizhelper.ExactQueryInt64(db, "knowledge_base_id", entryFilter.KnowledgeBaseId)
	}

	// 多字段模糊搜索
	if entryFilter.Keyword != "" {
		db = bizhelper.FuzzSearchEx(db, []string{"knowledge_title", "knowledge_details", "keywords"}, entryFilter.Keyword, false)
	}

	return db
}

// QueryKnowledgeBaseEntryPaging 分页查询知识库条目
func QueryKnowledgeBaseEntryPaging(db *gorm.DB, entryFilter *ypb.SearchKnowledgeBaseEntryFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.KnowledgeBaseEntry, error) {
	// 1. 设置查询的数据模型
	db = db.Model(&schema.KnowledgeBaseEntry{})

	// 2. 应用过滤条件
	db = FilterKnowledgeBaseEntry(db, entryFilter)

	// 3. 应用排序和分页相关的预处理
	db = bizhelper.OrderByPaging(db, paging)

	// 4. 执行分页查询
	ret := make([]*schema.KnowledgeBaseEntry, 0)
	pag, db := bizhelper.YakitPagingQuery(db, paging, &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return pag, ret, nil
}

// GetKnowledgeBaseEntryByFilter 根据过滤条件获取知识库条目（兼容旧接口）
func GetKnowledgeBaseEntryByFilter(db *gorm.DB, id int64, keyword string, filter *ypb.Paging) (*bizhelper.Paginator, []*schema.KnowledgeBaseEntry, error) {
	entryFilter := &ypb.SearchKnowledgeBaseEntryFilter{
		KnowledgeBaseId: id,
		Keyword:         keyword,
	}
	return QueryKnowledgeBaseEntryPaging(db, entryFilter, filter)
}

func GetKnowledgeBaseEntryByUUID(db *gorm.DB, uuid string) (*schema.KnowledgeBaseEntry, error) {
	db = db.Model(&schema.KnowledgeBaseEntry{})
	var knowledgeBase schema.KnowledgeBaseEntry
	err := db.Where("hidden_index = ?", uuid).First(&knowledgeBase).Error
	if err != nil {
		return nil, utils.Errorf("get KnowledgeBase failed: %s", err)
	}
	return &knowledgeBase, nil
}
