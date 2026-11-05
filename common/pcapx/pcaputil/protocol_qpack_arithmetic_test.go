package pcaputil

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func qpackArithmeticInteger(value uint64, prefix uint, high byte) []byte {
	mask := uint64(1<<prefix) - 1
	if value < mask {
		return []byte{high | byte(value)}
	}
	out := []byte{high | byte(mask)}
	value -= mask
	for value >= 128 {
		out = append(out, byte(value&127)|128)
		value >>= 7
	}
	return append(out, byte(value))
}

func qpackArithmeticBlock(dir int) []byte {
	// RIC=2, Base=2^63: the mathematical post-Base indices are 2^64
	// and 2^64+1, never the two existing entries at absolute 0 and 1.
	base := uint64(1) << 63
	out := append([]byte{3}, qpackArithmeticInteger(base-2, 7, 0)...)
	out = append(out, qpackArithmeticInteger(base, 4, 0x10)...)
	out = append(out, qpackArithmeticInteger(base+1, 4, 0x10)...)
	if dir == 0 {
		out = append(out, 0xd1, 0xd7) // static GET and https
	}
	return out
}

func TestQPACKPostBaseArithmeticCannotSelectWrappedEntry(t *testing.T) {
	var c qpackConn
	c.maxCap[0], c.table[0].capacity = 128, 128
	require.NoError(t, c.table[0].insert(qpackEntry{name: "x-proof", value: "unrelated table entry"}, 16))
	base := uint64(1)<<63 + 1
	index := uint64(1)<<63 - 1
	mathematical := new(big.Int).Add(new(big.Int).SetUint64(base), new(big.Int).SetUint64(index))
	require.Greater(t, mathematical.BitLen(), 64)
	block := append([]byte{2}, qpackArithmeticInteger(base-1, 7, 0)...)
	block = append(block, qpackArithmeticInteger(index, 4, 0x10)...)
	headers, meta, err := c.decodeSection(0, block)
	require.Error(t, err)
	require.Empty(t, headers)
	require.Nil(t, meta)
	require.Equal(t, uint64(1), c.table[0].insertCount)
	require.Len(t, c.table[0].entries, 1)
}

func TestQPACKArithmeticPreservesRequiredIntegerRange(t *testing.T) {
	for prefix := uint(1); prefix <= 8; prefix++ {
		for _, value := range []uint64{0, 1<<prefix - 1, 1 << 30, 1 << 61, 1<<62 - 1} {
			wire := qpackArithmeticInteger(value, prefix, 0)
			got, n, err := qpackPrefixed(wire, prefix)
			require.NoError(t, err)
			require.Equal(t, value, got)
			require.Equal(t, len(wire), n)
		}
	}
	// A large unused Base with no dynamic references remains legal. Do not
	// repair arithmetic by narrowing the mandatory 62-bit integer support.
	var c qpackConn
	block := append([]byte{0}, qpackArithmeticInteger(1<<62-1, 7, 0)...)
	block = append(block, 0xd9)
	headers, meta, err := c.decodeSection(0, block)
	require.NoError(t, err)
	require.Equal(t, uint64(1<<62-1), meta["Base"])
	require.Equal(t, []map[string]any{{"Name": ":status", "Value": "200", "Never Index": false}}, headers)
}

// Native ownership and secrets are explicitly declared algorithm fixtures.
// Separate captured-peer controls qualify the actual wire and derived packets.
func TestQPACKArithmeticNativeFieldFailureKeepsOtherStreams(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for _, dir := range []int{0, 1} {
				t.Run(fmt.Sprintf("v%x/%s/dir%d", version, algorithm, dir), func(t *testing.T) {
					q, key := quicConformanceContext(t, version, algorithm)
					n := n09QPACKOwner(t, q)
					capacity, encoder, valid := n09QPACKFixture(dir)
					require.Equal(t, "decoded", n.send(t, 1-dir, 10, 1, "1-RTT", key, uint64(3-dir), 0, h3ControlSETTINGS(1, capacity)).Status)
					require.Equal(t, "decoded", n.send(t, dir, 11, 2, "1-RTT", key, uint64(6+dir), 0, h3EncoderStream(encoder)).Status)
					block := qpackArithmeticBlock(dir)
					framed := h3Frame(1, block)
					wire := quicConformancePacket(t, key, "1-RTT", 3, 0, quicStreamFrame(0, 0, false, framed))
					e := n.dispatch(t, dir, 12, wire)
					require.Equal(t, "malformed", e.Status, e.Error)
					require.Equal(t, "http3", e.Protocol)
					require.Equal(t, true, e.Session["Authentication Verified"])
					require.Contains(t, e.Error, "QPACK_DECOMPRESSION_FAILED")
					require.NotContains(t, e.Session, "Headers")
					require.Equal(t, ErrMalformedMessage, q.streams[0].h3.fieldFailure[dir])
					require.Zero(t, q.streams[0].h3.headers[dir])
					require.Equal(t, uint64(2), q.qpack.table[dir].insertCount)
					qpackArithmeticRecoveredFieldOrigin(t, version, algorithm, dir)
					require.NotContains(t, q.spaces[dir][quicSpaceApplication].rejected, uint64(3), "authenticated packet and encoder state survive application rejection")
					fault, ok := e.Session["QPACK Field Error"].(map[string]any)
					require.True(t, ok)
					require.Equal(t, "QPACK_DECOMPRESSION_FAILED", fault["Error Name"])
					require.Equal(t, e.ID, fault["Original Field PDU ID"])
					require.Equal(t, block, fault["Rejected QPACK Block"])
					require.NotContains(t, e.Session, "HTTP3 Connection Error")
					require.False(t, q.closed, "an observed violation does not prove peer closure")
					fault["Original Field PDU ID"] = uint64(999)
					e.Raw[0] ^= 1
					later := n.send(t, dir, 13, 4, "1-RTT", key, 4, 0, h3Frame(1, valid))
					require.Equal(t, "decoded", later.Status, later.Error)
					require.NotEmpty(t, later.Session["Headers"], "other streams remain useful as passive observations")
					require.NotContains(t, later.Session, "QPACK Field Error")
					require.NotContains(t, later.Session, "HTTP3 Connection Error")
					bad := n.send(t, dir, 14, 5, "1-RTT", key, 0, uint64(len(framed)), h3Frame(0, []byte("after invalid field")))
					require.Equal(t, "malformed", bad.Status)
					require.NotContains(t, bad.Session, "Headers")
					require.Zero(t, q.streams[0].h3.dataBytes)
					again := n.dispatch(t, dir, 15, wire)
					require.NotContains(t, again.Session, "Headers")
					require.Zero(t, q.streams[0].h3.headers[dir])
					require.Equal(t, uint64(2), q.qpack.table[dir].insertCount)
				})
			}
		}
	}
}

func qpackArithmeticRecoveredFieldOrigin(t *testing.T, version uint32, algorithm string, dir int) {
	t.Helper()
	q, key := quicConformanceContext(t, version, algorithm)
	n := n09QPACKOwner(t, q)
	capacity, encoder, valid := n09QPACKFixture(dir)
	framed := h3Frame(1, valid)
	previous := n.send(t, dir, 20, 1, "1-RTT", key, 0, 0, framed)
	n09QPACKRequireContext(t, previous, "peer SETTINGS not observed")
	n.send(t, 1-dir, 21, 2, "1-RTT", key, uint64(3-dir), 0, h3ControlSETTINGS(1, capacity))
	n.send(t, dir, 22, 3, "1-RTT", key, uint64(6+dir), 0, h3EncoderStream(encoder))
	require.Equal(t, 1, q.streams[0].h3.headers[dir])
	current := n.send(t, dir, 23, 4, "1-RTT", key, 0, uint64(len(framed)), h3Frame(1, qpackArithmeticBlock(dir)))
	require.Equal(t, "malformed", current.Status)
	fact := current.Session["QPACK Field Error"].(map[string]any)
	require.Equal(t, current.ID, fact["Original Field PDU ID"], "a previous recovered header is not the source of the next field section")
	require.NotEqual(t, previous.ID, fact["Original Field PDU ID"])
	require.Equal(t, []uint64{current.ID}, fact["Byte Source"].(map[string]any)["Parent PDUs"])
	require.Equal(t, "context-required", previous.Status)
}

func TestQPACKArithmeticLateContextKeepsOriginalFieldBytes(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for _, dir := range []int{0, 1} {
				for _, encoderFirst := range []bool{false, true} {
					t.Run(fmt.Sprintf("v%x/%s/dir%d/encoderFirst%t", version, algorithm, dir, encoderFirst), func(t *testing.T) {
						q, key := quicConformanceContext(t, version, algorithm)
						n := n09QPACKOwner(t, q)
						capacity, encoder, _ := n09QPACKFixture(dir)
						block := qpackArithmeticBlock(dir)
						original := n.send(t, dir, 10, 1, "1-RTT", key, 0, 0, h3Frame(1, block))
						n09QPACKRequireContext(t, original, "peer SETTINGS not observed")
						var trigger *ProtocolEvent
						if encoderFirst {
							n.send(t, dir, 11, 2, "1-RTT", key, uint64(6+dir), 0, h3EncoderStream(encoder))
							trigger = n.send(t, 1-dir, 12, 3, "1-RTT", key, uint64(3-dir), 0, h3ControlSETTINGS(1, capacity))
						} else {
							n.send(t, 1-dir, 11, 2, "1-RTT", key, uint64(3-dir), 0, h3ControlSETTINGS(1, capacity))
							trigger = n.send(t, dir, 12, 3, "1-RTT", key, uint64(6+dir), 0, h3EncoderStream(encoder))
						}
						fact, ok := trigger.Session["QPACK Field Error"].(map[string]any)
						require.True(t, ok, "%v", trigger.Session)
						require.Equal(t, block, fact["Rejected QPACK Block"])
						require.Equal(t, original.ID, fact["Original Field PDU ID"])
						source := fact["Byte Source"].(map[string]any)
						require.Equal(t, []uint64{original.ID}, source["Parent PDUs"])
						require.Equal(t, []map[string]any{{"Packet": uint64(10), "Section": uint32(2), "Interface": 7, "Encapsulation": n.domain.Encapsulation}}, source["Packet References"])
						require.Equal(t, ErrMalformedMessage, q.streams[0].h3.fieldFailure[dir])
						require.False(t, q.streams[0].h3.blocked[dir])
						require.Zero(t, q.streams[0].h3.headers[dir])
						require.Equal(t, uint64(2), q.qpack.table[dir].insertCount)
						require.Equal(t, "context-required", original.Status)
						require.NotContains(t, original.Session, "QPACK Field Error", "published observations remain immutable")
						require.NotContains(t, trigger.Session, "HTTP3 Connection Error")
						n.a.closeUDPSessions()
						require.Zero(t, n.a.buffered.Load())
					})
				}
			}
		}
	}
}

func FuzzQPACKPostBaseArithmeticAndState(f *testing.F) {
	for _, v := range [][2]uint64{{0, 0}, {1, 0}, {1 << 63, 1 << 63}, {1<<63 + 1, 1<<63 - 1}, {1<<62 - 1, 1}, {^uint64(0), 1}, {0, ^uint64(0)}, {2, ^uint64(0) - 1}} {
		f.Add(v[0], v[1])
	}
	f.Fuzz(func(t *testing.T, base, index uint64) {
		var c qpackConn
		c.maxCap[0], c.table[0].capacity = 128, 128
		require.NoError(t, c.table[0].insert(qpackEntry{name: "x-proof", value: "source-owned entry"}, 16))
		block := []byte{2}
		if base == 0 {
			block = append(block, 0x80)
		} else {
			block = append(block, qpackArithmeticInteger(base-1, 7, 0)...)
		}
		block = append(block, qpackArithmeticInteger(index, 4, 0x10)...)
		headers, _, err := c.decodeSection(0, block)
		mathematical := new(big.Int).Add(new(big.Int).SetUint64(base), new(big.Int).SetUint64(index))
		if mathematical.Sign() != 0 {
			require.Error(t, err, "the sole table entry has absolute index zero")
			require.Empty(t, headers)
		} else {
			require.NoError(t, err)
			require.Len(t, headers, 1)
		}
		require.Equal(t, uint64(1), c.table[0].insertCount)
		require.Len(t, c.table[0].entries, 1)
	})
}
