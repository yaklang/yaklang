package pcaputil

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

// This observer is explicitly given complete framed records after a hole. It
// does not claim default TCP reassembly releases data beyond a missing segment.
func TestTLS13RecoveredEpochRetiresOldHandshakeFragments(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		t.Run(tlsCipherTestName(suite), func(t *testing.T) {
			flow, s := boundaryTLSFlow(t, suite)
			reverseFragment := []byte{4, 0}
			s.handshake[0], s.handshakeAuthenticated[0] = bytes.Clone(reverseFragment), true
			_, err := flow.consumeTLS(1, &ProtocolEvent{Raw: boundaryTLSRecord(t, s, 1, 2, []byte{24, 0, 0, 1}, 22, 0)})
			require.NoError(t, err)
			old := bytes.Clone(s.secret[1])
			next, err := quicHKDFExpandLabel(old, "traffic upd", len(old))
			require.NoError(t, err)
			sender := *s
			require.NoError(t, sender.install13(1, next))
			// The last KU byte in old-key record sequence3 was not observed.
			e := &ProtocolEvent{ID: 10, Raw: boundaryTLSRecord(t, &sender, 1, 0, []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"), 23, 0)}
			_, err = flow.consumeTLS(1, e)
			require.NoError(t, err)
			require.Equal(t, true, e.Session["Authentication Verified"])
			require.Equal(t, true, e.Session["Unobserved KeyUpdate Inferred From Authentication"])
			require.Equal(t, true, e.Session["Missing Earlier Plaintext"])
			require.Empty(t, s.handshake[1])
			require.False(t, s.handshakeAuthenticated[1])
			require.Equal(t, 4, e.Session["Discarded Handshake Fragment Bytes"])
			require.Equal(t, reverseFragment, s.handshake[0])
			require.True(t, s.handshakeAuthenticated[0])
			require.Equal(t, next, s.secret[1])
			require.Equal(t, uint64(1), s.seq[1])
			flow.deliverTLS(1, e)
			require.NotNil(t, s.child)
			require.True(t, s.child.directions[1].stopped, "application decoder must not cross the missing plaintext")
			require.True(t, s.child.directions[1].tlsPlaintextGap)
			require.False(t, s.child.directions[0].stopped)
			require.Empty(t, s.child.protocol)
			require.Nil(t, s.plaintext)
			// The first new-epoch KU must not be prefixed by the old header.
			ku := []byte{24, 0, 0, 1, 0}
			e = &ProtocolEvent{Raw: boundaryTLSRecord(t, &sender, 1, 1, ku, 22, 0)}
			_, err = flow.consumeTLS(1, e)
			require.NoError(t, err)
			messages := e.Session["Handshake Messages"].([]map[string]any)
			require.Len(t, messages, 1)
			require.Equal(t, ku, messages[0]["Encoded Key Update"])
			require.Zero(t, s.seq[1])
			require.NotEqual(t, next, s.secret[1])
		})
	}
}

func TestTLS13InferredApplicationEpochRetiresFinishedFragment(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		t.Run(tlsCipherTestName(suite), func(t *testing.T) {
			flow, s := boundaryTLSFlow(t, suite)
			s.app[1] = false
			_, err := flow.consumeTLS(1, &ProtocolEvent{Raw: boundaryTLSRecord(t, s, 1, 2, []byte{20, 0, 0, 32}, 22, 0)})
			require.NoError(t, err)
			appSecret := bytes.Repeat([]byte{0x63}, len(s.secret[1]))
			flow.a.tlsSecrets = &TLSKeyLog{secrets: map[string][]byte{"SERVER_TRAFFIC_SECRET_0:" + hex.EncodeToString(s.random): appSecret}}
			sender := *s
			require.NoError(t, sender.install13(1, appSecret))
			e := &ProtocolEvent{Raw: boundaryTLSRecord(t, &sender, 1, 0, []byte("application epoch"), 23, 0)}
			_, err = flow.consumeTLS(1, e)
			require.NoError(t, err)
			require.Equal(t, true, e.Session["Application Epoch Inferred From Authenticated Record"])
			require.Empty(t, s.handshake[1])
			require.False(t, s.handshakeAuthenticated[1])
			require.Equal(t, 4, e.Session["Discarded Handshake Fragment Bytes"])
			require.Equal(t, appSecret, s.secret[1])
			require.Equal(t, uint64(1), s.seq[1])
			// No application data before sequence0 is absent here. Preserve the
			// existing application-gap policy, independently of the retired HS.
			require.False(t, flow.plaintextGap[1])
		})
	}
}
