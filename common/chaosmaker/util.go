package chaosmaker

import (
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"yaklang.io/yaklang/common/consts"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/suricata"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yak/yaklib/codec"
)

func ParseRuleFromRawSuricataRules(content string) []*ChaosMakerRule {

	var rules []*ChaosMakerRule
	for line := range utils.ParseLines(content) {
		log.Infof("start to handle line: %v", line)
		raw, err := suricata.Parse(line)
		if err != nil {
			log.Errorf("cannot parse suricata raw rules: %s", err)
			continue
		}
		for _, r := range raw {
			rules = append(rules, NewChaosMakerRuleFromSuricata(r))
		}
	}

	return rules
}

func ParseRuleFromHTTPRequestRawJSON(content string) []*ChaosMakerRule {
	var rules []*ChaosMakerRule
	for i := range utils.ParseLines(content) {
		var r = map[string]string{}
		err := json.Unmarshal([]byte(i), &r)
		if err != nil {
			log.Error(err)
			continue
		}
		if ret, _ := r["request_base64"]; ret == "" {
			spew.Dump(r)
			continue
		} else {
			raw, _ := codec.DecodeBase64(ret)
			_ = raw
			title, _ := r["title"]
			db := consts.GetGormProfileDatabase()
			if db != nil {
				rules = append(rules, NewHTTPRequestChaosMakerRule(title, raw))
			} else {
				log.Error("database empty")
			}
		}
	}
	return rules
}
