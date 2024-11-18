package match

import (
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
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

func BenchmarkGroup_FeedFrame_HTTP(b *testing.B) {
	ruleStr := `alert http any any -> any any (msg:"Detected abc11111 in HTTP header"; content:"abc11211"; http_header; sid:1000001; rev:1;)
alert http any any -> any any (msg:"Detected abc11111 in HTTP header"; content:"abc11311"; http_header; sid:1000001; rev:1;)
alert http any any -> any any (msg:"Detected abc11111 in HTTP header"; content:"abc11411"; http_header; sid:1000001; rev:1;)
alert http any any -> any any (msg:"Detected abc11111 in HTTP header"; content:"abc11111"; http_header; sid:1000001; rev:1;)`

	ruleIns, err := surirule.Parse(ruleStr)
	if err != nil {
		b.Fatal(err)
	}
	groupIns := NewGroup(WithGroupOnMatchedCallback(func(_ gopacket.Packet, _ *surirule.Rule) {
		return
	}))
	groupIns.LoadRules(ruleIns...)

	bytes, err := pcapx.PacketBuilder(
		pcapx.WithEthernet_NextLayerType("ip"),
		pcapx.WithEthernet_SrcMac("00:00:00:00:00:00"),
		pcapx.WithEthernet_DstMac("00:00:00:00:00:00"),
		pcapx.WithIPv4_SrcIP("192.168.1.11"),
		pcapx.WithIPv4_DstIP("192.168.1.12"),
		pcapx.WithTCP_SrcPort(53155),
		pcapx.WithTCP_DstPort(80),
		pcapx.WithPayload([]byte(`GET / HTTP/1.1
Host: www.example.com
User-Agent: abc11111
`)))
	if err != nil {
		log.Errorf("build packet failed: %v", err)
	}

	// do cache
	groupIns.FeedFrame(bytes)
	groupIns.Wait()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 200; j++ {
			groupIns.FeedFrame(bytes)
		}
		groupIns.Wait()
	}
}

func benchmarkgroupFeedframeTcp(rulecount, pkcount int, b *testing.B) {
	var ruleStr string
	for i := 0; i < rulecount; i++ {
		ruleStr += fmt.Sprintf("alert tcp any any -> any any (msg:\"Detected abc%d1111\"; content:\"abc%d1111\"; sid:%d; rev:1;)\n", i, i, i+1000000)
	}

	ruleIns, err := surirule.Parse(ruleStr)
	if err != nil {
		b.Fatal(err)
	}
	groupIns := NewGroup(WithGroupOnMatchedCallback(func(_ gopacket.Packet, _ *surirule.Rule) {
		return
	}))
	groupIns.LoadRules(ruleIns...)

	bytes, err := pcapx.PacketBuilder(
		pcapx.WithEthernet_NextLayerType("ip"),
		pcapx.WithEthernet_SrcMac("00:00:00:00:00:00"),
		pcapx.WithEthernet_DstMac("00:00:00:00:00:00"),
		pcapx.WithIPv4_SrcIP("192.168.1.11"),
		pcapx.WithIPv4_DstIP("192.168.1.12"),
		pcapx.WithTCP_SrcPort(53155),
		pcapx.WithTCP_DstPort(80),
		pcapx.WithPayload([]byte(`abc11111`)))
	if err != nil {
		log.Errorf("build packet failed: %v", err)
	}

	// do cache
	groupIns.FeedFrame(bytes)
	groupIns.Wait()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < pkcount; j++ {
			groupIns.FeedFrame(bytes)
		}
		groupIns.Wait()
	}
}

func TestGroup_FeedFrame_TCP(t *testing.T) {
	fmt.Printf("rulecount: %d, pkcount: %d\n", 1, 1)
	fmt.Println(testing.Benchmark(func(b *testing.B) {
		benchmarkgroupFeedframeTcp(1, 1, b)
	}).String())
	fmt.Printf("rulecount: %d, pkcount: %d\n", 1, 100)
	fmt.Println(testing.Benchmark(func(b *testing.B) {
		benchmarkgroupFeedframeTcp(1, 100, b)
	}).String())
	fmt.Printf("rulecount: %d, pkcount: %d\n", 100, 1)
	fmt.Println(testing.Benchmark(func(b *testing.B) {
		benchmarkgroupFeedframeTcp(100, 1, b)
	}).String())
	fmt.Printf("rulecount: %d, pkcount: %d\n", 100, 100)
	fmt.Println(testing.Benchmark(func(b *testing.B) {
		benchmarkgroupFeedframeTcp(100, 100, b)
	}).String())
	fmt.Printf("rulecount: %d, pkcount: %d\n", 100, 1000)
	fmt.Println(testing.Benchmark(func(b *testing.B) {
		benchmarkgroupFeedframeTcp(100, 1000, b)
	}).String())
	fmt.Printf("rulecount: %d, pkcount: %d\n", 1000, 100)
	fmt.Println(testing.Benchmark(func(b *testing.B) {
		benchmarkgroupFeedframeTcp(1000, 100, b)
	}).String())
}
