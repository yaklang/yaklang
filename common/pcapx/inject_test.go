package pcapx

import (
	"context"
	"net/http"
	_ "net/http/pprof"
	"yaklang/common/chaosmaker"
	"yaklang/common/consts"
	"yaklang/common/suricata"
	"testing"
)

func init() {
	consts.InitilizeDatabase("", "")
}

func TestChaosRules(t *testing.T) {
	// 注册/pprof路由
	go func() {
		http.ListenAndServe(":6060", nil)
	}()
	for i := range chaosmaker.YieldChaosMakerRules(consts.GetGormProfileDatabase(), context.Background()) {
		mk := chaosmaker.NewChaosMaker()
		mk.FeedRule(i)
		for traffic := range mk.Generate() {
			InjectChaosTraffic(traffic)
		}
	}
}

func TestDebugChaosRules(t *testing.T) {
	type ruleTest struct {
		rule         string
		trafficCount int
		id           string
	}

	var testRules = []*ruleTest{
		//{
		//	rule:         `alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (ppxxmr.com)"; dns_query; content:"ppxxmr.com"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017030; rev:1;)`,
		//	trafficCount: 1,
		//	id:           "debug",
		//},
		//{
		//	rule:         `alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (backup-pool.com)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|0b|backup-pool|03|com|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017009; rev:1;)`,
		//	trafficCount: 2,
		//	id:           "debug",
		//},
		//{
		//	rule:         `alert icmp $EXTERNAL_NET any -> $HOME_NET any (msg:"GPL ICMP L3retriever Ping"; icode:0; itype:8; content:"ABCDEFGHIJKLMNOPQRSTUVWABCDEFGHI"; depth:32; reference:arachnids,311; classtype:attempted-recon; sid:2100466; rev:5;)`,
		//	trafficCount: 2,
		//	id:           "debug",
		//},
		{
			rule:         `alert udp any any -> any any (udp.hdr; content:"|00 08|"; offset:4; depth:2; sid:1234; rev:5;)`,
			trafficCount: 2,
			id:           "debug",
		},
		//{
		//	rule:         `alert tcp $HOME_NET any -> $EXTERNAL_NET any (msg:"Cryptocurrency Miner Check By Submit"; flow:to_server,established; content:"|22|method|22 3a|"; fast_pattern; content:"|22|submit|22 2c|"; distance:0; within:10; content:"|22|params|22 3a 7b|"; distance:0; within:15; content:"result|22 3a|"; nocase; distance:0; classtype:trojan-activity; sid:3013015; rev:1; metadata:Detecting Mining SuricataRules by Charmly;)`,
		//	trafficCount: 2,
		//	id:           "debug",
		//},
	}
	for _, testRule := range testRules {
		rules, err := suricata.Parse(testRule.rule)
		if err != nil {
			panic(err)
		}
		var fRule []*chaosmaker.ChaosMakerRule
		for _, r := range rules {
			fRule = append(fRule, chaosmaker.NewChaosMakerRuleFromSuricata(r))
		}
		mk := chaosmaker.NewChaosMaker()
		mk.FeedRule(fRule...)
		for traffic := range mk.Generate() {
			InjectChaosTraffic(traffic)
		}
	}

}
