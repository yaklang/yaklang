package yakit

import (
	"context"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gorm.io/gorm"
)

func SaveAIYakTool(db *gorm.DB, tool *schema.AIYakTool) (int64, error) {
	db = db.Model(&schema.AIYakTool{})
	if tool == nil {
		return 0, utils.Error("ai tool is nil")
	}
	// gorm v2: FirstOrCreate 需要 db 的 Model 已就位，但 Where/Assign 累积进 Statement 无隔离风险(单 finisher)。
	if db := db.Session(&gorm.Session{}).Where("name = ?", tool.Name).Assign(tool.ToUpdateMap()).FirstOrCreate(tool); db.Error != nil {
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
	if tool == nil {
		return 0, utils.Error("ai tool is nil")
	}

	// 先查询获取现有记录的 CreatedAt
	var existing schema.AIYakTool
	// gorm v2: 上游 db 可能已被 Where 污染；First 用独立 Session 隔离。
	if err := db.Session(&gorm.Session{}).Where("id = ?", tool.ID).First(&existing).Error; err != nil {
		return 0, utils.Errorf("find AIYakTool failed: %s", err)
	}

	tool.CreatedAt = existing.CreatedAt
	tool.Author = existing.Author
	tool.IsFavorite = existing.IsFavorite
	if db := db.Model(&schema.AIYakTool{}).Where("id = ?", tool.ID).Updates(tool.ToUpdateMap()); db.Error != nil {
		return 0, utils.Errorf("update AIYakTool failed: %s", db.Error)
	}
	return db.RowsAffected, nil
}

func GetAIYakTool(db *gorm.DB, name string) (*schema.AIYakTool, error) {
	db = db.Model(&schema.AIYakTool{})
	var tool schema.AIYakTool
	if err := db.Session(&gorm.Session{}).Where("name = ?", name).First(&tool).Error; err != nil {
		return nil, err
	}
	return &tool, nil
}

func GetAIYakToolByID(db *gorm.DB, id uint) (*schema.AIYakTool, error) {
	db = db.Model(&schema.AIYakTool{})
	var tool schema.AIYakTool
	if err := db.Session(&gorm.Session{}).Where("id = ?", id).First(&tool).Error; err != nil {
		return nil, err
	}
	return &tool, nil
}

func FilterAIYakTool(db *gorm.DB, filter *ypb.AIToolFilter) *gorm.DB {
	db = db.Model(&schema.AIYakTool{})
	if filter == nil {
		return db
	}
	db = bizhelper.ExactQueryString(db, "name", filter.GetToolName())
	db = bizhelper.ExactQueryStringArrayOr(db, "name", filter.GetToolNames())
	if filter.GetID() > 0 {
		db = bizhelper.ExactQueryInt64(db, "id", filter.GetID())
	}
	if filter.GetKeyword() != "" {
		db = bizhelper.FuzzSearchEx(db, []string{"name", "keywords", "description", "path"}, filter.GetKeyword(), false)
	}
	if filter.GetOnlyFavorites() {
		db = db.Where("is_favorite = ?", true)
	}
	return db
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

func CountAIYakTools(db *gorm.DB, filter *ypb.AIToolFilter) (int64, error) {
	db = FilterAIYakTool(db, filter)
	var count int64
	if db := db.Count(&count); db.Error != nil {
		return 0, utils.Errorf("count AIYakTool failed: %s", db.Error)
	}
	return count, nil
}

func YieldAIYakTools(ctx context.Context, db *gorm.DB, filter *ypb.AIToolFilter) chan *schema.AIYakTool {
	db = FilterAIYakTool(db, filter)
	return bizhelper.YieldModel[*schema.AIYakTool](ctx, db)
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
	if db := db.Where("id IN (?)", ids).Unscoped().Delete(&schema.AIYakTool{}); db.Error != nil {
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
	if err := db.Session(&gorm.Session{}).Where("name = ?", toolName).First(&tool).Error; err != nil {
		return false, utils.Errorf("AI tool not found: %s", err)
	}

	// Toggle the favorite status
	tool.IsFavorite = !tool.IsFavorite

	// gorm v2: Session({}) 仍 clone 已被 Model(&AIYakTool{}) 污染的 Statement(空主键)，Save 按 ID=0 走 UPDATE 无 WHERE 守卫报错。用 NewDB 拿全空 Statement，Save 正确按 tool.ID 走 UPDATE id=?。
	if err := db.Session(&gorm.Session{NewDB: true}).Save(&tool).Error; err != nil {
		return false, utils.Errorf("failed to update AI tool favorite status: %s", err)
	}

	return tool.IsFavorite, nil
}

// ToggleAIYakToolFavoriteByID toggles the favorite status of an AI tool by ID
func ToggleAIYakToolFavoriteByID(db *gorm.DB, toolID uint) (bool, error) {
	db = db.Model(&schema.AIYakTool{})

	var tool schema.AIYakTool
	if err := db.Session(&gorm.Session{}).Where("id = ?", toolID).First(&tool).Error; err != nil {
		return false, utils.Errorf("AI tool not found: %s", err)
	}

	// Toggle the favorite status
	tool.IsFavorite = !tool.IsFavorite

	// gorm v2: Session({}) 仍 clone 已被 Model(&AIYakTool{}) 污染的 Statement(空主键)，Save 按 ID=0 走 UPDATE 无 WHERE 守卫报错。用 NewDB 拿全空 Statement，Save 正确按 tool.ID 走 UPDATE id=?。
	if err := db.Session(&gorm.Session{NewDB: true}).Save(&tool).Error; err != nil {
		return false, utils.Errorf("failed to update AI tool favorite status: %s", err)
	}

	return tool.IsFavorite, nil
}

// YieldAllAITools yields all AI tools from the database
func YieldAllAITools(ctx context.Context, db *gorm.DB) chan *schema.AIYakTool {
	db = db.Model(&schema.AIYakTool{})
	return bizhelper.YieldModel[*schema.AIYakTool](ctx, db)
}
