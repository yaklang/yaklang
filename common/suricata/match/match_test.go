package match

import (
	"encoding/hex"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/data"
	"github.com/yaklang/yaklang/common/suricata/pcre"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"testing"
)

type Test struct {
	hex   string
	match bool
}
type Case struct {
	rule string
	test []Test
}

var testcases = []Case{
	{
		rule: "alert dns 192.168.3.1 any -> any any (msg:\"Observed DNS Query to public CryptoMining pool Domain (pool.minergate.com)\"; dns_opcode:0; dns_query; content:\"pool.minergate.com\"; nocase; isdataat:!1,relative; classtype:coin-mining; sid:3017000; rev:1;)",
		test: []Test{
			// pool.minergate.com dns request src 192.168.3.18
			{"3066d026811b6afd6158af5c08004500004039b5000080117994c0a80312c0a80301d9c60035002cd822000a0100000100000000000004706f6f6c096d696e65726761746503636f6d0000010001", false},
			// copilot-telemetry.githubusercontent.com dns response
			{"6afd6158af5c3066d026811b0800450000b5de8040004011d453c0a80301c0a803120035ed1b00a1a19652e68080000100030000000011636f70696c6f742d74656c656d657472791167697468756275736572636f6e74656e7403636f6d0000010001c00c00050001000005c3001c19636f70696c6f742d74656c656d657472792d73657276696365c01ec0450005000100000497001c12676c622d646235326332636638626535343406676974687562c030c06d000100010000000100048c527116", false},
			// baidu.com dns response
			{"6afd6158af5c3066d026811b080045000057a86d400040110ac5c0a80301c0a803120035dab70043a3e300028080000100020000000005626169647503636f6d0000010001c00c000100010000001c0004279c420ac00c000100010000001c00046ef24442", false},
			// pool.minergate.com dns response
			{"6afd6158af5c3066d026811b0800450000506d32400040114607c0a80301c0a803120035e5fe003c263300028080000100010000000004706f6f6c096d696e65726761746503636f6d0000010001c00c000100010000003c0004c7109ebe", true},
		},
	}, {
		rule: "alert dns any any -> any any (dns_query;startswith;content:\"pool\";dns_query;content:\"com\";distance:11;)",
		test: []Test{
			// pool.minergate.com dns request src 192.168.3.18
			{"3066d026811b6afd6158af5c08004500004039b5000080117994c0a80312c0a80301d9c60035002cd822000a0100000100000000000004706f6f6c096d696e65726761746503636f6d0000010001", true},
		},
	}, {
		rule: "alert dns 192.168.3.1 53 -> 192.168.3.18 51218 (dns_query;content:bai;offset:4;depth:3;dns_query;content:ow;distance:2;within:4;dns_query;content:cn;distance:1;isdataat:!1,relative;aaa;)",
		test: []Test{
			{"6afd6158af5c3066d026811b08004500004c60764000401152c7c0a80301c0a803120035c81200387f0900028080000100010000000003617069076261696d656f7702636e0000010001c00c00010001000002580004514472bd", true},
		},
	}, {
		// Multiple Buffer Matching
		rule: "alert dns 192.168.3.1 53 -> 192.168.3.18 51218 (dns_query;content:bai;offset:4;depth:3;content:ow;distance:2;within:4;content:cn;distance:1;isdataat:!1,relative;aaa;)",
		test: []Test{
			{"6afd6158af5c3066d026811b08004500004c60764000401152c7c0a80301c0a803120035c81200387f0900028080000100010000000003617069076261696d656f7702636e0000010001c00c00010001000002580004514472bd", true},
		},
	},
	{
		rule: "alert http any any -> any any (msg:httptest;content:\"/\";http.uri;content:\"/\";http.uri.raw;content:GET;http.method;content:HTTP/1.1;http.protocol;content:\"GET / HTTP/1.1|0d 0a|\";http.request_line;content:\"Mozilla/5.0 (Windows NT; Windows NT 10.0; zh-CN) WindowsPowerShell/5.1.22621.1778\";http.user_agent;endswith;content:\"|0d 0a|Accept-Encoding|0d 0a|Host|0d 0a|User-Agent|0d 0a 0d 0a|\";http.header_names;)",
		test: []Test{
			// curl http://baimeow.cn
			{"3066d026811b6afd6158af5c0800450000c2866340008006f9e4c0a80312dde4d84e1a750050a3a72252fab0b8745018040193350000474554202f20485454502f312e310d0a486f73743a206261696d656f772e636e0d0a557365722d4167656e743a204d6f7a696c6c612f352e30202857696e646f7773204e543b2057696e646f7773204e542031302e303b207a682d434e292057696e646f7773506f7765725368656c6c2f352e312e32323632312e313737380d0a4163636570742d456e636f64696e673a20677a69700d0a0d0a", true},
		},
	}, {
		rule: "alert http any any -> any any (msg:httptest;content:https;http.location;startswith;content:slt;http.server;nocase;content:keep-alive;http.connection;content:302;http.stat_code;startswith;content:Aug;http.header;content:2023;http.header;distance:1;",
		test: []Test{
			// 302 redirect
			{"6afd6158af5c3066d026811b0800450001276ec7400036065b1cdde4d84ec0a8031200501a75fab0b874a3a722ec50180ffe576d0000485454502f312e312033303220466f756e640d0a4c6f636174696f6e3a2068747470733a2f2f6261696d656f772e636e2f0d0a436f6e74656e742d4c656e6774683a20300d0a582d4e57532d4c4f472d555549443a20393433363237373032323431383037313837350d0a436f6e6e656374696f6e3a206b6565702d616c6976650d0a5365727665723a20534c540d0a446174653a205468752c2030332041756720323032332030393a35383a333520474d540d0a582d43616368652d4c6f6f6b75703a2052657475726e204469726563746c790d0a5374726963742d5472616e73706f72742d53656375726974793a206d61782d6167653d313b0d0a0d0a", true},
		},
	}, {
		rule: `alert http any any -> any any (msg:"config.pinyin.sogou";http.server;content:nginx;http.server_body;content:"[setting]|0a|";pcre:"/([a-z]+=\\d+\\s?)+/iRQ"`,
		test: []Test{
			// Response of GET config.pinyin.sogou.com/api/popup/lotus.php
			{"6afd6158af5c3066d026811b0800450000e46922400031067762310773cec0a80312005025da75014a7224214d39501800774fe50000485454502f312e3120323030204f4b0d0a5365727665723a206e67696e780d0a446174653a205765642c2030392041756720323032332030383a31343a323720474d540d0a436f6e74656e742d547970653a206170706c69636174696f6e2f6f637465742d73747265616d0d0a436f6e74656e742d4c656e6774683a2033330d0a436f6e6e656374696f6e3a206b6565702d616c6976650d0a0d0a5b73657474696e675d0a5573654e65743d300a4e6578745472793d323539323030", true},
			{"6afd6158af5c3066d026811b0800450000e46922400031067762310773cec0a80312005025da75014a7224214d39501800774fe50000485454502f312e3120323030204f4b0d0a5365727665723a206e67696e780d0a446174653a205765642c2030392041756720323032332030383a31343a323720474d540d0a436f6e74656e742d547970653a206170706c69636174696f6e2f6f637465742d73747265616d0d0a436f6e74656e742d4c656e6774683a2033330d0a436f6e6e656374696f6e3a206b6565702d616c6976650d0a0d0a5b73657474696e675d0a3055555555553d300a4e6578745472793d323539323030", false},
		},
	}, {
		rule: "alert http any any -> any any (msg:httptest;content:https;http.location;startswith;content:slt;http.server;nocase;content:keep-alive;http.connection;content:302;http.stat_code;startswith;content:Aug;http.header;content:!2023;http.header;distance:1;",
		test: []Test{
			{"6afd6158af5c3066d026811b0800450001276ec7400036065b1cdde4d84ec0a8031200501a75fab0b874a3a722ec50180ffe576d0000485454502f312e312033303220466f756e640d0a4c6f636174696f6e3a2068747470733a2f2f6261696d656f772e636e2f0d0a436f6e74656e742d4c656e6774683a20300d0a582d4e57532d4c4f472d555549443a20393433363237373032323431383037313837350d0a436f6e6e656374696f6e3a206b6565702d616c6976650d0a5365727665723a20534c540d0a446174653a205468752c2030332041756720323032332030393a35383a333520474d540d0a582d43616368652d4c6f6f6b75703a2052657475726e204469726563746c790d0a5374726963742d5472616e73706f72742d53656375726974793a206d61782d6167653d313b0d0a0d0a", false},
		},
	},
}

func TestMUSTPASS_Match(t *testing.T) {
	for _, v := range testcases {
		rs, err := rule.Parse(v.rule)
		if err != nil {
			t.Error(err)
			return
		}
		r := rs[0]
		for _, te := range v.test {
			bytes, err := hex.DecodeString(te.hex)
			if err != nil {
				t.Error(err)
				return
			}
			matcher := New(r)
			if matcher.Match(bytes) != te.match {
				spew.Dump(r, te)
				t.Error("match failed")
			}
		}
	}
}

func TestRule_Match2(t *testing.T) {
	v := Case{
		// Multiple Buffer Matching
		rule: "alert dns 192.168.3.1 53 -> 192.168.3.18 51218 (dns_query;content:bai;offset:4;depth:3;content:ow;distance:2;within:4;content:cn;distance:1;isdataat:!1,relative;aaa;)",
		test: []Test{
			{"6afd6158af5c3066d026811b08004500004c60764000401152c7c0a80301c0a803120035c81200387f0900028080000100010000000003617069076261696d656f7702636e0000010001c00c00010001000002580004514472bd", true},
		},
	}
	rs, err := rule.Parse(v.rule)
	if err != nil {
		t.Error(err)
		return
	}
	r := rs[0]
	for _, te := range v.test {
		bytes, err := hex.DecodeString(te.hex)
		if err != nil {
			t.Error(err)
			return
		}
		matcher := New(r)
		if matcher.Match(bytes) != te.match {
			spew.Dump(v)
			t.Error("match failed")
		}
	}
}

func TestMUSTPASS_FastPattern(t *testing.T) {
	res1 := testing.Benchmark(BenchmarkRule_Match)
	res2 := testing.Benchmark(BenchmarkRule_Match_FastPattern)
	t.Logf("normal pattern: %s\n", res1.String())
	t.Logf("fast pattern: %s\n", res2.String())
	if res1.NsPerOp() < res2.NsPerOp() {
		t.Error("fast pattern is slower than normal pattern")
	}
}

func BenchmarkRule_Match(b *testing.B) {
	raw := `alert http any any -> any any (msg:httptest;content:https;http.location;startswith;content:slt;http.server;nocase;content:keep-alive;http.connection;content:302;http.stat_code;startswith;content:Aug;http.header;pcre:"/[A-Z][a-z]*(-[A-Z][a-z]*)*(?<!abc):\\s[a-z]+(-[a-z]*)*=\\d(?=;)/";content:2024;http.header;distance:1;)`
	rs, err := rule.Parse(raw)
	if err != nil {
		b.Error(err)
		return
	}
	r := New(rs[0])
	bytes, _ := hex.DecodeString("6afd6158af5c3066d026811b0800450001276ec7400036065b1cdde4d84ec0a8031200501a75fab0b874a3a722ec50180ffe576d0000485454502f312e312033303220466f756e640d0a4c6f636174696f6e3a2068747470733a2f2f6261696d656f772e636e2f0d0a436f6e74656e742d4c656e6774683a20300d0a582d4e57532d4c4f472d555549443a20393433363237373032323431383037313837350d0a436f6e6e656374696f6e3a206b6565702d616c6976650d0a5365727665723a20534c540d0a446174653a205468752c2030332041756720323032332030393a35383a333520474d540d0a582d43616368652d4c6f6f6b75703a2052657475726e204469726563746c790d0a5374726963742d5472616e73706f72742d53656375726974793a206d61782d6167653d313b0d0a0d0a")
	pk := gopacket.NewPacket(bytes, layers.LayerTypeEthernet, gopacket.NoCopy)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if r.MatchPackage(pk) {
			spew.Dump(r)
			log.Fatal("match should be failed")
		}
	}
	b.StopTimer()
}

func BenchmarkRule_Match_FastPattern(b *testing.B) {
	raw := `alert http any any -> any any (msg:httptest;content:https;http.location;startswith;content:slt;http.server;nocase;content:keep-alive;http.connection;content:302;http.stat_code;startswith;content:Aug;http.header;pcre:"/[A-Z][a-z]*(-[A-Z][a-z]*)*(?<!abc):\\s[a-z]+(-[a-z]*)*=\\d(?=;)/";content:2024;http.header;fast_pattern;distance:1;)`
	rs, err := rule.Parse(raw)
	if err != nil {
		b.Error(err)
		return
	}
	r := New(rs[0])
	bytes, _ := hex.DecodeString("6afd6158af5c3066d026811b0800450001276ec7400036065b1cdde4d84ec0a8031200501a75fab0b874a3a722ec50180ffe576d0000485454502f312e312033303220466f756e640d0a4c6f636174696f6e3a2068747470733a2f2f6261696d656f772e636e2f0d0a436f6e74656e742d4c656e6774683a20300d0a582d4e57532d4c4f472d555549443a20393433363237373032323431383037313837350d0a436f6e6e656374696f6e3a206b6565702d616c6976650d0a5365727665723a20534c540d0a446174653a205468752c2030332041756720323032332030393a35383a333520474d540d0a582d43616368652d4c6f6f6b75703a2052657475726e204469726563746c790d0a5374726963742d5472616e73706f72742d53656375726974793a206d61782d6167653d313b0d0a0d0a")
	pk := gopacket.NewPacket(bytes, layers.LayerTypeEthernet, gopacket.NoCopy)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if r.MatchPackage(pk) {
			spew.Dump(r)
			log.Fatal("match should be failed")
		}
	}
}

func TestPCRE(t *testing.T) {
	p, err := pcre.ParsePCREStr("/abc?/i")
	if err != nil {
		return
	}
	matcher, err := p.Matcher()
	if err != nil {
		return
	}
	matches := matcher.Match([]byte("abc abc ab"))
	_ = matches
	assert.Equal(t, 3, len(matches), "match failed")
	assert.Equal(t, data.Matched{Pos: 0, Len: 3}, matches[0], "match failed")
	assert.Equal(t, data.Matched{Pos: 4, Len: 3}, matches[1], "match failed")
	assert.Equal(t, data.Matched{Pos: 8, Len: 2}, matches[2], "match failed")
}

func TestMSS(t *testing.T) {
	// MSS 1460
	const raw = `3066d026811b6afd6158af5c08004500003423504000800654aac0a8031298c3264cf40a0050e017798f000000008002faf0a3530000020405b40103030801010402`
	bytes, _ := hex.DecodeString(raw)
	rs, err := rule.Parse("alert tcp any any -> any any (msg:\"MSS 1460\";tcp.mss:>1459;sid:1;)")
	if err != nil {
		t.Error(err)
	}
	r := rs[0]
	matched := New(r).Match(bytes)
	t.Log(matched)
}

func TestTCP(t *testing.T) {
	const raw = `6afd6158af5c3066d026811b0800450000c31b8340003306246f6f2fd459c0a80312dd1a2fc2ac90c643636435aa501800043fab0000a1af6a0b385442e7e25712b0fce7ccd5df4af2190bcd7d68e4bd131e4de89b8f87cd4deb8ad5530bc7a4684fe22411d6558c4c9e3c67d48451eca0c74e0685f630483293e10716459aa74b0892bc3ea54507f612d51d48da82bf84f86f3022fc2d0aab0ba01f6dbf5223254c134cef67e48ef2b8e21412e1c4cfb36556994ae599a1681dba5c8d6186e98586191504be6c0454c4d2dda8cafb1687`
	bytes, _ := hex.DecodeString(raw)
	rs, err := rule.Parse("alert tcp any any -> any any (msg:\"tcp\";content:\"|ca fb 16|\";sid:1;)")
	if err != nil {
		t.Error(err)
	}
	r := rs[0]
	matched := New(r).Match(bytes)
	t.Log(matched)
}

func TestDistance(t *testing.T) {
	for _, testcase := range []struct {
		name    string
		ruleStr string
		traffic string
		expect  bool
	}{
		// check content:com.bea.console.handles.JndiBindingHandle; content:AdminServer; distance:0;
		// traffic content com.bea.console.handles.JndiBindingHandleAdminServer
		{
			name:    "distance0",
			ruleStr: "alert http any any -> any 7001 (msg:\"Exploit CVE-2021-2109 on Oracle Weblogic Server\"; flow:to_server,established; content:\"/console/consolejndi.portal\"; startswith; http_uri; content:\"com.bea.console.handles.JndiBindingHandle\"; content:\"AdminServer\"; distance:0; reference:cve,CVE-2021-2109; classtype:web-application-attack; sid:20212109; rev:1;)",
			traffic: "MGbQJoEb+E2Jka9SCABHAADSAAAAAEAGhcus4HVvmt0jcgIEBbQDBAcA51AbWfLIF6yUK2LFcBgGj8IKAAACBAW0AwMHAFRSQUNFIC9jb25zb2xlL2NvbnNvbGVqbmRpLnBvcnRhbF06JEdUIEhUVFAvMS4xDQpIb3N0OiBMUjcuNXQ4Lnk4cQ0KQ29udGVudC1MZW5ndGg6IDY0DQoNCj0/P3FGc0VnbGNvbS5iZWEuY29uc29sZS5oYW5kbGVzLkpuZGlCaW5kaW5nSGFuZGxlQWRtaW5TZXJ2ZXIsJjU=",
			expect:  true,
		},
		// check content:com.bea.console.handles.JndiBindingHandle; content:AdminServer; distance:0;
		// traffic content com.bea.console.handles.JndiBindingHandle..AdminServer
		{
			name:    "distance0",
			ruleStr: "alert http any any -> any 7001 (msg:\"Exploit CVE-2021-2109 on Oracle Weblogic Server\"; flow:to_server,established; content:\"/console/consolejndi.portal\"; startswith; http_uri; content:\"com.bea.console.handles.JndiBindingHandle\"; content:\"AdminServer\"; distance:0; reference:cve,CVE-2021-2109; classtype:web-application-attack; sid:20212109; rev:1;)",
			traffic: "MGbQJoEb+E2Jka9SCABHAAGTAAAAAEAGVD4/Vgjp16XxhgIEBbQDBAcA77IbWbGR1tYs2JlNcBgEHKwoAAACBAW0AwMHAFBBVENIIC9jb25zb2xlL2NvbnNvbGVqbmRpLnBvcnRhbEs/LmcrIEhUVFAvMS4xDQpIb3N0OiAxMzAuY0hBLlhXYQ0KQ29udGVudC1MZW5ndGg6IDI1Ng0KDQp8KmYgVVxeNSVvX3M2RmpjJiIyIH0sNDhgYF40ZnspQ2xBPGtpYEUqQDxNej5abWhbbDgnL1hQUHg/THQlc0okKT1yIy5ERSdoMTVAYG9mS0tULjsqIiIlcTVGQWNhezJUW3otMWVIdkMzXz5KeyslMDhQYjUqVHxuXnkjIFZ4Tn1LJkM8JUlvWT9CfiYoVW5fdyJsPCwlQklEKUluJmV6JnRsbVZlYzRHP2xYXzZ6YEJHbk42V14+TUMqMVteY29tLmJlYS5jb25zb2xlLmhhbmRsZXMuSm5kaUJpbmRpbmdIYW5kbGVQLEFkbWluU2VydmVyeS9pNGQ5WmM+V3Rb",
			expect:  true,
		},
		// check content:com.bea.console.handles.JndiBindingHandle; content:AdminServer; distance:3;
		// traffic content com.bea.console.handles.JndiBindingHandle..AdminServer
		{
			name:    "distance3",
			ruleStr: "alert http any any -> any 7001 (msg:\"Exploit CVE-2021-2109 on Oracle Weblogic Server\"; flow:to_server,established; content:\"/console/consolejndi.portal\"; startswith; http_uri; content:\"com.bea.console.handles.JndiBindingHandle\"; content:\"AdminServer\"; distance:3; reference:cve,CVE-2021-2109; classtype:web-application-attack; sid:20212109; rev:1;)",
			traffic: "MGbQJoEb+E2Jka9SCABHAAGTAAAAAEAGVD4/Vgjp16XxhgIEBbQDBAcA77IbWbGR1tYs2JlNcBgEHKwoAAACBAW0AwMHAFBBVENIIC9jb25zb2xlL2NvbnNvbGVqbmRpLnBvcnRhbEs/LmcrIEhUVFAvMS4xDQpIb3N0OiAxMzAuY0hBLlhXYQ0KQ29udGVudC1MZW5ndGg6IDI1Ng0KDQp8KmYgVVxeNSVvX3M2RmpjJiIyIH0sNDhgYF40ZnspQ2xBPGtpYEUqQDxNej5abWhbbDgnL1hQUHg/THQlc0okKT1yIy5ERSdoMTVAYG9mS0tULjsqIiIlcTVGQWNhezJUW3otMWVIdkMzXz5KeyslMDhQYjUqVHxuXnkjIFZ4Tn1LJkM8JUlvWT9CfiYoVW5fdyJsPCwlQklEKUluJmV6JnRsbVZlYzRHP2xYXzZ6YEJHbk42V14+TUMqMVteY29tLmJlYS5jb25zb2xlLmhhbmRsZXMuSm5kaUJpbmRpbmdIYW5kbGVQLEFkbWluU2VydmVyeS9pNGQ5WmM+V3Rb",
			expect:  false,
		},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			traffic, err := codec.DecodeBase64(testcase.traffic)
			if err != nil {
				t.Fatal(err)
			}
			ruleIns, err := rule.Parse(testcase.ruleStr)
			if err != nil {
				t.Fatal(err)
			}
			r := New(ruleIns[0])
			matched := r.Match(traffic)
			assert.Equal(t, testcase.expect, matched, "match failed")
		})
	}
}
