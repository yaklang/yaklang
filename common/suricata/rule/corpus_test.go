package rule

import (
	"os"
	"testing"
)

func readEmergingScanRules(tb testing.TB) string {
	tb.Helper()
	data, err := os.ReadFile("../rules/emerging-scan.rules")
	if err != nil {
		tb.Fatal(err)
	}
	return string(data)
}

func TestCorpusCapabilityBaseline(t *testing.T) {
	type expectation struct {
		raw         string
		rules       int
		diagnostics int
		unsupported int
		stateful    int
	}
	corpora := map[string]expectation{
		"emerging-scan": {raw: readEmergingScanRules(t), rules: 284, diagnostics: 675, unsupported: 0, stateful: 675},
		"badrules":      {raw: badrule, rules: 177, diagnostics: 361, unsupported: 0, stateful: 361},
		"badrules-mini": {raw: badrule_mini, rules: 7, diagnostics: 32, unsupported: 20, stateful: 12},
	}
	for name, expected := range corpora {
		t.Run(name, func(t *testing.T) {
			rules, err := Parse(expected.raw)
			if err != nil {
				t.Fatal(err)
			}
			if len(rules) != expected.rules {
				t.Fatalf("rules=%d, want %d", len(rules), expected.rules)
			}
			diagnostics := ValidateRules(rules)
			byStatus := map[KeywordCapabilityStatus]int{}
			for _, diagnostic := range diagnostics {
				byStatus[diagnostic.Capability]++
				if diagnostic.Reason == "" {
					t.Fatalf("diagnostic for sid=%d keyword=%s has no reason", diagnostic.SID, diagnostic.Keyword)
				}
			}
			if len(diagnostics) != expected.diagnostics || byStatus[KeywordUnsupported] != expected.unsupported || byStatus[KeywordStatefulRequired] != expected.stateful {
				t.Fatalf("diagnostics=%d unsupported=%d stateful=%d, want %d/%d/%d", len(diagnostics), byStatus[KeywordUnsupported], byStatus[KeywordStatefulRequired], expected.diagnostics, expected.unsupported, expected.stateful)
			}
			t.Logf("rules=%d diagnostics=%d unsupported=%d stateful=%d", len(rules), len(diagnostics), byStatus[KeywordUnsupported], byStatus[KeywordStatefulRequired])
		})
	}
}

func BenchmarkParseCorpus(b *testing.B) {
	raw := readEmergingScanRules(b)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Parse(raw); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseStrictSingleRule(b *testing.B) {
	raw := `alert tcp $HOME_NET any -> $EXTERNAL_NET $HTTP_PORTS (content:"yak"; flags:S; sid:1;)`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ParseStrictRule(raw); err != nil {
			b.Fatal(err)
		}
	}
}

func FuzzParseStrict(f *testing.F) {
	f.Add(`alert tcp any any -> any any (content:"yak"; sid:1;)`)
	f.Add(`alert dns any any -> any 53 (dns.query; content:"example.com"; sid:2;)`)
	f.Add(`alert tcp [1.2.3.4,!10.0.0.0/8] [80,443] <> any any (flags:S; sid:3;)`)
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 64<<10 {
			t.Skip()
		}
		_, _ = ParseStrictRule(raw)
	})
}
