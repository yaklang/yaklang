package fp

import (
	"context"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

func TestMatchUDPPortDoesNotRewriteSharedTransport(t *testing.T) {
	rules, err := ParseNmapServiceProbeToRuleMap([]byte(`Probe UDP Local q|probe|
rarity 1
ports 9
match local m|^banner$|
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := NewConfig(
		WithTransportProtos(TCP),
		WithNmapRule(rules),
		WithActiveMode(true),
		WithDisableWebFingerprint(true),
		WithProbeTimeout(50*time.Millisecond),
		WithProbesMax(1),
	)
	matcher, err := NewFingerprintMatcher(nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _ = matcher.MatchWithContext(ctx, "127.0.0.1", utils.EncodeUDPPort(9))
	if !cfg.CanOnlyScanTCP() {
		t.Fatalf("shared transport changed: %v", cfg.TransportProtos)
	}
}
