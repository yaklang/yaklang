package pcaputil

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func keyUpdateStateForTest(t *testing.T) *binTLS {
	t.Helper()
	s := newBinTLS()
	s.version, s.suite, s.client = 0x304, 0x1301, 0
	s.app[0], s.seq[0] = true, 7
	require.NoError(t, s.install13(0, bytes.Repeat([]byte{0x42}, 32)))
	s.seq[0] = 7
	return s
}

// The fuzz boundary starts after record authentication. Actual AEAD and peer
// plaintext are verified separately against the independently captured stream.
func FuzzTLS13KeyUpdateMessageAuthority(f *testing.F) {
	f.Add([]byte{24, 0, 0, 1, 0}, byte(7))
	f.Add([]byte{24, 0, 0, 1, 1}, byte(7))
	f.Add([]byte{24, 0, 0, 1, 2}, byte(7))
	f.Add([]byte{24, 0, 0, 0}, byte(0))
	f.Fuzz(func(t *testing.T, input []byte, flags byte) {
		if len(input) > 64 || len(input) == 0 || input[0] != 24 {
			return
		}
		s := keyUpdateStateForTest(t)
		if flags&1 == 0 {
			s.version = 0x303
		}
		s.app[0] = flags&2 != 0
		wire := bytes.Clone(input)
		secret, iv, seq := bytes.Clone(s.secret[0]), bytes.Clone(s.iv[0]), s.seq[0]
		m, err := s.handshakeMessage(0, wire, flags&4 != 0)
		valid := flags&7 == 7 && len(input) == 5 && bytes.Equal(input[:4], []byte{24, 0, 0, 1}) && input[4] <= 1
		if !valid {
			require.Error(t, err)
			require.Nil(t, m)
			require.Equal(t, secret, s.secret[0])
			require.Equal(t, iv, s.iv[0])
			require.Equal(t, seq, s.seq[0])
			return
		}
		require.NoError(t, err)
		require.Equal(t, input[4], m["Key Update Request"])
		require.Equal(t, input, m["Encoded Key Update"])
		require.NotEqual(t, secret, s.secret[0])
		require.Zero(t, s.seq[0])
		clear(wire)
		require.Equal(t, input, m["Encoded Key Update"])
	})
}

func TestTLS13KeyUpdateFieldsOwnAuthenticatedHandshake(t *testing.T) {
	for _, request := range []byte{0, 1} {
		s := keyUpdateStateForTest(t)
		wire := []byte{24, 0, 0, 1, request}
		original := bytes.Clone(wire)
		m, err := s.handshakeMessage(0, wire, true)
		require.NoError(t, err)
		require.Equal(t, request, m["Key Update Request"])
		require.Equal(t, []string{"update_not_requested", "update_requested"}[request], m["Key Update Request Name"])
		require.Equal(t, original, m["Encoded Key Update"])
		require.NotContains(t, m, "Wire Field Ranges", "decrypted offsets are not ciphertext offsets")
		clear(wire)
		require.Equal(t, original, m["Encoded Key Update"])
		require.Zero(t, s.seq[0])
		info := map[string]any{"Negotiated Version": uint16(0x304), "Authentication Verified": true, "Handshake Messages": []map[string]any{m}}
		e := &ProtocolEvent{Protocol: "tls", Status: "decoded", nativeTLSObservation: true, Session: cloneSession(info), semanticFields: cloneSession(info)}
		for _, poisoned := range []bool{false, true} {
			if poisoned {
				e.Protocol, e.Session, e.Fields = "tcp", map[string]any{"Authentication Verified": false}, map[string]any{"forged": true}
			}
			var found int
			for _, v := range e.DisplayFieldValues().Values {
				if v.Name == "tls.handshake.key_update.request_update" {
					found++
					require.Equal(t, uint64(request), v.Value)
					require.Equal(t, "derived", v.Origin)
					require.Nil(t, v.Range)
				}
			}
			require.Equal(t, 1, found)
		}
	}
}

func TestTLS13KeyUpdateRejectsInvalidStateAtomically(t *testing.T) {
	for _, name := range []string{"version", "unauthenticated", "before-finished", "enum", "empty", "long"} {
		t.Run(name, func(t *testing.T) {
			s := keyUpdateStateForTest(t)
			wire, authenticated := []byte{24, 0, 0, 1, 0}, true
			switch name {
			case "version":
				s.version = 0x303
			case "unauthenticated":
				authenticated = false
			case "before-finished":
				s.app[0] = false
			case "enum":
				wire[4] = 2
			case "empty":
				wire = []byte{24, 0, 0, 0}
			case "long":
				wire = []byte{24, 0, 0, 2, 0, 0}
			}
			secret, iv, sequence := bytes.Clone(s.secret[0]), bytes.Clone(s.iv[0]), s.seq[0]
			m, err := s.handshakeMessage(0, wire, authenticated)
			require.Error(t, err)
			require.Nil(t, m)
			require.Equal(t, secret, s.secret[0])
			require.Equal(t, iv, s.iv[0])
			require.Equal(t, sequence, s.seq[0])
		})
	}
}

func TestTLS13KeyUpdateSDKSummaryNamesObservedMessage(t *testing.T) {
	s := keyUpdateStateForTest(t)
	m, err := s.handshakeMessage(0, []byte{24, 0, 0, 1, 1}, true)
	require.NoError(t, err)
	e := &ProtocolEvent{Protocol: "tls", Status: "decoded", Session: map[string]any{"Record Type": byte(23), "Record Length": 22, "Authentication Verified": true, "Content Visibility": "authenticated-plaintext", "Handshake Messages": []map[string]any{m}}}
	before, err := json.Marshal(e.Session)
	require.NoError(t, err)
	e.summarizeProtocol()
	require.Contains(t, e.Summary, "KeyUpdate")
	require.Contains(t, e.Summary, "update_requested")
	after, err := json.Marshal(e.Session)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
