package stream_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func l2tp3TestAVP(typ uint16, value []byte) []byte {
	w := make([]byte, 6+len(value))
	binary.BigEndian.PutUint16(w, 0x8000|uint16(len(w)))
	binary.BigEndian.PutUint16(w[4:], typ)
	copy(w[6:], value)
	return w
}
func l2tp3TestControl(typ uint16, udp bool, avps ...[]byte) []byte {
	w := make([]byte, 12)
	binary.BigEndian.PutUint16(w, 0xc803)
	binary.BigEndian.PutUint32(w[4:], 0x12345678)
	binary.BigEndian.PutUint16(w[8:], 0xfffe)
	binary.BigEndian.PutUint16(w[10:], 0xffff)
	if typ != 0 {
		w = append(w, l2tp3TestAVP(0, []byte{byte(typ >> 8), byte(typ)})...)
	}
	for _, a := range avps {
		w = append(w, a...)
	}
	binary.BigEndian.PutUint16(w[2:], uint16(len(w)))
	if !udp {
		w = append([]byte{0, 0, 0, 0}, w...)
	}
	return w
}

func TestL2TPv3ControlWireBoundaries(t *testing.T) {
	for _, udp := range []bool{false, true} {
		w := l2tp3TestControl(6, udp, l2tp3TestAVP(25, []byte{0x12, 0x34, 0x56, 0x78}))
		m, err := DecodeL2TPv3Wire(w, udp, 64)
		require.NoError(t, err)
		require.Equal(t, "HELLO", m["Packet Name"])
		require.Equal(t, uint64(0x12345678), m["Control Connection ID"])
		require.Equal(t, uint64(0xfffe), m["Ns"])
		require.Equal(t, uint64(0x12345678), m["AVPs"].([]map[string]any)[1]["Physical Channel ID"])
		for cut := 0; cut < len(w); cut++ {
			_, err = DecodeL2TPv3Wire(w[:cut], udp, 64)
			require.Error(t, err, "udp=%t cut=%d", udp, cut)
		}
		_, err = DecodeL2TPv3Wire(append(bytes.Clone(w), 0), udp, 64)
		require.Error(t, err, "padding must be stripped by actual IP/UDP length, not AVP parser")
		_, err = DecodeL2TPv3Wire(w, udp, 1)
		require.ErrorIs(t, err, ErrObservedWireBudget)
		ack, err := DecodeL2TPv3Wire(l2tp3TestControl(0, udp), udp, 64)
		require.NoError(t, err)
		require.True(t, ack["Zero Length Body"].(bool))
	}
}

func TestL2TPv3ControlAuthenticationAndUnknowns(t *testing.T) {
	digest := func(alg byte, n int) []byte { return l2tp3TestAVP(59, append([]byte{alg}, make([]byte, n)...)) }
	_, err := DecodeL2TPv3Wire(l2tp3TestControl(6, false, digest(0, 16), digest(1, 20)), false, 64)
	require.NoError(t, err, "two adjacent digests are explicitly allowed by RFC3931")
	for _, avps := range [][][]byte{{digest(0, 15)}, {digest(1, 16)}, {digest(0, 16), digest(1, 20), digest(0, 16)}, {l2tp3TestAVP(25, []byte{0, 0, 0, 1}), digest(0, 16)}} {
		_, err = DecodeL2TPv3Wire(l2tp3TestControl(6, false, avps...), false, 64)
		require.Error(t, err)
	}
	m, err := DecodeL2TPv3Wire(l2tp3TestControl(6, false, l2tp3TestAVP(67, []byte{1, 2, 3})), false, 64)
	require.NoError(t, err)
	require.True(t, m["Unparsed Mandatory Parameters"].(bool))
	require.True(t, m["Unparsed Parameters"].(bool))
	a := m["AVPs"].([]map[string]any)[1]
	require.NotContains(t, a, "Physical Channel ID", "reserved type67 must not alias type25")
	require.Equal(t, []byte{1, 2, 3}, a["Unknown Value"])
	duplicate := l2tp3TestAVP(25, []byte{0, 0, 0, 1})
	m, err = DecodeL2TPv3Wire(l2tp3TestControl(6, false, duplicate, duplicate), false, 64)
	require.NoError(t, err)
	require.True(t, m["Ambiguous Parameters"].(bool))
	_, err = DecodeL2TPv3Wire(l2tp3TestControl(1, false), false, 64)
	require.Error(t, err, "two header fields cannot establish SCCRQ")
	_, err = DecodeL2TPv3Wire(l2tp3TestControl(7, false), false, 64)
	require.Error(t, err, "outgoing call also has mandatory session parameters")
}

func TestL2TPv3VLANHeaderIsCompleteAndBounded(t *testing.T) {
	w := append([]byte{0, 0, 0, 7}, make([]byte, 14)...)
	w[16], w[17] = 0x81, 0
	ctx := L2TPv3DataContext{Pseudowire: 4}
	m, err := DecodeL2TPv3Data(w, false, ctx, 1)
	require.Error(t, err)
	require.False(t, m["Payload Decoded"].(bool))
	w = append(w, 0, 7, 0x88, 0xa8)
	w = append(w, 0, 8, 0x08, 0)
	_, err = DecodeL2TPv3Data(w, false, ctx, 1)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	_, err = DecodeL2TPv3Data(w, false, ctx, 2)
	require.NoError(t, err)
}

func TestL2TPv3DataRequiresReceiverContext(t *testing.T) {
	ethernet := make([]byte, 34)
	copy(ethernet, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 0x08, 0x00, 0x45})
	for _, udp := range []bool{false, true} {
		prefix := []byte{0x12, 0x34, 0x56, 0x78}
		if udp {
			prefix = append([]byte{0, 3, 0, 0}, prefix...)
		}
		wire := append(bytes.Clone(prefix), []byte{0, 0, 0, 0, 0x40, 0xff, 0xff, 0xff}...)
		wire = append(wire, ethernet...)
		m, err := DecodeL2TPv3Wire(wire, udp, 64)
		require.NoError(t, err)
		require.False(t, m["Payload Decoded"].(bool))
		require.NotContains(t, m, "Ethernet Source")
		ctx := L2TPv3DataContext{Cookie: []byte{0, 0, 0, 0}, Sublayer: 1, Pseudowire: 5, Sequencing: 2}
		m, err = DecodeL2TPv3Data(wire, udp, ctx, 64)
		require.NoError(t, err)
		require.True(t, m["Payload Decoded"].(bool))
		require.Equal(t, uint64(0xffffff), m["Sequence Number"])
		require.Equal(t, "06:07:08:09:0a:0b", m["Ethernet Source"])
		require.Equal(t, []byte{0, 0, 0, 0}, m["Cookie"], "an all-zero four-byte cookie is not an absent cookie")
		ranges := m["Wire Field Ranges"].(map[string]any)
		require.Equal(t, []any{uint64(len(prefix)), uint64(len(prefix) + 4)}, ranges["Cookie"])
		wire[len(prefix)] = 1
		require.Equal(t, byte(0), m["Cookie"].([]byte)[0], "owned observation must survive capture-buffer reuse")
		m, err = DecodeL2TPv3Data(wire, udp, ctx, 64)
		require.Error(t, err)
		require.False(t, m["Payload Decoded"].(bool))
		require.Contains(t, m["Wire Field Ranges"].(map[string]any), "Cookie", "partial context preserves proven byte ranges")
		wire[len(prefix)] = 0
		ctx.Sequencing = 1
		_, err = DecodeL2TPv3Data(wire, udp, ctx, 64)
		require.Error(t, err, "level1 IP frames must not set S")
		wire[len(prefix)+4] = 0
		_, err = DecodeL2TPv3Data(wire, udp, ctx, 64)
		require.NoError(t, err)
		ctx.Sequencing = 2
		_, err = DecodeL2TPv3Data(wire, udp, ctx, 64)
		require.Error(t, err, "level2 requires S on every frame")
	}
}

func FuzzL2TPv3BoundedWire(f *testing.F) {
	f.Add(l2tp3TestControl(6, false), false)
	f.Add(l2tp3TestControl(0, true), true)
	f.Add([]byte{0x12, 0x34, 0x56, 0x78, 0, 0, 0, 0}, false)
	f.Fuzz(func(t *testing.T, w []byte, udp bool) {
		if len(w) > 65536 {
			return
		}
		m, err := DecodeL2TPv3Wire(w, udp, 32)
		if err == nil && m["Control"] != true {
			require.False(t, m["Payload Decoded"].(bool))
		}
		for _, c := range []L2TPv3DataContext{{Pseudowire: 5}, {Cookie: []byte{0, 0, 0, 0}, Pseudowire: 5, Sublayer: 1, Sequencing: 2}} {
			m, err = DecodeL2TPv3Data(w, udp, c, 32)
			if err == nil {
				require.True(t, m["Payload Decoded"].(bool))
				require.GreaterOrEqual(t, len(m["Ethernet Bytes"].([]byte)), 14)
			}
		}
	})
}
