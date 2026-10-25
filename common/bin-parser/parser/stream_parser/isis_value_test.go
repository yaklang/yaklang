package stream_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func isisWireVector(t testing.TB, index int) []byte {
	t.Helper()
	v := []string{
		"831b01000f01000001010001000100001e001b0101000100010001",
		"831b01001001000002010001000100001e001b0101000100010001",
		"831401001101000003010001000100001e001401",
		"831b010012010000001b04b0010001000100000000000001dd1d01",
		"831b010014010000001b04b0010001000100000000000001df1903",
		"83210100180100000021010001000100000000000000000000ffffffffffffffff",
		"83210100190100000021010001000100000000000000000000ffffffffffffffff",
		"831101001a010000001101000100010000",
		"831101001b010000001101000100010000",
	}
	w, err := hex.DecodeString(v[index])
	require.NoError(t, err)
	return w
}

func isisWireAdd(t testing.TB, w []byte, values ...[]byte) []byte {
	t.Helper()
	w = bytes.Clone(w)
	for _, v := range values {
		w = append(w, v...)
	}
	at := 8
	if w[4]&31 == 15 || w[4]&31 == 16 || w[4]&31 == 17 {
		at = 17
	}
	binary.BigEndian.PutUint16(w[at:], uint16(len(w)))
	if w[4]&31 == 18 || w[4]&31 == 20 {
		isisWireSetChecksum(w, 12, 24)
	}
	return w
}

// An independent checksum construction formula; decoding verifies residues.
func isisWireSetChecksum(w []byte, start, check int) {
	w[check], w[check+1] = 0, 0
	a, b := 0, 0
	for _, v := range w[start:] {
		a = (a + int(v)) % 255
		b = (b + a) % 255
	}
	x := ((len(w)-check-1)*a - b) % 255
	if x <= 0 {
		x += 255
	}
	y := 510 - a - x
	if y > 255 {
		y -= 255
	}
	w[check], w[check+1] = byte(x), byte(y)
}

func TestISISWireAllFixedHeadersAndBoundaries(t *testing.T) {
	for i := 0; i < 9; i++ {
		w := isisWireVector(t, i)
		m, err := DecodeISISWire(w, 64)
		require.NoError(t, err)
		require.EqualValues(t, len(w), m["PDU Length"])
		require.EqualValues(t, w[4]&31, m["PDU Type"])
		require.Equal(t, false, m["Authentication Verified"])
		for cut := 0; cut < len(w); cut++ {
			_, err := DecodeISISWire(w[:cut], 64)
			require.Error(t, err)
		}
		_, err = DecodeISISWire(append(bytes.Clone(w), 0), 64)
		require.Error(t, err)
	}
}

func TestISISWireChecksumAndPurgeScopes(t *testing.T) {
	w := isisWireVector(t, 3)
	m, err := DecodeISISWire(w, 64)
	require.NoError(t, err)
	require.Equal(t, true, m["Checksum Verified"])
	w[13] ^= 1
	m, err = DecodeISISWire(w, 64)
	require.Error(t, err)
	require.NotNil(t, m, "known fields remain available with an explicit complete-message integrity failure")
	require.Equal(t, false, m["Checksum Verified"])
	require.EqualValues(t, binary.BigEndian.Uint16(w[24:]), m["Wire Checksum"])
	w[10], w[11] = 0, 0
	m, err = DecodeISISWire(w, 64)
	require.NoError(t, err)
	require.Equal(t, false, m["Checksum Verified"], "purge must not claim a verified checksum")
	require.EqualValues(t, binary.BigEndian.Uint16(w[24:]), m["Wire Checksum"], "purge must not replace captured checksum octets")
	hello := isisWireAdd(t, isisWireVector(t, 2), []byte{12, 2, 0, 0})
	m, err = DecodeISISWire(hello, 64)
	require.NoError(t, err)
	row := m["TLVs"].([]map[string]any)[0]
	require.Equal(t, true, row["Checksum Accepted"])
	require.Equal(t, false, row["Checksum Verified"])
	isisWireSetChecksum(hello, 0, len(hello)-2)
	m, err = DecodeISISWire(hello, 64)
	require.NoError(t, err)
	require.Equal(t, true, m["TLVs"].([]map[string]any)[0]["Checksum Verified"])
	hello[len(hello)-1] ^= 1
	m, err = DecodeISISWire(hello, 64)
	require.Error(t, err)
	require.Equal(t, true, m["Optional Checksum Failure"])
	_, err = DecodeISISWire(isisWireAdd(t, isisWireVector(t, 2), []byte{12, 2, 0, 0, 12, 2, 0, 0}), 64)
	require.Error(t, err)
	_, err = DecodeISISWire(isisWireAdd(t, isisWireVector(t, 3), []byte{12, 2, 0, 0}), 64)
	require.Error(t, err, "optional checksum is not selected in LSPs")
}

func TestISISWireUnknownContextBudgetAndNoInheritance(t *testing.T) {
	w := isisWireAdd(t, isisWireVector(t, 2), []byte{240, 15, 0, 0, 0, 0, 7, 1, 2, 3, 4, 5, 6, 0, 0, 0, 8})
	m, err := DecodeISISWire(w, 64)
	require.NoError(t, err)
	rows := m["TLVs"].([]map[string]any)
	require.EqualValues(t, 8, rows[0]["Neighbor Extended Local Circuit ID"])
	clear(w)
	require.Equal(t, "0102.0304.0506", rows[0]["Neighbor System ID"])
	next, err := DecodeISISWire(isisWireVector(t, 2), 64)
	require.NoError(t, err)
	require.Empty(t, next["TLVs"], "a later independent Hello cannot inherit previous neighbor state")
	wrong := isisWireAdd(t, isisWireVector(t, 3), []byte{240, 1, 0})
	m, err = DecodeISISWire(wrong, 64)
	require.NoError(t, err)
	require.Equal(t, true, m["Unselected Extensions"])
	require.NotContains(t, m["TLVs"].([]map[string]any)[0], "Adjacency State")
	_, err = DecodeISISWire(isisWireAdd(t, isisWireVector(t, 2), []byte{8, 0, 8, 0}), 1)
	require.ErrorIs(t, err, ErrObservedWireBudget)
}

func TestISISWireLegacyESNeighborsKeepPerRecordMetricOctets(t *testing.T) {
	w := isisWireAdd(t, isisWireVector(t, 3), []byte{3, 20, 1, 0x82, 0x83, 0x84, 1, 2, 3, 4, 5, 6, 5, 0x86, 7, 8, 6, 5, 4, 3, 2, 1})
	m, err := DecodeISISWire(w, 64)
	require.NoError(t, err)
	rows := m["TLVs"].([]map[string]any)[0]["Neighbors"].([]map[string]any)
	require.Len(t, rows, 2)
	for i, values := range [][4]uint64{{1, 2, 3, 4}, {5, 6, 7, 8}} {
		for j, name := range []string{"Default", "Delay", "Expense", "Error"} {
			require.EqualValues(t, values[j], rows[i][name+" Metric"], "metrics use four distinct wire octets")
		}
	}
	require.Equal(t, false, rows[0]["Delay Metric Supported"])
	require.Equal(t, "0102.0304.0506", rows[0]["System ID"])
	require.Equal(t, "0605.0403.0201", rows[1]["System ID"])
}

func TestISISWireLANNeighborSNPAIsNotSystemIdentity(t *testing.T) {
	w := isisWireAdd(t, isisWireVector(t, 0), []byte{6, 6, 0, 0x25, 0x45, 0x60, 0x17, 0xc0})
	m, err := DecodeISISWire(w, 64)
	require.NoError(t, err)
	row := m["TLVs"].([]map[string]any)[0]["IS Neighbors"].([]map[string]any)[0]
	require.Equal(t, "00:25:45:60:17:c0", row["SNPA"])
	require.NotContains(t, row, "System ID", "a MAC-length SNPA is not an IS-IS System ID")
}

func TestISISWireMalformedTLVRetainsOnlyEarlierCompleteObservations(t *testing.T) {
	w := isisWireAdd(t, isisWireVector(t, 3), []byte{137, 2, 'r', '1', 3, 4, 0, 128, 128, 128})
	m, err := DecodeISISWire(w, 64)
	require.Error(t, err)
	require.Equal(t, true, m["Fixed Header Complete"])
	require.Equal(t, false, m["Wire Grammar Complete"])
	rows := m["TLVs"].([]map[string]any)
	require.Len(t, rows, 2)
	require.Equal(t, "r1", rows[0]["Hostname"])
	require.Equal(t, "Malformed TLV value", rows[1]["TLV Name"])
	require.NotContains(t, rows[1], "Neighbors")
	require.Equal(t, true, m["Checksum Verified"], "complete-PDU integrity is independent of malformed TLV semantics")
}

func TestISISWireAttachedDefaultBitIsNotPartitionRepair(t *testing.T) {
	for _, flags := range []byte{0x0b, 0x83} {
		w := isisWireVector(t, 3)
		w[26] = flags
		isisWireSetChecksum(w, 12, 24)
		m, err := DecodeISISWire(w, 64)
		require.NoError(t, err)
		require.Equal(t, flags&8 != 0, m["Attached Default Metric"])
		require.Equal(t, flags&128 != 0, m["Partition Repair"])
	}
}

func TestISISWirePrefixAndSubTLVNamespacesRemainDistinct(t *testing.T) {
	w := isisWireAdd(t, isisWireVector(t, 3),
		[]byte{22, 17, 1, 2, 3, 4, 5, 6, 7, 0, 0, 10, 6, 3, 4, 0, 0, 0, 7},
		[]byte{135, 14, 0, 0, 0, 9, 0x49, 192, 255, 6, 3, 4, 0, 0, 0, 7})
	m, err := DecodeISISWire(w, 64)
	require.NoError(t, err)
	rows := m["TLVs"].([]map[string]any)
	link := rows[0]["Neighbors"].([]map[string]any)[0]["Sub-TLVs"].([]map[string]any)[0]
	prefix := rows[1]["Reachability Entries"].([]map[string]any)[0]
	unknown := prefix["Sub-TLVs"].([]map[string]any)[0]
	require.EqualValues(t, 7, link["Administrative Group"])
	require.NotContains(t, unknown, "Administrative Group", "prefix sub-TLV 3 is not TE link sub-TLV 3")
	require.Equal(t, "192.255.0.0", prefix["Prefix Address"], "literal non-prefix bits remain visible")
	require.Equal(t, "192.128.0.0/9", prefix["Canonical Prefix"])
	require.Equal(t, true, m["Unselected Extensions"])
}

func FuzzISISBoundedWire(f *testing.F) {
	for i := 0; i < 9; i++ {
		f.Add(isisWireVector(f, i))
	}
	for _, tlv := range [][]byte{{1, 4, 3, 0x49, 0, 1}, {240, 15, 0, 0, 0, 0, 7, 1, 2, 3, 4, 5, 6, 0, 0, 0, 8}, {12, 2, 0, 0}, {8, 0, 129, 2, 0xcc, 0x8e}} {
		f.Add(isisWireAdd(f, isisWireVector(f, 2), tlv))
	}
	for _, tlv := range [][]byte{{2, 12, 0, 1, 0x81, 0x42, 0xc3, 1, 2, 3, 4, 5, 6, 7}, {135, 7, 0, 0, 0, 9, 9, 192, 255}, {236, 11, 0, 0, 0, 9, 0, 33, 0x20, 1, 0x0d, 0xb8, 255}, {3, 4, 0, 128, 128, 128}} {
		f.Add(isisWireAdd(f, isisWireVector(f, 3), tlv))
	}
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 4096 {
			return
		}
		check := func(w []byte) {
			m, err := DecodeISISWire(w, 64)
			if m != nil {
				require.EqualValues(t, len(w), m["PDU Length"])
				require.Equal(t, false, m["Authentication Verified"])
				if err == nil {
					require.Equal(t, true, m["Wire Grammar Complete"])
				}
				var spans func(any)
				spans = func(v any) {
					switch row := v.(type) {
					case map[string]any:
						for key, value := range row {
							if key != "Wire Field Ranges" {
								spans(value)
								continue
							}
							for _, pair := range value.(map[string]any) {
								p := pair.([]any)
								lo, hi := p[0].(uint64), p[1].(uint64)
								require.LessOrEqual(t, lo, hi)
								require.LessOrEqual(t, hi, uint64(len(w)))
							}
						}
					case []map[string]any:
						for _, child := range row {
							spans(child)
						}
					}
				}
				spans(m)
			}
		}
		check(w)
		// Preserve the original refusal test above, then repair only the outer
		// PDU length so mutations can reach deep TLV/sub-TLV bounds as well.
		if len(w) >= 20 {
			repaired := bytes.Clone(w)
			at := 8
			typ := w[4] & 31
			if typ == 15 || typ == 16 || typ == 17 {
				at = 17
			}
			binary.BigEndian.PutUint16(repaired[at:], uint16(len(repaired)))
			check(repaired)
		}
	})
}
