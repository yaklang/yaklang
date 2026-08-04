package scannode

import (
	"testing"

	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/utils/bruteutils"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
)

func TestRiskKindFromVuln(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		vuln *Vuln
		want string
	}{
		{
			name: "weak password plugin",
			vuln: &Vuln{Plugin: "weakpassword/http"},
			want: legionRiskKindWeakPassword,
		},
		{
			name: "explicit risk type",
			vuln: &Vuln{RiskType: "Compliance Risk"},
			want: "compliance_risk",
		},
		{
			name: "cve falls back to vulnerability",
			vuln: &Vuln{CVE: "CVE-2026-0001"},
			want: legionRiskKindVulnerability,
		},
		{
			name: "fallback security risk",
			vuln: &Vuln{},
			want: legionRiskKindSecurityRisk,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := riskKindFromVuln(tt.vuln); got != tt.want {
				t.Fatalf("riskKindFromVuln() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWeakPasswordDedupeKeyDoesNotUsePassword(t *testing.T) {
	t.Parallel()

	first := weakPasswordDedupeKey(&bruteutils.BruteItemResult{
		Target:   "127.0.0.1:22",
		Type:     "ssh",
		Username: "root",
		Password: "one",
	})
	second := weakPasswordDedupeKey(&bruteutils.BruteItemResult{
		Target:   "127.0.0.1:22",
		Type:     "ssh",
		Username: "root",
		Password: "two",
	})
	if first != second {
		t.Fatalf("dedupe key should ignore password, got %q and %q", first, second)
	}
}

func TestFingerprintHelpers(t *testing.T) {
	t.Parallel()

	result := &fp.MatchResult{
		Target: "127.0.0.1",
		Port:   443,
		Fingerprint: &fp.FingerprintInfo{
			Proto:       fp.TCP,
			ServiceName: "nginx",
		},
	}

	if got := fingerprintTitle(result); got != "nginx" {
		t.Fatalf("fingerprintTitle() = %q, want %q", got, "nginx")
	}
	if got := fingerprintIdentityKey(result); got == "" {
		t.Fatal("fingerprintIdentityKey() returned empty string")
	}
}

func TestPocVulnDedupeKeyPrefersUUID(t *testing.T) {
	t.Parallel()

	vuln := &tools.PocVul{
		UUID:   "uuid-1",
		Target: "http://127.0.0.1",
	}
	if got := pocVulnDedupeKey(vuln); got != "uuid-1" {
		t.Fatalf("pocVulnDedupeKey() = %q, want %q", got, "uuid-1")
	}
}
