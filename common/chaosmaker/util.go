package chaosmaker

import (
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func ParseRuleFromRawSuricataRules(content string) []*rule.Storage {

	var rules []*rule.Storage
	for line := range utils.ParseLines(content) {
		log.Infof("start to handle line: %v", line)
		raw, err := surirule.Parse(line)
		if err != nil {
			log.Errorf("cannot parse suricata raw rules: %s", err)
			continue
		}
		for _, r := range raw {
			rules = append(rules, rule.NewRuleFromSuricata(r))
		}
	}

	return rules
}

func ParseRuleFromHTTPRequestRawJSON(content string) []*rule.Storage {
	var rules []*rule.Storage
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
				rules = append(rules, rule.NewHTTPRequestRule(title, raw))
			} else {
				log.Error("database empty")
			}
		}
	}
	return rules
}

func MockCompleteTCPFlow(raw []byte,toClient bool, mtu int) [][]byte {
	flows, err := pcapx.CompleteTCPFlow(raw,toClient, mtu)
	if err != nil {
		log.Errorf("create tcp flow failed: %v", err)
		return nil
	}
	return flows
}
