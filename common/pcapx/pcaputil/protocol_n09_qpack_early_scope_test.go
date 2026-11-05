package pcaputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestN09QPACKNativeEarlyRangesOnlyBelongToObservedH3(t *testing.T) {
	for _, alpn := range []string{"", "doq", "other"} {
		t.Run("alpn="+alpn, func(t *testing.T) {
			q, early, _, finish := earlyRecoveryFixture(t, quicVersion1, 0x1301, true, false)
			finish(true)
			n := n09QPACKOwner(t, q)
			q.alpn = alpn
			e := n.send(t, 0, 11, 0, "0-RTT", early, 0, 100, []byte{0})
			require.Equal(t, "decoded", e.Status)
			require.Nil(t, q.streams[0].earlyBytes[0])
			require.Nil(t, q.streams[0].h3)
		})
	}
}

func TestN09QPACKNativeResetReleasesEarlyRangesWithoutContiguousH3(t *testing.T) {
	q, early, app, finish := earlyRecoveryFixture(t, quicVersion1, 0x1301, true, false)
	finish(true)
	n := n09QPACKOwner(t, q)
	e := n.send(t, 0, 11, 0, "0-RTT", early, 0, 100, []byte{0})
	require.Equal(t, "decoded", e.Status)
	require.NotEmpty(t, q.streams[0].earlyBytes[0])
	require.Nil(t, q.streams[0].h3)
	reset := append([]byte{4, 0, 0}, quicPutVarint(101)...)
	e = n.dispatch(t, 0, 12, quicConformancePacket(t, app, "1-RTT", 2, 0, reset))
	require.Equal(t, "decoded", e.Status)
	require.True(t, q.streams[0].resetDirection[0])
	require.Nil(t, q.streams[0].earlyBytes[0])
	require.Zero(t, cap(q.streams[0].earlyBytes[0]))
}
