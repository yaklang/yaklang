package stream_parser

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func mldGoldenReport(t *testing.T) []byte {
	t.Helper()
	// Two independent records: INCLUDE(two sources, one auxiliary word),
	// BLOCK_OLD_SOURCES(no sources). The second record begins at byte64.
	w, err := hex.DecodeString("8f00123400000002" +
		"01010002ff3e0000000000000000000000001234" +
		"20010db8000000000000000000000001" +
		"00000000000000000000ffffc0000201" + "deadbeef" +
		"06000000ff0200000000000000000000000000fb")
	require.NoError(t, err)
	require.Len(t, w, 84)
	return w
}

func mldValue(r MLDWireResult, name string) []MLDWireField {
	var values []MLDWireField
	for _, f := range r.Values {
		if f.Name == name {
			values = append(values, f)
		}
	}
	return values
}

func TestMLDRecordsConsumeSourcesAndAuxiliaryWords(t *testing.T) {
	w := mldGoldenReport(t)
	r, err := DecodeMLDWire(w, 4096, 4)
	require.NoError(t, err)
	m := r.Fields["MLD"].(map[string]any)
	records := m["Address Records"].([]map[string]any)
	require.Len(t, records, 2)
	require.Equal(t, []string{"2001:db8::1", "::ffff:192.0.2.1"}, records[0]["Sources"])
	require.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, records[0]["Auxiliary Data (uninterpreted)"])
	require.Equal(t, uint8(6), records[1]["Record Type"])
	require.Equal(t, "ff02::fb", records[1]["Multicast Address"])
	require.Equal(t, []string{}, records[1]["Sources"])
	values := mldValue(r, "icmpv6.mldr.mar.record_type")
	require.Equal(t, []int{8, 64}, []int{values[0].Offset, values[1].Offset})
	require.Equal(t, []int{12, 68}, []int{mldValue(r, "icmpv6.mldr.mar.multicast_address")[0].Offset, mldValue(r, "icmpv6.mldr.mar.multicast_address")[1].Offset})
	require.Equal(t, "deadbeef", mldValue(r, "icmpv6.mldr.mar.auxiliary_data")[0].Value)
	// A returned tree owns its byte slices; input and other decodes do not alias.
	records[0]["Auxiliary Data (uninterpreted)"].([]byte)[0] = 0
	require.Equal(t, byte(0xde), w[60])
	r, err = DecodeMLDWire(append(bytes.Clone(w), 1, 2, 3), 4096, 4)
	require.NoError(t, err)
	require.Equal(t, []byte{1, 2, 3}, r.Fields["MLD"].(map[string]any)["Additional Data (uninterpreted)"])
}

func TestMLDEveryReportCutKeepsOnlyWholeCapturedFields(t *testing.T) {
	w := mldGoldenReport(t)
	for cut := 0; cut < len(w); cut++ {
		r, err := DecodeMLDWire(w[:cut], 4096, 4096, MLDWireContext{DeclaredBytes: len(w)})
		require.ErrorIs(t, err, ErrMLDTruncated, "cut%d", cut)
		for _, f := range r.Values {
			require.GreaterOrEqual(t, f.Offset, 0)
			require.Positive(t, f.Length)
			require.LessOrEqual(t, f.Offset+f.Length, cut, "cut%d field%s", cut, f.Name)
			if f.Name == "icmpv6.mldr.mar.source_address" || f.Name == "icmpv6.mldr.mar.multicast_address" {
				require.Equal(t, 16, f.Length)
			}
		}
		if cut == 64 {
			require.Len(t, mldValue(r, "icmpv6.mldr.mar.record_type"), 1, "absent second record has no invented type")
		}
	}
}

func TestMLDQueryCodesVersionsAndDeclaredCaptureExtent(t *testing.T) {
	w, err := hex.DecodeString("82001234ffff0000ff3e000000000000000000000000abcd" + "faff0001" + "20010db8000000000000000000000005")
	require.NoError(t, err)
	r, err := DecodeMLDWire(w, 4096, 4096)
	require.NoError(t, err)
	require.Equal(t, uint8(2), r.Version)
	require.Equal(t, uint16(65535), mldValue(r, "icmpv6.mld.maximum_response_code.raw")[0].Value)
	require.Equal(t, uint32(8387584), mldValue(r, "icmpv6.mld.maximum_response_code")[0].Value)
	require.Equal(t, uint8(255), mldValue(r, "icmpv6.mld.qqi.raw")[0].Value)
	require.Equal(t, uint16(31744), mldValue(r, "icmpv6.mld.qqi")[0].Value)
	require.Equal(t, true, mldValue(r, "icmpv6.mld.flag.s")[0].Value)
	require.Equal(t, uint8(2), mldValue(r, "icmpv6.mld.flag.qrv")[0].Value)
	require.Equal(t, uint8(15), mldValue(r, "icmpv6.mld.flag.reserved")[0].Value)
	// A snaplen24 prefix of a declared44-byte query is v2, not a v1 message.
	r, err = DecodeMLDWire(w[:24], 4096, 4096, MLDWireContext{DeclaredBytes: len(w)})
	require.ErrorIs(t, err, ErrMLDTruncated)
	require.Equal(t, uint8(2), r.Version)
	require.Empty(t, mldValue(r, "icmpv6.mld.maximum_response_delay"))
	require.Empty(t, mldValue(r, "icmpv6.mld.nb_sources"), "no invented source-count zero")
	r, err = DecodeMLDWire(w[:24], 4096, 4096)
	require.NoError(t, err)
	require.Equal(t, uint8(1), r.Version)
	require.Equal(t, uint16(65535), mldValue(r, "icmpv6.mld.maximum_response_delay")[0].Value)
	r, err = DecodeMLDWire(w[:24], 4096, 4096, MLDWireContext{DeclaredBytes: -1})
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.Equal(t, uint8(0), r.Version)
	require.Empty(t, mldValue(r, "icmpv6.mld.maximum_response_delay"))
	// Captured bytes may include padding when the enclosing extent is unknown.
	// They cannot promote a common query prefix to version2 or a floating code.
	r, err = DecodeMLDWire(w, 4096, 4096, MLDWireContext{DeclaredBytes: -1})
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.Zero(t, r.Version)
	require.Empty(t, mldValue(r, "icmpv6.mld.maximum_response_code"))
	require.Empty(t, mldValue(r, "icmpv6.mld.flag"))
	require.Empty(t, mldValue(r, "icmpv6.mld.nb_sources"))
	for size := 25; size <= 27; size++ {
		r, err = DecodeMLDWire(w[:size], 4096, 4096)
		require.ErrorIs(t, err, ErrMLDMalformed)
		require.Equal(t, uint8(0), r.Version)
		require.Equal(t, "ff3e::abcd", mldValue(r, "icmpv6.mld.multicast_address")[0].Value)
	}
}

func TestMLDResourceAndNonsemanticBounds(t *testing.T) {
	w := mldGoldenReport(t)
	r, err := DecodeMLDWire(w, 4096, 2)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	require.Len(t, mldValue(r, "icmpv6.mldr.mar.source_address"), 1)
	require.Len(t, mldValue(r, "icmpv6.mldr.mar.record_type"), 1)
	_, err = DecodeMLDWire(w, len(w)-1, 4096)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	w[8] = 250
	r, err = DecodeMLDWire(w, 4096, 4096)
	require.NoError(t, err, "unknown record kinds retain their layout; no membership is inferred")
	require.Contains(t, r.Fields["MLD"].(map[string]any)["Observation Scope"], "unverified")
	_, err = DecodeMLDWire([]byte{128, 0, 0, 0}, 4096, 4096)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrMLDTruncated), "an Echo header does not select MLD")
	// Report/Done's delay-position word is unused, never a response timer.
	v1, err := hex.DecodeString("83001234abcd0000ff0200000000000000000000000000fb")
	require.NoError(t, err)
	r, err = DecodeMLDWire(v1, 4096, 4096)
	require.NoError(t, err)
	require.Equal(t, uint16(0xabcd), r.Fields["MLD"].(map[string]any)["Unused Delay Word"])
	require.Empty(t, mldValue(r, "icmpv6.mld.maximum_response_delay"))
}
