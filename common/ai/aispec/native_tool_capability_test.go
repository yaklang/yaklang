package aispec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type capabilityTestClient struct {
	AIClient
	native bool
}

func (c *capabilityTestClient) SupportsNativeToolCalling() bool { return c.native }

func TestProviderSupportsNativeToolCalling(t *testing.T) {
	for _, tc := range []struct {
		name   string
		client AIClient
		want   bool
	}{
		{"native", &capabilityTestClient{native: true}, true},
		{"text", &capabilityTestClient{}, false},
		{"legacy", struct{ AIClient }{}, true},
		{"nil", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name := "capability-test-" + tc.name
			Register(name, func() AIClient { return tc.client })
			t.Cleanup(func() { Unregister(name) })
			require.Equal(t, tc.want, ProviderSupportsNativeToolCalling(name))
		})
	}
	require.True(t, ProviderSupportsNativeToolCalling("unregistered-capability-test"))
}
