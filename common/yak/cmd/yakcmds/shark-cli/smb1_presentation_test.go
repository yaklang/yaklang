package sharkcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestSMB1DialectOccurrencesKeepTheirOwnWireScope(t *testing.T) {
	for _, throughJSON := range []bool{false, true} {
		t.Run(fmt.Sprint(throughJSON), func(t *testing.T) {
			raw := make([]byte, 50)
			copy(raw[40:43], "SMB")
			copy(raw[45:48], "SMB")
			owner := func(start uint64) map[string]any {
				return map[string]any{"Name": "SMB", "Raw Name Bytes": []byte("SMB"), "NUL Terminator": uint8(0), "Wire Field Ranges": map[string]any{
					"Name": []any{start, start + 3}, "Raw Name Bytes": []any{start, start + 3}, "NUL Terminator": []any{start + 3, start + 4},
				}}
			}
			e := &pcaputil.ProtocolEvent{ID: 1, Protocol: "smb", Transport: "tcp", Status: "decoded", Raw: raw, Length: len(raw),
				Fields:  map[string]any{"Dialects": []any{owner(40), owner(45)}, "Unlocated Context": map[string]any{"Name": "SMB"}},
				Session: map[string]any{"Packet Name": "SMB_COM_NEGOTIATE", "SMB2 Upgrade Offered": true},
			}
			if throughJSON {
				encoded, err := json.Marshal(e)
				require.NoError(t, err)
				var restored pcaputil.ProtocolEvent
				require.NoError(t, json.Unmarshal(encoded, &restored))
				e = &restored
			}
			row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "TCP", Layers: []string{"Ethernet", "IPv6", "TCP"}})
			require.Equal(t, "SMB1", row.Protocol)
			require.Equal(t, []string{"Ethernet", "IPv6", "TCP", "SMB1"}, row.ProtocolStack)
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			require.Len(t, sources, 1)
			require.Equal(t, raw, sources[0].Data)
			var starts []int
			unlocated := 0
			for _, f := range fields {
				require.NotContains(t, f.text, "Wire Field Ranges")
				if !strings.HasPrefix(f.text, "Name:") {
					continue
				}
				if f.hasRange {
					starts = append(starts, f.start)
					require.Equal(t, 3, f.end-f.start)
					require.True(t, bytes.Equal([]byte("SMB"), sources[f.source-1].Data[f.start:f.end]))
				} else {
					unlocated++
				}
			}
			require.Equal(t, []int{40, 45}, starts)
			require.Equal(t, 1, unlocated)
		})
	}
}
