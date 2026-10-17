//go:build servicescan_internet

package fp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestServicescanPublicEndpoints(t *testing.T) {
	matcher, err := NewDefaultFingerprintMatcher(NewConfig(
		WithTransportProtos(TCP),
		WithActiveMode(true),
		WithDisableWebFingerprint(false),
		WithProbeTimeout(4*time.Second),
		WithProbesMax(10),
		WithProbesConcurrentMax(10),
	))
	require.NoError(t, err)

	targets := []struct {
		host string
		port int
		want string
	}{
		{host: "scanme.nmap.org", port: 22, want: "ssh"},
		{host: "scanme.nmap.org", port: 80, want: "http"},
		{host: "example.com", port: 443, want: "http"},
		{host: "one.one.one.one", port: 443, want: "http"},
	}
	for _, target := range targets {
		target := target
		t.Run(target.host, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			result, err := matcher.MatchWithContext(ctx, target.host, target.port)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, OPEN, result.State)
			t.Logf("%s:%d service=%s banner=%q", target.host, target.port, result.GetServiceName(), result.GetBanner())
			require.Contains(t, result.GetServiceName(), target.want)
		})
	}
}
