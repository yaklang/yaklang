package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func SaveAIYakTool(db *gorm.DB, tool *schema.AIYakTool) (int64, error) {
	db = db.Model(&schema.AIYakTool{})
	if db := db.Where("name = ?", tool.Name).Assign(tool).FirstOrCreate(&schema.AIYakTool{}); db.Error != nil {
		return 0, utils.Errorf("create/update AIYakTool failed: %s", db.Error)
	}
	return db.RowsAffected, nil
}

func CreateAIYakTool(db *gorm.DB, tool *schema.AIYakTool) (int64, error) {
	db = db.Model(&schema.AIYakTool{})
	if db := db.Create(tool); db.Error != nil {
		return 0, utils.Errorf("create/update AIYakTool failed: %s", db.Error)
	}
	return db.RowsAffected, nil
}

func UpdateAIYakToolByID(db *gorm.DB, tool *schema.AIYakTool) (int64, error) {
	// 先查询获取现有记录的 CreatedAt
	var existing schema.AIYakTool
	if err := db.Where("id = ?", tool.ID).First(&existing).Error; err != nil {
		return 0, utils.Errorf("find AIYakTool failed: %s", err)
	}
	// 设置 CreatedAt 以确保执行 UPDATE 而不是 INSERT
	tool.CreatedAt = existing.CreatedAt
	if db := db.Save(tool); db.Error != nil {
		return 0, utils.Errorf("update AIYakTool failed: %s", db.Error)
	}
	return db.RowsAffected, nil
}

func GetAIYakTool(db *gorm.DB, name string) (*schema.AIYakTool, error) {
	db = db.Model(&schema.AIYakTool{})
	var tool schema.AIYakTool
	if err := db.Where("name = ?", name).First(&tool).Error; err != nil {
		return nil, err
	}
	return &tool, nil
}

func GetAIYakToolByID(db *gorm.DB, id uint) (*schema.AIYakTool, error) {
	db = db.Model(&schema.AIYakTool{})
	var tool schema.AIYakTool
	if err := db.Where("id = ?", id).First(&tool).Error; err != nil {
		return nil, err
	}
	return &tool, nil
}

func SearchAIYakToolByPath(db *gorm.DB, path string) ([]*schema.AIYakTool, error) {
	db = db.Model(&schema.AIYakTool{})
	var tools []*schema.AIYakTool
	db = bizhelper.FuzzSearchEx(db, []string{"path"}, path, false)
	if err := db.Find(&tools).Error; err != nil {
		return nil, err
	}
	return tools, nil
}
func SearchAIYakTool(db *gorm.DB, keywords string) ([]*schema.AIYakTool, error) {
	db = db.Model(&schema.AIYakTool{})
	var tools []*schema.AIYakTool
	if keywords != "" {
		db = bizhelper.FuzzSearchEx(db, []string{"name", "keywords", "description", "path"}, keywords, false)
	}

	if err := db.Find(&tools).Error; err != nil {
		return nil, err
	}
	return tools, nil
}

func DeleteAIYakTools(db *gorm.DB, names ...string) (int64, error) {
	db = db.Model(&schema.AIYakTool{})
	if db := db.Where("name IN (?)", names).Unscoped().Delete(&schema.AIYakTool{}); db.Error != nil {
		return 0, utils.Errorf("delete AIYakTool failed: %s", db.Error)
	}
	return db.RowsAffected, nil
}

func DeleteAIYakToolByID(db *gorm.DB, ids ...uint) (int64, error) {
	db = db.Model(&schema.AIYakTool{})
	if db := db.Where("id IN (?)", ids).Delete(&schema.AIYakTool{}); db.Error != nil {
		return 0, utils.Errorf("delete AIYakTool failed: %s", db.Error)
	}
	return db.RowsAffected, nil
}

// SearchAIYakToolWithPagination adds pagination support to AIYakTool search
func SearchAIYakToolWithPagination(db *gorm.DB, keywords string, onlyFavorites bool, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.AIYakTool, error) {
	orderBy := paging.GetOrderBy()
	order := paging.GetOrder()
	page := int(paging.GetPage())
	limit := int(paging.GetLimit())

	db = db.Model(&schema.AIYakTool{})

	// Apply fuzzy search if keywords provided
	if keywords != "" {
		db = bizhelper.FuzzSearchEx(db, []string{"name", "keywords", "description", "path"}, keywords, false)
	}

	// Apply favorite filter if requested
	if onlyFavorites {
		db = db.Where("is_favorite = ?", true)
	}

	// Apply ordering
	if orderBy == "" {
		orderBy = "updated_at"
	}
	if order == "" {
		order = "desc"
	}
	db = bizhelper.QueryOrder(db, orderBy, order)

	// Perform paginated query
	var tools []*schema.AIYakTool
	paginator, db := bizhelper.Paging(db, page, limit, &tools)
	if db.Error != nil {
		return nil, nil, utils.Errorf("search AIYakTool with pagination failed: %s", db.Error)
	}

	return paginator, tools, nil
}

// ToggleAIYakToolFavorite toggles the favorite status of an AI tool
func ToggleAIYakToolFavorite(db *gorm.DB, toolName string) (bool, error) {
	db = db.Model(&schema.AIYakTool{})

	var tool schema.AIYakTool
	if err := db.Where("name = ?", toolName).First(&tool).Error; err != nil {
		return false, utils.Errorf("AI tool not found: %s", err)
	}

	// Toggle the favorite status
	tool.IsFavorite = !tool.IsFavorite

	if err := db.Save(&tool).Error; err != nil {
		return false, utils.Errorf("failed to update AI tool favorite status: %s", err)
	}

	return tool.IsFavorite, nil
}

// ToggleAIYakToolFavoriteByID toggles the favorite status of an AI tool by ID
func ToggleAIYakToolFavoriteByID(db *gorm.DB, toolID uint) (bool, error) {
	db = db.Model(&schema.AIYakTool{})

	var tool schema.AIYakTool
	if err := db.Where("id = ?", toolID).First(&tool).Error; err != nil {
		return false, utils.Errorf("AI tool not found: %s", err)
	}

	// Toggle the favorite status
	tool.IsFavorite = !tool.IsFavorite

	if err := db.Save(&tool).Error; err != nil {
		return false, utils.Errorf("failed to update AI tool favorite status: %s", err)
	}

	return tool.IsFavorite, nil
}
