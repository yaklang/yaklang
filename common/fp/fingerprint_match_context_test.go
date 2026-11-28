package fp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/netx"
)

func TestFingerprintCancellationInterruptsPendingDNS(t *testing.T) {
	const host = "fingerprint-cancel-dns.invalid"
	lookupStarted := make(chan struct{})
	releaseLookup := make(chan struct{})
	lookupFinished := make(chan struct{})
	previous := netx.GetDefaultOptions()
	netx.SetDefaultDNSOptions(
		func(*netx.ReliableDNSConfig) {
			close(lookupStarted)
			<-releaseLookup
		},
		netx.WithDNSDisabledDomain(host),
		netx.WithDNSOnFinished(func() { close(lookupFinished) }),
	)
	t.Cleanup(func() {
		close(releaseLookup)
		select {
		case <-lookupFinished:
		case <-time.After(time.Second):
			t.Error("DNS fixture did not finish")
		}
		netx.SetDefaultDNSOptions(previous...)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var finished *MatchResult
	matcher := &Matcher{Config: NewConfig(
		WithProbeTimeout(5*time.Second),
		WithOnFinished(func(result *MatchResult) { finished = result }),
	)}
	done := make(chan struct{})
	var result *MatchResult
	var err error
	go func() {
		defer close(done)
		result, err = matcher.MatchWithContext(ctx, host, 443)
	}()
	select {
	case <-lookupStarted:
	case <-time.After(time.Second):
		t.Fatal("fingerprint did not reach DNS lookup")
	}
	start := time.Now()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fingerprint still waits for DNS after task cancellation")
	}
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, CLOSED, result.State)
	require.Same(t, result, finished)
	t.Logf("fingerprint returned %s after cancellation", time.Since(start))
}
