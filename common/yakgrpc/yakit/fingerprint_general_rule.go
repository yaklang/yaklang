package yakit

import (
	"encoding/json"
	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"github.com/yaklang/yaklang/embed"
)

func FilterGeneralRule(db *gorm.DB, filter *ypb.FingerprintFilter) *gorm.DB {
	if filter == nil {
		return db
	}

	if len(filter.GetGroupName()) > 0 {
		db = db.Joins("JOIN general_rule_and_group ON general_rule_and_group.general_rule_id = general_rules.id").
			Joins("JOIN general_rule_groups ON general_rule_groups.id = general_rule_and_group.general_rule_group_id").
			Where("general_rule_groups.group_name IN (?)", filter.GetGroupName()).
			Group("general_rules.id")
	}
	db = bizhelper.ExactQueryStringArrayOr(db, "rule_name", filter.RuleName)
	db = bizhelper.ExactQueryStringArrayOr(db, "vendor", filter.Vendor)
	db = bizhelper.ExactQueryStringArrayOr(db, "product", filter.Product)
	db = bizhelper.ExactQueryInt64ArrayOr(db, "id", filter.IncludeId)
	keywordFields := []string{
		"vendor", "product", "part", "rule_name",
		"version", "match_expression", "language",
	}
	db = bizhelper.FuzzSearchEx(db, keywordFields, filter.GetKeyword(), false)
	return db
}

func QueryGeneralRule(db *gorm.DB, filter *ypb.FingerprintFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.GeneralRule, error) {
	db = db.Model(&schema.GeneralRule{}).Preload("Groups")
	db = FilterGeneralRule(db, filter)
	db = bizhelper.OrderByPaging(db, paging)
	ret := []*schema.GeneralRule{}
	pag, db := bizhelper.YakitPagingQuery(db, paging, &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return pag, ret, nil
}

func QueryGeneralRuleFast(db *gorm.DB, filter *ypb.FingerprintFilter) ([]*schema.GeneralRule, error) {
	db = db.Model(&schema.GeneralRule{}).Preload("Groups")
	db = FilterGeneralRule(db, filter)
	var ret []*schema.GeneralRule
	if err := db.Find(&ret).Error; err != nil {
		return nil, err
	}
	return ret, nil
}

func GetGeneralRuleByID(db *gorm.DB, id int64) (*schema.GeneralRule, error) {
	rule := &schema.GeneralRule{}
	if db := db.Where("id = ?", id).First(rule); db.Error != nil {
		return nil, db.Error
	}
	return rule, nil
}

func GetGeneralRuleByRuleName(db *gorm.DB, ruleName string) (*schema.GeneralRule, error) {
	rule := &schema.GeneralRule{}
	if db := db.Model(rule).Preload("Groups").Where("rule_name = ?", ruleName).First(rule); db.Error != nil {
		return nil, db.Error
	}
	return rule, nil
}

// CreateGeneralRule create general rule, if rule.ID is not 0, it will be ignored, will set new
func CreateGeneralRule(db *gorm.DB, rule *schema.GeneralRule) (fErr error) {
	fErr = utils.GormTransaction(db, func(tx *gorm.DB) error {
		if err := tx.Omit("id", "groups").Create(rule).Error; err != nil {
			return utils.Errorf("create fingerprint generalRule failed: %s", err)
		}
		if err := CreateGeneralRuleGroupFromRule(tx, rule); err != nil {
			return err
		}
		return CreateGeneralRuleAndGroupAssociations(tx, []*schema.GeneralRule{rule}, rule.Groups)
	})

	return
}

// UpdateGeneralRuleByRuleName update general rule by rule name(unique index)
func UpdateGeneralRuleByRuleName(outDb *gorm.DB, ruleName string, rule *schema.GeneralRule) (effectRows int64, fErr error) {
	err := utils.GormTransaction(outDb, func(tx *gorm.DB) error {
		db := tx.Model(rule).Omit("id", "Groups") // not update groups
		if db = db.Where("rule_name = ?", ruleName).Updates(rule); db.Error != nil {
			log.Errorf("update generalRule(by rule_name) failed: %s", db.Error)
			return db.Error
		}
		var newRule schema.GeneralRule
		if err := tx.Where("rule_name = ?", ruleName).First(&newRule).Error; err != nil {
			return err
		}
		effectRows = db.RowsAffected
		if err := CreateGeneralRuleGroupFromRule(tx, rule); err != nil {
			return err
		}
		return UpdateGeneralRuleAndGroupAssociations(tx, []*schema.GeneralRule{&newRule}, rule.Groups)
	})
	return effectRows, err
}

// UpdateGeneralRule update general rule by id(primary key)
func UpdateGeneralRule(outDb *gorm.DB, rule *schema.GeneralRule) (effectRows int64, fErr error) {
	err := utils.GormTransaction(outDb, func(tx *gorm.DB) error {
		db := tx.Model(rule).Omit("Groups") // not update groups
		if db = db.Where("id = ?", rule.ID).Updates(rule); db.Error != nil {
			log.Errorf("update generalRule(by rule_name) failed: %s", db.Error)
			return db.Error
		}
		effectRows = db.RowsAffected
		if err := CreateGeneralRuleGroupFromRule(tx, rule); err != nil {
			return err
		}
		return UpdateGeneralRuleAndGroupAssociations(tx, []*schema.GeneralRule{rule}, rule.Groups)
	})
	return effectRows, err
}

// BatchDeleteGeneralRuleGroupAssociations batch delete general rule group associations,
func BatchDeleteGeneralRuleGroupAssociations(outDb *gorm.DB, rules []*schema.GeneralRule, groupName []string) (effectRows int64, fErr error) {
	err := utils.GormTransaction(outDb, func(tx *gorm.DB) error {
		groups, err := GetGeneralRuleGroupByNames(tx, groupName)
		if err != nil {
			return err
		}
		return DeleteGeneralRuleGroupAssociations(tx, rules, groups)
	})
	return effectRows, err
}

// BatchAppendGeneralRuleGroupAssociations  batch append general rule group associations,  rule should exist in database; group if not exist, will create it
func BatchAppendGeneralRuleGroupAssociations(outDb *gorm.DB, rules []*schema.GeneralRule, groupName []string) (effectRows int64, fErr error) {
	err := utils.GormTransaction(outDb, func(tx *gorm.DB) error {
		groups := lo.Map(groupName, func(item string, _ int) *schema.GeneralRuleGroup {
			return &schema.GeneralRuleGroup{GroupName: item}
		})
		if err := CreateGeneralMultipleRuleGroup(tx, groups); err != nil {
			return err
		}
		return AppendGeneralRuleGroupAssociations(tx, rules, groups)
	})
	return effectRows, err
}

func DeleteGeneralRuleByName(db *gorm.DB, ruleName string) (fErr error) {
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		rule, err := GetGeneralRuleByRuleName(tx, ruleName) // should get rule primary key for clear associations
		if err != nil {
			return err
		}
		id := rule.ID
		if err := tx.Model(&schema.GeneralRule{}).Where("rule_name = ?", ruleName).Unscoped().Delete(&schema.GeneralRule{}).Error; err != nil {
			return utils.Errorf("delete GeneralRule failed: %s", db.Error)
		}

		return DeleteGeneralRuleGroupAssociationsByIDOR(tx, []uint{id}, nil)
	})
}

func DeleteGeneralRuleByID(db *gorm.DB, id int64) (fErr error) {
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Unscoped().Delete(&schema.GeneralRule{}).Error; err != nil {
			return err
		}
		return DeleteGeneralRuleGroupAssociationsByIDOR(tx, []uint{uint(id)}, nil)
	})
}

func DeleteGeneralRuleByFilter(outDb *gorm.DB, filter *ypb.FingerprintFilter) (rowCount int64, fErr error) {
	fErr = utils.GormTransaction(outDb, func(tx *gorm.DB) error {
		db := FilterGeneralRule(tx, filter)
		var ids []uint
		if err := db.Model(&schema.GeneralRule{}).Pluck("id", &ids).Error; err != nil {
			return utils.Errorf("query GeneralRule ids failed: %s", err)
		}
		if db = db.Unscoped().Delete(&schema.GeneralRule{}); db.Error != nil {
			return utils.Errorf("delete GeneralRule failed: %s", db.Error)
		}
		rowCount = db.RowsAffected
		return DeleteGeneralRuleGroupAssociationsByIDOR(tx, ids, nil)
	})
	return
}

func ClearGeneralRule(db *gorm.DB) {
	db.DropTableIfExists(&schema.GeneralRule{})
	if db := db.Exec(`UPDATE SQLITE_SEQUENCE SET SEQ=0 WHERE NAME='general_rules';`); db.Error != nil {
		log.Errorf("update sqlite sequence failed: %s", db.Error)
	}
	if db := db.Exec(`UPDATE SQLITE_SEQUENCE SET SEQ=0 WHERE NAME='general_rule_and_group';`); db.Error != nil {
		log.Errorf("update sqlite sequence failed: %s", db.Error)
	}
	db.AutoMigrate(&schema.GeneralRule{})
	return
}

func InsertBuiltinGeneralRules(db *gorm.DB) error {
	builtinRule, err := embed.Asset("data/fp-general-rule.json.gz")
	if err != nil {
		return err
	}
	var rules []*schema.GeneralRule
	err = json.Unmarshal(builtinRule, &rules)
	if err != nil {
		return err
	}

	err = utils.GormTransaction(db, func(tx *gorm.DB) error {
		for _, rule := range rules {
			if err := CreateGeneralRule(tx, rule); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return utils.Wrapf(err, "insert builtin general rules failed")
	}
	return nil
}

func CreateGeneralRuleGroupFromRule(db *gorm.DB, rule *schema.GeneralRule) error {
	return CreateGeneralMultipleRuleGroup(db, rule.Groups)
}
