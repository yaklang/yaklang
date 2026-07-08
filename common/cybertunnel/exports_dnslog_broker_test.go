package cybertunnel

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveLocalDNSLogBroker_EmptyOrWildcard(t *testing.T) {
	for _, mode := range []string{"", "*"} {
		t.Run(mode, func(t *testing.T) {
			broker, resolvedMode, err := resolveLocalDNSLogBroker(mode)
			require.NoError(t, err)
			require.NotNil(t, broker)
			require.NotEmpty(t, resolvedMode)
			require.Equal(t, broker.Name(), resolvedMode)
		})
	}
}

func TestResolveLocalDNSLogBroker_UnknownMode(t *testing.T) {
	_, _, err := resolveLocalDNSLogBroker("not-a-real-broker")
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "no existed")
}
