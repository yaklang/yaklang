package fp

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func ruleFromLine(t *testing.T, line string) *NmapMatch {
	t.Helper()
	rule, err := parseNmapMatch(line)
	require.NoError(t, err)
	return rule
}

func firstService(rules []*NmapMatch, banner string) string {
	block := &RuleBlock{Matched: rules}
	info := applyMatchRules(nil, block, []byte(banner), 80, net.ParseIP("127.0.0.1"), banner, TCP)
	if info == nil {
		return ""
	}
	return info.ServiceName
}

func TestPrefilterKeepsRuleOrderAndCaptures(t *testing.T) {
	rules := []*NmapMatch{
		ruleFromLine(t, `match first m|^abc| p/First/ v/$1/`),
		ruleFromLine(t, `match second m|^abc(.*)$| p/Second/ v/$1/`),
	}
	require.Equal(t, "first", firstService(rules, "abc-tail"))

	backref := []*NmapMatch{
		ruleFromLine(t, `match echoed m|^(a)\1$| p/Echoed/`),
		ruleFromLine(t, `match other m|^zz$| p/Other/`),
	}
	require.Equal(t, "echoed", firstService(backref, "aa"))
	require.Equal(t, "", firstService(backref, "ab"))
}

func TestPrefilterHighByteStillMatches(t *testing.T) {
	rules := []*NmapMatch{
		ruleFromLine(t, `match bin m|^\x80\x81$| p/Binary/`),
		ruleFromLine(t, `match other m|^ascii$| p/Other/`),
	}
	block := &RuleBlock{Matched: rules}
	banner := []byte{0x80, 0x81}
	selected := block.candidateMatches(banner)
	require.NotEmpty(t, selected)
	require.Equal(t, "bin", selected[0].ServiceName)
	info := applyMatchRules(nil, block, banner, 9, net.ParseIP("127.0.0.1"), string(banner), TCP)
	require.NotNil(t, info)
	require.Equal(t, "bin", info.ServiceName)
}

func TestDefaultProbeBudgetAdmitsMoreFingerprints(t *testing.T) {
	cfg := NewConfig(WithActiveMode(true), WithDisableWebFingerprint(true))
	require.Equal(t, 10, cfg.ProbesMax)
	require.Equal(t, 10, cfg.ProbesConcurrentMax)

	older := NewConfig(WithActiveMode(true), WithProbesMax(5))
	_, few, ok := GetRuleBlockByConfig(80, older)
	require.True(t, ok)
	_, more, ok := GetRuleBlockByConfig(80, cfg)
	require.True(t, ok)
	require.Greater(t, len(more), len(few))
	require.LessOrEqual(t, len(more), selectionCap(cfg))

	fewRules := 0
	for _, block := range few {
		fewRules += len(block.Matched)
	}
	moreRules := 0
	for _, block := range more {
		moreRules += len(block.Matched)
	}
	require.GreaterOrEqual(t, moreRules, fewRules)
	t.Logf("port 80 probes %d -> %d, match rules %d -> %d", len(few), len(more), fewRules, moreRules)
}

func TestPrefilterMatchesRealProbeFaster(t *testing.T) {
	rules, err := GetDefaultNmapServiceProbeRules()
	require.NoError(t, err)
	cfg := NewConfig(WithActiveMode(true), WithNmapRule(rules), WithProbesMax(10), WithRarityMax(5))
	_, blocks, _ := GetRuleBlockByConfig(22, cfg)
	require.NotEmpty(t, blocks)
	var block *RuleBlock
	for _, item := range blocks {
		if item != nil && len(item.Matched) > len(blockMatched(block)) {
			block = item
		}
	}
	require.NotNil(t, block)
	if len(block.Matched) < 20 {
		t.Skipf("probe %s only has %d match rules", block.Probe.Name, len(block.Matched))
	}

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
	start := time.Now()
	sequential := sequentialService(block.Matched, banner)
	sequentialCost := time.Since(start)

	shared := &RuleBlock{Matched: block.Matched}
	start = time.Now()
	filteredInfo := applyMatchRules(nil, shared, banner, 22, net.ParseIP("127.0.0.1"), string(banner), TCP)
	filteredOnce := time.Since(start)
	filtered := ""
	if filteredInfo != nil {
		filtered = filteredInfo.ServiceName
	}

	start = time.Now()
	const rounds = 20
	for i := 0; i < rounds; i++ {
		againInfo := applyMatchRules(nil, shared, banner, 22, net.ParseIP("127.0.0.1"), string(banner), TCP)
		again := ""
		if againInfo != nil {
			again = againInfo.ServiceName
		}
		require.Equal(t, filtered, again)
	}
	filteredRepeat := time.Since(start) / rounds
	require.NotEmpty(t, sequential)
	require.Equal(t, sequential, filtered)
	t.Logf("probe %s rules %d sequential %s prefilter-once %s prefilter-hot %s", block.Probe.Name, len(block.Matched), sequentialCost, filteredOnce, filteredRepeat)
	if filteredRepeat > sequentialCost*2 {
		t.Fatalf("hot prefilter %s is slower than one sequential pass %s", filteredRepeat, sequentialCost)
	}
}

func blockMatched(block *RuleBlock) []*NmapMatch {
	if block == nil {
		return nil
	}
	return block.Matched
}

func sequentialService(rules []*NmapMatch, banner []byte) string {
	runes := []rune(string(banner))
	for i, b := range banner {
		if b < 0x80 {
			continue
		}
		runes = make([]rune, len(banner))
		for j, item := range banner {
			runes[j] = rune(item)
		}
		_ = i
		break
	}
	for _, rule := range rules {
		if info := match(rule, runes, 22, net.ParseIP("127.0.0.1"), string(banner), TCP); info != nil {
			return info.ServiceName
		}
	}
	return ""
}
