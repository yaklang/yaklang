package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func smbBindingLeaves(value any, key string) []any {
	var result []any
	switch value := value.(type) {
	case map[string]any:
		for name, child := range value {
			if name == key {
				result = append(result, child)
			}
			result = append(result, smbBindingLeaves(child, key)...)
		}
	case []any:
		for _, child := range value {
			result = append(result, smbBindingLeaves(child, key)...)
		}
	}
	return result
}

func TestSMBExplicitLegacyBindingRetainsCanonicalFields(t *testing.T) {
	// A complete SMB_COM_TREE_DISCONNECT is deliberately outside the native
	// request-only NEGOTIATE profile. An explicit legacy rule still owns it.
	wire := make([]byte, 35)
	copy(wire, []byte{0xff, 'S', 'M', 'B', 0x71})
	binary.LittleEndian.PutUint16(wire[30:32], 0x1379)
	for _, deferred := range []bool{false, true} {
		for _, workers := range []int{1, 3} {
			t.Run(fmt.Sprintf("deferred%v/workers%d", deferred, workers), func(t *testing.T) {
				var events []*ProtocolEvent
				cfg := BinParserConfig{Deferred: deferred, OnEvent: func(e *ProtocolEvent) { events = append(events, e) }}
				cfg.Bindings = []BinParserBinding{{
					Port: 34567, Protocol: "smb", Rule: "application-layer.smb", Entry: "SMB",
					Probe: func(p []byte) bool { return bytes.HasPrefix(p, wire[:4]) },
					Frame: func(p []byte) (int, error) { return len(wire), nil },
				}}
				pcap := binTestPcap(t, []tcpStep{{seq: 0, syn: true}, {seq: 1, data: string(wire)}}, 34567, false, false)
				require.NoError(t, ReplayPcap(bytes.NewReader(pcap), WithBinParserConfig(cfg), WithTCPReassemblyWorkers(workers)))
				require.Len(t, events, 1)
				event := events[0]
				require.False(t, event.nativeSessionMessage)
				fields, err := event.GetFields()
				require.NoError(t, err)
				require.NotEmpty(t, fields)
				commands, mids := smbBindingLeaves(fields, "Command"), smbBindingLeaves(fields, "MID")
				require.Len(t, commands, 1)
				require.Len(t, mids, 1)
				require.EqualValues(t, 0x71, commands[0])
				require.EqualValues(t, 0x1379, mids[0])
				encoded, err := json.Marshal(fields)
				require.NoError(t, err)
				var decoded map[string]any
				require.NoError(t, json.Unmarshal(encoded, &decoded))
				require.EqualValues(t, 0x71, smbBindingLeaves(decoded, "Command")[0])
				require.EqualValues(t, 0x1379, smbBindingLeaves(decoded, "MID")[0])
			})
		}
	}
}
