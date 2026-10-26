package sharkcli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestNTPControlIPCFieldsAndReassembledByteArena(t *testing.T) {
	e := &pcaputil.ProtocolEvent{ID: 1, Protocol: "ntp", Transport: "udp", Status: "decoded", Raw: []byte{0x16, 0x82, 0, 7, 0, 0, 0, 0, 0, 3, 0, 3, 'd', 'e', 'f'}, Session: map[string]any{"Packet Name": "Control", "Sequence Number": uint16(7), "Fragment Offset": uint16(3), "Data Count": uint16(3), "Control Data": []byte("def"), "Reassembled Control Data": []byte("abcdef"), "Wire Field Ranges": map[string]any{"Sequence Number": []any{2, 4}, "Control Data": []any{12, 15}}}}
	wire, err := json.Marshal(e)
	require.NoError(t, err)
	var snapshot pcaputil.ProtocolEvent
	require.NoError(t, json.Unmarshal(wire, &snapshot))
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{&snapshot})
	foundSequence, foundFragment, foundAssembly := false, false, false
	for _, f := range fields {
		switch {
		case strings.HasPrefix(f.text, "Sequence Number:"):
			require.True(t, f.hasRange)
			require.Equal(t, 2, f.start)
			require.Equal(t, 4, f.end)
			foundSequence = true
		case strings.HasPrefix(f.text, "Control Data:"):
			require.Contains(t, f.text, "64 65 66")
			require.True(t, f.hasRange)
			require.Equal(t, 12, f.start)
			require.Equal(t, 15, f.end)
			require.Equal(t, e.Raw, sources[f.source-1].Data)
			foundFragment = true
		case strings.HasPrefix(f.text, "Reassembled Control Data:"):
			require.True(t, f.hasRange)
			require.Equal(t, []byte("abcdef"), sources[f.source-1].Data)
			require.Equal(t, 0, f.start)
			require.Equal(t, 6, f.end)
			require.Contains(t, sources[f.source-1].Name, "reassembled")
			foundAssembly = true
		}
	}
	require.True(t, foundSequence)
	require.True(t, foundFragment)
	require.True(t, foundAssembly)
}
