package pcaputil

import (
	"bytes"
	"encoding/binary"
	"github.com/stretchr/testify/require"
	"testing"
)

func recoveryTLSRecord(t *testing.T, s *binTLS, dir int, seq uint64, p []byte) []byte {
	t.Helper()
	w := []byte{23, 3, 3, 0, 0}
	payload := append(bytes.Clone(p), 23)
	binary.BigEndian.PutUint16(w[3:], uint16(len(payload)+16))
	n := bytes.Clone(s.iv[dir])
	for i := 0; i < 8; i++ {
		n[11-i] ^= byte(seq >> uint(i*8))
	}
	return append(w, s.keys[dir].Seal(nil, n, payload, w)...)
}

func TestTLSAuthenticatedRecordRecoveryKeepsReverseDirection(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		t.Run(tlsCipherTestName(suite), func(t *testing.T) {
			a, _ := remainingParser(t)
			state := newBinTLS()
			state.version, state.suite, state.client = 0x304, suite, 0
			state.random = bytes.Repeat([]byte{4}, 32)
			state.app = [2]bool{true, true}
			n := 32
			if suite == 0x1302 {
				n = 48
			}
			for d := 0; d < 2; d++ {
				require.NoError(t, state.install13(d, bytes.Repeat([]byte{byte(d + 1)}, n)))
			}
			child := &binFlow{a: a, id: 1, protocol: "http2", h2: newBinHTTP2(0)}
			require.True(t, child.retain(&child.directions[0], []byte("partial frame")))
			state.child = child
			f := &binFlow{a: a, id: 1, protocol: "tls", tls: state, transportFlow: &TrafficFlow{}}
			bad := recoveryTLSRecord(t, state, 0, 0, []byte("bad"))
			bad[len(bad)-1] ^= 1
			e := &ProtocolEvent{Raw: bad}
			_, err := f.consumeTLS(0, e)
			require.True(t, tlsAuthenticationError(err))
			require.Equal(t, true, e.Session["Record Failure Isolated"])
			require.True(t, child.directions[0].stopped)
			require.False(t, child.directions[1].stopped)
			require.Empty(t, child.directions[0].buffer)
			e = &ProtocolEvent{Raw: recoveryTLSRecord(t, state, 0, 1, []byte("intact later record"))}
			_, err = f.consumeTLS(0, e)
			require.NoError(t, err)
			require.Equal(t, true, e.Session["Authentication Verified"])
			require.Equal(t, true, e.Session["Record Recovery Verified"])
			e = &ProtocolEvent{Raw: recoveryTLSRecord(t, state, 1, 0, []byte("reverse unaffected"))}
			_, err = f.consumeTLS(1, e)
			require.NoError(t, err)
			require.Equal(t, true, e.Session["Authentication Verified"])
			f.closeSession()
			require.Zero(t, a.stats().BufferedBytes)
		})
	}
}
func tlsCipherTestName(suite uint16) string {
	return map[uint16]string{0x1301: "aes128", 0x1302: "aes256", 0x1303: "chacha"}[suite]
}

func TestTLSRecoveryDoesNotCommitFailedTrialsOrInventKeyUpdate(t *testing.T) {
	a, _ := remainingParser(t)
	s := newBinTLS()
	s.version, s.suite, s.client = 0x304, 0x1301, 0
	s.app[0] = true
	s.random = bytes.Repeat([]byte{3}, 32)
	require.NoError(t, s.install13(0, bytes.Repeat([]byte{1}, 32)))
	f := &binFlow{a: a, id: 1, tls: s, transportFlow: &TrafficFlow{}}
	bad := recoveryTLSRecord(t, s, 0, 0, []byte("corrupt"))
	bad[len(bad)-1] ^= 1
	_, err := f.consumeTLS(0, &ProtocolEvent{Raw: bad})
	require.Error(t, err)
	e := &ProtocolEvent{Raw: recoveryTLSRecord(t, s, 0, 5, []byte("after missing whole records"))}
	_, err = f.consumeTLS(0, e)
	require.NoError(t, err)
	require.EqualValues(t, 5, e.Session["Recovered Record Sequence"])
	require.Equal(t, true, e.Session["Missing Earlier Plaintext"])
	oldSecret := bytes.Clone(s.secret[0])
	next, err := quicHKDFExpandLabel(oldSecret, "traffic upd", 32)
	require.NoError(t, err)
	sender := *s
	require.NoError(t, sender.install13(0, next))
	e = &ProtocolEvent{Raw: recoveryTLSRecord(t, &sender, 0, 0, []byte("new key epoch"))}
	_, err = f.consumeTLS(0, e)
	require.NoError(t, err)
	require.Equal(t, true, e.Session["Unobserved KeyUpdate Inferred From Authentication"])
	secret := bytes.Clone(s.secret[0])
	for i := 0; i < 4; i++ {
		_, err = f.consumeTLS(0, &ProtocolEvent{Raw: bad})
		require.Error(t, err)
		require.Equal(t, secret, s.secret[0])
		require.Nil(t, s.plaintext)
	}
	e = &ProtocolEvent{Raw: bad}
	_, err = f.consumeTLS(0, e)
	require.NoError(t, err)
	require.Equal(t, false, e.Session["Authentication Verified"])
	require.Equal(t, "recovery-exhausted", e.Session["Secret Lookup Status"])
	f.closeSession()
	require.Zero(t, a.stats().BufferedBytes)
}

func TestMidstreamNewSYNSeparatesNativeApplicationGeneration(t *testing.T) {
	for _, workers := range []int{1, 2, 4} {
		steps := []tcpStep{{seq: 101, data: "*1\r\n$4\r\nPING\r\n"}, {seq: 501, reverse: true, data: "+PONG\r\n"}, {seq: 9000, syn: true}, {seq: 19000, ack: 9001, syn: true, synack: true, reverse: true}, {seq: 9001, ack: 19001, data: "GET /new HTTP/1.1\r\nHost: audit.invalid\r\n\r\n"}, {seq: 19001, ack: 9041, reverse: true, data: "HTTP/1.1 204 No Content\r\n\r\n"}}
		es, stats, err := binReplay(t, binTestPcap(t, steps, 45000, false, false), workers, WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true}))
		require.NoError(t, err)
		require.Zero(t, stats.BufferedBytes)
		ids := map[string]uint64{}
		for _, e := range es {
			if e.Status == "decoded" {
				require.Empty(t, e.Error)
				ids[e.Protocol] = e.FlowID
			}
		}
		require.NotZero(t, ids["redis"])
		require.NotZero(t, ids["http"])
		require.NotEqual(t, ids["redis"], ids["http"])
	}
}
