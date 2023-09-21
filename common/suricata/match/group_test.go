package match

import (
	"github.com/google/gopacket"
	"github.com/yaklang/yaklang/common/log"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"testing"
)

// Group's testcase
// should do more thing for yak.suricata
// well, take easy!
// ---------

func TestGroup_Match(t *testing.T) {
	ruleStr := `alert http any any -> any any (msg:"Detected abc11111 in HTTP header"; content:"abc11111"; http_header; sid:1000001; rev:1;)`
	ruleIns, err := surirule.Parse(ruleStr)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	groupIns := NewGroup(WithGroupOnMatchedCallback(func(packet gopacket.Packet, match *surirule.Rule) {
		log.Info("match success")
		count++
	}))
	groupIns.LoadRule(ruleIns[0])
	groupIns.FeedHTTPRequestBytes([]byte(`GET / HTTP/1.1
Host: www.example.com
User-Agent: abc11111
`))
	groupIns.Wait()
	if count != 1 {
		t.Fatal("match failed")
	}
}

func TestGroup_MatchRequest(t *testing.T) {
	ruleStr := `alert http any any -> any any (msg:"Detected abc11111 in HTTP header"; content:"abc11111"; http_header; sid:1000001; rev:1;)`
	ruleIns, err := surirule.Parse(ruleStr)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`GET / HTTP/1.1
Host: www.example.com
User-Agent: abc11111
`)
	result := New(ruleIns[0]).MatchHTTPFlow(&HttpFlow{
		Req: raw,
	})
	if !result {
		t.Fatal("match failed")
	}
}
