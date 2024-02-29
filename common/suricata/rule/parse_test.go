package rule

import (
	_ "embed"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

//go:embed badrules.rules
var badrule string

//go:embed badrules1.rules
var badrule_mini string

func TestMUSTPASS_block(t *testing.T) {
	rules, err := Parse(badrule)
	if err != nil {
		t.Fatal(err)
	}
	_ = rules
}

func TestMUSTPASS_block1(t *testing.T) {
	rules, err := Parse(badrule_mini)
	if err != nil {
		t.Fatal(err)
	}
	_ = rules
}

func TestMUSTPASS_Parse(t *testing.T) {
	rules, err := Parse(`alert http any any -> any any (msg:"webshell_caidao_php"; flow:established; content:"POST";http_method; content:".php"; http_uri; content:"base64_decode"; http_client_body; classtype:shellcode-detect; sid:3016009; rev:1; metadata:by al0ne;)
alert http $EXTERNAL_NET any -> $HOME_NET any (msg: "China hacker tools caidao response - column directory"; flow: established,to_client; content:"200"; http_stat_code; content:!"<html>"; http_server_body; content:"|2d 3e|"; http_server_body; depth:2; pcre:"/[\w\d]+\.\w{2,3}\s+\d{4}-\d{2}-\d{2}\s[\d:]{8}/RQ"; classtype:shellcode-detect; sid: 3016010; rev: 1; metadata:created_at 2018_09_13,by al0ne; )
alert tcp $EXTERNAL_NET any -> $HOME_NET any (msg: "CobaltStrike login server"; flow:established; content:"Cyberspace"; depth:200; content:"Somewhere"; distance:0; content:"cobaltstrike"; distance:0; content:"AdvancedPenTesting";distance:0; classtype:exploit-kit; sid:3016001; rev:1; metadata:by al0ne;)
alert http $HOME_NET any -> $EXTERNAL_NET any (msg: "CobaltStrike download.windowsupdate.com C2 Profile"; flow: established; content:"msdownload"; http_uri; pcre:"/\/c\/msdownload\/update\/others\/[\d]{4}/\d{2}/\d{7,8}_[\d\w-_]{50,}\.cab/UR"; reference:url,github.com/bluscreenofjeff/MalleableC2Profiles/blob/master/microsoftupdate_getonly.profile; classtype:exploit-kit; sid: 3016002; rev: 1; metadata:created_at 2018_09_25,by al0ne; )
alert http $EXTERNAL_NET any -> $HOME_NET any (msg: "CobaltStrike HTTP beacon response"; flow: established; content:"200"; http_stat_code; content:!"Server:"; http_header; content:"application/octet-stream"; http_header; distance:0;  content:"Content-Length: 0"; http_header; distance:0; threshold: type both, track by_src, count 5, seconds 60; classtype:exploit-kit; sid: 3016003; rev: 1; metadata:created_at 2018_11_15,by al0ne;)
alert http $HOME_NET any -> $EXTERNAL_NET any (msg: "CobaltStrike ARP Scan module"; flow:established; content:"POST"; http_method; content:"(ARP)"; http_client_body; content:"Scanner module is complete"; http_client_body; distance:0; classtype:exploit-kit; sid:3016004; rev:1; metadata:created_at 2018_11_15,by al0ne;)
#alert http any any -> any any (msg:"CobatlStrikt team servers 200 OK Space"; flow:from_server,established; content:"200"; http_stat_code; content:"HTTP/1.1 200 OK|20|"; threshold: type both, track by_src, count 3, seconds 60; reference:url,blog.fox-it.com/2019/02/26/identifying-cobalt-strike-team-servers-in-the-wild/;  sid:3016011; rev:1; metadata:created_at 2019_02_27,by al0ne;)
alert tcp any any -> any any (msg:"CobaltStrike C2 Server"; flow:to_client; content:"HTTP/1.1 200 OK |0d0a|"; fast_pattern; depth:18; content:"Date: "; pcre:"/^HTTP/1.1 200 OK \r\nContent-Type: [^\r\n]{0,100}\r\nDate: [^\r\n]{0,100} GMT\r\n(Content-Length: \d+\r\n)\r\n/"; threshold:type limit, track by_dst, count 1, seconds 600; classtype:exploit-kit; priority:2; sid:3016012; metadata:created_at 2019_08_05; rev:3;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (pool.minergate.com)"; dns_query; content:"pool.minergate.com"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017000; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (pool.minergate.com)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|04|pool|09|minergate|03|com|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017001; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (pool.minexmr.com)"; dns_query; content:"pool.minexmr.com"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017002; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (pool.minexmr.com)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|04|pool|07|minexmr|03|com|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:301703; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (opmoner.com)"; dns_query; content:"opmoner.com"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017004; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (opmoner.com)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|07|opmoner|03|com|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017005; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (crypto-pool.fr)"; dns_query; content:"crypto-pool.fr"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017006; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (crypto-pool.fr)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|0b|crypto-pool|02|fr|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017007; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (backup-pool.com)"; dns_query; content:"backup-pool.com"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017008; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (backup-pool.com)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|0b|backup-pool|03|com|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017009; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (monerohash.com)"; dns_query; content:"monerohash.com"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017010; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (monerohash.com)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|0a|monerohash|03|com|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017011; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (poolto.be)"; dns_query; content:"poolto.be"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017012; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (poolto.be)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|06|poolto|02|be|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017013; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (xminingpool.com)"; dns_query; content:"xminingpool.com"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017014; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (xminingpool.com)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|0b|xminingpool|03|com|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017015; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (prohash.net)"; dns_query; content:"prohash.net"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017016; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (prohash.net)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|07|prohash|03|net|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017017; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (dwarfpool.com)"; dns_query; content:"dwarfpool.com"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017018; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (dwarfpool.com)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|09|dwarfpool|03|com|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017019; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (crypto-pools.org)"; dns_query; content:"crypto-pools.org"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017020; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (crypto-pools.org)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|0c|crypto-pools|03|org|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017021; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (monero.net)"; dns_query; content:"monero.net"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017022; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (monero.net)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|06|monero|03|net|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017023; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (hashinvest.net)"; dns_query; content:"hashinvest.net"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017024; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (hashinvest.net)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|0a|hashinvest|03|net|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017025; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (moneropool.com)"; dns_query; content:"moneropool.com"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017026; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (moneropool.com)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|0a|moneropool|03|com|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017027; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (xmrpool.eu)"; dns_query; content:"xmrpool.eu"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017028; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (xmrpool.eu)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|07|xmrpool|02|eu|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017029; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (ppxxmr.com)"; dns_query; content:"ppxxmr.com"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017030; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (ppxxmr.com)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|06|ppxxmr|03|com|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017031; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (alimabi.cn)"; dns_query; content:"alimabi.cn"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017032; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (alimabi.cn)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|07|alimabi|02|cn|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017033; rev:1;)
alert dns $HOME_NET any -> any any (msg:"Observed DNS Query to public CryptoMining pool Domain (aeon-pool.com)"; dns_query; content:"aeon-pool.com"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017034; rev:1;)
alert udp $HOME_NET any -> any 53 (msg:"Observed DNS Query to public CryptoMining pool Domain (aeon-pool.com)"; content:"|01|"; offset:2; depth:1; content:"|00 01 00 00 00 00 00|"; distance:1; within:7; content:"|09|aeon-pool|03|com|00|"; nocase; distance:0; fast_pattern; classtype:coin-mining; sid:3017035; rev:1;)
alert udp $HOME_NET any -> $EXTERNAL_NET 53 (msg:"Suspicious dns request"; flow:established,to_server; content:"|01 00|"; depth:4; pcre:"/\x00\x10\x00\x01|\x00\x0f\x00\x01|\x00\x05\x00\x01/"; dsize:>200; classtype:trojan-activity; sid:3011001; rev:1; metadata:created_at 2018_11_09,by al0ne;)
alert tcp $HOME_NET any -> $EXTERNAL_NET any (msg:"Cryptocurrency Miner Check By Submit"; flow:to_server,established; content:"|22|method|22 3a|"; fast_pattern; content:"|22|submit|22 2c|"; distance:0; within:10; content:"|22|params|22 3a 7b|"; distance:0; within:15; content:"result|22 3a|"; nocase; distance:0; classtype:trojan-activity; sid:3013015; rev:1; metadata:Detecting Mining SuricataRules by Charmly;)
alert tcp $EXTERNAL_NET any -> $HOME_NET any (msg:"Pools Response Cryptocurrency Miner"; flow:to_client,established; content:"|22|method|22 3a|"; nocase; content:"|22|params|22 3a|"; nocase; content:"|22|blob|22 3a|"; nocase; content:"|22|job_id|22 3a|"; nocase; classtype:trojan-activity; sid:3013016; rev:1; metadata:Detecting Mining SuricataRules by Charmly;)
alert tcp any any -> any any (msg: "Hacker backdoor or shell  Microsoft Corporation"; flow:to_server,established; content:"|20 4d 69 63 72 6f 73 6f 66 74 20 43 6f 72 70 6f 72 61 74 69 6f 6e|"; depth:200; content:"WHOIS database"; nocase; classtype:trojan-activity; sid:3003001; rev:2; metadata:created_at 2018_09_26,updated_at 2019_08_06,by al0ne;)
alert tcp any any -> any any (msg: "Hacker backdoor or shell Microsoft Windows"; flow:established; content:"|4D 69 63 72 6F 73 6F 66 74 20 57 69 6E 64 6F 77 73 20 5B|"; depth:200; classtype:trojan-activity; sid:3003002; rev:1; metadata:by al0ne;)
alert http any any -> any any (msg:"***Windows Powershell Request UserAgent***"; flow:established; content:"PowerShell"; http_user_agent; pcre:"/PowerShell|WindowsPowerShell/i"; classtype:trojan-activity; sid:3013001; rev:1; metadata:by al0ne;)
alert http any any -> any any (msg:"***Linux wget/curl download .sh script***"; flow:established,to_server; content:".sh"; http_uri;  pcre:"/curl|Wget|linux-gnu/Vi"; classtype:trojan-activity; sid:3013002; rev:1; metadata:by al0ne;)
alert http $EXTERNAL_NET any -> $HOME_NET any (msg: "Suspicious netstat command traffic"; flow: established,to_client; content:"Active Internet connections"; http_server_body; depth:28; content:"tcp"; http_server_body; distance:0; classtype:trojan-activity; sid: 3013003; rev: 1; metadata:created_at 2018_09_26,by al0ne;)
alert tcp $HOME_NET any -> any any (msg: "http GET data"; flow: established;  content:"|47 45 54|"; depth: 10; content:"|0d 0a 0d 0a|"; depth:500; pcre:"/\x0d\x0a\x0d\x0a[^GETPOSTPUTHEAD\{\<\-][\x00-\xff]{100,200}/"; classtype:trojan-activity; sid: 3013004; rev: 1; metadata:created_at 2018_10_17,by al0ne;)
alert http any any -> any any (msg:"msfconsole powershell response"; flow:established; content:!"<html>"; content:!"<script>"; content:"|70 6f 77 65 72 73 68 65 6c 6c 2e 65 78 65|"; http_server_body; content:"|46 72 6f 6d 42 61 73 65 36 34 53 74 72 69 6e 67|"; http_server_body; classtype:exploit-kit; sid:3016005; rev:1;)
alert tcp $HOME_NET any -> any 3306 (msg: "mysql general_log write file"; flow: established;  content:"|03|"; depth: 5; content:"|67 65 6e 65 72 61 6c 5f 6c 6f 67 5f 66 69 6c 65|"; distance:0; classtype:trojan-activity; sid: 3013005; rev: 1; metadata:created_at 2018_11_20,by al0ne;)
alert http $EXTERNAL_NET any -> $HOME_NET any (msg: "Weevely PHP Backdoor Response"; flow: established,to_client; content:"200"; http_stat_code; content:!"<html>"; pcre:"/<(\w+)>[a-zA-Z0-9+\/]{20,}(?:[a-zA-Z0-9+\/]{1}[a-zA-Z0-9+\/=]{1}|==)<\/\w+>/Q"; classtype:shellcode-detect; sid: 3016006; rev: 1; metadata:created_at 2018_09_03,by al0ne;)
alert http $HOME_NET any -> $EXTERNAL_NET any  (msg: "Powershell Empire HTTP Request "; flow: established, to_server; content:".php"; http_uri;  pcre:"/session=[a-zA-Z0-9+/]{20,300}([a-zA-Z0-9+/]{1}[a-zA-Z0-9+/=]{1}|==)/ACi"; flowbits:set,empire; classtype:shellcode-detect; sid: 3016007; rev: 1; metadata:created_at 2018_09_03,by al0ne;)
alert http $EXTERNAL_NET any -> $HOME_NET any (msg: "Powershell Empire HTTP Response "; flow: established,to_client; content:"200"; http_stat_code; flowbits: isset,empire; content:"Cache-Control: no-cache, no-store, must-revalidate"; http_header; content: "Server: Microsoft-IIS/7.5"; http_header; distance: 0; classtype:shellcode-detect; sid: 3016008; rev: 1; metadata:created_at 2018_09_03,by al0ne;)
alert http any any -> any any (msg:"webshell_caidao_php"; flow:established; content:"POST";http_method; content:".php"; http_uri; content:"base64_decode"; http_client_body; classtype:shellcode-detect; sid:3016009; rev:1; metadata:by al0ne;)
alert http $EXTERNAL_NET any -> $HOME_NET any (msg: "China hacker tools caidao response - column directory"; flow: established,to_client; content:"200"; http_stat_code; content:!"<html>"; http_server_body; content:"|2d 3e|"; http_server_body; depth:2; pcre:"/[\w\d]+\.\w{2,3}\s+\d{4}-\d{2}-\d{2}\s[\d:]{8}/RQ"; classtype:shellcode-detect; sid: 3016010; rev: 1; metadata:created_at 2018_09_13,by al0ne; )
alert http any any -> any any  (msg: "Behinder3 PHP HTTP Request"; flow: established, to_server; content:".php"; http_uri;  pcre:"/[a-zA-Z0-9+/]{1000,}=/i"; flowbits:set,behinder3;noalert; classtype:shellcode-detect; sid: 3016017; rev: 1; metadata:created_at 2020_08_17,by al0ne;)
alert http any any -> any any (msg: "Behinder3  PHP HTTP Response"; flow: established,to_client; content:"200"; http_stat_code; flowbits: isset,behinder3; pcre:"/[a-zA-Z0-9+/]{100,}=/i"; classtype:shellcode-detect; sid: 3016018; rev: 1; metadata:created_at 2020_08_17,by al0ne;)
alert http any any -> any any (msg: "test\"\;";)

`)
	if err != nil {
		panic(err)
		return
	}
	if len(rules) != 62 {
		spew.Dump(rules)
		panic("parse failed")
	}
	if rules[60].ContentRuleConfig.ContentRules[1].PCRE != "/[a-zA-Z0-9+/]{100,}=/i" {
		spew.Dump(rules[60])
		panic("parse failed")
	}
	if len(rules[1].ContentRuleConfig.ContentRules) != 4 {
		spew.Dump(rules[2])
		panic("parse failed")
	}
}

func TestParse2(t *testing.T) {
	rules, err := Parse(`alert tcp any [3690, 9418] <> any any (msg: "ATTACK [PTsecurity] SVN/Git Remote Code Execution through malicious (svn+,git+)ssh:// URL (Multiple CVEs)"; flow: established; content: "ssh://"; nocase; pcre: "/ssh:\/\/(?:[^@\s]+@)?(?:[\w\:\.\-\[\]\@]+[^\w\:\.\-\[\]\@\/\ ]|[^\w\:\.\-\[\]\@\/\ ][\w\:\.\-\[\]\@])/i"; reference: cve, 2017-9800; reference: cve, 2017-12426; reference: cve, 2017-1000116; reference: cve, 2017-1000117; reference: url, subversion.apache.org/security/CVE-2017-9800-advisory.txt; classtype: attempted-admin; reference: url, github.com/ptresearch/AttackDetection; sid: 10001757; rev: 3; )`)
	if err != nil {
		panic(err)
	}
	spew.Dump(rules)
}

func TestMUSTPASS_Parse_PortSyntaxAndMatch(t *testing.T) {
	for _, i := range [][]any{
		// rule, include, exclude
		{`alert tcp any [111,222] <> any any (msg: "abc")`, []int{111, 222}, []int{22}},
		{`alert tcp any any <> any any (msg: "abc")`, []int{111, 222}, []int{}},
		{`alert tcp any [79:222,!80] <> any any (msg: "abc")`, []int{79, 81, 86, 200}, []int{80, 999}},
		{`alert tcp any ![222,!80] <> any any (msg: "abc")`, []int{80}, []int{222}},
		{`alert tcp any !22 <> any any (msg: "abc")`, []int{11}, []int{22}},
		{`alert tcp any [1,![23,41]] <> any any (msg: "abc")`, []int{1}, []int{23, 41, 222}},
	} {
		r, err := Parse(utils.InterfaceToString(i[0]))
		if err != nil {
			panic(err)
		}

		if len(r) <= 0 {
			panic("NO RULE")
		}

		var whitePorts = i[1].([]int)
		var blackPorts = i[2].([]int)

		for _, subRule := range r {
			for _, wp := range whitePorts {
				if !subRule.SourcePort.Match(wp) {
					msg := fmt.Sprintf("rule: %v not match: %v", i[0], wp)
					panic(msg)
				}
			}

			for _, bp := range blackPorts {
				if subRule.SourcePort.Match(bp) {
					msg := fmt.Sprintf("rule: %v match blacklist ports: %v", i[0], bp)
					panic(msg)
				}
			}
		}
	}
}

func TestMUSTPASS_Parse_AddrSyntaxAndMatch(t *testing.T) {
	var envs = []string{"HOME=2.3.4.5", "HOME2=1.2.3.4"}
	for _, i := range [][]any{
		// rule, include, exclude
		{`alert tcp any any <> any any (msg: "abc")`, []string{"8.8.8.8", "4.4.4.4"}, []string{}},
		{`alert tcp $HOME any <> any any (msg: "abc")`, []string{"2.3.4.5"}, []string{"8.8.8.8"}},
		{`alert tcp !$HOME2 any <> any any (msg: "abc")`, []string{"2.3.4.5", "8.8.8.8"}, []string{"1.2.3.4"}},
		{`alert tcp [$HOME2,$HOME] any <> any any (msg: "abc")`, []string{"2.3.4.5", "1.2.3.4"}, []string{"3.3.3.3"}},
		{`alert tcp [$HOME2,$HOME,3.3.3.3/24] any <> any any (msg: "abc")`, []string{"2.3.4.5", "1.2.3.4", "3.3.3.1"}, []string{"3.3.2.3"}},
		{`alert tcp [$HOME2,$HOME,!3.3.3.3/24] any <> any any (msg: "abc")`, []string{"2.3.4.5", "1.2.3.4"}, []string{"3.3.2.3", "3.3.3.1"}},
		{`alert tcp ![$HOME2,$HOME] any <> any any (msg: "abc")`, []string{"3.3.2.3", "3.3.3.1"}, []string{"2.3.4.5", "1.2.3.4"}},
		{`alert tcp 127.0.0.1/24 any <> any any (msg: "abc")`, []string{"127.0.0.2"}, []string{"2.3.4.5", "1.2.3.4"}},
		{`alert tcp !127.0.0.1/24 any <> any any (msg: "abc")`, []string{"1.0.0.2"}, []string{"127.0.0.1", "127.0.0.2"}},
		{`alert tcp ff80::1/64 any <> fe80::5d03:c04a:1f87:e661/64 any (msg: "abc")`, []string{"ff80::1"}, []string{"fe80::5d03:c04a:1f87:1", "fe80::5d03:c04a:1f87:2"}},
		{`alert tcp ff80::1/64 any <> fe80:8e9c:5d03:c04a:1f87:e661:ff91:1 8080 (msg: "abc")`, []string{"ff80::2"}, []string{"fe80:8e9c:5d03:c04a:1f87:e661:ff91:1"}},
		{`alert tcp !ff80::1/64 any <> !fe80:8e9c:5d03:c04a:1f87:e661:ff91:1 8080 (msg: "abc")`, []string{"ff81::1"}, []string{"ff80::1"}},
	} {
		r, err := Parse(utils.InterfaceToString(i[0]), envs...)
		if err != nil {
			panic(err)
		}

		if len(r) <= 0 {
			panic("NO RULE")
		}

		var whitePorts = i[1].([]string)
		var blackPorts = i[2].([]string)

		for _, subRule := range r {
			for _, wp := range whitePorts {
				if !subRule.SourceAddress.Match(wp) {
					msg := fmt.Sprintf("rule: %v not match: %v\nenv: %v", i[0], wp, envs)
					panic(msg)
				}
			}

			for _, bp := range blackPorts {
				if subRule.SourceAddress.Match(bp) {
					msg := fmt.Sprintf("rule: %v match blacklist addr: %v\nenv: %v", i[0], bp, envs)
					panic(msg)
				}
			}
		}
	}
}
