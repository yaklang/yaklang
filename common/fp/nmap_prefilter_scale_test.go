package fp

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jinzhu/copier"
	"github.com/stretchr/testify/require"
)

func heaviestBlock(t *testing.T, port int) *RuleBlock {
	t.Helper()
	rules, err := GetDefaultNmapServiceProbeRules()
	require.NoError(t, err)
	cfg := NewConfig(WithActiveMode(true), WithNmapRule(rules), WithProbesMax(10), WithRarityMax(5))
	_, blocks, _ := GetRuleBlockByConfig(port, cfg)
	require.NotEmpty(t, blocks)
	var block *RuleBlock
	for _, item := range blocks {
		if item != nil && len(item.Matched) > len(blockMatched(block)) {
			block = item
		}
	}
	require.NotNil(t, block)
	require.GreaterOrEqual(t, len(block.Matched), 20)
	return block
}

func serviceOf(info *FingerprintInfo) string {
	if info == nil {
		return ""
	}
	return info.ServiceName
}

func TestLargeScanRuleLookupStaysCheap(t *testing.T) {
	rules, err := GetDefaultNmapServiceProbeRules()
	require.NoError(t, err)
	t.Logf("default service probes %d", len(rules))
	base := NewConfig(
		WithNmapRule(rules),
		WithActiveMode(true),
		WithDisableWebFingerprint(true),
		WithProbesMax(10),
		WithRarityMax(5),
	)
	matcher, err := NewDefaultFingerprintMatcher(base)
	require.NoError(t, err)

	_, blocks, _ := matcher.ruleBlocks(80, base)
	require.NotEmpty(t, blocks)

	const hits = 1000
	start := time.Now()
	for i := 0; i < hits; i++ {
		_, got, _ := matcher.ruleBlocks(80, base)
		require.Same(t, blocks[0], got[0])
	}
	elapsed := time.Since(start)
	t.Logf("1000 cached rule lookups: %s (%d ns) avg %s", elapsed, elapsed.Nanoseconds(), elapsed/hits)
	require.Less(t, elapsed, 500*time.Millisecond)

	const copies = 200
	start = time.Now()
	for i := 0; i < copies; i++ {
		copied := NewConfig()
		require.NoError(t, copier.Copy(copied, base))
		_, got, _ := matcher.ruleBlocks(80, copied)
		require.NotEmpty(t, got)
		require.Same(t, blocks[0], got[0])
	}
	copyElapsed := time.Since(start)
	t.Logf("200 copied-config rule lookups: %s avg %s", copyElapsed, copyElapsed/copies)
	require.Less(t, copyElapsed, time.Second)
}

func TestHotMatchScaleConcurrentCancelAndEdges(t *testing.T) {
	block := heaviestBlock(t, 80)
	banners := [][]byte{
		[]byte("SSH-2.0-OpenSSH_9.0\r\n"),
		[]byte("HTTP/1.0 200 OK\r\nServer: Apache/2.4.54\r\n\r\n"),
	}
	banner := banners[0]
	for _, candidate := range banners {
		if sequentialService(block.Matched, candidate) != "" {
			banner = candidate
			break
		}
	}
	want := sequentialService(block.Matched, banner)
	require.NotEmpty(t, want)

	probe := *block.Probe
	probe.Name = block.Probe.Name + "-scale"
	shared := &RuleBlock{Probe: &probe, Matched: block.Matched}
	require.Equal(t, want, serviceOf(applyMatchRules(nil, shared, banner, 80, net.ParseIP("127.0.0.1"), string(banner), TCP)))

	const rounds = 100
	start := time.Now()
	for i := 0; i < rounds; i++ {
		got := serviceOf(applyMatchRules(nil, shared, banner, 80, net.ParseIP("127.0.0.1"), string(banner), TCP))
		require.Equal(t, want, got)
	}
	avg := time.Since(start) / rounds
	t.Logf("probe %s rules %d hot match avg %s over %d", probe.Name, len(shared.Matched), avg, rounds)
	require.Less(t, avg, 5*time.Millisecond)

	var wg sync.WaitGroup
	var bad atomic.Int32
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				got := serviceOf(applyMatchRules(nil, shared, banner, 80, net.ParseIP("127.0.0.1"), string(banner), TCP))
				if got != want {
					bad.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	require.Zero(t, bad.Load())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start = time.Now()
	require.Nil(t, applyMatchRules(ctx, shared, banner, 80, net.ParseIP("127.0.0.1"), string(banner), TCP))
	cancelCost := time.Since(start)
	t.Logf("canceled match %s", cancelCost)
	require.Less(t, cancelCost, 50*time.Millisecond)

	require.Nil(t, applyMatchRules(nil, shared, nil, 80, net.ParseIP("127.0.0.1"), "", TCP))
	require.Nil(t, applyMatchRules(nil, shared, []byte{}, 80, net.ParseIP("127.0.0.1"), "", TCP))

	huge := make([]byte, 20480)
	copy(huge, []byte("GET / HTTP/1.0\r\nHost: example\r\n\r\n"))
	for i := 64; i < len(huge); i++ {
		huge[i] = byte(i%26) + 'a'
	}
	start = time.Now()
	_ = applyMatchRules(nil, shared, huge, 80, net.ParseIP("127.0.0.1"), string(huge[:32]), TCP)
	hugeCost := time.Since(start)
	t.Logf("20480-byte hot match %s", hugeCost)
	require.Less(t, hugeCost, 200*time.Millisecond)

	late := []*NmapMatch{
		ruleFromLine(t, `match early m|^nope$| p/Early/`),
		ruleFromLine(t, `match middle m|^still-no$| p/Middle/`),
		ruleFromLine(t, `match late m|^finally$| p/Late/`),
	}
	require.Equal(t, "late", firstService(late, "finally"))
	require.Equal(t, "", firstService(late, ""))
}

func TestConcurrentPrefilterCompileIsShared(t *testing.T) {
	block := heaviestBlock(t, 80)
	probe := *block.Probe
	probe.Name = block.Probe.Name + "-flight"
	banner := []byte("HTTP/1.0 200 OK\r\nServer: Apache/2.4.54\r\n\r\n")
	var compiled atomic.Int32
	onServicePrefilterCompile.Store(func(p *NmapProbe) {
		if p == &probe {
			compiled.Add(1)
		}
	})
	t.Cleanup(func() {
		onServicePrefilterCompile.Store(func(*NmapProbe) {})
	})

	var wg sync.WaitGroup
	var bad atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			item := &RuleBlock{Probe: &probe, Matched: block.Matched}
			info := applyMatchRules(nil, item, banner, 80, net.ParseIP("127.0.0.1"), string(banner), TCP)
			if serviceOf(info) == "" && sequentialService(block.Matched, banner) != "" {
				bad.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Zero(t, bad.Load())
	t.Logf("concurrent compiles of one probe: %d", compiled.Load())
	require.Equal(t, int32(1), compiled.Load())
}

func TestRawPrefilterCoversSequentialHits(t *testing.T) {
	cases := []struct {
		line   string
		banner []byte
		want   string
	}{
		{`match bin m|^\x80\x81$| p/Binary/`, []byte{0x80, 0x81}, "bin"},
		{`match one m|^.$| p/One/`, []byte{0x80}, "one"},
		{`match wide m|\x00\x01| p/Wide/`, []byte{0x00, 0x01, 0x02}, "wide"},
		{`match ascii m|^HTTP| p/Ascii/`, []byte("HTTP/1.0"), "ascii"},
		{`match pair m|^\xC2\x80$| p/Pair/`, []byte{0xC2, 0x80}, "pair"},
		{`match fold m|^abc$|i p/Fold/`, []byte("AbC"), "fold"},
		{`match foldbin m|^\xE9$|i p/FoldBin/`, []byte{0xC9}, "foldbin"},
	}
	var missed []string
	for _, item := range cases {
		rule := ruleFromLine(t, item.line)
		filter := compileNmapPrefilter([]*NmapMatch{rule})
		selected := filter.selectMatches([]*NmapMatch{rule}, item.banner)
		got := ""
		runes := make([]rune, len(item.banner))
		for i, b := range item.banner {
			runes[i] = rune(b)
		}
		for _, candidate := range selected {
			if info := match(candidate, runes, 9, net.ParseIP("127.0.0.1"), string(item.banner), TCP); info != nil {
				got = info.ServiceName
				break
			}
		}
		if got != item.want {
			missed = append(missed, item.want)
			t.Logf("raw prefilter missed %s banner %x selected %d", item.want, item.banner, len(selected))
		}
	}

	block := heaviestBlock(t, 80)
	filter := compileNmapPrefilter(block.Matched)
	banners := [][]byte{
		[]byte("SSH-2.0-OpenSSH_9.0\r\n"),
		[]byte("HTTP/1.0 200 OK\r\nServer: Apache/2.4.54\r\n\r\n"),
		{0x16, 0x03, 0x01, 0x00, 0x2c, 0x01, 0x00},
		{0x80, 0x81, 0x00, 0xff},
		[]byte("GET /\x80 HTTP/1.0\r\n\r\n"),
	}
	for _, banner := range banners {
		want := sequentialService(block.Matched, banner)
		selected := filter.selectMatches(block.Matched, banner)
		got := ""
		runes := make([]rune, len(banner))
		for i, b := range banner {
			runes[i] = rune(b)
		}
		for _, candidate := range selected {
			if info := match(candidate, runes, 80, net.ParseIP("127.0.0.1"), string(banner), TCP); info != nil {
				got = info.ServiceName
				break
			}
		}
		t.Logf("banner %q sequential %q prefilter %q selected %d/%d", banner, want, got, len(selected), len(block.Matched))
		if want != got {
			missed = append(missed, want+"!="+got)
		}
	}
	require.Empty(t, missed)
}
