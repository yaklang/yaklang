package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
)

// Receive validation selects configuration/TCN layouts by type. The archived
// filename calls this RSTP, but version2/type0 carries configuration fields.
// Preserve the literal version and immutable positive parse; its name/version
// cannot prove a rapid-format message.
func TestSTPFrozenRapidLabelDoesNotOverrideConfigurationType(t *testing.T) {
	data, err := trafficfixture.ReadFile("testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-rstp.pcap")
	require.NoError(t, err)
	require.Equal(t, "25949d0050030be8c08cfb0fbf014019fad8e3a2860809340047ac9b3966d977", fmt.Sprintf("%x", sha256.Sum256(data)))
	r, err := pcapgo.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	frame, _, err := r.ReadPacketData()
	require.NoError(t, err)
	input := frame[17:]
	require.Len(t, input, 35)
	require.Equal(t, []byte{0, 0, 2, 0}, input[:4])
	v, err := ParseStructured(input, "stp", "STPFields")
	require.NoError(t, err)
	m := v["fields"].(map[string]any)["Message"].(map[string]any)
	require.Equal(t, "STP", m["Protocol Name"])
	require.Equal(t, "Configuration", m["Packet Name"])
	require.EqualValues(t, 2, m["Version"])
	require.NotEmpty(t, m["Conformance Warning"])
	require.NotContains(t, m, "Version 1 Length")
	require.NotContains(t, m, "Port Role")
	control := append(bytes.Clone(input), 0)
	control[3] = 2
	v, err = ParseStructured(control, "stp", "STPFields")
	require.NoError(t, err)
	m = v["fields"].(map[string]any)["Message"].(map[string]any)
	require.Equal(t, "RSTP", m["Protocol Name"])
}
