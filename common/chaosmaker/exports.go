package chaosmaker

import (
	"context"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/match"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func yieldRules() chan *rule.Storage {
	return rule.YieldRules(consts.GetGormProfileDatabase().Model(&rule.Storage{}), context.Background())
}

func YieldRulesByKeywords(keywords string, protos ...string) chan *rule.Storage {
	return YieldRulesByKeywordsWithType("", keywords, protos...)
}

func YieldSuricataRulesByKeywords(keywords string, protos ...string) chan *rule.Storage {
	return YieldRulesByKeywordsWithType("suricata", keywords, protos...)
}

func YieldRulesByKeywordsWithType(ruleType string, keywords string, protos ...string) chan *rule.Storage {
	db := consts.GetGormProfileDatabase().Model(&rule.Storage{})
	protos = utils.RemoveRepeatedWithStringSlice(protos)
	if len(protos) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "protocol", protos)
	}
	if ruleType != "" {
		db = db.Where("rule_type = ?", ruleType)
	}
	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
		"name", "keywords",
	}, utils.PrettifyListFromStringSplitEx(keywords, ",", "|"), false)
	return rule.YieldRules(db, context.Background())
}

func LoadSuricataToDatabase(raw string) error {
	rules, err := surirule.Parse(raw)
	if err != nil {
		return err
	}
	for _, r := range rules {
		err := rule.SaveSuricata(consts.GetGormProfileDatabase(), r)
		if err != nil {
			log.Warnf("save suricata error: %s", err)
		}
	}
	return nil
}

var (
	ChaosMakerExports = map[string]any{
		"NewSuricataMatcherGroup": match.NewGroup,
		"groupCallback":           match.WithGroupOnMatchedCallback,

		"NewSuricataMatcher":           match.New,
		"ParseSuricata":                surirule.Parse,
		"YieldRules":                   yieldRules,
		"YieldRulesByKeyword":          YieldRulesByKeywords,
		"YieldSuricataRulesByKeywords": YieldRulesByKeywords,
		"LoadSuricataToDatabase":       LoadSuricataToDatabase,
		"TrafficGenerator":             NewChaosMaker,
	}
)

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {
		match.RegisterSuricataRuleLoader(func(query string) (chan *surirule.Rule, error) {
			var rc = make(chan *surirule.Rule)
			go func() {
				defer close(rc)
				for result := range YieldSuricataRulesByKeywords(query) {
					if result.RuleType == "suricata" {
						srule, err := surirule.Parse(result.SuricataRaw)
						if err != nil {
							continue
						}
						for _, r := range srule {
							rc <- r
						}
					}
				}
			}()
			return rc, nil
		})
		return nil
	}, "register-suricata-rule-loader")
}
