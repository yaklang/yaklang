package bin_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func TestOSPFv2ObservedCanonicalCatalogAndWireScope(t *testing.T) {
	w := []byte{2, 1, 0, 44, 192, 0, 2, 1, 0, 0, 0, 0, 0, 0, 17, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		255, 255, 255, 0, 0, 10, 2, 1, 0, 0, 0, 40, 192, 0, 2, 1, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(w[12:], stream_parser.OSPFv2Checksum(w))
	v, err := ParseStructured(w, "ospfv2_observed", "OSPFv2ControlFields")
	require.NoError(t, err)
	fields := v["fields"].(map[string]any)["Message"].(map[string]any)
	expected, err := stream_parser.DecodeOSPFv2Wire(w, 4096)
	require.NoError(t, err)
	require.Equal(t, expected, fields)
	for cut := 0; cut < len(w); cut++ {
		_, err = ParseStructured(w[:cut], "ospfv2_observed", "OSPFv2ControlFields")
		require.Error(t, err)
	}
	found := false
	for _, p := range ProtocolCatalog {
		if p.Name == "OSPFv2 Observed Control Fields" {
			found = true
			require.Equal(t, "OSPF", p.ProfileOf)
			require.Equal(t, statusPartial, p.Status)
			require.Equal(t, "ospfv2_observed.yaml", p.RuleFile)
			require.Equal(t, "OSPFv2ControlFields", p.EntryNode)
		}
	}
	require.True(t, found)
}
