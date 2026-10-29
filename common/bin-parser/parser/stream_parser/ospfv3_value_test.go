package stream_parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// This independent Fletcher insertion is also checked with literal zero-sum
// residues below. LS Age is deliberately excluded from both residue checks.
func ospfv3TestLSA(body []byte, typ uint16) []byte {
	w := make([]byte, 20)
	binary.BigEndian.PutUint16(w[2:], typ)
	w[7], w[11], w[12], w[15] = 1, 2, 0x80, 1
	w = append(w, body...)
	binary.BigEndian.PutUint16(w[18:], uint16(len(w)))
	c0, c1 := 0, 0
	for _, b := range w[2:] {
		c0 = (c0 + int(b)) % 255
		c1 = (c1 + c0) % 255
	}
	x := ((len(w)-17)*c0 - c1) % 255
	if x <= 0 {
		x += 255
	}
	y := 510 - c0 - x
	if y > 255 {
		y -= 255
	}
	w[16], w[17] = byte(x), byte(y)
	return w
}

func ospfv3TestPacket(typ byte, body []byte) []byte {
	w := append([]byte{3, typ, 0, 0, 192, 0, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0}, body...)
	binary.BigEndian.PutUint16(w[2:], uint16(len(w)))
	return w
}

func TestOSPFv3PrefixWordWidthsAndIndependentLSAs(t *testing.T) {
	// Literal bit/word pairs straddle every word boundary and include /0.
	bits := []byte{0, 1, 31, 32, 33, 64, 65, 127, 128}
	widths := []int{0, 4, 4, 4, 8, 8, 12, 16, 16}
	body := []byte{0, 9, 0x20, 1, 0, 0, 0, 1, 192, 0, 2, 2}
	var wantStarts []int
	for i, n := range bits {
		wantStarts = append(wantStarts, 20+20+len(body)+4)
		body = append(body, n, 0, 0, byte(i))
		body = append(body, bytes.Repeat([]byte{0xab}, widths[i])...)
	}
	lsa := ospfv3TestLSA(body, 0x2009)
	require.True(t, ospfv3LSAChecksum(lsa))
	w := ospfv3TestPacket(4, append([]byte{0, 0, 0, 2}, append(bytes.Clone(lsa), lsa...)...))
	m, err := DecodeOSPFv3Wire(w, OSPFv3WireContext{}, 64)
	require.NoError(t, err)
	rows := m["LSAs"].([]map[string]any)
	require.Len(t, rows, 2)
	for j, row := range rows {
		require.Equal(t, true, row["Checksum Verified"])
		prefixes := row["Body"].(map[string]any)["Prefixes"].([]map[string]any)
		require.Len(t, prefixes, len(bits))
		for i, p := range prefixes {
			require.Equal(t, uint64(bits[i]), p["Prefix Length"])
			require.Equal(t, uint64(widths[i]), p["Encoded Prefix Bytes"])
			require.Len(t, p["Encoded Prefix"], widths[i])
			span := p["Wire Field Ranges"].(map[string]any)["Encoded Prefix"]
			require.Equal(t, []any{uint64(wantStarts[i] + j*len(lsa)), uint64(wantStarts[i] + j*len(lsa) + widths[i])}, span)
		}
	}
	before := bytes.Clone(w)
	first := rows[0]["Body"].(map[string]any)["Prefixes"].([]map[string]any)[1]
	first["Encoded Prefix"].([]byte)[0] ^= 1
	require.Equal(t, before, w, "returned leaves cannot alter original PDU")
	// A same-ID later advertisement supplies its own metrics and prefixes.
	changed := ospfv3TestLSA([]byte{0, 0, 0x20, 1, 0, 0, 0, 1, 192, 0, 2, 2}, 0x2009)
	fresh, err := DecodeOSPFv3Wire(ospfv3TestPacket(4, append([]byte{0, 0, 0, 1}, changed...)), OSPFv3WireContext{}, 32)
	require.NoError(t, err)
	require.Empty(t, fresh["LSAs"].([]map[string]any)[0]["Body"].(map[string]any)["Prefixes"])
}

func TestOSPFv3TruncationHeaderOnlyChecksumAndBudgets(t *testing.T) {
	body := []byte{0, 1, 0x20, 1, 0, 0, 0, 1, 192, 0, 2, 2, 64, 0, 0, 7, 0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0}
	lsa := ospfv3TestLSA(body, 0x2009)
	w := ospfv3TestPacket(4, append([]byte{0, 0, 0, 1}, lsa...))
	for cut := 0; cut < len(w); cut++ {
		_, err := DecodeOSPFv3Wire(w[:cut], OSPFv3WireContext{}, 32)
		require.Error(t, err, "cut %d", cut)
	}
	partial, err := DecodeOSPFv3Wire(w[:len(w)-1], OSPFv3WireContext{}, 32)
	require.Error(t, err)
	row := partial["LSAs"].([]map[string]any)[0]
	require.Equal(t, false, row["Checksum Verified"])
	p := row["Body"].(map[string]any)["Prefixes"].([]map[string]any)[0]
	require.Equal(t, uint64(64), p["Prefix Length"])
	require.NotContains(t, p, "Prefix", "partial wire prefix is never a complete address")
	// DB summaries/acks carry only20-byte LSA headers. Their advertised
	// LSA Length is not an available body and cannot validate its checksum.
	ack, err := DecodeOSPFv3Wire(ospfv3TestPacket(5, lsa[:20]), OSPFv3WireContext{}, 32)
	require.NoError(t, err)
	require.Equal(t, false, ack["LSA Headers"].([]map[string]any)[0]["Checksum Verified"])
	_, err = DecodeOSPFv3Wire(w, OSPFv3WireContext{}, 1)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	bad := bytes.Clone(w)
	bad[len(bad)-1] ^= 1
	_, err = DecodeOSPFv3Wire(bad, OSPFv3WireContext{}, 32)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrObservedWireContext))
}

func TestOSPFv3UnknownLSAAndTrailerStayExplicit(t *testing.T) {
	unknown := ospfv3TestLSA([]byte{1, 2, 3, 4}, 0xa123)
	m, err := DecodeOSPFv3Wire(ospfv3TestPacket(4, append([]byte{0, 0, 0, 1}, unknown...)), OSPFv3WireContext{}, 32)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.Equal(t, []byte{1, 2, 3, 4}, m["LSAs"].([]map[string]any)[0]["Body"].(map[string]any)["Opaque Body"])
	hello := make([]byte, 20)
	hello[6] = 4 // AT bit0x400 in the24-bit Options.
	w := ospfv3TestPacket(1, hello)
	w[12], w[13] = 0x12, 0x34 // RFC7166 says ignore it, even when nonzero.
	m, err = DecodeOSPFv3Wire(w, OSPFv3WireContext{}, 32)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.Equal(t, false, m["Checksum Verified"])
	require.Equal(t, false, m["Authentication Verified"])
	_, err = DecodeOSPFv3Wire(append(ospfv3TestPacket(1, make([]byte, 20)), 0, 0, 0, 0), OSPFv3WireContext{}, 32)
	require.ErrorIs(t, err, ErrObservedWireContext)
}

func TestOSPFv3ExternalTruncationCannotShiftPrefixIntoMetric(t *testing.T) {
	// All three bytes of the metric are required before any prefix header.
	for _, typ := range []uint16{0x2003, 0x4005, 0x2007} {
		for _, body := range [][]byte{{0}, {0, 64}, {0, 64, 0}} {
			lsa := ospfv3TestLSA(body, typ)
			m, err := DecodeOSPFv3Wire(ospfv3TestPacket(4, append([]byte{0, 0, 0, 1}, lsa...)), OSPFv3WireContext{}, 32)
			require.Error(t, err)
			r := m["LSAs"].([]map[string]any)[0]["Body"].(map[string]any)
			require.NotContains(t, r, "Prefixes")
		}
	}
	w := ospfv3TestPacket(1, make([]byte, 20))
	w[14] = 64
	m, err := DecodeOSPFv3Wire(w, OSPFv3WireContext{}, 32)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.NotContains(t, m, "Hello", "unselected instance must not borrow base IPv6 interpretation")
}

func TestOSPFv3InterAreaAndExternalOptionalFieldBoundaries(t *testing.T) {
	for _, c := range []struct {
		typ    uint16
		body   []byte
		metric uint64
	}{
		{0x2003, []byte{0, 0, 0, 7, 64, 0, 0, 0, 0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0}, 7},
		{0x2004, []byte{0, 0, 0, 0x13, 0, 0, 0, 8, 192, 0, 2, 99}, 8},
		{0x4005, []byte{3, 0, 0, 9, 64, 0, 0x20, 1, 0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 5, 1, 2, 3, 4, 192, 0, 2, 7}, 9},
		{0x2007, []byte{0, 0, 0, 10, 0, 0, 0, 0}, 10},
	} {
		lsa := ospfv3TestLSA(c.body, c.typ)
		w := ospfv3TestPacket(4, append([]byte{0, 0, 0, 1}, lsa...))
		m, err := DecodeOSPFv3Wire(w, OSPFv3WireContext{}, 32)
		require.NoError(t, err)
		body := m["LSAs"].([]map[string]any)[0]["Body"].(map[string]any)
		require.Equal(t, c.metric, body["Metric"])
		if c.typ == 0x2004 {
			require.Equal(t, "192.0.2.99", body["Destination Router"])
		}
		if c.typ == 0x4005 {
			require.Equal(t, "2001:db8::5", body["Forwarding Address"])
			require.Equal(t, uint64(0x01020304), body["External Route Tag"])
			require.Equal(t, "192.0.2.7", body["Referenced Link State ID"])
		}
		for cut := 20; cut < len(w); cut++ {
			_, err = DecodeOSPFv3Wire(w[:cut], OSPFv3WireContext{}, 32)
			require.Error(t, err, "LSA%04x cut%d", c.typ, cut)
		}
	}
}

func ospfv3TestContext() OSPFv3WireContext {
	return OSPFv3WireContext{Source: netip.MustParseAddr("fe80::1"), Destination: netip.MustParseAddr("ff02::5")}
}

// Independent one's-complement insertion: do not use the candidate checksum
// helper to manufacture its own expected checksum.
func ospfv3TestSign(w []byte) []byte {
	w = bytes.Clone(w)
	w[12], w[13] = 0, 0
	c := ospfv3TestContext()
	pseudo := append(c.Source.AsSlice(), c.Destination.AsSlice()...)
	n := int(binary.BigEndian.Uint16(w[2:4]))
	pseudo = append(pseudo, 0, 0, byte(n>>8), byte(n), 0, 0, 0, 89)
	pseudo = append(pseudo, w[:n]...)
	if len(pseudo)%2 != 0 {
		pseudo = append(pseudo, 0)
	}
	sum := uint32(0)
	for at := 0; at < len(pseudo); at += 2 {
		sum += uint32(pseudo[at])<<8 | uint32(pseudo[at+1])
	}
	for sum > 65535 {
		sum = sum&65535 + sum>>16
	}
	binary.BigEndian.PutUint16(w[12:14], ^uint16(sum))
	return w
}

func TestOSPFv3UnsupportedLSADoesNotHideBoundedSiblingOrIntegrity(t *testing.T) {
	unknown := ospfv3TestLSA([]byte{3, 4, 0, 24}, 0xa123)
	known := ospfv3TestLSA([]byte{0, 0, 0, 0x13, 192, 0, 2, 9}, 0x2002)
	w := ospfv3TestSign(ospfv3TestPacket(4, append([]byte{0, 0, 0, 2}, append(bytes.Clone(unknown), known...)...)))
	m, err := DecodeOSPFv3Wire(w, ospfv3TestContext(), 32)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.Equal(t, true, m["Checksum Verified"])
	rows := m["LSAs"].([]map[string]any)
	require.Len(t, rows, 2)
	require.Equal(t, unknown[20:], rows[0]["Body"].(map[string]any)["Opaque Body"])
	for _, row := range rows {
		require.Equal(t, true, row["Checksum Verified"])
	}
	router := rows[1]["Body"].(map[string]any)["Attached Routers"].([]map[string]any)[0]
	require.Equal(t, "192.0.2.9", router["Attached Router"])
	require.Equal(t, []any{uint64(68), uint64(72)}, router["Wire Field Ranges"].(map[string]any)["Attached Router"])
	require.NotContains(t, m, "Unparsed Body")
	badPacket := bytes.Clone(w)
	badPacket[12] ^= 1
	m, err = DecodeOSPFv3Wire(badPacket, ospfv3TestContext(), 32)
	require.ErrorContains(t, err, "checksum failed")
	require.False(t, errors.Is(err, ErrObservedWireContext), "a proven bad sum outranks soft unsupported semantics")
	require.Equal(t, false, m["Checksum Verified"])
	require.Len(t, m["LSAs"], 2, "bad packet integrity does not fabricate missing wire observations")
	badLSA := bytes.Clone(w)
	badLSA[43] ^= 1
	badLSA = ospfv3TestSign(badLSA)
	m, err = DecodeOSPFv3Wire(badLSA, ospfv3TestContext(), 32)
	require.ErrorContains(t, err, "Fletcher checksum failed")
	require.False(t, errors.Is(err, ErrObservedWireContext))
	require.Equal(t, true, m["Checksum Verified"], "packet and LSA checksums have different scopes")
	require.Equal(t, false, m["LSAs"].([]map[string]any)[0]["Checksum Verified"])
	require.Len(t, m["LSAs"], 1, "hard LSA integrity failure stops the body parser")
	for _, packet := range [][]byte{w, badPacket} {
		m, err = DecodeOSPFv3Wire(packet, ospfv3TestContext(), 1)
		require.ErrorIs(t, err, ErrObservedWireBudget)
		require.Equal(t, packet[12] == w[12], m["Checksum Verified"])
		if packet[12] != w[12] {
			require.ErrorContains(t, err, "checksum failed")
			require.ErrorIs(t, err, ErrOSPFv3WireIntegrity)
		} else {
			require.False(t, errors.Is(err, ErrOSPFv3WireIntegrity))
		}
	}
	// The declared envelope, not a plausible later header, owns the bytes.
	truncated := bytes.Clone(w)
	truncated[38], truncated[39] = 0xff, 0xff
	truncated = ospfv3TestSign(truncated)
	m, err = DecodeOSPFv3Wire(truncated, ospfv3TestContext(), 32)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrObservedWireContext))
	require.Len(t, m["LSAs"], 1)
}

func TestOSPFv3IndependentPacketChecksumContexts(t *testing.T) {
	hello := make([]byte, 20)
	hello[6] = 2 // LLS-only, AT clear. The LLS block is outside Packet Length.
	for _, tail := range [][]byte{{0xff, 0xfe, 0, 1}, {0, 0, 0, 1}} {
		w := append(ospfv3TestSign(ospfv3TestPacket(1, hello)), tail...)
		m, err := DecodeOSPFv3Wire(w, ospfv3TestContext(), 32)
		require.ErrorIs(t, err, ErrObservedWireContext, "LLS semantics are not decoded")
		require.Equal(t, true, m["Checksum Verified"], "LLS checksum cannot change the OSPF packet sum")
		require.Equal(t, true, m["LLS Bit"])
		require.Equal(t, false, m["Authentication Trailer Bit"])
		require.Contains(t, m["LLS Scope"], "unverified")
		require.Contains(t, m, "Hello")
	}
	hello[6] = 6 // AT+LLS: even a nonzero packet checksum must be ignored.
	w := ospfv3TestPacket(1, hello)
	w[12], w[13] = 0x12, 0x34
	m, err := DecodeOSPFv3Wire(append(w, 0, 0, 0, 1), ospfv3TestContext(), 32)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.Equal(t, false, m["Checksum Verified"])
	require.True(t, strings.HasPrefix(m["Checksum Scope"].(string), "omitted:"))
	short := ospfv3TestSign(ospfv3TestPacket(1, hello[:6]))
	m, err = DecodeOSPFv3Wire(short, ospfv3TestContext(), 32)
	require.Error(t, err)
	require.Contains(t, m["Checksum Scope"], "Options unavailable")
	require.Equal(t, false, m["Checksum Verified"])
	unknownAF := ospfv3TestPacket(3, nil)
	unknownAF[14] = 64
	m, err = DecodeOSPFv3Wire(ospfv3TestSign(unknownAF), ospfv3TestContext(), 32)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.Equal(t, true, m["Checksum Verified"], "AF body semantics do not change the IPv6 pseudo-header")
	cut := ospfv3TestSign(ospfv3TestPacket(1, make([]byte, 20)))
	cut[14] = 64
	m, err = DecodeOSPFv3Wire(cut[:len(cut)-1], ospfv3TestContext(), 32)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrObservedWireContext), "declared truncation outranks unknown AF")
	require.Contains(t, m["Checksum Scope"], "not completely captured")
	context := ospfv3TestContext()
	context.PseudoHeaderUnavailable = true
	m, err = DecodeOSPFv3Wire(ospfv3TestSign(unknownAF), context, 32)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.Equal(t, false, m["Checksum Verified"])
	require.Contains(t, m["Checksum Scope"], "final IPv6 pseudo-header destination unavailable")
}

func TestOSPFv3ShortLSAHeaderRetainsOnlyCompleteScalarPrefix(t *testing.T) {
	unknown := ospfv3TestLSA([]byte{1, 2, 3, 4}, 0xa123)
	known := ospfv3TestLSA([]byte{0, 0, 0, 0x13, 192, 0, 2, 9}, 0x2002)
	w := ospfv3TestSign(ospfv3TestPacket(4, append([]byte{0, 0, 0, 2}, append(bytes.Clone(unknown), known...)...)))
	for captured := 0; captured < 20; captured++ {
		m, err := DecodeOSPFv3Wire(w[:44+captured], ospfv3TestContext(), 32)
		require.Error(t, err)
		rows := m["LSAs"].([]map[string]any)
		if captured < 2 {
			require.Len(t, rows, 1, "no observation slot without a complete scalar")
			continue
		}
		require.Len(t, rows, 2)
		row := rows[1]
		require.NotNil(t, row)
		require.NotContains(t, row, "Body")
		require.NotContains(t, row, "Length", "uncaptured Length cannot license continuation")
		require.Equal(t, false, row["Checksum Verified"])
		for _, f := range []struct {
			name     string
			at, size int
		}{
			{"LS Age", 0, 2}, {"LS Type", 2, 2}, {"Link State ID", 4, 4},
			{"Advertising Router", 8, 4}, {"Sequence", 12, 4}, {"Checksum", 16, 2},
		} {
			if captured >= f.at+f.size {
				require.Contains(t, row, f.name)
				require.Equal(t, []any{uint64(44 + f.at), uint64(44 + f.at + f.size)}, row["Wire Field Ranges"].(map[string]any)[f.name])
			} else {
				require.NotContains(t, row, f.name)
			}
		}
	}
	// Exhaustion before the first prefix cannot invent a nil prefix child.
	link := ospfv3TestLSA(append(make([]byte, 23), 1, 64, 0, 0, 0), 8)
	m, err := DecodeOSPFv3Wire(ospfv3TestPacket(4, append([]byte{0, 0, 0, 1}, link...)), OSPFv3WireContext{}, 1)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	body := m["LSAs"].([]map[string]any)[0]["Body"].(map[string]any)
	require.Empty(t, body["Prefixes"])
}

func FuzzOSPFv3BoundedOccurrences(f *testing.F) {
	f.Add(ospfv3TestPacket(1, make([]byte, 20)), uint8(32))
	f.Add(ospfv3TestPacket(4, []byte{0, 0, 0, 0}), uint8(1))
	unknown := ospfv3TestLSA([]byte{1, 2, 3, 4}, 0xa123)
	known := ospfv3TestLSA([]byte{0, 0, 0, 0x13}, 0x2001)
	f.Add(ospfv3TestSign(ospfv3TestPacket(4, append([]byte{0, 0, 0, 2}, append(bytes.Clone(unknown), known...)...))), uint8(8))
	f.Fuzz(func(t *testing.T, w []byte, b uint8) {
		if len(w) > 65536 {
			return
		}
		original := bytes.Clone(w)
		var walk func(any)
		walk = func(v any) {
			switch x := v.(type) {
			case map[string]any:
				if ranges, ok := x["Wire Field Ranges"].(map[string]any); ok {
					for _, v := range ranges {
						p := v.([]any)
						require.LessOrEqual(t, p[0].(uint64), p[1].(uint64))
						require.LessOrEqual(t, p[1].(uint64), uint64(len(w)))
					}
				}
				for k, v := range x {
					if k != "Wire Field Ranges" {
						walk(v)
					}
				}
			case []map[string]any:
				for _, v := range x {
					require.NotNil(t, v, "a refused child is not an observed occurrence")
					walk(v)
				}
			}
		}
		for _, context := range []OSPFv3WireContext{{}, ospfv3TestContext()} {
			m, _ := DecodeOSPFv3Wire(w, context, int(b%64)+1)
			require.Equal(t, original, w)
			walk(m)
		}
	})
}
