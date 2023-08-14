package suricata

import (
	"encoding/hex"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
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
		rs, err := Parse(v.rule)
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
			if r.Match(bytes) != te.match {
				spew.Dump(r, te)
				t.Error("match failed")
			}
		}
	}
}

func TestRule_Match2(t *testing.T) {
	v := Case{
		rule: `alert http any any -> any any (msg:"config.pinyin.sogou";http.server;content:nginx;http.server_body;content:"[setting]|0a|";pcre:"/([a-z]+=\\d+\\s?)+/iRQ"`,
		test: []Test{
			// Response of GET config.pinyin.sogou.com/api/popup/lotus.php
			{"6afd6158af5c3066d026811b0800450000e46922400031067762310773cec0a80312005025da75014a7224214d39501800774fe50000485454502f312e3120323030204f4b0d0a5365727665723a206e67696e780d0a446174653a205765642c2030392041756720323032332030383a31343a323720474d540d0a436f6e74656e742d547970653a206170706c69636174696f6e2f6f637465742d73747265616d0d0a436f6e74656e742d4c656e6774683a2033330d0a436f6e6e656374696f6e3a206b6565702d616c6976650d0a0d0a5b73657474696e675d0a5573654e65743d300a4e6578745472793d323539323030", true},
			{"6afd6158af5c3066d026811b0800450000e46922400031067762310773cec0a80312005025da75014a7224214d39501800774fe50000485454502f312e3120323030204f4b0d0a5365727665723a206e67696e780d0a446174653a205765642c2030392041756720323032332030383a31343a323720474d540d0a436f6e74656e742d547970653a206170706c69636174696f6e2f6f637465742d73747265616d0d0a436f6e74656e742d4c656e6774683a2033330d0a436f6e6e656374696f6e3a206b6565702d616c6976650d0a0d0a5b73657474696e675d0a3055555555553d300a4e6578745472793d323539323030", false},
		},
	}
	rs, err := Parse(v.rule)
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
		if r.Match(bytes) != te.match {
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
	rule := "alert http any any -> any any (msg:httptest;content:https;http.location;startswith;content:slt;http.server;nocase;content:keep-alive;http.connection;content:302;http.stat_code;startswith;content:Aug;http.header;content:2024;http.header;distance:1;"
	rs, err := Parse(rule)
	if err != nil {
		b.Error(err)
		return
	}
	r := rs[0]
	bytes, _ := hex.DecodeString("6afd6158af5c3066d026811b0800450001276ec7400036065b1cdde4d84ec0a8031200501a75fab0b874a3a722ec50180ffe576d0000485454502f312e312033303220466f756e640d0a4c6f636174696f6e3a2068747470733a2f2f6261696d656f772e636e2f0d0a436f6e74656e742d4c656e6774683a20300d0a582d4e57532d4c4f472d555549443a20393433363237373032323431383037313837350d0a436f6e6e656374696f6e3a206b6565702d616c6976650d0a5365727665723a20534c540d0a446174653a205468752c2030332041756720323032332030393a35383a333520474d540d0a582d43616368652d4c6f6f6b75703a2052657475726e204469726563746c790d0a5374726963742d5472616e73706f72742d53656375726974793a206d61782d6167653d313b0d0a0d0a")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Match(bytes)
	}
	b.StopTimer()
}

func BenchmarkRule_Match_FastPattern(b *testing.B) {
	rule := "alert http any any -> any any (msg:httptest;content:https;http.location;startswith;content:slt;http.server;nocase;content:keep-alive;http.connection;content:302;http.stat_code;startswith;content:Aug;http.header;content:2024;http.header;fast_pattern;distance:1;"
	rs, err := Parse(rule)
	if err != nil {
		b.Error(err)
		return
	}
	r := rs[0]
	bytes, _ := hex.DecodeString("6afd6158af5c3066d026811b0800450001276ec7400036065b1cdde4d84ec0a8031200501a75fab0b874a3a722ec50180ffe576d0000485454502f312e312033303220466f756e640d0a4c6f636174696f6e3a2068747470733a2f2f6261696d656f772e636e2f0d0a436f6e74656e742d4c656e6774683a20300d0a582d4e57532d4c4f472d555549443a20393433363237373032323431383037313837350d0a436f6e6e656374696f6e3a206b6565702d616c6976650d0a5365727665723a20534c540d0a446174653a205468752c2030332041756720323032332030393a35383a333520474d540d0a582d43616368652d4c6f6f6b75703a2052657475726e204469726563746c790d0a5374726963742d5472616e73706f72742d53656375726974793a206d61782d6167653d313b0d0a0d0a")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Match(bytes)
	}
}

func TestPCRE(t *testing.T) {
	pcre, err := ParsePCREStr("/abc?/i")
	if err != nil {
		return
	}
	matcher, err := pcre.Matcher()
	if err != nil {
		return
	}
	matches := matcher.Match([]byte("abc abc ab"))
	_ = matches
	assert.Equal(t, 3, len(matches), "match failed")
	assert.Equal(t, matched{pos: 0, len: 3}, matches[0], "match failed")
	assert.Equal(t, matched{pos: 4, len: 3}, matches[1], "match failed")
	assert.Equal(t, matched{pos: 8, len: 2}, matches[2], "match failed")
}
