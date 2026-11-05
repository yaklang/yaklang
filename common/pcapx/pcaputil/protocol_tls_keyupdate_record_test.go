package pcaputil

import (
	"bytes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Authored AEAD regressions complement the independently encrypted source176
// packet mutations. Test secrets are public constants, never capture key logs.
func boundaryTLSFlow(t *testing.T, suite uint16) (*binFlow, *binTLS) {
	t.Helper()
	a, _ := remainingParser(t)
	s := newBinTLS()
	s.version, s.suite, s.client = 0x304, suite, 0
	s.random = bytes.Repeat([]byte{0x44}, 32)
	s.app, s.seq = [2]bool{true, true}, [2]uint64{2, 2}
	n := 32
	if suite == 0x1302 {
		n = 48
	}
	for dir := 0; dir < 2; dir++ {
		require.NoError(t, s.install13(dir, bytes.Repeat([]byte{byte(0x41 + dir)}, n)))
	}
	f := &binFlow{a: a, id: 1, protocol: "tls", tls: s, transportFlow: &TrafficFlow{}}
	t.Cleanup(func() { f.closeSession(); require.Zero(t, a.stats().BufferedBytes) })
	return f, s
}

func boundaryTLSRecord(t *testing.T, s *binTLS, dir int, seq uint64, content []byte, typ byte, padding int) []byte {
	t.Helper()
	plain := append(bytes.Clone(content), typ)
	plain = append(plain, make([]byte, padding)...)
	w := []byte{23, 3, 3, 0, 0}
	binary.BigEndian.PutUint16(w[3:], uint16(len(plain)+s.keys[dir].Overhead()))
	nonce := bytes.Clone(s.iv[dir])
	for i := 0; i < 8; i++ {
		nonce[11-i] ^= byte(seq >> uint(8*i))
	}
	return append(w, s.keys[dir].Seal(nil, nonce, plain, w)...)
}

func boundaryTLSReject(t *testing.T, s *binTLS, secret, iv []byte, key cipher.AEAD, seq uint64, result map[string]any, err error) {
	t.Helper()
	assert.Equal(t, sha256.Sum256(secret), sha256.Sum256(s.secret[1]), "must not install a new epoch before the record boundary")
	assert.Equal(t, iv, s.iv[1])
	assert.Same(t, key, s.keys[1])
	assert.Equal(t, seq, s.seq[1], "old authenticated record sequence may advance, but must not reset to the new epoch")
	assert.Nil(t, s.plaintext, "invalid interleaving must not expose application plaintext")
	assert.Zero(t, s.content)
	require.Error(t, err)
	var typed *ProtocolError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ErrMalformedMessage, typed.Kind)
	require.Nil(t, result)
}

func TestTLS13KeyUpdateRequiresFinalRecordBoundary(t *testing.T) {
	ku := []byte{24, 0, 0, 1, 0}
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		t.Run(tlsCipherTestName(suite), func(t *testing.T) {
			for name, tail := range map[string][]byte{"second-KeyUpdate": ku, "partial-next-header": ku[:2]} {
				t.Run(name, func(t *testing.T) {
					f, s := boundaryTLSFlow(t, suite)
					secret, iv, key := bytes.Clone(s.secret[1]), bytes.Clone(s.iv[1]), s.keys[1]
					raw := boundaryTLSRecord(t, s, 1, 2, append(bytes.Clone(ku), tail...), 22, 0)
					result, err := f.consumeTLS(1, &ProtocolEvent{Raw: raw})
					boundaryTLSReject(t, s, secret, iv, key, 3, result, err)
				})
			}
		})
	}
}

func TestTLS13KeyUpdateFragmentsKeepOldEpochUntilRecordEnd(t *testing.T) {
	ku := []byte{24, 0, 0, 1, 1}
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		t.Run(tlsCipherTestName(suite), func(t *testing.T) {
			for split := 1; split < len(ku); split++ {
				f, s := boundaryTLSFlow(t, suite)
				secret, iv, key := bytes.Clone(s.secret[1]), bytes.Clone(s.iv[1]), s.keys[1]
				e := &ProtocolEvent{Raw: boundaryTLSRecord(t, s, 1, 2, ku[:split], 22, 0)}
				_, err := f.consumeTLS(1, e)
				require.NoError(t, err)
				require.Equal(t, true, e.Session["Handshake Incomplete"])
				require.Empty(t, e.Session["Handshake Messages"])
				require.Equal(t, secret, s.secret[1])
				require.Equal(t, iv, s.iv[1])
				require.Same(t, key, s.keys[1])
				require.Equal(t, uint64(3), s.seq[1])
				e = &ProtocolEvent{Raw: boundaryTLSRecord(t, s, 1, 3, ku[split:], 22, 7)}
				_, err = f.consumeTLS(1, e)
				require.NoError(t, err)
				require.Equal(t, false, e.Session["Handshake Incomplete"])
				messages := e.Session["Handshake Messages"].([]map[string]any)
				require.Len(t, messages, 1)
				require.Equal(t, ku, messages[0]["Encoded Key Update"])
				require.NotEqual(t, secret, s.secret[1])
				require.Zero(t, s.seq[1])
				require.Equal(t, uint64(2), s.seq[0])
			}
		})
	}
}

func TestTLS13HandshakeFragmentRejectsInterleavedApplication(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		t.Run(tlsCipherTestName(suite), func(t *testing.T) {
			f, s := boundaryTLSFlow(t, suite)
			secret, iv, key := bytes.Clone(s.secret[1]), bytes.Clone(s.iv[1]), s.keys[1]
			_, err := f.consumeTLS(1, &ProtocolEvent{Raw: boundaryTLSRecord(t, s, 1, 2, []byte{24, 0, 0, 1}, 22, 0)})
			require.NoError(t, err)
			result, err := f.consumeTLS(1, &ProtocolEvent{Raw: boundaryTLSRecord(t, s, 1, 3, []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"), 23, 0)})
			boundaryTLSReject(t, s, secret, iv, key, 4, result, err)
		})
	}
}

func TestTLS13KeyUpdateAllowsPaddingAndEarlierTicket(t *testing.T) {
	// A minimal authored TLS1.3 NST. No NST exists in real source131; this
	// positive is an explicit unit control, not a claim about that capture.
	ticket := []byte{4, 0, 0, 14, 0, 0, 0, 60, 0, 0, 0, 1, 0, 0, 1, 0x42, 0, 0}
	ku := []byte{24, 0, 0, 1, 0}
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		t.Run(tlsCipherTestName(suite), func(t *testing.T) {
			for _, prefix := range [][]byte{nil, ticket} {
				f, s := boundaryTLSFlow(t, suite)
				e := &ProtocolEvent{Raw: boundaryTLSRecord(t, s, 1, 2, append(bytes.Clone(prefix), ku...), 22, 7)}
				_, err := f.consumeTLS(1, e)
				require.NoError(t, err)
				messages := e.Session["Handshake Messages"].([]map[string]any)
				want := 1
				if len(prefix) > 0 {
					want = 2
					require.Equal(t, byte(4), messages[0]["Type"])
				}
				require.Len(t, messages, want)
				require.Equal(t, ku, messages[len(messages)-1]["Encoded Key Update"])
				require.Zero(t, s.seq[1])
			}
		})
	}
}

func FuzzTLS13KeyUpdateRecordBoundary(f *testing.F) {
	f.Add(byte(0), []byte{}, byte(0))
	f.Add(byte(0), []byte{24, 0, 0, 1, 0}, byte(1))
	f.Add(byte(1), []byte{24, 0}, byte(2))
	f.Add(byte(2), []byte{}, byte(7))
	f.Add(byte(3), []byte("HTTP/1.1"), byte(4))
	f.Fuzz(func(t *testing.T, mode byte, tail []byte, amount byte) {
		if len(tail) > 64 {
			return
		}
		flow, s := boundaryTLSFlow(t, 0x1301)
		secret, iv, key := bytes.Clone(s.secret[1]), bytes.Clone(s.iv[1]), s.keys[1]
		ku := []byte{24, 0, 0, 1, 0}
		seq := uint64(2)
		content := append(bytes.Clone(ku), tail...)
		typ := byte(22)
		padding := 0
		switch mode % 4 {
		case 1, 3:
			split := int(amount%4) + 1
			_, err := flow.consumeTLS(1, &ProtocolEvent{Raw: boundaryTLSRecord(t, s, 1, seq, ku[:split], 22, 0)})
			require.NoError(t, err)
			require.Equal(t, secret, s.secret[1])
			seq++
			content = append(bytes.Clone(ku[split:]), tail...)
			if mode%4 == 3 {
				content = bytes.Clone(tail)
				typ = 23
			}
		case 2:
			content = bytes.Clone(ku)
			padding = int(amount % 32)
		}
		e := &ProtocolEvent{Raw: boundaryTLSRecord(t, s, 1, seq, content, typ, padding)}
		result, err := flow.consumeTLS(1, e)
		valid := mode%4 != 3 && (mode%4 == 2 || len(tail) == 0)
		if !valid {
			boundaryTLSReject(t, s, secret, iv, key, seq+1, result, err)
			return
		}
		require.NoError(t, err)
		require.Zero(t, s.seq[1])
		require.NotEqual(t, secret, s.secret[1])
		require.Nil(t, s.plaintext)
		messages := e.Session["Handshake Messages"].([]map[string]any)
		require.Len(t, messages, 1)
		require.Equal(t, ku, messages[0]["Encoded Key Update"])
	})
}
