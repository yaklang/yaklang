package match

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchStreamUsesRealHeaderAndReassembledPayload(t *testing.T) {
	packet := buildTCPPacket(t, []byte("first-"))
	matcher := compileTestRule(t, `alert tcp 192.0.2.1 12345 -> 192.0.2.2 80 (content:"first-second"; sid:70;)`)
	require.False(t, matcher.MatchPackage(packet))
	matched, err := matcher.MatchStream(packet, []byte("first-second"))
	require.NoError(t, err)
	require.True(t, matched)

	wrongHeader := compileTestRule(t, `alert tcp 192.0.2.1 12345 -> 192.0.2.2 81 (content:"first-second"; sid:71;)`)
	matched, err = wrongHeader.MatchStream(packet, []byte("first-second"))
	require.NoError(t, err)
	require.False(t, matched)
}
