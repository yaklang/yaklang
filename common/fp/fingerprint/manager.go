package fingerprint

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
)

func SaveRules(rules ...rule.GeneralRule) error {
	db := consts.GetGormProjectDatabase()
	db = db.Model(&rule.GeneralRule{}).Save(rules)
	if db.Error != nil {
		return db.Error
	}
	return nil
}
func GetRules() ([]*rule.GeneralRule, error) {
	var rules []*rule.GeneralRule
	db := consts.GetGormProjectDatabase()
	db = db.Model(&rule.GeneralRule{}).Find(rules)
	if db.Error != nil {
		return nil, db.Error
	}
	return rules, nil
}
