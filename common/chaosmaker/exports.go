package chaosmaker

import (
	"context"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/suricata/match"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

func yieldRules() chan *rule.Storage {
	return rule.YieldRules(consts.GetGormProfileDatabase().Model(&rule.Storage{}), context.Background())
}

func YieldRulesByKeywords(keywords string, protos ...string) chan *rule.Storage {
	db := consts.GetGormProfileDatabase().Model(&rule.Storage{})
	protos = utils.RemoveRepeatedWithStringSlice(protos)
	if len(protos) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "protocol", protos)
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
		rule.SaveSuricata(consts.GetGormProfileDatabase(), r)
	}
	return nil
}

var (
	ChaosMakerExports = map[string]any{
		"SuricataMatcher":        match.New,
		"ParseSuricata":          surirule.Parse,
		"YieldRules":             yieldRules,
		"YieldRulesByKeyword":    YieldRulesByKeywords,
		"LoadSuricataToDatabase": LoadSuricataToDatabase,
		"TrafficGenerator":       NewChaosMaker,
	}
)
