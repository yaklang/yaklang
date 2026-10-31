package stream_parser

import (
	"bytes"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func ripngTestEntry(address string, tag uint16, prefix, metric byte) []byte {
	a := netip.MustParseAddr(address).As16()
	return append(a[:], byte(tag>>8), byte(tag), prefix, metric)
}

func TestRIPngCompleteWireAndMessageLocalNextHop(t *testing.T) {
	w := []byte{2, 1, 0, 0}
	for _, entry := range [][]byte{
		ripngTestEntry("2001:db8:1::1", 2, 64, 1),
		ripngTestEntry("fe80::9", 0xffff, 255, 255),
		ripngTestEntry("2001:db8:2::", 7, 64, 16),
		ripngTestEntry("2001:db8::99", 0, 0, 255),
		ripngTestEntry("2001:db8:3::", 8, 64, 3),
	} {
		w = append(w, entry...)
	}
	m, err := DecodeRIPngWire(w, 8)
	require.NoError(t, err)
	require.EqualValues(t, 3, m["Route Count"])
	require.EqualValues(t, 2, m["Next Hop Count"])
	rows := m["Entries"].([]map[string]any)
	require.Equal(t, "originator", rows[0]["Effective Next Hop Advisory"])
	require.Equal(t, "2001:db8:1::/64", rows[0]["Canonical Prefix"])
	require.Equal(t, "2001:db8:1::1", rows[0]["IPv6 Prefix"], "literal host bits remain visible")
	require.Equal(t, "Next Hop", rows[1]["Entry Type"])
	require.EqualValues(t, 255, rows[1]["Prefix Length"], "ignored advisory octets are not normalized")
	require.NotEmpty(t, rows[1]["Ignored Next Hop Parameters"])
	require.Equal(t, "fe80::9", rows[2]["Effective Next Hop Advisory"])
	require.Equal(t, true, rows[2]["Advertised Unreachable"])
	require.NotEmpty(t, rows[3]["Conformance Warning"])
	require.Equal(t, "originator", rows[4]["Effective Next Hop Advisory"])
	clear(w)
	require.Equal(t, "fe80::9", rows[1]["IPv6 Prefix"], "returned scalar values own their contents")
	next, err := DecodeRIPngWire(append([]byte{2, 1, 0, 0}, ripngTestEntry("2001:db8:4::", 0, 64, 1)...), 8)
	require.NoError(t, err)
	require.Equal(t, "originator", next["Entries"].([]map[string]any)[0]["Effective Next Hop Advisory"])
	request, err := DecodeRIPngWire(append([]byte{1, 1, 0, 0}, ripngTestEntry("::", 0, 0, 16)...), 8)
	require.NoError(t, err)
	require.Equal(t, true, request["Whole Table Request"])
	require.NotContains(t, request["Entries"].([]map[string]any)[0], "Advertised Unreachable", "query infinity is not an unreachable advertisement")
}

func TestRIPngBoundariesRefusePartialEntriesAtomically(t *testing.T) {
	w := append([]byte{2, 1, 0, 0}, ripngTestEntry("2001:db8::", 7, 64, 2)...)
	for cut := 0; cut < len(w); cut++ {
		if cut == 4 {
			continue // explicitly bounded empty message; automatic ingress refuses it
		}
		m, err := DecodeRIPngWire(w[:cut], 8)
		require.Error(t, err)
		require.Nil(t, m)
	}
	for _, edit := range []func([]byte){
		func(b []byte) { b[0] = 8 }, func(b []byte) { b[1] = 2 },
		func(b []byte) { b[22] = 129 }, func(b []byte) { b[23] = 0 }, func(b []byte) { b[23] = 17 },
	} {
		bad := bytes.Clone(w)
		edit(bad)
		m, err := DecodeRIPngWire(bad, 8)
		require.Error(t, err)
		require.Nil(t, m)
	}
	_, err := DecodeRIPngWire(append(bytes.Clone(w), w[4:]...), 1)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	_, err = DecodeRIPngWire(w, 0)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	w[3] = 1
	m, err := DecodeRIPngWire(w, 8)
	require.NoError(t, err)
	require.NotEmpty(t, m["Conformance Warning"])
}

func FuzzRIPngBoundedWire(f *testing.F) {
	f.Add(append([]byte{2, 1, 0, 0}, ripngTestEntry("2001:db8::", 0, 64, 1)...))
	f.Add(append([]byte{1, 1, 0, 0}, ripngTestEntry("::", 0, 0, 16)...))
	f.Add(append([]byte{2, 1, 0, 0}, ripngTestEntry("fe80::1", 65535, 255, 255)...))
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 2048 {
			return
		}
		if m, err := DecodeRIPngWire(w, 64); err == nil {
			require.EqualValues(t, len(w), m["PDU Length"])
			require.Equal(t, false, m["Authentication Verified"])
			require.Equal(t, (len(w)-4)/20, len(m["Entries"].([]map[string]any)))
		}
	})
}
