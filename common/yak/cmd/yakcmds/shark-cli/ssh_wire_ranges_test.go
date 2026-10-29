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

func sshDetailRangeOwner(values map[string]any, name string, start, end int) map[string]any {
	values["Wire Field Ranges"] = map[string]any{name: []any{uint64(start), uint64(end)}}
	return values
}

func TestSSHDetailRangesKeepNestedOwnersAndSessionOverridesThroughJSON(t *testing.T) {
	for _, throughJSON := range []bool{false, true} {
		t.Run(fmt.Sprint(throughJSON), func(t *testing.T) {
			wire := bytes.Repeat([]byte{0x58}, 192)
			copy(wire[14:25], "ssh-ed25519")
			copy(wire[105:116], "ssh-ed25519")
			e := &pcaputil.ProtocolEvent{
				ID: 21, Protocol: "ssh", Transport: "tcp", Status: "decoded", Raw: wire, Length: len(wire),
				Fields: map[string]any{
					"Packet Length": uint32(188), "Encryption C2S": "offer",
					"Wire Field Ranges": map[string]any{"Packet Length": []any{uint64(0), uint64(4)}, "Encryption C2S": []any{uint64(26), uint64(31)}},
					"Payload": map[string]any{
						"Message Number": byte(31), "Encryption C2S": "offer",
						"Wire Field Ranges": map[string]any{"Message Number": []any{uint64(5), uint64(6)}, "Encryption C2S": []any{uint64(26), uint64(31)}},
						"Host Key":          sshDetailRangeOwner(map[string]any{"Host Key Format": "ssh-ed25519"}, "Host Key Format", 14, 25),
						"Signature":         sshDetailRangeOwner(map[string]any{"Signature Format": "ssh-ed25519"}, "Signature Format", 105, 116),
						"Unlocated Context": map[string]any{"Message Number": byte(31)},
					},
				},
				Session: map[string]any{"Packet Name": "KEX_ECDH_REPLY", "Encryption C2S": "selected", "Identification": "SSH-2.0-earlier", "Host Key Format": "ssh-ed25519", "Wire Field Ranges": map[string]any{"Host Key Format": []any{uint64(14), uint64(25)}}},
			}
			if throughJSON {
				encoded, err := json.Marshal(e)
				require.NoError(t, err)
				var restored pcaputil.ProtocolEvent
				require.NoError(t, json.Unmarshal(encoded, &restored))
				e = &restored
			}
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			require.Len(t, sources, 1, "literal occurrences should share their owned complete PDU source")
			require.Equal(t, wire, sources[0].Data)
			seen := map[string]int{}
			messageRanges, messageContexts := 0, 0
			for _, f := range fields {
				if strings.HasPrefix(f.text, "Wire Field Ranges") {
					t.Fatal("range metadata must not become user-facing protocol fields")
				}
				for label, span := range map[string][2]int{
					"Packet Length:": {0, 4}, "Host Key Format:": {14, 25}, "Signature Format:": {105, 116}, "Encryption C2S: offer": {26, 31},
				} {
					if strings.HasPrefix(f.text, label) {
						seen[label]++
						require.True(t, f.hasRange, f.text)
						require.Equal(t, 1, f.source, f.text)
						require.Equal(t, span[0], f.start, f.text)
						require.Equal(t, span[1], f.end, f.text)
					}
				}
				if strings.HasPrefix(f.text, "Message Number:") {
					if f.hasRange {
						messageRanges++
						require.Equal(t, 5, f.start)
						require.Equal(t, 6, f.end)
					} else {
						messageContexts++
					}
				}
				if strings.HasPrefix(f.text, "Encryption C2S: selected") || strings.HasPrefix(f.text, "Identification:") {
					require.False(t, f.hasRange, "selected or carried context must not borrow a wire offer")
					seen["context"]++
				}
				if f.branch && (f.text == "Payload" || f.text == "Host Key" || f.text == "Signature") {
					require.False(t, f.hasRange, "a composite has no invented aggregate literal span")
				}
			}
			require.Equal(t, map[string]int{"Packet Length:": 1, "Host Key Format:": 2, "Signature Format:": 1, "Encryption C2S: offer": 1, "context": 2}, seen)
			require.Equal(t, 1, messageRanges)
			require.Equal(t, 1, messageContexts)
		})
	}
}

func TestSSHDetailRangesRejectInvalidOrAbsentOccurrences(t *testing.T) {
	for _, span := range []any{nil, []any{0, 0}, []any{-1, 2}, []any{1, 65}, []any{2, 1}, []any{1.5, 3.0}, []any{1}, []any{1, 2, 3}} {
		t.Run(fmt.Sprint(span), func(t *testing.T) {
			e := &pcaputil.ProtocolEvent{ID: 1, Protocol: "ssh", Transport: "tcp", Status: "decoded", Raw: make([]byte, 64), Length: 64,
				Fields: map[string]any{"Payload": map[string]any{"Message Number": byte(20), "Wire Field Ranges": map[string]any{"Message Number": span}}},
			}
			fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			seen := false
			for _, f := range fields {
				if strings.HasPrefix(f.text, "Message Number:") {
					seen = true
					require.False(t, f.hasRange, "zero-width/invalid ranges cannot highlight neighbouring bytes")
				}
			}
			require.True(t, seen)
		})
	}
}
