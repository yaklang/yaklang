package fp

import (
	"bytes"
	"runtime"
	"testing"

	regexp "github.com/VillanCh/go-pcre2-lite/regexp2"
	"github.com/jinzhu/copier"
	"github.com/stretchr/testify/require"
)

func regressionProbeRules(t *testing.T, data string) map[*NmapProbe][]*NmapMatch {
	t.Helper()
	rules, err := ParseNmapServiceProbeToRuleMap([]byte(data))
	require.NoError(t, err)
	require.Len(t, rules, 1)
	return rules
}

func TestRuleBlockCacheSeparatesRuleContents(t *testing.T) {
	const original = "Probe TCP Fixture q|ping|\nrarity 1\nports 18080\nmatch fixture m|^first$| p/First/\n"
	cases := []struct {
		name string
		edit func(*NmapProbe, []*NmapMatch)
	}{
		{"payload", func(p *NmapProbe, _ []*NmapMatch) { p.Payload = "pong" }},
		{"ports", func(p *NmapProbe, _ []*NmapMatch) { p.DefaultPorts = []int{18081} }},
		{"regexp", func(_ *NmapProbe, m []*NmapMatch) {
			m[0] = ruleFromLine(t, `match fixture m|^second$| p/First/`)
		}},
		{"regexp options", func(_ *NmapProbe, m []*NmapMatch) {
			m[0].MatchRule.(*lazyRegexp).options |= regexp.IgnoreCase
		}},
		{"service", func(_ *NmapProbe, m []*NmapMatch) { m[0].ServiceName = "second" }},
		{"product", func(_ *NmapProbe, m []*NmapMatch) { m[0].ProductVerbose = "Second" }},
		{"version", func(_ *NmapProbe, m []*NmapMatch) { m[0].Version = "2" }},
		{"cpe", func(_ *NmapProbe, m []*NmapMatch) { m[0].CPEs = []string{"cpe:/a:test:second"} }},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			first := NewConfig(WithNmapRule(regressionProbeRules(t, original)))
			matcher, err := NewDefaultFingerprintMatcher(first)
			require.NoError(t, err)
			matcher.ruleBlocks(18080, first)

			next := regressionProbeRules(t, original)
			var want *NmapProbe
			for probe, matches := range next {
				want = probe
				item.edit(probe, matches)
			}
			_, blocks, best := matcher.ruleBlocks(18080, NewConfig(WithNmapRule(next)))
			require.Len(t, blocks, 1)
			require.Same(t, want, blocks[0].Probe)
			require.Equal(t, item.name != "ports", best)
		})
	}
}

func TestRuleBlockCacheInvalidatesBothRuleOptions(t *testing.T) {
	first := regressionProbeRules(t, "Probe TCP First q|ping|\nrarity 1\nports 18080\nmatch first m|^first$|\n")
	next := regressionProbeRules(t, "Probe TCP Second q|pong|\nrarity 1\nports 18080\nmatch second m|^second$|\n")
	base := NewConfig(WithNmapRule(first))
	matcher, err := NewDefaultFingerprintMatcher(base)
	require.NoError(t, err)
	matcher.ruleBlocks(18080, base)
	for name, option := range map[string]ConfigOption{
		"WithFingerprintRule": WithFingerprintRule(next),
		"WithNmapRule":        WithNmapRule(next),
	} {
		t.Run(name, func(t *testing.T) {
			config := NewConfig()
			require.NoError(t, copier.Copy(config, base))
			option(config)
			_, blocks, _ := matcher.ruleBlocks(18080, config)
			require.Len(t, blocks, 1)
			require.Equal(t, "second", blocks[0].Matched[0].ServiceName)
		})
	}
}

func TestPrefilterCacheSeparatesOrderedMatches(t *testing.T) {
	probe := &NmapProbe{Name: "SharedRegression", Proto: TCP, Payload: "ping"}
	first := ruleFromLine(t, `match first m|^first$|`)
	second := ruleFromLine(t, `match second m|^second$|`)
	for _, matches := range [][]*NmapMatch{{first}, {second}, {first, second}, {second, first}} {
		block := &RuleBlock{Probe: probe, Matched: matches}
		for _, banner := range []string{"first", "second"} {
			selected := block.candidateMatches([]byte(banner))
			require.Equal(t, sequentialService(matches, []byte(banner)), sequentialService(selected, []byte(banner)))
		}
	}
}

func TestPrefilterPreservesPCRESemantics(t *testing.T) {
	cases := []struct {
		name, pattern string
		banner        []byte
	}{
		{"final newline", `^hello$`, []byte("hello\n")},
		{"explicit newline", `^hello\n$`, []byte("hello\n")},
		{"repeated end assertion", `^hello$\n$`, []byte("hello\n")},
		{"literal dollar", `^hello\$$`, []byte("hello$\n")},
		{"empty match", `^$`, []byte("\n")},
		{"vertical whitespace", `^hello\s$`, []byte("hello\v")},
		{"unicode word", `^\w$`, []byte{0xe9}},
		{"unicode boundary", `\b\xE9\b`, []byte{0xe9}},
		{"unicode digit", `^\d$`, []byte("1")},
		{"high byte literal", `^\x80$`, []byte{0x80}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			rules := []*NmapMatch{ruleFromLine(t, "match fixture m|"+item.pattern+"|")}
			want := sequentialService(rules, item.banner)
			require.Equal(t, "fixture", want)
			require.Equal(t, want, firstService(rules, string(item.banner)))
		})
	}
}

func TestPrefilterConfirmsRelaxedEndAnchors(t *testing.T) {
	rules := []*NmapMatch{ruleFromLine(t, `match fixture m|^hello$|`)}
	block := &RuleBlock{Matched: rules}
	require.NotEmpty(t, block.candidateMatches([]byte("hello-extra")))
	require.Empty(t, firstService(rules, "hello-extra"))
}

func TestServicePrefilterClosesScanWorkers(t *testing.T) {
	if runtime.GOMAXPROCS(0) < 2 {
		t.Skip("native parallel scans require multiple processors")
	}
	block := &RuleBlock{Matched: []*NmapMatch{
		ruleFromLine(t, `match merged m|[a-z]{3}|`),
		ruleFromLine(t, `match assertion m|\b[a-z]{2}\b|`),
	}}
	banner := bytes.Repeat([]byte("abc "), 256)
	block.candidateMatches(banner)
	t.Cleanup(func() { _ = block.prefilter.db.Close() })
	before := runtime.NumGoroutine()
	for i := 0; i < 20; i++ {
		require.NotEmpty(t, block.candidateMatches(banner))
	}
	runtime.GC()
	runtime.Gosched()
	require.LessOrEqual(t, runtime.NumGoroutine()-before, 8, "scan workers must not outlive their scratch")
}
