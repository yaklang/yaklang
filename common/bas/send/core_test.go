// Package send
// @Author bcy2007  2023/9/18 14:49
package send

import (
	"fmt"
	"github.com/yaklang/yaklang/common/chaosmaker"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/pcapx"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"packet/core"
	"packet/utils"
	"packet/utils/log"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestRuleSplit(t *testing.T) {
	ruleStr := `alert http any any -> $HOME_NET 8082 (msg:"ET EXPLOIT BlueCoat CAS v1.3.7.1 Report Email Command Injection attempt"; flow:to_server,established; http.method; content:"POST"; nocase; http.uri; content:"/report-email/send"; nocase; http.request_body; content:"/dev-report-overview.html"; nocase; content:"|3B|"; distance:0; pcre:"/\/dev-report-overview\.html[^\"]*?\x3b/i`
	//ruleStr := `alert http any any -> [$HOME_NET,$HTTP_SERVERS] any (msg:"ET EXPLOIT Cisco ASA/Firepower Unauthenticated File Read  (CVE-2020-3452) M3"; flow:established,to_server; http.method; content:"GET"; http.uri; content:"/+CSCO"; fast_pattern; content:"=.."; distance:0; reference:url,tools.cisco.com/security/center/content/CiscoSecurityAdvisory/cisco-sa-asaftd-ro-path-KJuQhB86; reference:cve,2020-3452; classtype:attempted-user; sid:2030585; rev:1; metadata:affected_product Web_Server_Applications, attack_target Networking_Equipment, created_at 2020_07_23, deployment Perimeter, deployment Datacenter, deployment SSLDecrypt, former_category EXPLOIT, signature_severity Major, updated_at 2020_07_23;)`
	//ruleStr := `alert tcp any any -> any any (msg:"Cryptocurrency Miner Check By Submit"; flow:to_server,established; content:"|22|method|22 3a|"; fast_pattern; content:"|22|submit|22 2c|"; distance:0; within:10; content:"|22|params|22 3a 7b|"; distance:0; within:15; content:"result|22 3a|"; nocase; distance:0; classtype:trojan-activity; sid:3013015; rev:1; metadata:Detecting Mining SuricataRules by Charmly;)`
	//starterCompiler, _ := regexp.Compile(`alert\s+[a-zA-Z]+\s+`)
	//result := starterCompiler.Split(ruleStr, 2)
	//fmt.Println(result[1])
	itemCompiler, _ := regexp.Compile(`(any|\$[a-zA-Z_]+|\d+|\[.+])`)
	items := itemCompiler.FindAllString(ruleStr, 4)
	//fmt.Println(items)
	for _, item := range items {
		fmt.Println(item)
	}
	targetStr := fmt.Sprintf("%v %v -> %v %v", "1.2.3.4/32", items[1], "172.16.0.8/32", items[3])
	targetCompiler, _ := regexp.Compile(`(any|\$[a-zA-Z_]+|\d+|\[.+])\s+(any|\$[a-zA-Z_]+|\d+|\[.+])\s+->\s+(any|\$[a-zA-Z_]+|\d+|\[.+])\s+(any|\$[a-zA-Z_]+|\d+|\[.+])`)
	//target := targetCompiler.FindAllString(ruleStr, -1)
	//fmt.Println(target[0])
	finalStr := targetCompiler.ReplaceAllString(ruleStr, targetStr)
	t.Log(finalStr)
}

func TestReadRule(t *testing.T) {
	rulePath := "/Users/chenyangbao/1.txt"
	contentByte, err := utils.ReadFile(rulePath)
	if err != nil {
		log.Errorf("read rule by path error: %v", err)
		return
	}
	content := string(contentByte)
	rules := strings.Split(content, "\n")
	//log.Info(rules)
	for _, rule := range rules {
		log.Info(rule)
	}
}

func TestRule(t *testing.T) {
	_ = RuleParse()
}

func TestPcap(t *testing.T) {
	//handle, _ := pcaputil.OpenFile("test.pcap")
	//handle.WritePacketData()
	//handle.Close()
}

func TestParseIPAddressToByte(t *testing.T) {
	ipaddress := "192.168.0.1"
	items := strings.Split(ipaddress, ".")
	if len(items) != 4 {
		t.Error("length error")
		return
	}
	result := make([]byte, 0)
	for _, item := range items {
		num, err := strconv.Atoi(item)
		if err != nil {
			t.Error(err)
			return
		}
		log.Info(num)
		result = append(result, byte(num))
	}
	t.Log(result)
}

func RuleParse() error {
	//src := "1.2.3.4"
	//dst := "172.16.102.8"
	//ruleStr := `alert http 1.2.3.4 any -> 172.16.102.8 any (msg: "China hacker tools caidao response - column directory"; flow: established,to_client; content:"200"; http_stat_code; content:!"<html>"; http_server_body; content:"|2d 3e|"; http_server_body; depth:2; pcre:"/[\w\d]+\.\w{2,3}\s+\d{4}-\d{2}-\d{2}\s[\d:]{8}/RQ"; classtype:shellcode-detect; sid: 3016010; rev: 1; metadata:created_at 2018_09_13,by al0ne; )`
	ruleStr := `alert tcp 1.2.3.4/32 any -> 172.16.102.51/32 any (msg:"Cryptocurrency Miner Check By Submit"; flow:to_server,established; content:"|22|method|22 3a|"; fast_pattern; content:"|22|submit|22 2c|"; distance:0; within:10; content:"|22|params|22 3a 7b|"; distance:0; within:15; content:"result|22 3a|"; nocase; distance:0; classtype:trojan-activity; sid:3013015; rev:1; metadata:Detecting Mining SuricataRules by Charmly;)`
	rules, err := surirule.Parse(ruleStr)
	if err != nil {
		return utils.Errorf("parse suricate rule error: %v", err)
	}
	var fRule []*rule.Storage
	for _, r := range rules {
		fRule = append(fRule, rule.NewRuleFromSuricata(r))
	}
	mk := chaosmaker.NewChaosMaker()
	mk.FeedRule(fRule...)
	traffics := make([][]byte, 0)

	var traffic []byte
	for traffic = range mk.Generate() {
		traffics = append(traffics, traffic)
	}

	for _, traffic = range traffics {
		for _, t := range traffic {
			fmt.Printf("%02x ", t)
		}
		fmt.Println()
		result, err := core.PacketDataAnalysis(traffic)
		if err != nil {
			log.Errorf("packet data analysis error: %v", err)
			continue
		}
		if len(result) != 0 {
			log.Infof("%v", utils.Md5(result))
		}
		pcapx.InjectRaw(traffic)
	}

	return nil
}

func TestSlice(t *testing.T) {
	temp := make([][]int, 0)
	temp = append(temp, []int{1, 2, 3})
	temp = append(temp, []int{4, 5, 6, 7, 8})
	t.Log(temp)
}
