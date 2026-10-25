package bin_parser

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSTPCanonicalWireEntries(t *testing.T) {
	for _, pvst := range []bool{false, true} {
		w := append(bytes.Clone(stpConfigBPDU()), 0)
		w[2], w[3] = 2, 2
		entry, name := "STPFields", "RSTP"
		if pvst {
			w = append(w, 0, 0, 0, 2, 0, 7)
			entry, name = "PVSTFields", "RPVST+"
		}
		v, err := ParseStructured(w, "stp", entry)
		require.NoError(t, err)
		m := v["fields"].(map[string]any)["Message"].(map[string]any)
		require.Equal(t, name, m["Protocol Name"])
		for cut := 0; cut < len(w); cut++ {
			_, err = ParseStructured(w[:cut], "stp", entry)
			require.Error(t, err)
		}
		_, err = ParseStructured(append(bytes.Clone(w), 0xff), "stp", entry)
		require.Error(t, err)
		llc := []byte{0x42, 0x42, 3}
		if pvst {
			llc = []byte{0xaa, 0xaa, 3, 0, 0, 12, 1, 11}
		}
		frame := ethernet8023([]byte{1, 0x80, 0xc2, 0, 0, 0}, []byte{2, 0, 0, 0, 0, 1}, append(llc, w...))
		_, err = ParseStructured(frame, "ethernet", "Ethernet")
		require.NoError(t, err, "canonical Ethernet carrier")
	}
	// Preserve the historical leaf tree after the complete-message probe.
	w := stpConfigBPDU()
	n := parseRule(t, w, "stp", "STP")
	require.EqualValues(t, 8, uintVal(t, mustChild(t, n, "Config").Child("Root Priority")))
	for _, bad := range [][]byte{append(bytes.Clone(w), 0), {0, 0, 2, 128}} {
		_, err := ParseStructured(bad, "stp", "STP")
		require.Error(t, err)
	}
	// Standard MST extension, one complete MSTI. This control is authored
	// independently of the malformed historical RSTP fixture.
	mst := append(bytes.Clone(w), 0, 0, 80, 0)
	mst[2], mst[3] = 3, 2
	name := make([]byte, 32)
	copy(name, []byte("region-a"))
	mst = append(mst, name...)
	mst = append(mst, 0, 7)
	mst = append(mst, bytes.Repeat([]byte{0x12}, 16)...)
	mst = append(mst, 0, 0, 0, 11, 0x80, 0, 2, 0, 0, 0, 0, 3, 20)
	mst = append(mst, 0x7c, 0x80, 1, 2, 0, 0, 0, 0, 4, 0, 0, 0, 12, 0x90, 0x80, 19)
	v, err := ParseStructured(mst, "stp", "STPFields")
	require.NoError(t, err)
	fields := v["fields"].(map[string]any)["Message"].(map[string]any)
	require.Equal(t, "MSTP", fields["Protocol Name"])
	rows := fields["MST Instances"].([]map[string]any)
	require.Len(t, rows, 1)
	require.EqualValues(t, 9, rows[0]["Bridge Priority"])
	_, err = ParseStructured(mst, "stp", "STP")
	require.NoError(t, err, "historical entry consumes the validated complete MST tree")
	_, err = ParseStructured(mst[:len(mst)-1], "stp", "STPFields")
	require.Error(t, err)
}
