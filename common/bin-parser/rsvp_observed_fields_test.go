package bin_parser

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"testing"
)

func TestRSVPObservedCanonicalAndCatalog(t *testing.T) {
	w := []byte{0x10, 1, 0, 0, 64, 0, 0, 16, 0, 8, 5, 1, 0, 0, 0x75, 0x30}
	v, err := ParseStructured(w, "rsvp_observed", "RSVPControlFields")
	require.NoError(t, err)
	want, err := stream_parser.DecodeRSVPWire(w, 4096)
	require.NoError(t, err)
	require.Equal(t, want, v["fields"].(map[string]any)["Message"])
	for _, cut := range []int{0, 1, 7, 9, 14} {
		_, err = ParseStructured(w[:cut], "rsvp_observed", "RSVPControlFields")
		require.Error(t, err)
	}
	found := false
	for _, p := range ProtocolCatalog {
		if p.Name == "RSVP Observed Control Fields" {
			found = true
			require.Equal(t, statusPartial, p.Status)
			require.Equal(t, "RSVP", p.ProfileOf)
			require.Equal(t, "rsvp_observed.yaml", p.RuleFile)
		}
	}
	require.True(t, found)
}

func TestRSVPIntservCanonicalRetainsExactFloatStructure(t *testing.T) {
	w := []byte{0x10, 1, 0, 0, 64, 0, 0, 44, 0, 36, 12, 2, 0, 0, 0, 7, 1, 0, 0, 6, 127, 0, 0, 5, 0x45, 0xbb, 0x80, 0, 0x45, 0xbb, 0x80, 0, 0x7f, 0x80, 0, 0, 0, 0, 0, 1, 0, 0, 5, 0xdc}
	v, err := ParseStructured(w, "rsvp_observed", "RSVPControlFields")
	require.NoError(t, err)
	want, err := stream_parser.DecodeRSVPWire(w, 4096)
	require.NoError(t, err)
	require.Equal(t, want, v["fields"].(map[string]any)["Message"])
	services := want["Objects"].([]map[string]any)[0]["Body"].(map[string]any)["Services"].([]map[string]any)
	parameter := services[0]["Parameters"].([]map[string]any)[0]["Body"].(map[string]any)
	require.Equal(t, "+Infinity", parameter["Peak Data Rate"].(map[string]any)["Value"])
}
