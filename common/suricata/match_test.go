package suricata

import (
	"encoding/hex"
	"github.com/davecgh/go-spew/spew"
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
		rule: "alert http any any -> any any (msg:httptest;content:\"/\";http.uri;content:\"/\";http.uri.raw;content:GET;http.method;content:HTTP/1.1;http.protocol;content:\"GET / HTTP/1.1|0d 0a|\";http.request_line;content:\"Mozilla/5.0 (Windows NT; Windows NT 10.0; zh-CN) WindowsPowerShell/5.1.22621.1778\";endswith;content:\"|0d 0a|Host|0d 0a|User-Agent|0d 0a|Accept-Encoding|0d 0a 0d 0a|\";http.header_names;)",
		test: []Test{
			// curl http://baimeow.cn
			{"3066d026811b6afd6158af5c0800450000c2866340008006f9e4c0a80312dde4d84e1a750050a3a72252fab0b8745018040193350000474554202f20485454502f312e310d0a486f73743a206261696d656f772e636e0d0a557365722d4167656e743a204d6f7a696c6c612f352e30202857696e646f7773204e543b2057696e646f7773204e542031302e303b207a682d434e292057696e646f7773506f7765725368656c6c2f352e312e32323632312e313737380d0a4163636570742d456e636f64696e673a20677a69700d0a0d0a", true},
		},
	}, {
		rule: "alert http any any -> any any (msg:httptest;content:\"/\";http.uri;content:\"/\";http.uri.raw;content:GET;http.method;content:HTTP/1.1;http.protocol;content:\"GET / HTTP/1.1|0d 0a|\";http.request_line;content:\"Mozilla/5.0 (Windows NT; Windows NT 10.0; zh-CN) WindowsPowerShell/5.1.22621.1778\";endswith;content:\"|0d 0a|Host|0d 0a|User-Agent|0d 0a|Accept-Encoding|0d 0a 0d 0a|\";http.header_names;)",
		test: []Test{
			{"3066d026811b6afd6158af5c0800450000c2866340008006f9e4c0a80312dde4d84e1a750050a3a72252fab0b8745018040193350000474554202f20485454502f312e310d0a486f73743a206261696d656f772e636e0d0a557365722d4167656e743a204d6f7a696c6c612f352e30202857696e646f7773204e543b2057696e646f7773204e542031302e303b207a682d434e292057696e646f7773506f7765725368656c6c2f352e312e32323632312e313737380d0a4163636570742d456e636f64696e673a20677a69700d0a0d0a", true},
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
				spew.Dump(v, te)
				t.Error("match failed")
			}
		}
	}
}

func TestRule_Match2(t *testing.T) {
	v := Case{
		rule: "alert http any any -> any any (msg:httptest;content:https;http.location;startswith;content:slt;http.server;nocase;content:keep-alive;http.connection;content:302;http.stat_code;startswith;content:Aug;http.header;content:2023;http.header;distance:1;",
		test: []Test{
			{"6afd6158af5c3066d026811b0800450001276ec7400036065b1cdde4d84ec0a8031200501a75fab0b874a3a722ec50180ffe576d0000485454502f312e312033303220466f756e640d0a4c6f636174696f6e3a2068747470733a2f2f6261696d656f772e636e2f0d0a436f6e74656e742d4c656e6774683a20300d0a582d4e57532d4c4f472d555549443a20393433363237373032323431383037313837350d0a436f6e6e656374696f6e3a206b6565702d616c6976650d0a5365727665723a20534c540d0a446174653a205468752c2030332041756720323032332030393a35383a333520474d540d0a582d43616368652d4c6f6f6b75703a2052657475726e204469726563746c790d0a5374726963742d5472616e73706f72742d53656375726974793a206d61782d6167653d313b0d0a0d0a", true},
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
