package match

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
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
	defer groupIns.Close()
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
	defer groupIns.Close()
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
	defer groupIns.Close()
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

func mustParseGroupRules(t testing.TB, raw string) []*surirule.Rule {
	t.Helper()
	rules, err := surirule.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func waitForGroupPending(t testing.TB, group *Group, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		group.pendingMu.Lock()
		pending := group.pending
		group.pendingMu.Unlock()
		if pending == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("group pending count did not reach %d", want)
}

func TestGroup_LoadRuleResults(t *testing.T) {
	group := NewGroup()
	defer group.Close()

	valid := mustParseGroupRules(t, `alert tcp any any -> any any (msg:"valid"; content:"valid"; sid:2000001; rev:1;)`)[0]
	invalid := *valid
	invalid.Protocol = "ftp"
	results := group.LoadRulesWithResults(valid, &invalid, nil)
	if len(results) != 3 {
		t.Fatalf("got %d load results, want 3", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("valid rule failed to compile: %v", results[0].Err)
	}
	if results[1].Err == nil {
		t.Fatal("unsupported rule returned no error")
	}
	if results[2].Err == nil {
		t.Fatal("nil rule returned no error")
	}
}

func TestGroup_TryFeedBackpressureAndContext(t *testing.T) {
	group := newGroup(0)
	defer group.Close()
	if err := group.LoadRuleWithError(mustParseGroupRules(t, `alert http any any -> any any (msg:"block"; sid:2000004; rev:1;)`)[0]); err != nil {
		t.Fatal(err)
	}

	// Hold matcher access after the consumer receives the first flow. With an
	// unbuffered queue, subsequent feeds then see deterministic backpressure.
	group.matcherMu.Lock()
	if err := group.FeedHTTPRequestBytesContext(context.Background(), []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")); err != nil {
		group.matcherMu.Unlock()
		t.Fatal(err)
	}
	defer group.matcherMu.Unlock()

	if err := group.TryFeedHTTPRequestBytes([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")); !errors.Is(err, ErrGroupQueueFull) {
		t.Fatalf("Try feed error = %v, want ErrGroupQueueFull", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := group.FeedHTTPRequestBytesContext(ctx, []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Context feed error = %v, want context deadline", err)
	}
}

func TestGroup_CloseIsIdempotentAndRejectsFeed(t *testing.T) {
	group := NewGroup()
	var closeWG sync.WaitGroup
	for i := 0; i < 8; i++ {
		closeWG.Add(1)
		go func() {
			defer closeWG.Done()
			group.Close()
		}()
	}
	closeWG.Wait()

	if err := group.TryFeedHTTPRequestBytes([]byte("GET / HTTP/1.1\r\n\r\n")); !errors.Is(err, ErrGroupClosed) {
		t.Fatalf("feed after Close error = %v, want ErrGroupClosed", err)
	}
	select {
	case <-group.consumerDone:
	default:
		t.Fatal("consumer goroutine still running after Close")
	}
}

func TestGroup_CloseReleasesBlockedFeed(t *testing.T) {
	group := newGroup(0)
	if err := group.LoadRuleWithError(mustParseGroupRules(t, `alert http any any -> any any (msg:"block"; sid:2000005; rev:1;)`)[0]); err != nil {
		t.Fatal(err)
	}
	group.matcherMu.Lock()
	if err := group.FeedHTTPRequestBytesContext(context.Background(), []byte("GET / HTTP/1.1\r\n\r\n")); err != nil {
		group.matcherMu.Unlock()
		t.Fatal(err)
	}

	feedResult := make(chan error, 1)
	go func() {
		feedResult <- group.FeedHTTPRequestBytesContext(context.Background(), []byte("GET / HTTP/1.1\r\n\r\n"))
	}()
	waitForGroupPending(t, group, 2)

	closeDone := make(chan struct{})
	go func() {
		group.Close()
		close(closeDone)
	}()
	select {
	case err := <-feedResult:
		if !errors.Is(err, ErrGroupClosed) {
			group.matcherMu.Unlock()
			t.Fatalf("blocked feed error = %v, want ErrGroupClosed", err)
		}
	case <-time.After(time.Second):
		group.matcherMu.Unlock()
		t.Fatal("Close did not release blocked feed")
	}
	group.matcherMu.Unlock()
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("Close did not complete")
	}
}

func TestGroup_FeedCopiesCallerBuffer(t *testing.T) {
	var matches atomic.Int64
	group := NewGroup(WithGroupOnMatchedCallback(func(_ gopacket.Packet, _ *surirule.Rule) {
		matches.Add(1)
	}))
	defer group.Close()
	if err := group.LoadRuleWithError(mustParseGroupRules(t, `alert http any any -> any any (msg:"owned"; content:"owned-value"; http_header; sid:2000003; rev:1;)`)[0]); err != nil {
		t.Fatal(err)
	}

	raw := []byte("GET / HTTP/1.1\r\nX-Test: owned-value\r\n\r\n")
	if err := group.TryFeedHTTPRequestBytes(raw); err != nil {
		t.Fatal(err)
	}
	for i := range raw {
		raw[i] = 'x'
	}
	group.Wait()
	if matches.Load() != 1 {
		t.Fatalf("got %d matches after caller buffer mutation, want 1", matches.Load())
	}
}

func TestGroup_WaitIncludesCallbackCompletion(t *testing.T) {
	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	group := NewGroup(WithGroupOnMatchedCallback(func(_ gopacket.Packet, _ *surirule.Rule) {
		close(callbackStarted)
		<-releaseCallback
	}))
	defer group.Close()
	if err := group.LoadRuleWithError(mustParseGroupRules(t, `alert http any any -> any any (msg:"wait"; content:"wait-value"; http_header; sid:2000006; rev:1;)`)[0]); err != nil {
		t.Fatal(err)
	}
	if err := group.TryFeedHTTPRequestBytes([]byte("GET / HTTP/1.1\r\nX-Test: wait-value\r\n\r\n")); err != nil {
		t.Fatal(err)
	}

	select {
	case <-callbackStarted:
	case <-time.After(time.Second):
		t.Fatal("callback did not start")
	}
	waitDone := make(chan struct{})
	go func() {
		group.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
		t.Fatal("Wait returned before callback completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseCallback)
	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after callback completed")
	}
}

func TestGroup_ConcurrentLoadAndConsume(t *testing.T) {
	var matches atomic.Int64
	group := NewGroup(WithGroupOnMatchedCallback(func(_ gopacket.Packet, _ *surirule.Rule) {
		matches.Add(1)
	}))
	defer group.Close()
	ruleTemplate := `alert http any any -> any any (msg:"concurrent"; content:"concurrent-value"; http_header; sid:%d; rev:1;)`
	request := []byte("GET / HTTP/1.1\r\nX-Test: concurrent-value\r\n\r\n")

	workerRules := make([][]*surirule.Rule, 4)
	for worker := range workerRules {
		for i := 0; i < 20; i++ {
			ruleRaw := fmt.Sprintf(ruleTemplate, 2100000+worker*100+i)
			workerRules[worker] = append(workerRules[worker], mustParseGroupRules(t, ruleRaw)[0])
		}
	}

	var workers sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		worker := worker
		workers.Add(1)
		go func() {
			defer workers.Done()
			for _, currentRule := range workerRules[worker] {
				if err := group.LoadRuleWithError(currentRule); err != nil {
					t.Errorf("load rule: %v", err)
					return
				}
			}
		}()
	}
	for worker := 0; worker < 4; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := 0; i < 50; i++ {
				for {
					err := group.TryFeedHTTPRequestBytes(request)
					if err == nil {
						break
					}
					if !errors.Is(err, ErrGroupQueueFull) {
						t.Errorf("feed request: %v", err)
						return
					}
				}
			}
		}()
	}
	workers.Wait()
	group.Wait()
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
