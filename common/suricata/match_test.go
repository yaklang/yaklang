package suricata

import (
	"encoding/hex"
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestRule_Match(t *testing.T) {
	type Test struct {
		hex   string
		match bool
	}
	type Case struct {
		rule string
		test []Test
	}

	cases := []Case{
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
		},
	}
	for _, v := range cases {
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
}
