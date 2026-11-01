package bin_parser

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func TestGTPv2ObservedCanonicalCatalogAndSharedWireTree(t *testing.T) {
	w := []byte{0x40, 1, 0, 9, 0, 0, 1, 0, 3, 0, 1, 0, 7}
	v, err := ParseStructured(w, "gtpv2_observed", "GTPv2ControlFields")
	require.NoError(t, err)
	want, err := stream_parser.DecodeGTPv2Wire(w, 4096)
	require.NoError(t, err)
	require.Equal(t, want, v["fields"].(map[string]any)["Message"])
	for cut := 0; cut < len(w); cut++ {
		_, err := ParseStructured(w[:cut], "gtpv2_observed", "GTPv2ControlFields")
		require.Error(t, err)
	}
	found := false
	for _, p := range ProtocolCatalog {
		if p.Name == "GTPv2 Observed Control Fields" {
			found = true
			require.Equal(t, statusPartial, p.Status)
			require.Equal(t, "GTPv2", p.ProfileOf)
			require.Equal(t, "gtpv2_observed.yaml", p.RuleFile)
		}
	}
	require.True(t, found)
}
