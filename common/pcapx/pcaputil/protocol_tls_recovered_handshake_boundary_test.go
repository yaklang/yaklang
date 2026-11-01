package pcaputil

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTLS13RecoveredKeyUpdateDoesNotGuessHandshakeBoundary(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, dir := range []int{0, 1} {
			t.Run(fmt.Sprintf("suite%04x/dir%d", suite, dir), func(t *testing.T) {
				flow, state := boundaryTLSFlow(t, suite)
				reverse := bytes.Clone(state.secret[1-dir])
				fragment := []byte{24, 0, 0, 1}
				first := &ProtocolEvent{Raw: boundaryTLSRecord(t, state, dir, 2, fragment, 22, 0)}
				_, err := flow.consumeTLS(dir, first)
				require.NoError(t, err)
				require.Equal(t, fragment, state.handshake[dir])
				next, err := quicHKDFExpandLabel(state.secret[dir], "traffic upd", len(state.secret[dir]))
				require.NoError(t, err)
				sender := *state
				require.NoError(t, sender.install13(dir, next))
				ku := []byte{24, 0, 0, 1, 0}
				for sequence := uint64(0); sequence < 2; sequence++ {
					event := &ProtocolEvent{Raw: boundaryTLSRecord(t, &sender, dir, sequence, ku, 22, 0)}
					_, err = flow.consumeTLS(dir, event)
					require.NoError(t, err)
					require.Equal(t, true, event.Session["Authentication Verified"])
					require.NotContains(t, event.Session, "Handshake Messages", "a fitting header does not prove message alignment")
					require.Equal(t, "unavailable after missing protected handshake bytes", event.Session["Handshake Parsing"])
					require.Empty(t, state.handshake[dir])
					require.True(t, state.handshakeGap[dir])
					require.Equal(t, next, state.secret[dir], "no unproven second key update may be committed")
					require.Equal(t, sequence+1, state.seq[dir])
					if sequence == 0 {
						require.Equal(t, true, event.Session["Unobserved KeyUpdate Inferred From Authentication"])
						require.Equal(t, len(fragment), event.Session["Discarded Handshake Fragment Bytes"])
						require.Equal(t, "unavailable after authenticated key-epoch recovery", event.Session["Handshake Boundary"])
					}
				}
				require.Equal(t, reverse, state.secret[1-dir])
				require.Equal(t, uint64(2), state.seq[1-dir])
				require.False(t, state.handshakeGap[1-dir])
			})
		}
	}
}
