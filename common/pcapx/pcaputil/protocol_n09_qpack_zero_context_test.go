package pcaputil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestN09QPACKNativeObservedMaximumWithoutEntriesIsTypedRefusal(t *testing.T) {
	for _, capacity := range []uint64{0, 1, 31} {
		t.Run(fmt.Sprint(capacity), func(t *testing.T) {
			q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
			n := n09QPACKOwner(t, q)
			e := n.send(t, 1, 10, 1, "1-RTT", key, 3, 0, h3ControlSETTINGS(1, capacity))
			require.Equal(t, "decoded", e.Status)
			_, _, payload := n09QPACKFixture(0)
			e = n.send(t, 0, 11, 1, "1-RTT", key, 0, 0, h3Frame(1, payload))
			require.Equal(t, "malformed", e.Status)
			require.Equal(t, string(ErrMalformedMessage), e.ExpertCode)
			require.False(t, q.streams[0].h3.blocked[0])
			require.NotContains(t, e.Session, "Headers")
			require.Zero(t, q.qpack.table[0].insertCount)
		})
	}
}

func TestN09QPACKNativeEarlyDynamicPrefixCannotBorrowCurrentZero(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
			for _, settingsFirst := range []bool{false, true} {
				t.Run(fmt.Sprintf("v%x/%x/settingsFirst%v", version, suite, settingsFirst), func(t *testing.T) {
					q, early, _, finish := earlyRecoveryFixture(t, version, suite, true, false)
					finish(true)
					n := n09QPACKOwner(t, q)
					settings := func() {
						e := n.send(t, 1, 12, 1, "1-RTT", q.keys.app[1][0], 3, 0, h3ControlSETTINGS())
						require.Equal(t, "decoded", e.Status)
						require.NotContains(t, e.Session, "QPACK Resumed Field Contexts")
					}
					if settingsFirst {
						settings()
					}
					_, _, payload := n09QPACKFixture(0)
					e := n.send(t, 0, 11, 0, "0-RTT", early, 0, 0, h3Frame(1, payload))
					n09QPACKRequireContext(t, e, "remembered SETTINGS")
					if !settingsFirst {
						settings()
					}
					require.True(t, q.streams[0].h3.rememberedWaiting[0])
					require.Zero(t, q.streams[0].h3.headers[0])
					require.False(t, q.retired)
				})
			}
		}
	}
}
