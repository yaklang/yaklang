package fp

import (
	"context"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRuleBlockCacheSurvivesTransportCopy(t *testing.T) {
	rules, err := ParseNmapServiceProbeToRuleMap([]byte(`Probe TCP Shared q|probe|
rarity 1
ports 18080
match shared m|^a$|
Probe UDP SharedUDP q|udp|
rarity 1
ports 18080
match sharedudp m|^b$|
`))
	require.NoError(t, err)

	base := NewConfig(
		WithNmapRule(rules),
		WithTransportProtos(TCP),
		WithActiveMode(true),
		WithProbesMax(5),
		WithRarityMax(9),
	)
	matcher, err := NewFingerprintMatcher(nil, base)
	require.NoError(t, err)

	copied := NewConfig(
		WithNmapRule(rules),
		WithTransportProtos(TCP),
		WithActiveMode(true),
		WithProbesMax(5),
		WithRarityMax(9),
	)
	_, first, ok := matcher.ruleBlocks(18080, copied)
	require.True(t, ok)
	require.NotEmpty(t, first)
	_, second, ok := matcher.ruleBlocks(18080, copied)
	require.True(t, ok)
	require.Same(t, first[0], second[0])
	require.Equal(t, TCP, first[0].Probe.Proto)

	udpRules, err := ParseNmapServiceProbeToRuleMap([]byte(`Probe TCP Shared q|probe|
rarity 1
ports 18080
match shared m|^a$|
Probe UDP SharedUDP q|udp|
rarity 1
ports 18080
match sharedudp m|^b$|
`))
	require.NoError(t, err)
	udp := NewConfig(
		WithNmapRule(udpRules),
		WithTransportProtos(UDP),
		WithActiveMode(true),
		WithProbesMax(5),
		WithRarityMax(9),
	)
	_, udpBlocks, ok := matcher.ruleBlocks(18080, udp)
	require.True(t, ok)
	require.NotEmpty(t, udpBlocks)
	require.Equal(t, UDP, udpBlocks[0].Probe.Proto)
	require.NotSame(t, first[0], udpBlocks[0])

	short := NewConfig(
		WithNmapRule(rules),
		WithTransportProtos(TCP),
		WithActiveMode(true),
		WithProbesMax(1),
		WithRarityMax(9),
	)
	_, shortBlocks, ok := matcher.ruleBlocks(18080, short)
	require.True(t, ok)
	require.Len(t, shortBlocks, 1)
	require.NotSame(t, first[0], shortBlocks[0])
	_, fullAgain, _ := matcher.ruleBlocks(18080, copied)
	require.Same(t, first[0], fullAgain[0])
	require.GreaterOrEqual(t, len(fullAgain), len(shortBlocks))
}

func TestServiceProbeReportsOpenOnce(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.SetDeadline(time.Now().Add(time.Second))
			buf := make([]byte, 5)
			_, err = io.ReadFull(conn, buf)
			if err == nil && string(buf) == "probe" {
				_, _ = conn.Write([]byte("fixture-banner"))
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done })

	port := listener.Addr().(*net.TCPAddr).Port
	rules, err := ParseNmapServiceProbeToRuleMap([]byte(
		"Probe TCP LocalFixture q|probe|\nrarity 1\nports " + strconv.Itoa(port) + "\nmatch local-fixture m|^fixture-banner$|\n",
	))
	require.NoError(t, err)

	var opens atomic.Int32
	matcher, err := NewDefaultFingerprintMatcher(NewConfig(
		WithTransportProtos(TCP),
		WithProbesMax(3),
		WithNmapRule(rules),
		WithProbeTimeout(300*time.Millisecond),
		WithActiveMode(true),
		WithDisableWebFingerprint(true),
		WithOnPortOpenCallback(func(result *MatchResult) {
			if result != nil && result.State == OPEN {
				opens.Add(1)
			}
		}),
	))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := matcher.MatchWithContext(ctx, "127.0.0.1", port)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, OPEN, result.State)
	require.Contains(t, result.GetServiceName(), "local-fixture")
	require.Equal(t, int32(1), opens.Load())
}
