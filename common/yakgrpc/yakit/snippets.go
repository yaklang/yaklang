package yakit

import (
	"errors"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// CreateSnippet 创建自定义代码签名
func CreateSnippet(db *gorm.DB, customCode *schema.Snippets) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}
	if customCode == nil {
		return utils.Errorf("custom code signing is nil")
	}
	if customCode.SnippetName == "" {
		return utils.Errorf("custom code name cannot be empty")
	}
	customCode.SnippetId = uuid.NewString()

	var existing schema.Snippets
	if err := db.Where("snippet_name = ?", customCode.SnippetName).First(&existing).Error; err == nil {
		return utils.Errorf("custom code name already exists")
	}

	return db.Create(customCode).Error
}

// QuerySnippets 根据过滤器获取自定义代码签名
func QuerySnippets(db *gorm.DB, filter *ypb.SnippetsFilter) ([]*schema.Snippets, error) {
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}
	if filter == nil {
		return nil, utils.Errorf("filter cannot be nil")
	}

	var data []*schema.Snippets

	db = db.Model(&schema.Snippets{})
	if filter.GetName() != nil {
		db = bizhelper.ExactQueryStringArrayOr(db, "snippet_name", filter.GetName())
	}
	if err := db.Find(&data).Error; err != nil {
		return nil, err
	}
	return data, nil
}

// GetSnippetsByName 根据名称获取自定义代码签名
func GetSnippetsByName(db *gorm.DB, name string) (*schema.Snippets, error) {
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}
	if name == "" {
		return nil, utils.Errorf("custom code name cannot be empty")
	}

	var customCode schema.Snippets
	if err := db.Where("snippet_name = ?", name).First(&customCode).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.Errorf("custom code signing not found")
		}
		return nil, err
	}

	return &customCode, nil
}

// GetSnippetsByID 根据ID获取自定义代码签名
func GetSnippetsByID(db *gorm.DB, id uint) (*schema.Snippets, error) {
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}
	if id == 0 {
		return nil, utils.Errorf("custom code ID cannot be zero")
	}

	var customCode schema.Snippets
	if err := db.First(&customCode, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.Errorf("custom code signing not found")
		}
		return nil, err
	}

	return &customCode, nil
}

// GetAllSnippetss 获取所有自定义代码签名
func GetAllSnippetss(db *gorm.DB) ([]schema.Snippets, error) {
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}

	var customCodes []schema.Snippets
	if err := db.Find(&customCodes).Error; err != nil {
		return nil, err
	}

	return customCodes, nil
}

// GetSnippetssWithPagination 分页获取自定义代码签名
func GetSnippetssWithPagination(db *gorm.DB, page, pageSize int) ([]schema.Snippets, int64, error) {
	if db == nil {
		return nil, 0, utils.Errorf("database connection is nil")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	var total int64
	if err := db.Model(&schema.Snippets{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var customCodes []schema.Snippets
	offset := (page - 1) * pageSize
	if err := db.Offset(offset).Limit(pageSize).Find(&customCodes).Error; err != nil {
		return nil, 0, err
	}

	return customCodes, total, nil
}

// UpdateSnippet 更新自定义代码签名
func UpdateSnippet(db *gorm.DB, target string, customCode *schema.Snippets) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}
	if customCode == nil {
		return utils.Errorf("custom code signing is nil")
	}

	var existing schema.Snippets
	if err := db.Where("snippet_name = ?", target).First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Errorf("custom code signing not found")
		}
		return err
	}

	if customCode.SnippetName != "" && customCode.SnippetName != target {
		var data []*schema.Snippets
		db := bizhelper.ExactQueryString(db, "snippet_name", customCode.SnippetName)
		if err := db.Find(&data).Error; err != nil {
			return err
		}
		if len(data) > 0 {
			return utils.Errorf("new custom code signing is found")
		}
	}

	customCode.ID = existing.ID
	customCode.CreatedAt = existing.CreatedAt
	customCode.SnippetId = existing.SnippetId
	return db.Save(customCode).Error
}

// DeleteSnippets 根据过滤器删除自定义代码签名
func DeleteSnippets(db *gorm.DB, filter *ypb.SnippetsFilter) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}
	if filter == nil {
		return utils.Errorf("filter cannot be nil")
	}

	if len(filter.Name) == 0 {
		return db.Delete(&schema.Snippets{}).Error
	}
	db = db.Model(&schema.Snippets{})
	if filter.GetName() != nil {
		db = bizhelper.ExactQueryStringArrayOr(db, "snippet_name", filter.GetName())
	}

	return db.Unscoped().Delete(&schema.Snippets{}).Error
}

// DeleteSnippetsByName 根据名称删除自定义代码签名
func DeleteSnippetsByName(db *gorm.DB, name string) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}
	if name == "" {
		return utils.Errorf("custom code name cannot be empty")
	}

	var customCode schema.Snippets
	if err := db.Where("snippet_name = ?", name).First(&customCode).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Errorf("custom code signing not found")
		}
		return err
	}

	return db.Delete(&customCode).Error
}

// GetSnippetssCount 获取自定义代码签名总数
func GetSnippetssCount(db *gorm.DB) (int64, error) {
	if db == nil {
		return 0, utils.Errorf("database connection is nil")
	}

	var count int64
	if err := db.Model(&schema.Snippets{}).Count(&count).Error; err != nil {
		return 0, err
	}

	return count, nil
}

// BulkCreateSnippetss 批量创建自定义代码签名
func BulkCreateSnippetss(db *gorm.DB, customCodes []schema.Snippets) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}
	if len(customCodes) == 0 {
		return utils.Errorf("custom codes cannot be empty")
	}

	for _, customCode := range customCodes {
		if customCode.SnippetName == "" {
			return utils.Errorf("custom code name cannot be empty")
		}
	}

	names := make(map[string]bool)
	for _, customCode := range customCodes {
		if names[customCode.SnippetName] {
			return utils.Errorf("duplicate custom code name found in batch")
		}
		names[customCode.SnippetName] = true
	}

	var existingNames []string
	if err := db.Model(&schema.Snippets{}).Where("snippet_name IN ?", names).Pluck("snippet_name", &existingNames).Error; err != nil {
		return err
	}

	if len(existingNames) > 0 {
		return utils.Errorf("some custom code names already exist in database")
	}

	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	for _, customCode := range customCodes {
		if err := tx.Create(&customCode).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

// BulkDeleteSnippetss 批量删除自定义代码签名
func BulkDeleteSnippetss(db *gorm.DB, names []string) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}
	if len(names) == 0 {
		return utils.Errorf("names cannot be empty")
	}

	for _, name := range names {
		if name == "" {
			return utils.Errorf("custom code name cannot be empty")
		}
	}

	var count int64
	if err := db.Model(&schema.Snippets{}).Where("snippet_name IN ?", names).Count(&count).Error; err != nil {
		return err
	}

	if count == 0 {
		return utils.Errorf("no custom code signings found to delete")
	}

	return db.Where("snippet_name IN ?", names).Delete(&schema.Snippets{}).Error
}

// SearchSnippetss 搜索自定义代码签名
func SearchSnippetss(db *gorm.DB, query string) ([]schema.Snippets, error) {
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}
	if query == "" {
		return GetAllSnippetss(db)
	}

	var customCodes []schema.Snippets
	searchQuery := "%" + query + "%"
	if err := db.Where("snippet_name LIKE ? OR snippet_data LIKE ?", searchQuery, searchQuery).Find(&customCodes).Error; err != nil {
		return nil, err
	}

	return customCodes, nil
}
