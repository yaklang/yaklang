package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestIPFIXTCPEverySplitCoalescingAndDirection(t *testing.T) {
	w := ipfixNativeMessage(603, ipfixNativeTemplate())
	data := ipfixNativeMessage(603, nf9NativeData())
	for cut := 1; cut < len(w); cut++ {
		s := observedTestSession(t, 4739)
		require.Empty(t, s.Feed(0, time.Unix(1, 0), w[:cut]).Events)
		r := s.Feed(0, time.Unix(2, 0), append(append([]byte(nil), w[cut:]...), data...))
		require.Len(t, r.Events, 2)
		require.Equal(t, "decoded", r.Events[0].Status)
		require.Equal(t, "decoded", r.Events[1].Status)
		require.Equal(t, []any{uint64(17)}, r.Events[1].DisplayFields()["cflow.packets"])
		r = s.Feed(1, time.Unix(3, 0), data)
		require.Len(t, r.Events, 1)
		require.Equal(t, "context-required", r.Events[0].Status, "opposite direction has no template")
	}
}
func TestIPFIXTCPUnwithdrawnRedefinitionNeedsExplicitRecovery(t *testing.T) {
	s := observedTestSession(t, 4739)
	feed := func(seq uint32, sets ...[]byte) *ProtocolEvent {
		r := s.Feed(0, time.Unix(1, 0), ipfixNativeMessage(seq, sets...))
		require.Len(t, r.Events, 1)
		return r.Events[0]
	}
	require.Equal(t, "decoded", feed(0, ipfixNativeTemplate()).Status)
	changed := nf9NativeSet(2, []byte{1, 0, 0, 2, 0, 12, 0, 4, 0, 2, 0, 4})
	require.Equal(t, "context-required", feed(0, changed, nf9NativeData()).Status)
	require.Equal(t, "context-required", feed(0, changed, nf9NativeData()).Status, "repeating a conflicting definition cannot clear quarantine")
	require.Equal(t, "decoded", feed(0, nf9NativeSet(2, []byte{1, 0, 0, 0}), changed, nf9NativeData()).Status)
	r := feed(1, nf9NativeData())
	require.Equal(t, "decoded", r.Status)
	require.Equal(t, []any{"192.0.2.9"}, r.DisplayFields()["cflow.dstaddr"])
	fresh := observedTestSession(t, 4739)
	r2 := fresh.Feed(0, time.Unix(2, 0), ipfixNativeMessage(2, nf9NativeData()))
	require.Len(t, r2.Events, 1)
	require.Equal(t, "context-required", r2.Events[0].Status, "fresh transport generation cannot inherit earlier schema")
}
func TestIPFIXTCPMalformedCompleteMessageDoesNotHideNextBoundary(t *testing.T) {
	s := observedTestSession(t, 4739)
	s.Feed(0, time.Unix(1, 0), ipfixNativeMessage(0, ipfixNativeTemplate()))
	bad := ipfixNativeMessage(0, nf9NativeSet(2, []byte{1, 0, 0, 1, 0x80, 8, 0, 4}))
	r := s.Feed(0, time.Unix(2, 0), append(bad, ipfixNativeMessage(0, nf9NativeData())...))
	require.Len(t, r.Events, 2)
	require.Equal(t, "malformed", r.Events[0].Status)
	require.Equal(t, "context-required", r.Events[1].Status)
	recovery := ipfixNativeMessage(0, nf9NativeSet(2, []byte{0, 2, 0, 0}), ipfixNativeTemplate(), nf9NativeData())
	r = s.Feed(0, time.Unix(3, 0), recovery)
	require.Len(t, r.Events, 1)
	require.Equal(t, "decoded", r.Events[0].Status)
}
