package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"runtime/debug"
)

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

func GRPCGeneralRuleToSchemaGeneralRule(gr *ypb.FingerprintRule) *schema.GeneralRule {
	if gr == nil {
		return nil
	}
	cpe := &schema.CPE{}
	if gr.Cpe != nil {
		cpe.Part = gr.Cpe.Part
		cpe.Vendor = gr.Cpe.Vendor
		cpe.Product = gr.Cpe.Product
		cpe.Version = gr.Cpe.Version
		cpe.Update = gr.Cpe.Update
		cpe.Edition = gr.Cpe.Edition
		cpe.Language = gr.Cpe.Language
	}
	return &schema.GeneralRule{
		CPE:             cpe,
		RuleName:        gr.RuleName,
		WebPath:         gr.WebPath,
		ExtInfo:         gr.ExtInfo,
		MatchExpression: gr.MatchExpression,
	}
}

func CreateGeneralRule(db *gorm.DB, rule *schema.GeneralRule) (fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = utils.Errorf("met panic error: %v", err)
			debug.PrintStack()
		}
	}()
	rule.ID = 0
	if db = db.Create(rule); db.Error != nil {
		return utils.Errorf("insert HTTPFlow failed: %s", db.Error)
	}
	return
}

func UpdateGeneralRule(outDb *gorm.DB, rule *schema.GeneralRule) (fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = utils.Errorf("met panic error: %v", err)
			debug.PrintStack()
		}
	}()
	db := outDb.Model(rule)
	if rule.ID > 0 {
		if db = db.Where("id = ?", rule.ID).Updates(rule); db.Error != nil {
			log.Errorf("update generalRule(by id) failed: %s", db.Error)
			return db.Error
		}
	} else if rule.RuleName != "" {
		if db = db.Where("rule_name = ?", rule.RuleName).Updates(rule); db.Error != nil {
			log.Errorf("update generalRule(by rule_name) failed: %s", db.Error)
			return db.Error
		}
	} else {
		return utils.Errorf("no id or rule_name provided")
	}
	return nil
}

func CreateOrUpdateGeneralRule(db *gorm.DB, name string, i *schema.GeneralRule) (fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = utils.Errorf("met panic error: %v", err)
		}
	}()

	if db := db.Where("rule_name = ?", name).Assign(i).FirstOrCreate(&schema.GeneralRule{}); db.Error != nil {
		return utils.Errorf("create/update GeneralRule failed: %s", db.Error)
	}
	return nil
}

func DeleteGeneralRuleByName(db *gorm.DB, ruleName string) (fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = utils.Errorf("met panic error: %v", err)
		}
	}()
	if db = db.Where("rule_name = ?", ruleName).Unscoped().Delete(&schema.GeneralRule{}); db.Error != nil {
		return utils.Errorf("delete GeneralRule failed: %s", db.Error)
	}
	return nil
}

func DeleteGeneralRuleByID(db *gorm.DB, id int64) (fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = utils.Errorf("met panic error: %v", err)
		}
	}()
	if db = db.Where("id = ?", id).Delete(&schema.GeneralRule{}); db.Error != nil {
		return utils.Errorf("delete GeneralRule failed: %s", db.Error)
	}
	return nil
}

func DeleteGeneralRuleByIds(db *gorm.DB, ids []int64) (fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = utils.Errorf("met panic error: %v", err)
		}
	}()
	if db = db.Where("id IN (?)").Unscoped().Delete(&schema.GeneralRule{}); db.Error != nil {
		return utils.Errorf("delete GeneralRule failed: %s", db.Error)
	}
	return nil
}

func GetGeneralRuleByID(db *gorm.DB, id int64) (*schema.GeneralRule, error) {
	rule := &schema.GeneralRule{}
	if db = db.Where("id = ?", id).First(rule); db.Error != nil {
		return nil, db.Error
	}
	return rule, nil
}
