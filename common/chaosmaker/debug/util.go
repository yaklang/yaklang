package debug

import (
	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
)

type Rule struct {
	Content string
}

func DelInvalidRules() {
	dbPath := "root:123456@tcp(127.0.0.1:3306)/suricata_rules?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(`mysql`, dbPath)
	if err != nil {
		log.Errorf("open db failed: %v", err)
		return
	}
	allRules := []*Rule{}
	db.Table("bas_rules").Find(&allRules)
	lo.ForEach(allRules, func(item *Rule, index int) {
		ruleStr := item.Content
		_, err := surirule.Parse(ruleStr)
		if err != nil {
			db.Table("bas_rules").Where("content = ?", item.Content).Delete(item)
		}
	})
	return
}
func GetAllRules() []string {
	dbPath := "root:123456@tcp(127.0.0.1:3306)/suricata_rules?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(`mysql`, dbPath)
	if err != nil {
		log.Errorf("open db failed: %v", err)
		return []string{}
	}
	allRules := []*Rule{}
	db.Table("bas_rules").Find(&allRules)
	allRuleStr := []string{}
	lo.ForEach(allRules, func(item *Rule, index int) {
		ruleStr := item.Content
		allRuleStr = append(allRuleStr, ruleStr)
	})
	return allRuleStr
}
