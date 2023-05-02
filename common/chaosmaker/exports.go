package chaosmaker

import (
	"context"
	"yaklang.io/yaklang/common/consts"
	"yaklang.io/yaklang/common/suricata"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/bizhelper"
)

func yieldRules() chan *ChaosMakerRule {
	return YieldChaosMakerRules(consts.GetGormProfileDatabase().Model(&ChaosMakerRule{}), context.Background())
}

func YieldRulesByKeywords(keywords string, protos ...string) chan *ChaosMakerRule {
	db := consts.GetGormProfileDatabase().Model(&ChaosMakerRule{})
	protos = utils.RemoveRepeatedWithStringSlice(protos)
	if len(protos) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "protocol", protos)
	}
	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
		"name", "keywords",
	}, utils.PrettifyListFromStringSplitEx(keywords, ",", "|"), false)
	return YieldChaosMakerRules(db, context.Background())
}

func LoadSuricataToDatabase(raw string) error {
	rules, err := suricata.Parse(raw)
	if err != nil {
		return err
	}
	for _, r := range rules {
		SaveSuricata(consts.GetGormProfileDatabase(), r)
	}
	return nil
}

var (
	ChaosMakerExports = map[string]interface{}{
		"ParseSuricata":          suricata.Parse,
		"YieldRules":             yieldRules,
		"YieldRulesByKeyword":    YieldRulesByKeywords,
		"LoadSuricataToDatabase": LoadSuricataToDatabase,
		"TrafficGenerator":       NewChaosMaker,
	}
)
