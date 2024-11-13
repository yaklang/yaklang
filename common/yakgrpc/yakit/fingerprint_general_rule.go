package yakit

import (
	"encoding/json"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"github.com/yaklang/yaklang/embed"
)

func GRPCGeneralRuleToSchemaGeneralRule(gr *ypb.FingerprintRule) *schema.GeneralRule {
	if gr == nil {
		return nil
	}
	cpe := &schema.CPE{}
	if gr.CPE != nil {
		cpe.Part = gr.CPE.Part
		cpe.Vendor = gr.CPE.Vendor
		cpe.Product = gr.CPE.Product
		cpe.Version = gr.CPE.Version
		cpe.Update = gr.CPE.Update
		cpe.Edition = gr.CPE.Edition
		cpe.Language = gr.CPE.Language
	}
	rule := &schema.GeneralRule{
		CPE:             cpe,
		RuleName:        gr.RuleName,
		WebPath:         gr.WebPath,
		ExtInfo:         gr.ExtInfo,
		MatchExpression: gr.MatchExpression,
	}
	rule.ID = uint(gr.Id)
	return rule
}

func FilterGeneralRule(db *gorm.DB, filter *ypb.FingerprintFilter) *gorm.DB {
	if filter == nil {
		return db
	}
	if len(filter.GetVendor()) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "vendor", filter.Vendor)
	}
	if len(filter.GetProduct()) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "product", filter.Product)
	}
	if len(filter.GetIncludeId()) > 0 {
		db = bizhelper.ExactQueryInt64ArrayOr(db, "id", filter.IncludeId)
	}
	return db
}

func QueryGeneralRule(db *gorm.DB, filter *ypb.FingerprintFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.GeneralRule, error) {
	db = FilterGeneralRule(db, filter)
	db = bizhelper.OrderByPaging(db, paging)
	ret := []*schema.GeneralRule{}
	pag, db := bizhelper.Paging(db, int(paging.Page), int(paging.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return pag, ret, nil
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
	if db := db.Where("rule_name = ?", ruleName).First(rule); db.Error != nil {
		return nil, db.Error
	}
	return rule, nil
}

// CreateGeneralRule create general rule, if rule.ID is not 0, it will be ignored, will set new
func CreateGeneralRule(db *gorm.DB, rule *schema.GeneralRule) (fErr error) {
	if db := db.Omit("id").Create(rule); db.Error != nil {
		return utils.Errorf("create fingerprint generalRule failed: %s", db.Error)
	}
	return
}

// UpdateGeneralRuleByRuleName update general rule by rule name(unique index)
func UpdateGeneralRuleByRuleName(outDb *gorm.DB, ruleName string, rule *schema.GeneralRule) (effectRows int, fErr error) {
	oldId := rule.ID // keep schema struct id is current id
	rule.ID = 0
	defer func() {
		rule.ID = oldId
	}()
	db := outDb.Model(rule).Omit("id")
	if db = db.Where("rule_name = ?", ruleName).Updates(rule); db.Error != nil {
		rule.ID = oldId
		log.Errorf("update generalRule(by rule_name) failed: %s", db.Error)
		return 0, db.Error
	}
	return int(db.RowsAffected), nil
}

// UpdateGeneralRule update general rule by id(primary key)
func UpdateGeneralRule(outDb *gorm.DB, rule *schema.GeneralRule) (effectRows int, fErr error) {
	db := outDb.Model(rule).Omit("id")
	if db = db.Updates(rule); db.Error != nil {
		log.Errorf("update generalRule(by id) failed: %s", db.Error)
		return 0, db.Error
	}
	return int(db.RowsAffected), nil
}

// CreateOrUpdateGeneralRuleByRuleName create or update general rule by rule name(unique index)
func CreateOrUpdateGeneralRuleByRuleName(db *gorm.DB, ruleName string, rule *schema.GeneralRule) (fErr error) {
	var ruleCopy schema.GeneralRule
	oldId := rule.ID // keep schema struct id is current id
	rule.ID = 0
	if db := db.Where("rule_name = ?", ruleName).Assign(rule).FirstOrCreate(&ruleCopy); db.Error != nil {
		rule.ID = oldId
		log.Errorf("CreateOrUpdate generalRule(by rule_name) failed: %s", db.Error)
		return db.Error
	}
	rule.ID = ruleCopy.ID
	return nil
}

// CreateOrUpdateGeneralRule create or update general rule by id(primary key)
func CreateOrUpdateGeneralRule(db *gorm.DB, rule *schema.GeneralRule) (fErr error) {
	if db := db.Save(rule); db.Error != nil {
		log.Errorf("CreateOrUpdate generalRule(by id) failed: %s", db.Error)
		return db.Error
	}
	return nil
}

func DeleteGeneralRuleByName(db *gorm.DB, ruleName string) (fErr error) {
	if db := db.Where("rule_name = ?", ruleName).Unscoped().Delete(&schema.GeneralRule{}); db.Error != nil {
		return utils.Errorf("delete GeneralRule failed: %s", db.Error)
	}
	return nil
}

func DeleteGeneralRuleByID(db *gorm.DB, id int64) (fErr error) {
	if db := db.Where("id = ?", id).Unscoped().Delete(&schema.GeneralRule{}); db.Error != nil {
		return utils.Errorf("delete GeneralRule failed: %s", db.Error)
	}
	return nil
}

func DeleteGeneralRuleByFilter(outDb *gorm.DB, filter *ypb.FingerprintFilter) (rowCount int64, fErr error) {
	db := FilterGeneralRule(outDb, filter)
	if db = db.Unscoped().Delete(&schema.GeneralRule{}); db.Error != nil {
		return 0, utils.Errorf("delete GeneralRule failed: %s", db.Error)
	}
	return db.RowsAffected, nil
}

func ClearGeneralRule(db *gorm.DB) {
	db.DropTableIfExists(&schema.GeneralRule{})
	if db := db.Exec(`UPDATE SQLITE_SEQUENCE SET SEQ=0 WHERE NAME='general_rules';`); db.Error != nil {
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
