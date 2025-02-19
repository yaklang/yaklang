package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// CreateGeneralRuleGroup create general rule group , omit rules and id
func CreateGeneralRuleGroup(db *gorm.DB, rule *schema.GeneralRuleGroup) (fErr error) {
	if db := db.Omit("Rules").Model(&schema.GeneralRuleGroup{}).Create(rule); db.Error != nil {
		return utils.Errorf("create fingerprint generalRule group failed: %s", db.Error)
	}
	return
}

// RenameGeneralRuleGroupName update general rule group , omit rules
func RenameGeneralRuleGroupName(db *gorm.DB, name string, newNane string) (fErr error) {
	if db := db.Omit("id", "Rules").Model(&schema.GeneralRuleGroup{}).Where("group_name = ?", name).Update("group_name", newNane); db.Error != nil {
		return utils.Errorf("create fingerprint generalRule group failed: %s", db.Error)
	}
	return
}

// UpdateGeneralRuleGroup update general rule group , omit rules
func UpdateGeneralRuleGroup(db *gorm.DB, rule *schema.GeneralRuleGroup) (fErr error) {
	if db := db.Omit("id", "Rules").Model(&schema.GeneralRuleGroup{}).Updates(rule); db.Error != nil {
		return utils.Errorf("create fingerprint generalRule group failed: %s", db.Error)
	}
	return
}

// GetAllGeneralRuleGroup get all general rule group
func GetAllGeneralRuleGroup(db *gorm.DB) ([]*schema.GeneralRuleGroup, error) {
	var groups []*schema.GeneralRuleGroup
	if db := db.Preload("Rules").Find(&groups); db.Error != nil {
		return nil, db.Error
	}
	return groups, nil
}

// GetGeneralRuleGroupByName get all general rule group
func GetGeneralRuleGroupByName(db *gorm.DB, name string) (*schema.GeneralRuleGroup, error) {
	var group schema.GeneralRuleGroup
	if db := db.Preload("Rules").Where("group_name = ?", name).First(&group); db.Error != nil {
		return nil, db.Error
	}
	return &group, nil
}

// DeleteGeneralRuleGroupByName delete general rule group by group name
func DeleteGeneralRuleGroupByName(db *gorm.DB, name []string) (effectRows int64, fErr error) {
	fErr = utils.GormTransaction(db, func(tx *gorm.DB) error {
		db := tx.Model(&schema.SyntaxFlowGroup{})
		var ids []uint
		if err := db.Where("group_name IN (?)", name).Pluck("id", &ids).Error; err != nil {
			return err
		}
		if db = db.Unscoped().Where("group_name = ?", name).Delete(&schema.GeneralRuleGroup{}); db.Error != nil {
			return db.Error
		}
		effectRows = db.RowsAffected
		return DeleteGeneralRuleGroupAssociationsByID(tx, nil, ids)
	})
	return
}

func FirstOrCreateGeneralRuleGroup(db *gorm.DB, group *schema.GeneralRuleGroup) error {
	return db.Where(schema.GeneralRuleGroup{GroupName: group.GroupName}).FirstOrCreate(group).Error
}

// ! use in transaction
// CreateGeneralRuleAndGroupAssociations create general rule and group associations, require rule and group are in database
func CreateGeneralRuleAndGroupAssociations(db *gorm.DB, rule []*schema.GeneralRule, group []*schema.GeneralRuleGroup) error {
	for _, r := range rule {
		if err := db.Model(r).Association("Groups").Append(group).Error; err != nil {
			return err
		}
	}
	return nil
}

// ! use in transaction
// UpdateGeneralRuleAndGroupAssociations create general rule and group associations, require rule and group are in database
func UpdateGeneralRuleAndGroupAssociations(db *gorm.DB, rule []*schema.GeneralRule, group []*schema.GeneralRuleGroup) error {
	for _, r := range rule {
		if err := db.Model(r).Association("Groups").Replace(group).Error; err != nil {
			return err
		}
	}
	return nil
}

func DeleteGeneralRuleGroupAssociationsByID(db *gorm.DB, ruleID []uint, groupID []uint) error {
	return db.Table("general_rule_and_group").Where("general_rule_id IN (?) OR general_rule_group_id IN (?)", ruleID, groupID).Unscoped().Delete(nil).Error
}
