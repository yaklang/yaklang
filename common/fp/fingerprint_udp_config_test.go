package fp

import (
	"context"
	"strings"
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

func TestLegacyFingerprintCacheDoesNotCrossProtocol(t *testing.T) {
	const port = 9
	legacy := utils.HostPort("127.0.0.1", port)
	SetMatchResultCache(legacy, &MatchResult{
		Target: "127.0.0.1",
		Port:   port,
		State:  OPEN,
		Fingerprint: &FingerprintInfo{
			IP:          "127.0.0.1",
			Port:        port,
			Proto:       TCP,
			Banner:      "tcp-banner",
			ServiceName: "tcp-fixture",
		},
	})
	rules, err := ParseNmapServiceProbeToRuleMap([]byte(`Probe UDP Local q|probe|
rarity 1
ports 9
match local m|^udp-banner$|
`))
	if err != nil {
		t.Fatal(err)
	}
	udpCfg := NewConfig(
		WithCache(true),
		WithTransportProtos(UDP),
		WithNmapRule(rules),
		WithActiveMode(true),
		WithDisableWebFingerprint(true),
		WithProbeTimeout(50*time.Millisecond),
		WithProbesMax(1),
	)
	udpMatcher, err := NewFingerprintMatcher(nil, udpCfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, _ := udpMatcher.MatchWithContext(ctx, "127.0.0.1", utils.EncodeUDPPort(port))
	if result != nil && result.Fingerprint != nil && strings.Contains(result.Fingerprint.Banner, "tcp-banner") {
		t.Fatal("udp lookup returned the tcp cache")
	}

	tcpCfg := NewConfig(WithCache(true), WithTransportProtos(TCP), WithDisableWebFingerprint(true), WithActiveMode(false))
	tcpMatcher, err := NewFingerprintMatcher(nil, tcpCfg)
	if err != nil {
		t.Fatal(err)
	}
	result, err = tcpMatcher.MatchWithContext(ctx, "127.0.0.1", port)
	if err != nil || result == nil || result.Fingerprint == nil || !strings.Contains(result.Fingerprint.Banner, "tcp-banner") {
		t.Fatalf("tcp lookup missed the legacy cache: err=%v result=%#v", err, result)
	}
}
