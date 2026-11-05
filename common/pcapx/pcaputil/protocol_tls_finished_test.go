package pcaputil

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// These public test constants exercise framing and key-epoch mechanics. They
// do not claim to carry valid transcript MACs; the real PHA source oracle does.
func finishedTestMessage(n int) []byte {
	return append([]byte{20, byte(n >> 16), byte(n >> 8), byte(n)}, bytes.Repeat([]byte{0x5a}, n)...)
}

func TestTLS13PostHandshakeFinishedPreservesApplicationEpoch(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		t.Run(tlsCipherTestName(suite), func(t *testing.T) {
			f, s := boundaryTLSFlow(t, suite)
			n := 32
			if suite == 0x1302 {
				n = 48
			}
			secret, iv, key := bytes.Clone(s.secret[0]), bytes.Clone(s.iv[0]), s.keys[0]
			finished := boundaryTLSRecord(t, s, 0, 2, finishedTestMessage(n), 22, 3)
			want := []byte("GET /after-PHA HTTP/1.1\r\nHost: localhost\r\n\r\n")
			after := boundaryTLSRecord(t, s, 0, 3, want, 23, 0)
			_, err := f.consumeTLS(0, &ProtocolEvent{Raw: finished})
			require.NoError(t, err)
			require.Equal(t, uint64(3), s.seq[0], "PHA Finished uses the current application epoch")
			require.Equal(t, secret, s.secret[0])
			require.Equal(t, iv, s.iv[0])
			require.Same(t, key, s.keys[0])
			require.Equal(t, uint64(2), s.seq[1], "peer direction is independent")
			_, err = f.consumeTLS(0, &ProtocolEvent{Raw: after})
			require.NoError(t, err)
			require.Equal(t, want, s.plaintext)
			require.Equal(t, uint64(4), s.seq[0])
		})
	}
}

func TestTLSFinishedMalformedLengthCannotCommitEpoch(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, n := range []int{0, 12, 31, 33, 47, 49} {
			t.Run(fmt.Sprintf("%04x/%d", suite, n), func(t *testing.T) {
				_, s := boundaryTLSFlow(t, suite)
				s.app[0] = false
				secret, iv, key := bytes.Clone(s.secret[0]), bytes.Clone(s.iv[0]), s.keys[0]
				_, err := s.handshakeMessage(0, finishedTestMessage(n), true)
				require.Error(t, err)
				require.False(t, s.app[0])
				require.Equal(t, uint64(2), s.seq[0])
				require.Equal(t, secret, s.secret[0])
				require.Equal(t, iv, s.iv[0])
				require.Same(t, key, s.keys[0])
			})
		}
	}
}

func TestTLS13FinishedInitialTransitionAndCleartext(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		t.Run(tlsCipherTestName(suite), func(t *testing.T) {
			for _, dir := range []int{0, 1} {
				_, s := boundaryTLSFlow(t, suite)
				n := 32
				if suite == 0x1302 {
					n = 48
				}
				s.app[dir] = false
				wire := finishedTestMessage(n)
				_, err := s.handshakeMessage(dir, wire, false)
				require.Error(t, err, "cleartext Finished cannot advance TLS1.3")
				require.False(t, s.app[dir])
				_, err = s.handshakeMessage(dir, wire, true)
				require.NoError(t, err)
				require.True(t, s.app[dir])
				require.Zero(t, s.seq[dir])
				require.Nil(t, s.keys[dir])
				require.Nil(t, s.iv[dir])
				require.Nil(t, s.secret[dir])
				require.Equal(t, uint64(2), s.seq[1-dir])
			}
		})
	}
}

func TestTLSFinishedEnvelopeSizesAndOwnership(t *testing.T) {
	for _, spec := range []struct{ version, suite uint16 }{
		{0x304, 0x1301}, {0x304, 0x1302}, {0x304, 0x1303},
		{0x303, 0xc02f}, {0x303, 0xc02b}, {0x303, 0xc030}, {0x303, 0xc02c},
		{0x303, 0x009c}, {0x303, 0x009d}, {0x303, 0xcca8}, {0x303, 0xcca9},
	} {
		t.Run(fmt.Sprintf("%04x/%04x", spec.version, spec.suite), func(t *testing.T) {
			n := 12
			if spec.version == 0x304 {
				n = 32
				if spec.suite == 0x1302 {
					n = 48
				}
			}
			wire := finishedTestMessage(n)
			verify, err := DecodeTLSFinished(wire, spec.version, spec.suite)
			require.NoError(t, err)
			require.Equal(t, wire[4:], verify)
			for crop := 0; crop < len(wire); crop++ {
				_, err := DecodeTLSFinished(wire[:crop], spec.version, spec.suite)
				require.Error(t, err)
			}
			_, err = DecodeTLSFinished(append(bytes.Clone(wire), 0), spec.version, spec.suite)
			require.Error(t, err)
			clear(wire)
			require.Equal(t, bytes.Repeat([]byte{0x5a}, n), verify)
		})
	}
	for _, version := range []uint16{0, 0x301, 0x302, 0x305} {
		_, err := DecodeTLSFinished(finishedTestMessage(32), version, 0x1301)
		require.Error(t, err)
	}
	for _, suite := range []uint16{0, 0x1304, 0xc02f} {
		_, err := DecodeTLSFinished(finishedTestMessage(32), 0x304, suite)
		require.Error(t, err)
	}
}

func TestTLS13FinishedFragmentedApplicationEpochAndFields(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		n := 32
		if suite == 0x1302 {
			n = 48
		}
		wire := finishedTestMessage(n)
		for split := 1; split < len(wire); split++ {
			t.Run(fmt.Sprintf("%04x/%d", suite, split), func(t *testing.T) {
				f, s := boundaryTLSFlow(t, suite)
				key := s.keys[0]
				first := boundaryTLSRecord(t, s, 0, 2, wire[:split], 22, 0)
				last := boundaryTLSRecord(t, s, 0, 3, wire[split:], 22, 7)
				want := []byte("GET /after-split HTTP/1.1\r\n\r\n")
				after := boundaryTLSRecord(t, s, 0, 4, want, 23, 0)
				e := &ProtocolEvent{Protocol: "tls", Raw: first}
				_, err := f.consumeTLS(0, e)
				require.NoError(t, err)
				require.Equal(t, true, e.Session["Handshake Incomplete"])
				require.Empty(t, e.Session["Handshake Messages"])
				e = &ProtocolEvent{Protocol: "tls", Raw: last}
				_, err = f.consumeTLS(0, e)
				require.NoError(t, err)
				require.Equal(t, uint64(4), s.seq[0])
				require.Same(t, key, s.keys[0])
				messages := e.Session["Handshake Messages"].([]map[string]any)
				require.Len(t, messages, 1)
				require.Equal(t, wire, messages[0]["Encoded Finished"])
				require.Equal(t, "application", messages[0]["Finished Phase"])
				require.Contains(t, messages[0]["Finished Verification Status"], "transcript MAC not evaluated")
				fields := e.DisplayFieldValues()
				found := false
				for _, field := range fields.Values {
					if field.Name == "tls.handshake.verify_data" {
						found = true
						require.Equal(t, fmt.Sprintf("%x", wire[4:]), field.Value)
						require.Equal(t, "derived", field.Origin)
						require.Nil(t, field.Range)
					}
				}
				require.True(t, found)
				_, err = f.consumeTLS(0, &ProtocolEvent{Raw: after})
				require.NoError(t, err)
				require.Equal(t, want, s.plaintext)
			})
		}
	}
}

func TestTLSFinishedContextAndRoles(t *testing.T) {
	s := newBinTLS()
	s.seq[0] = 7
	m, err := s.handshakeMessage(0, finishedTestMessage(32), false)
	require.NoError(t, err)
	require.Contains(t, m["Finished Status"], "uninterpreted")
	require.NotContains(t, m, "Finished Verify Data")
	require.Equal(t, uint64(7), s.seq[0])
	_, s = boundaryTLSFlow(t, 0x1301)
	before := *s
	m, err = s.handshakeMessage(1, finishedTestMessage(32), true)
	require.Error(t, err)
	require.Nil(t, m)
	require.Equal(t, before, *s)
	s.version, s.suite = 0x303, 0xc02f
	m, err = s.handshakeMessage(1, finishedTestMessage(12), true)
	require.NoError(t, err)
	require.Equal(t, "handshake", m["Finished Phase"])
	require.Equal(t, uint64(2), s.seq[1], "TLS1.2 Finished does not restart the CCS record sequence")
}

func TestTLS13FinishedRequiresFinalRecordBoundary(t *testing.T) {
	for _, app := range []bool{false, true} {
		for _, tail := range [][]byte{{24, 0}, {24, 0, 0, 1, 0}} {
			t.Run(fmt.Sprintf("app=%t/tail=%d", app, len(tail)), func(t *testing.T) {
				f, s := boundaryTLSFlow(t, 0x1301)
				s.app[0] = app
				key, secret, iv := s.keys[0], bytes.Clone(s.secret[0]), bytes.Clone(s.iv[0])
				raw := boundaryTLSRecord(t, s, 0, 2, append(finishedTestMessage(32), tail...), 22, 7)
				result, err := f.consumeTLS(0, &ProtocolEvent{Raw: raw})
				require.Error(t, err)
				require.Nil(t, result)
				require.Equal(t, app, s.app[0])
				require.Equal(t, uint64(3), s.seq[0], "authenticated old-key record advances without installing/resetting an epoch")
				require.Same(t, key, s.keys[0])
				require.Equal(t, secret, s.secret[0])
				require.Equal(t, iv, s.iv[0])
			})
		}
	}
}

func FuzzTLS13FinishedEpoch(f *testing.F) {
	f.Add(byte(0), true, byte(0), finishedTestMessage(32))
	f.Add(byte(1), true, byte(0), finishedTestMessage(48))
	f.Add(byte(2), false, byte(1), finishedTestMessage(32))
	f.Add(byte(0), true, byte(1), finishedTestMessage(32))
	f.Add(byte(1), false, byte(0), []byte{20, 0, 0, 0})
	f.Fuzz(func(t *testing.T, choice byte, app bool, direction byte, encoded []byte) {
		if len(encoded) > 1024 {
			return
		}
		suite := []uint16{0x1301, 0x1302, 0x1303}[choice%3]
		n := 32
		if suite == 0x1302 {
			n = 48
		}
		check := func(wire []byte) {
			if len(wire) > 0 {
				wire[0] = 20 // This target mutates Finished framing/body, not other message grammars.
			}
			dir := int(direction % 2)
			s := newBinTLS()
			s.version, s.suite, s.client = 0x304, suite, 0
			s.app[dir], s.seq[dir] = app, 7
			require.NoError(t, s.install13(dir, bytes.Repeat([]byte{0x6d}, n)))
			before, input := *s, bytes.Clone(wire)
			m, err := s.handshakeMessage(dir, wire, true)
			require.Equal(t, input, wire)
			if err != nil {
				require.Nil(t, m)
				require.Equal(t, before, *s)
				return
			}
			require.Len(t, wire, n+4)
			require.Equal(t, byte(20), wire[0])
			require.Equal(t, wire[4:], m["Finished Verify Data"])
			if app {
				require.Zero(t, dir)
				require.Equal(t, uint64(7), s.seq[dir])
				require.Same(t, before.keys[dir], s.keys[dir])
				require.Equal(t, before.secret[dir], s.secret[dir])
			} else {
				require.True(t, s.app[dir])
				require.Zero(t, s.seq[dir])
				require.Nil(t, s.keys[dir])
			}
			clear(wire)
			require.Equal(t, input, m["Encoded Finished"])
		}
		// Framing mutations exercise rejection. Also force a complete Finished
		// to exercise epoch transitions, even when the arbitrary header is bad.
		check(bytes.Clone(encoded))
		valid := finishedTestMessage(n)
		copy(valid[4:], encoded)
		check(valid)
	})
}
