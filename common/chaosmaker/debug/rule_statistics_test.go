package debug

import (
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/chaosmaker"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"strings"
	"testing"
)

func TestRuleStatistics(t *testing.T) {
	rules := GetAllRules()
	i := 0
	for _, s := range rules {
		var err error
		func() {
			//defer func() {
			//	e := recover()
			//	if e != nil {
			//		err = fmt.Errorf("parse error: %v", e)
			//	}
			//}()
			mk := chaosmaker.NewChaosMaker()
			if !strings.Contains(s, "webshell_caidao_php") {
				return
			}
			ruleIns, err := surirule.Parse(s)
			if err != nil {
				log.Errorf("parse rule `%s` failed: %v", s, err)
				return
			}
			storageRules := lo.Map(ruleIns, func(item *surirule.Rule, index int) *rule.Storage {
				return rule.NewRuleFromSuricata(item)
			})
			mk.FeedRule(storageRules...)
			for traffic := range mk.Generate() {
				_ = traffic
			}
		}()
		if err != nil {
			i++
			println(s)
		}
	}
	fmt.Printf("generate failed rules number: %d\n", i)
}
func TestInvalidReRule(t *testing.T) {

}
func TestParseFailedStatistics(t *testing.T) {
	//DelInvalidRules()

	rules := GetAllRules()
	failedRules := []string{}
	for _, s := range rules {
		_, err := surirule.Parse(s)
		if err != nil {
			failedRules = append(failedRules, s)
			log.Errorf("parse rule `%s` failed: %v", s, err)
			continue
		}
	}
	lo.ForEach(failedRules, func(item string, index int) {
		println("rule: ", item)
	})
	fmt.Printf("failed rule number: %d\n", len(failedRules))
}
