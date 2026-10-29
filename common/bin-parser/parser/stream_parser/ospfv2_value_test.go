package stream_parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Independent one's-complement insertion: packet authentication bytes and all
// appended data are deliberately excluded. Zero is a valid packet checksum.
func ospfv2TestSign(w []byte) []byte {
	w = bytes.Clone(w)
	w[12], w[13] = 0, 0
	n := int(binary.BigEndian.Uint16(w[2:4]))
	var sum uint32
	for at := 0; at < n; at += 2 {
		if at >= 16 && at < 24 {
			continue
		}
		sum += uint32(w[at]) << 8
		if at+1 < n {
			sum += uint32(w[at+1])
		}
	}
	for sum > 65535 {
		sum = sum>>16 + sum&65535
	}
	binary.BigEndian.PutUint16(w[12:], ^uint16(sum))
	return w
}

func ospfv2TestPacket(typ byte, body []byte) []byte {
	w := append([]byte{2, typ, 0, 0, 192, 0, 2, 1, 0, 0, 0, 0, 0, 0, 17, 0, 0, 0, 0, 0, 0, 0, 0, 0}, body...)
	binary.BigEndian.PutUint16(w[2:], uint16(len(w)))
	return ospfv2TestSign(w)
}

func ospfv2TestHello(lls bool, neighbors ...byte) []byte {
	b := []byte{255, 255, 255, 0, 0, 10, 2, 1, 0, 0, 0, 40, 192, 0, 2, 1, 0, 0, 0, 0}
	if lls {
		b[6] |= 0x10
	}
	for _, n := range neighbors {
		b = append(b, 192, 0, 2, n)
	}
	return ospfv2TestPacket(1, b)
}

func ospfv2TestLLS() []byte {
	// 12-byte non-cryptographic block: one's-complement sum is exactlyffff.
	return []byte{0xff, 0xf6, 0, 3, 0, 1, 0, 4, 0, 0, 0, 1}
}

func ospfv2TestCryptographic() []byte {
	w := ospfv2TestHello(true)
	w[12], w[13], w[15], w[18], w[19], w[23] = 0, 0, 2, 7, 16, 9
	w = append(w, bytes.Repeat([]byte{0x21}, 16)...)
	// EOF followed by CA(sequence9, opaque16-byte authentication data).
	w = append(w, 0, 0, 0, 9, 0, 1, 0, 4, 0, 0, 0, 1, 0, 2, 0, 20, 0, 0, 0, 9)
	return append(w, bytes.Repeat([]byte{0x42}, 16)...)
}

func ospfv2TestLSA(typ byte, body []byte) []byte {
	// The independently inserted Fletcher pair uses the same RFC2328 header
	// offsets as v3; the literal v2 type lives in octet3, not a16-bit LS Type.
	w := ospfv3TestLSA(body, uint16(0x0200)|uint16(typ))
	return w
}

func TestOSPFv2AllFiveEnvelopesAndHeaderOnlyIntegrity(t *testing.T) {
	lsa := ospfv2TestLSA(2, []byte{255, 255, 255, 0, 192, 0, 2, 1})
	for typ, body := range map[byte][]byte{
		1: ospfv2TestHello(false)[24:],
		2: append([]byte{5, 220, 2, 7, 0, 0, 0, 9}, lsa[:20]...),
		3: {0, 0, 0, 5, 192, 0, 2, 2, 192, 0, 2, 1},
		4: append([]byte{0, 0, 0, 1}, lsa...), 5: lsa[:20],
	} {
		t.Run(fmt.Sprint(typ), func(t *testing.T) {
			w := ospfv2TestPacket(typ, body)
			m, err := DecodeOSPFv2Wire(w, 64)
			require.NoError(t, err)
			require.Equal(t, true, m["Checksum Verified"])
			require.Equal(t, false, m["Authentication Verified"])
			h := m["Header"].(map[string]any)
			require.Equal(t, uint64(17), h["Instance ID"])
			require.Equal(t, uint64(0), h["Authentication Type"])
			require.Equal(t, []any{uint64(14), uint64(15)}, h["Wire Field Ranges"].(map[string]any)["Instance ID"])
			if typ == 4 {
				require.Equal(t, true, m["LSAs"].([]map[string]any)[0]["Checksum Verified"])
			}
			if typ == 5 {
				require.Equal(t, false, m["LSA Headers"].([]map[string]any)[0]["Checksum Verified"])
			}
			for cut := 0; cut < len(w); cut++ {
				_, err = DecodeOSPFv2Wire(w[:cut], 64)
				require.Error(t, err, "cut%d", cut)
			}
		})
	}
}

func TestOSPFv2IndependentLLSAndBudgetErrors(t *testing.T) {
	w := append(ospfv2TestHello(true, 2, 3), ospfv2TestLLS()...)
	m, err := DecodeOSPFv2Wire(w, 64)
	require.NoError(t, err)
	require.Equal(t, true, m["LLS"].(map[string]any)["Checksum Verified"])
	bad := bytes.Clone(w)
	bad[len(bad)-1] ^= 1
	m, err = DecodeOSPFv2Wire(bad, 1)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	require.ErrorIs(t, err, ErrOSPFv2WireIntegrity)
	require.Equal(t, true, m["Checksum Verified"], "LLS lies outside the packet checksum extent")
	require.Equal(t, false, m["LLS"].(map[string]any)["Checksum Verified"])
	// Unknown TLV length1 has three padding octets. Padding belongs to LLS,
	// and cannot be interpreted as another TLV or another OSPF packet.
	lls := []byte{0, 0, 0, 3, 0x12, 0x34, 0, 1, 0x55, 0xaa, 0xbb, 0xcc}
	binary.BigEndian.PutUint16(lls, ospfv2Sum(lls))
	m, err = DecodeOSPFv2Wire(append(ospfv2TestHello(true), lls...), 64)
	require.NoError(t, err)
	tlv := m["LLS"].(map[string]any)["TLVs"].([]map[string]any)[0]
	require.Equal(t, []byte{0xaa, 0xbb, 0xcc}, tlv["Padding"])
	require.Equal(t, uint64(0x1234), tlv["TLV Type"])
	require.NotContains(t, tlv, "Type")
}

func TestOSPFv2CryptoDigestThenLLSAndNoInventedAuthentication(t *testing.T) {
	w := ospfv2TestCryptographic()
	m, err := DecodeOSPFv2Wire(w, 64)
	require.NoError(t, err)
	a := m["Authentication"].(map[string]any)
	require.Equal(t, bytes.Repeat([]byte{0x21}, 16), a["Authentication Digest"])
	require.Equal(t, []any{uint64(44), uint64(60)}, a["Wire Field Ranges"].(map[string]any)["Authentication Digest"])
	require.Equal(t, false, m["Checksum Verified"])
	require.Contains(t, m["Checksum Scope"], "omitted")
	lls := m["LLS"].(map[string]any)
	require.Equal(t, false, lls["Authentication Verified"])
	require.Equal(t, uint64(9), lls["Length Words"])
	ca := lls["TLVs"].([]map[string]any)[1]
	require.Equal(t, true, ca["Sequence Matches OSPF Header"])
	require.Equal(t, true, ca["Authentication Data Length Matches Header"])
	for _, at := range []int{12, 60, 79} {
		bad := bytes.Clone(w)
		bad[at] ^= 1
		_, err := DecodeOSPFv2Wire(bad, 64)
		require.ErrorIs(t, err, ErrOSPFv2WireFormat, "at%d", at)
	}
	noCA := append(bytes.Clone(w[:60]), ospfv2TestLLS()...)
	noCA[60], noCA[61], noCA[63] = 0, 0, 5
	noCA = append(noCA, 0x12, 0x34, 0, 4, 1, 2, 3, 4)
	_, err = DecodeOSPFv2Wire(noCA, 64)
	require.ErrorIs(t, err, ErrOSPFv2WireFormat)
	_, err = DecodeOSPFv2Wire(noCA, 1)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	require.NotContains(t, err.Error(), "lacks CA", "resource exhaustion cannot establish absence")
	unknown := bytes.Clone(w)
	unknown[15] = 3
	m, err = DecodeOSPFv2Wire(unknown, 64)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.NotContains(t, m, "LLS", "unknown authentication cannot supply a digest boundary")
	require.Equal(t, w[44:], m["Trailing Bytes"])
}

func TestOSPFv2ConclusiveLengthsAndPartialFields(t *testing.T) {
	w := ospfv2TestHello(false)
	for cut := 4; cut < 24; cut++ {
		bad := bytes.Clone(w[:cut])
		bad[2], bad[3] = 0, 0
		m, err := DecodeOSPFv2Wire(bad, 64)
		require.ErrorIs(t, err, ErrOSPFv2WireFormat, "cut%d", cut)
		require.Equal(t, uint64(0), m["Header"].(map[string]any)["Packet Length"])
		_, err = DecodeOSPFv2Wire(w[:cut], 64)
		require.Error(t, err)
		require.False(t, errors.Is(err, ErrOSPFv2WireFormat), "a missing captured suffix is not a proven contradiction")
	}
	lsa := ospfv2TestLSA(2, []byte{255, 255, 255, 0, 192, 0, 2, 1})
	full := ospfv2TestPacket(4, append([]byte{0, 0, 0, 1}, lsa...))
	for _, n := range []int{12, 18} {
		m, err := DecodeOSPFv2Wire(full[:28+n], 64)
		require.Error(t, err)
		row := m["LSAs"].([]map[string]any)[0]
		require.Contains(t, row, "Advertising Router")
		require.NotContains(t, row, "Body")
		require.NotContains(t, row, "Length")
		require.Equal(t, false, row["Checksum Verified"])
	}
}

func TestOSPFv2UnknownLSASiblingAndIndependentDigest(t *testing.T) {
	unknown := ospfv2TestLSA(99, []byte{1, 2, 3, 4})
	known := ospfv2TestLSA(2, []byte{255, 255, 255, 0, 192, 0, 2, 1})
	w := ospfv2TestPacket(4, append([]byte{0, 0, 0, 2}, append(unknown, known...)...))
	m, err := DecodeOSPFv2Wire(w, 64)
	require.ErrorIs(t, err, ErrObservedWireContext)
	rows := m["LSAs"].([]map[string]any)
	require.Len(t, rows, 2)
	require.Equal(t, uint64(2), rows[1]["LS Type"])
	require.Equal(t, true, rows[1]["Checksum Verified"])
	// A fully captured digest remains visible after an independent bad LSA.
	w = ospfv2TestPacket(4, append([]byte{0, 0, 0, 1}, known...))
	w[12], w[13], w[15], w[19], w[23] = 0, 0, 2, 16, 9
	w[49] ^= 1
	w = append(w, bytes.Repeat([]byte{0x77}, 16)...)
	m, err = DecodeOSPFv2Wire(w, 64)
	require.ErrorIs(t, err, ErrOSPFv2WireIntegrity)
	require.Equal(t, bytes.Repeat([]byte{0x77}, 16), m["Authentication"].(map[string]any)["Authentication Digest"])
	maskOnly := ospfv2TestLSA(2, []byte{255, 255, 255, 0})
	_, err = DecodeOSPFv2Wire(ospfv2TestPacket(4, append([]byte{0, 0, 0, 1}, maskOnly...)), 64)
	require.ErrorIs(t, err, ErrOSPFv2WireFormat)
}

func FuzzOSPFv2BoundedOccurrences(f *testing.F) {
	f.Add(ospfv2TestCryptographic(), uint8(64))
	f.Add(append(ospfv2TestHello(true, 2, 3), ospfv2TestLLS()...), uint8(1))
	f.Fuzz(func(t *testing.T, wire []byte, count uint8) {
		if len(wire) > 65535 {
			return
		}
		original := bytes.Clone(wire)
		fields, _ := DecodeOSPFv2Wire(wire, int(count)+1)
		require.Equal(t, original, wire)
		var walk func(any)
		walk = func(v any) {
			switch v := v.(type) {
			case map[string]any:
				if spans, ok := v["Wire Field Ranges"].(map[string]any); ok {
					for _, span := range spans {
						s := span.([]any)
						require.LessOrEqual(t, s[0].(uint64), s[1].(uint64))
						require.LessOrEqual(t, s[1].(uint64), uint64(len(wire)))
					}
				}
				for k, child := range v {
					if k != "Wire Field Ranges" {
						walk(child)
					}
				}
			case []map[string]any:
				for _, child := range v {
					require.NotNil(t, child)
					walk(child)
				}
			}
		}
		walk(fields)
	})
}
