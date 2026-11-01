package bin_parser

import (
	"bytes"
	"encoding/binary"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOSPFv3ObservedCanonicalAndAHParent(t *testing.T) {
	w := []byte{3, 1, 0, 36, 192, 0, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 7, 1, 0, 0, 0x13, 0, 10, 0, 40, 192, 0, 2, 1, 0, 0, 0, 0}
	v, err := ParseStructured(w, "ospfv3_observed", "OSPFv3ControlFields")
	require.NoError(t, err)
	fields := v["fields"].(map[string]any)["Message"].(map[string]any)
	require.Equal(t, "Hello", fields["Packet Name"])
	require.Equal(t, false, fields["Checksum Verified"], "stateless canonical entry has no IPv6 context")
	require.Equal(t, uint64(7), fields["Hello"].(map[string]any)["Interface ID"])
	for cut := 0; cut < len(w); cut++ {
		_, err = ParseStructured(w[:cut], "ospfv3_observed", "OSPFv3ControlFields")
		require.Error(t, err)
	}
	// Canonical IPv6's extension loop must reach its existing OSPFv3 entry
	// through a valid24-byte AH, without including AH in the OSPF PDU.
	parent := make([]byte, 40+24)
	parent[0], parent[5], parent[6], parent[7] = 0x60, 60, 51, 1
	parent[8], parent[24], parent[25], parent[39] = 0xfe, 0xff, 2, 5
	parent[40], parent[41] = 89, 4
	parent = append(parent, w...)
	v, err = ParseStructured(parent, "internet_protocol_version_6", "Internet Protocol Version 6")
	require.NoError(t, err)
	require.NotEmpty(t, v)
	parent[41] = 1 //12 bytes is not an IPv6 AH multiple of8.
	_, err = ParseStructured(parent, "internet_protocol_version_6", "Internet Protocol Version 6")
	require.Error(t, err)
}

func requireOSPFv3CanonicalWireScope(t *testing.T) {
	t.Helper()
	TestOSPFv3ObservedCanonicalAndAHParent(t)
	for typ, body := range map[byte][]byte{2: make([]byte, 12), 3: make([]byte, 12), 4: make([]byte, 4), 5: append(make([]byte, 18), 0, 20)} {
		w := append([]byte{3, typ, 0, 0, 192, 0, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0}, body...)
		binary.BigEndian.PutUint16(w[2:], uint16(len(w)))
		_, err := ParseStructured(w, "ospfv3_observed", "OSPFv3ControlFields")
		require.NoError(t, err)
		_, err = ParseStructured(w[:len(w)-1], "ospfv3_observed", "OSPFv3ControlFields")
		require.Error(t, err)
	}
	// The canonical complete-PDU entry also bounds its own collections.
	wide := append([]byte{3, 1, 0, 0, 192, 0, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0}, make([]byte, 20)...)
	wide = append(wide, bytes.Repeat([]byte{192, 0, 2, 2}, 4097)...)
	binary.BigEndian.PutUint16(wide[2:], uint16(len(wide)))
	_, err := ParseStructured(wide, "ospfv3_observed", "OSPFv3ControlFields")
	require.Error(t, err)
	catalog, roadmap := false, false
	for _, r := range ProtocolCatalog {
		if r.Name == "OSPFv3 Observed Control Fields" {
			catalog = true
			require.Equal(t, "OSPFv3", r.ProfileOf)
			require.Equal(t, statusPartial, r.Status)
			require.Equal(t, "ospfv3_observed.yaml", r.RuleFile)
			require.Equal(t, "OSPFv3ControlFields", r.EntryNode)
			require.NotEmpty(t, r.Notes)
		}
	}
	for _, r := range ProtocolRoadmap {
		if r.Name == "OSPFv3" {
			roadmap = true
			require.Equal(t, stPartial, r.Status)
			require.Equal(t, priP2, r.Priority)
			require.NotEmpty(t, r.Notes)
		}
	}
	require.True(t, catalog)
	require.True(t, roadmap)
}
