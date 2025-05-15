package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
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
func DeleteGeneralRuleGroupByName(outDb *gorm.DB, name []string) (effectRows int64, fErr error) {
	fErr = utils.GormTransaction(outDb, func(tx *gorm.DB) error {
		db := tx.Model(&schema.GeneralRuleGroup{})
		var ids []uint
		db = bizhelper.ExactQueryStringArrayOr(db, "group_name", name)
		if err := db.Pluck("id", &ids).Error; err != nil {
			return err
		}
		if db = db.Unscoped().Delete(&schema.GeneralRuleGroup{}); db.Error != nil {
			return db.Error
		}
		effectRows = db.RowsAffected
		return DeleteGeneralRuleGroupAssociationsByIDOR(tx, nil, ids)
	})
	return
}

func GetGeneralRuleGroupByNames(db *gorm.DB, name []string) ([]*schema.GeneralRuleGroup, error) {
	if len(name) <= 0 {
		return nil, nil
	}
	var groups []*schema.GeneralRuleGroup
	db = bizhelper.ExactQueryStringArrayOr(db, "group_name", name)
	if err := db.Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
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

func AppendGeneralRuleGroupAssociations(db *gorm.DB, rule []*schema.GeneralRule, group []*schema.GeneralRuleGroup) error {
	for _, r := range rule {
		if err := db.Model(r).Association("Groups").Replace(append(r.Groups, group...)).Error; err != nil {
			return err
		}
	}
	return nil
}

func DeleteGeneralRuleGroupAssociations(db *gorm.DB, rule []*schema.GeneralRule, group []*schema.GeneralRuleGroup) error {
	for _, r := range rule {
		if err := db.Model(r).Association("Groups").Delete(group).Error; err != nil {
			return err
		}
	}
	return nil
}

func DeleteGeneralRuleGroupAssociationsByIDOR(db *gorm.DB, ruleID []uint, groupID []uint) error {
	return db.Table("general_rule_and_group").Where("general_rule_id IN (?) OR general_rule_group_id IN (?)", ruleID, groupID).Unscoped().Delete(nil).Error
}

func CreateGeneralMultipleRuleGroup(db *gorm.DB, groups []*schema.GeneralRuleGroup) error {
	for _, group := range groups {
		err := FirstOrCreateGeneralRuleGroup(db, group)
		if err != nil {
			return err
		}
	}
	return nil
}
