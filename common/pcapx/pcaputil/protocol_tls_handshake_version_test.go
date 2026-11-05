package pcaputil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTLS13ReservedWireHandshakeCannotAdvanceState(t *testing.T) {
	// These are RFC8446 B.3 legacy/DTLS reservations, plus the transcript-only
	// message_hash from 4.4.1, rather than a blacklist of unsupported extensions.
	for _, kind := range []byte{0, 3, 12, 14, 16, 21, 22, 23, 254} {
		for _, sender := range []int{0, 1} {
			for _, authenticated := range []bool{false, true} {
				t.Run(fmt.Sprintf("type%d/sender%d/authenticated%t", kind, sender, authenticated), func(t *testing.T) {
					state := newBinTLS()
					state.version, state.suite, state.client = 0x304, 0x1301, 0
					before := *state
					message, err := state.handshakeMessage(sender, []byte{kind, 0, 0, 0}, authenticated)
					require.Error(t, err)
					var protocolErr *ProtocolError
					require.ErrorAs(t, err, &protocolErr)
					require.Equal(t, ErrMalformedMessage, protocolErr.Kind)
					require.Nil(t, message)
					require.Equal(t, before, *state)
				})
			}
		}
	}
}

func TestTLS13ProtectedHelloCannotBootstrapMissingContext(t *testing.T) {
	for _, kind := range []byte{1, 2} {
		for _, sender := range []int{0, 1} {
			t.Run(fmt.Sprintf("type%d/sender%d", kind, sender), func(t *testing.T) {
				ch, sh := n13AuditHelloPair(0x1301, false, true, 0)
				body := ch
				if kind == 2 {
					body = sh
				}
				state := newBinTLS()
				state.version, state.suite = 0x304, 0x1301
				before := *state
				_, err := state.handshakeMessage(sender, reliabilityHS(kind, body), true)
				require.Error(t, err)
				var protocolErr *ProtocolError
				require.ErrorAs(t, err, &protocolErr)
				require.Equal(t, ErrMalformedMessage, protocolErr.Kind)
				require.Equal(t, before, *state)
				// At the TLS layer clear Hellos remain valid, including QUIC
				// Initial Hellos (the caller validates the QUIC packet level).
				_, err = state.handshakeMessage(sender, reliabilityHS(kind, body), false)
				require.NoError(t, err)
			})
		}
	}
}

func TestTLSHandshakeVersionScopePreservesLegacyAndUnknownObservations(t *testing.T) {
	for _, version := range []uint16{0, 0x300, 0x301, 0x302, 0x303} {
		for _, kind := range []byte{0, 14} {
			state := newBinTLS()
			state.version, state.client = version, 0
			_, err := state.handshakeMessage(1, []byte{kind, 0, 0, 0}, false)
			require.NoError(t, err)
		}
	}
}

func TestTLS13ReservedRecordRetainsAuthenticationWithoutMessageAdmission(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, sender := range []int{0, 1} {
			for _, kind := range []byte{0, 3, 14, 21, 23, 254} {
				t.Run(fmt.Sprintf("suite%04x/sender%d/type%d", suite, sender, kind), func(t *testing.T) {
					flow, state, events := closeNotifyTestFlow(t, suite, 1, false)
					closeNotifyTestSend(t, flow, state, sender, 22, []byte{kind, 0, 0, 0}, false)
					require.Len(t, *events, 1)
					e := (*events)[0]
					require.Equal(t, "malformed", e.Status)
					require.Equal(t, true, e.Session["Authentication Verified"])
					require.NotContains(t, e.Session, "Handshake Messages")
					require.Equal(t, uint64(1), state.seq[sender])
					require.Zero(t, state.serverHandshakeStage)
					require.Zero(t, state.clientHandshakeStage)
					require.Nil(t, state.child)
				})
			}
		}
	}
}
