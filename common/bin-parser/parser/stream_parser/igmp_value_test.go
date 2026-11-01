package stream_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func igmpGoldenReport(t *testing.T) []byte {
	t.Helper()
	w, err := hex.DecodeString("2200123400000002" + "01010002ef010203c0000201c0000202deadbeef" + "06000000ef040506")
	require.NoError(t, err)
	require.Len(t, w, 36)
	return w
}

func igmpValues(r IGMPWireResult, name string) []IGMPWireField {
	var out []IGMPWireField
	for _, f := range r.Values {
		if f.Name == name {
			out = append(out, f)
		}
	}
	return out
}

func TestIGMPRecordsConsumeEverySourceAndFourByteAuxiliaryWord(t *testing.T) {
	w := igmpGoldenReport(t)
	r, err := DecodeIGMPWire(w, 4096, 4)
	require.NoError(t, err)
	m := r.Fields["IGMP"].(map[string]any)
	records := m["Group Records"].([]map[string]any)
	require.Len(t, records, 2)
	require.Equal(t, []string{"192.0.2.1", "192.0.2.2"}, records[0]["Sources"])
	require.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, records[0]["Auxiliary Data (uninterpreted)"])
	require.Equal(t, uint8(6), records[1]["Record Type"])
	require.Equal(t, "239.4.5.6", records[1]["Multicast Address"])
	addresses := igmpValues(r, "igmp.maddr")
	require.Equal(t, []int{12, 32}, []int{addresses[0].Offset, addresses[1].Offset})
	require.Equal(t, []int{28, 29}, records[1]["Wire Field Ranges"].(map[string]any)["Record Type"])
	records[0]["Auxiliary Data (uninterpreted)"].([]byte)[0] = 0
	require.Equal(t, byte(0xde), w[24])
}

func TestIGMPEveryCutKeepsOnlyWholeWireWordsAndNoMissingRecord(t *testing.T) {
	w := igmpGoldenReport(t)
	for cut := 0; cut < len(w); cut++ {
		r, err := DecodeIGMPWire(w[:cut], 4096, 4096, IGMPWireContext{DeclaredBytes: len(w)})
		require.ErrorIs(t, err, ErrIGMPTruncated, "cut%d", cut)
		for _, f := range r.Values {
			require.GreaterOrEqual(t, f.Offset, 0)
			require.Positive(t, f.Length)
			require.LessOrEqual(t, f.Offset+f.Length, cut)
			if f.Name == "igmp.saddr" || f.Name == "igmp.maddr" {
				require.Equal(t, 4, f.Length)
			}
		}
		if cut == 28 {
			require.Len(t, igmpValues(r, "igmp.record_type"), 1)
		}
	}
}

func TestIGMPQueryVersionLengthAndTimerUnitsAreIndependent(t *testing.T) {
	for _, test := range []struct {
		size         int
		code         byte
		version      uint8
		milliseconds uint32
	}{
		{8, 0, 1, 10000}, {8, 0x82, 2, 13000}, {12, 0x82, 3, 14400},
	} {
		w := make([]byte, test.size)
		w[0], w[1] = 0x11, test.code
		if len(w) >= 12 {
			w[8], w[9] = 0xff, 0x95
		}
		r, err := DecodeIGMPWire(w, 4096, 4096)
		require.NoError(t, err)
		require.Equal(t, test.version, r.Version)
		require.Equal(t, test.code, igmpValues(r, "igmp.max_resp")[0].Value)
		require.Equal(t, test.milliseconds, igmpValues(r, "igmp.max_resp.decoded_ms")[0].Value)
		if test.version == 3 {
			require.Equal(t, uint16(336), igmpValues(r, "igmp.qqic.decoded_seconds")[0].Value)
			require.Equal(t, uint8(15), igmpValues(r, "igmp.query.reserved")[0].Value)
		}
	}
	for size := 9; size <= 11; size++ {
		w := make([]byte, size)
		w[0] = 0x11
		r, err := DecodeIGMPWire(w, 4096, 4096)
		require.ErrorIs(t, err, ErrIGMPMalformed)
		require.Zero(t, r.Version)
	}
	query := []byte{0x11, 0x82, 0, 0, 0, 0, 0, 0, 2, 0x95, 0, 1, 192, 0, 2, 1}
	r, err := DecodeIGMPWire(query[:8], 4096, 4096, IGMPWireContext{DeclaredBytes: len(query)})
	require.ErrorIs(t, err, ErrIGMPTruncated)
	require.Equal(t, uint8(3), r.Version, "captured8bytes are not a v2 query")
	r, err = DecodeIGMPWire(query[:8], 4096, 4096, IGMPWireContext{DeclaredBytes: -1})
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.Zero(t, r.Version)
	require.Empty(t, igmpValues(r, "igmp.max_resp.decoded_ms"))
	// Unknown enclosing length may include link padding after a v1/v2 query.
	// Extra captured bytes cannot establish a v3 version or timer semantics.
	r, err = DecodeIGMPWire(query, 4096, 4096, IGMPWireContext{DeclaredBytes: -1})
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.Zero(t, r.Version)
	require.Empty(t, igmpValues(r, "igmp.max_resp.decoded_ms"))
	require.Empty(t, igmpValues(r, "igmp.query.flags"))
	require.Empty(t, igmpValues(r, "igmp.num_src"))
}

func TestIGMPLargeCountsAreDecodedOrExplicitlyBudgetLimitedNeverSkipped(t *testing.T) {
	// 260 records, beyond the legacy <256 shortcut. Each one has no sources.
	w := make([]byte, 8+260*8)
	w[0] = 0x22
	binary.BigEndian.PutUint16(w[6:8], 260)
	for at := 8; at < len(w); at += 8 {
		w[at] = 1
		copy(w[at+4:at+8], []byte{239, 1, 2, 3})
	}
	r, err := DecodeIGMPWire(w, 1<<20, 260)
	require.NoError(t, err)
	require.Len(t, igmpValues(r, "igmp.record_type"), 260)
	r, err = DecodeIGMPWire(w, 1<<20, 259)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	require.Len(t, igmpValues(r, "igmp.record_type"), 259)
	query := make([]byte, 12+65*4)
	query[0] = 0x11
	binary.BigEndian.PutUint16(query[10:12], 65)
	r, err = DecodeIGMPWire(query, 4096, 65)
	require.NoError(t, err)
	require.Len(t, igmpValues(r, "igmp.saddr"), 65)
	_, err = DecodeIGMPWire(query, len(query)-1, 4096)
	require.ErrorIs(t, err, ErrObservedWireBudget)
}

func TestIGMPUnknownTypesAndUnverifiedPropertiesCannotBecomeMembershipState(t *testing.T) {
	for _, typ := range []byte{0x13, 0x14, 0x1e, 0xff} {
		require.False(t, IGMPMembershipType(typ))
		r, err := DecodeIGMPWire([]byte{typ, 0, 0, 0, 0, 0, 0, 0}, 4096, 4096)
		require.ErrorIs(t, err, ErrObservedWireContext)
		require.Len(t, r.Values, 1)
		require.Zero(t, r.Version)
	}
	w := igmpGoldenReport(t)
	r, err := DecodeIGMPWire(w, 4096, 4096)
	require.NoError(t, err)
	for _, key := range []string{"Checksum Verified", "Membership Established", "Router Alert Verified", "Authenticated"} {
		require.NotContains(t, r.Fields, key)
	}
	// Input/trailing bytes and outputs own distinct buffers.
	withTail := append(bytes.Clone(w), 1, 2, 3)
	r, err = DecodeIGMPWire(withTail, 4096, 4096)
	require.NoError(t, err)
	require.Equal(t, []byte{1, 2, 3}, r.Fields["IGMP"].(map[string]any)["Additional Data (uninterpreted)"])
	require.False(t, errors.Is(err, ErrIGMPMalformed))
}
