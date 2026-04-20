package yakit

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func normalizeAIForgeUpsertData(forge *schema.AIForge) map[string]interface{} {
	if forge == nil {
		return nil
	}
	return forge.ToUpdateMap()
}

func setAIForgeCreateField(forge *schema.AIForge, field string, value interface{}) {
	if forge == nil {
		return
	}

	switch field {
	case "forge_name":
		if v, ok := value.(string); ok {
			forge.ForgeName = v
		}
	case "id":
		switch v := value.(type) {
		case uint:
			forge.ID = v
		case int:
			forge.ID = uint(v)
		case int64:
			forge.ID = uint(v)
		}
	}
}

func CreateOrUpdateAIForgeByName(db *gorm.DB, name string, forge *schema.AIForge) error {
	db = db.Model(&schema.AIForge{})
	if forge == nil {
		return utils.Error("ai forge is nil")
	}

	setAIForgeCreateField(forge, "forge_name", name)
	if db := db.Where("forge_name = ?", name).Assign(normalizeAIForgeUpsertData(forge)).FirstOrCreate(forge); db.Error != nil {
		return utils.Errorf("create/update AI Forge failed: %s", db.Error)
	}
	return nil
}

func CreateOrUpdateAIForgeByID(db *gorm.DB, id uint, forge *schema.AIForge) error {
	db = db.Model(&schema.AIForge{})
	if forge == nil {
		return utils.Error("ai forge is nil")
	}

	setAIForgeCreateField(forge, "id", id)
	if db := db.Where("id = ?", id).Assign(normalizeAIForgeUpsertData(forge)).FirstOrCreate(forge); db.Error != nil {
		return utils.Errorf("create/update AI Forge failed: %s", db.Error)
	}
	return nil
}

func CreateOrUpdateAIForge(db *gorm.DB, forge *schema.AIForge) error {
	if forge.ID > 0 {
		return CreateOrUpdateAIForgeByID(db, forge.ID, forge)
	}
	return CreateOrUpdateAIForgeByName(db, forge.ForgeName, forge)
}

func UpdateAIForgeByName(db *gorm.DB, name string, forge *schema.AIForge) error {
	if forge == nil {
		return utils.Error("ai forge is nil")
	}

	// 先查询获取现有记录的 ID 和 CreatedAt
	var existing schema.AIForge
	if err := db.Where("forge_name = ?", name).First(&existing).Error; err != nil {
		return utils.Errorf("find AI Forge failed: %s", err)
	}

	forge.ID = existing.ID
	forge.CreatedAt = existing.CreatedAt
	forge.Author = existing.Author
	if db := db.Model(&schema.AIForge{}).Where("id = ?", existing.ID).Updates(normalizeAIForgeUpsertData(forge)); db.Error != nil {
		return utils.Errorf("update AI Forge failed: %s", db.Error)
	}
	return nil
}

func UpdateAIForgeByID(db *gorm.DB, id uint, forge *schema.AIForge) error {
	if forge == nil {
		return utils.Error("ai forge is nil")
	}

	// 先查询获取现有记录的 CreatedAt
	var existing schema.AIForge
	if err := db.Where("id = ?", id).First(&existing).Error; err != nil {
		return utils.Errorf("find AI Forge failed: %s", err)
	}

	forge.ID = id
	forge.CreatedAt = existing.CreatedAt
	forge.Author = existing.Author
	if db := db.Model(&schema.AIForge{}).Where("id = ?", id).Updates(normalizeAIForgeUpsertData(forge)); db.Error != nil {
		return utils.Errorf("update AI Forge failed: %s", db.Error)
	}
	return nil
}

func UpdateAIForge(db *gorm.DB, forge *schema.AIForge) error {
	if forge.ID > 0 {
		return UpdateAIForgeByID(db, forge.ID, forge)
	}
	return UpdateAIForgeByName(db, forge.ForgeName, forge)
}

func CreateAIForge(db *gorm.DB, forge *schema.AIForge) error {
	if db := db.Create(forge); db.Error != nil {
		return utils.Errorf("create AI Forge failed: %s", db.Error)
	}
	return nil
}

func DeleteAIForgeByName(db *gorm.DB, name string) error {
	var forge schema.AIForge
	if db := db.Unscoped().Where("forge_name = ?", name).Delete(&forge); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteAIForge(db *gorm.DB, filter *ypb.AIForgeFilter) (int64, error) {
	db = FilterAIForge(db, filter)
	if db = db.Unscoped().Delete(&schema.AIForge{}); db.Error != nil {
		return 0, db.Error
	}
	return db.RowsAffected, nil
}

func GetAIForgeByName(db *gorm.DB, name string) (*schema.AIForge, error) {
	var forge schema.AIForge
	if db := db.Where("forge_name = ?", name).First(&forge); db.Error != nil {
		return nil, db.Error
	}
	return &forge, nil
}

func GetAIForgeByNameAndTypes(db *gorm.DB, name string, forgeTypes ...string) (*schema.AIForge, error) {
	query := db.Where("forge_name = ?", name)
	if len(forgeTypes) > 0 {
		query = query.Where("forge_type IN (?)", forgeTypes)
	}
	var forge schema.AIForge
	if db := query.First(&forge); db.Error != nil {
		return nil, db.Error
	}
	return &forge, nil
}

func GetAIForgesByType(db *gorm.DB, forgeType string) ([]*schema.AIForge, error) {
	var forges []*schema.AIForge
	if db := db.Where("forge_type = ?", forgeType).Find(&forges); db.Error != nil {
		return nil, db.Error
	}
	return forges, nil
}

func GetAIForgeByID(db *gorm.DB, id int64) (*schema.AIForge, error) {
	var forge schema.AIForge
	if db := db.Where("id = ?", id).First(&forge); db.Error != nil {
		return nil, db.Error
	}
	return &forge, nil
}

func FilterAIForge(db *gorm.DB, filter *ypb.AIForgeFilter) *gorm.DB {
	db = db.Model(&schema.AIForge{})
	if filter.GetShowTemporary() {
		db = db.Where("is_temporary = ?", true)
	}
	db = bizhelper.FuzzQueryLike(db, "forge_name", filter.GetForgeName())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "forge_name", filter.GetForgeNames())
	db = bizhelper.ExactQueryString(db, "forge_type", filter.GetForgeType())
	db = bizhelper.FuzzSearchEx(db, []string{
		"forge_name", "forge_content", "init_prompt", "persistent_prompt", "plan_prompt", "result_prompt",
	}, filter.GetKeyword(), false)
	db = bizhelper.ExactQueryStringArrayOr(db, "tags", filter.GetTag())
	if filter.GetId() > 0 {
		db = bizhelper.ExactQueryInt64(db, "id", filter.GetId())
	}
	return db
}

func QueryAIForge(db *gorm.DB, filter *ypb.AIForgeFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.AIForge, error) {
	db = FilterAIForge(db, filter)
	db = bizhelper.OrderByPaging(db, paging)
	var forges []*schema.AIForge
	pag, db := bizhelper.Paging(db, int(paging.Page), int(paging.Limit), &forges)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return pag, forges, nil
}

func GetAllAIForge(db *gorm.DB) ([]*schema.AIForge, error) {
	var forges []*schema.AIForge
	if db := db.Find(&forges); db.Error != nil {
		return nil, db.Error
	}
	return forges, nil
}

// YieldAllAIForges yields all AI forges from the database
func YieldAllAIForges(ctx context.Context, db *gorm.DB) chan *schema.AIForge {
	db = db.Model(&schema.AIForge{})
	return bizhelper.YieldModel[*schema.AIForge](ctx, db)
}

func CountAIForges(db *gorm.DB, filter *ypb.AIForgeFilter) (int64, error) {
	db = FilterAIForge(db, filter)
	var count int64
	if db := db.Count(&count); db.Error != nil {
		return 0, utils.Errorf("count AI Forges failed: %s", db.Error)
	}
	return count, nil
}

func YieldAIForges(ctx context.Context, db *gorm.DB, filter *ypb.AIForgeFilter) chan *schema.AIForge {
	db = FilterAIForge(db, filter)
	return bizhelper.YieldModel[*schema.AIForge](ctx, db)
}
