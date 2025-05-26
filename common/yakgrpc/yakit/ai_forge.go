package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func CreateOrUpdateAIForgeByName(db *gorm.DB, name string, forge *schema.AIForge) error {
	db = db.Model(&schema.AIForge{})
	if db := db.Where("forge_name = ?", name).Assign(forge).FirstOrCreate(&schema.AIForge{}); db.Error != nil {
		return utils.Errorf("create/update AI Forge failed: %s", db.Error)
	}
	return nil
}

func CreateOrUpdateAIForgeByID(db *gorm.DB, id uint, forge *schema.AIForge) error {
	db = db.Model(&schema.AIForge{})
	if db := db.Where("id = ?", id).Assign(forge).FirstOrCreate(&schema.AIForge{}); db.Error != nil {
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
	db = db.Model(&schema.AIForge{})
	if db := db.Where("forge_name = ?", name).Updates(&schema.AIForge{}); db.Error != nil {
		return utils.Errorf("update AI Forge failed: %s", db.Error)
	}
	return nil
}

func UpdateAIForgeByID(db *gorm.DB, id uint, forge *schema.AIForge) error {
	db = db.Model(&schema.AIForge{})
	if db := db.Where("id = ?", id).Updates(&schema.AIForge{}); db.Error != nil {
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

func FilterAIForge(db *gorm.DB, filter *ypb.AIForgeFilter) *gorm.DB {
	db.Model(&schema.AIForge{})
	db = bizhelper.FuzzQueryLike(db, "forge_name", filter.GetForgeName())
	db = bizhelper.ExactQueryString(db, "forge_type", filter.GetForgeType())
	db = bizhelper.FuzzSearchEx(db, []string{
		"forge_name", "forge_content", "init_prompt", "persistent_prompt", "plan_prompt", "result_prompt",
	}, filter.GetKeyword(), false)
	db = bizhelper.ExactQueryStringArrayOr(db, "tags", filter.GetTag())
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

func FilterAIForge(db *gorm.DB, filter *ypb.AIForgeFilter) *gorm.DB {
	db = db.Model(&schema.AIForge{})
	db = bizhelper.FuzzQueryLike(db, "forge_name", filter.GetForgeName())
	db = bizhelper.ExactQueryString(db, "forge_type", filter.GetForgeType())
	db = bizhelper.FuzzSearchEx(db, []string{
		"forge_name", "forge_content", "init_prompt", "persistent_prompt", "plan_prompt", "result_prompt",
	}, filter.GetKeyword(), false)
	db = bizhelper.ExactQueryStringArrayOr(db, "tags", filter.GetTag())
	return db
}

func QueryAIForge(db *gorm.DB, filter *ypb.AIForgeFilter, paging *ypb.Paging) (*bizhelper.Paginator, []schema.AIForge, error) {
	db = FilterAIForge(db, filter)
	db = bizhelper.OrderByPaging(db, paging)
	var forges []schema.AIForge
	pag, db := bizhelper.Paging(db, int(paging.Page), int(paging.Limit), &forges)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return pag, forges, nil
}
