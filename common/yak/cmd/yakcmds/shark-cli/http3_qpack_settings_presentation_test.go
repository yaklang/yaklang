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

func TestHTTP3RecoveredQPACKEncoderKeepsLogicalBytesAcrossWorker(t *testing.T) {
	for _, worker := range []bool{false, true} {
		for _, authenticated := range []bool{false, true} {
			for _, decoded := range []bool{false, true} {
				t.Run(fmt.Sprintf("worker%v/auth%v/decoded%v", worker, authenticated, decoded), func(t *testing.T) {
					status := "incomplete"
					if decoded {
						status = "decoded"
					}
					session := map[string]any{"Authentication Verified": authenticated, "HTTP3 Stream ID": uint64(3),
						"QPACK Resumed Encoders": []map[string]any{{"HTTP3 Stream ID": uint64(7), "Status": status, "Instruction Bytes": []byte{0x3f, 0x21},
							"Byte Source": map[string]any{"Offset": uint64(1), "Length": uint64(2), "Parent PDUs": []uint64{11}}}},
						"Unrelated": map[string]any{"Instruction Bytes": []byte{9, 8}}}
					if worker {
						wire, err := json.Marshal(session)
						require.NoError(t, err)
						decoder := json.NewDecoder(bytes.NewReader(wire))
						decoder.UseNumber()
						require.NoError(t, decoder.Decode(&session))
					}
					e := &pcaputil.ProtocolEvent{ID: 12, Protocol: "http3", Status: "decoded", Raw: []byte("later encrypted SETTINGS packet"), Session: session}
					fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
					found := 0
					for _, field := range fields {
						if strings.HasPrefix(field.text, "Instruction Bytes:") && field.hasRange {
							found++
							require.True(t, authenticated && decoded)
							require.Greater(t, field.source, 0)
							source := sources[field.source-1]
							require.Equal(t, "HTTP/3 QPACK encoder · retained stream 7 · SETTINGS is context", source.Name)
							require.Equal(t, []byte{0x3f, 0x21}, source.Data[field.start:field.end])
							require.NotEqual(t, e.Raw, source.Data)
						}
					}
					want := 0
					if authenticated && decoded {
						want = 1
					}
					require.Equal(t, want, found)
				})
			}
		}
	}
}

func TestHTTP3RecoveredQPACKInstructionsPrecedeLargeProvenance(t *testing.T) {
	ranges := make([]any, 2000)
	for i := range ranges {
		ranges[i] = map[string]any{"Offset": uint64(i), "Parent PDU": uint64(i + 1)}
	}
	e := &pcaputil.ProtocolEvent{Protocol: "http3", Status: "decoded", Session: map[string]any{"Authentication Verified": true,
		"QPACK Resumed Encoders": []map[string]any{{"HTTP3 Stream ID": uint64(7), "Status": "decoded", "Instruction Bytes": []byte{0x3f, 0x21},
			"QPACK Instructions": []map[string]any{{"Instruction": "Set Dynamic Table Capacity", "Capacity": uint64(64)}}, "Byte Source": map[string]any{"Ranges": ranges}}}}}
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	index := -1
	for i, field := range fields {
		if strings.HasPrefix(field.text, "Instruction Bytes:") {
			index = i
			require.True(t, field.hasRange)
			require.Equal(t, []byte{0x3f, 0x21}, sources[field.source-1].Data)
		}
	}
	require.GreaterOrEqual(t, index, 0)
	require.Less(t, index, 30, "instruction semantics and bytes must precede the potentially truncated witness tree")
}

func TestHTTP3QPACKNestedWitnessCannotAuthorizeInstructionBytes(t *testing.T) {
	for _, worker := range []bool{false, true} {
		t.Run(fmt.Sprintf("worker%v", worker), func(t *testing.T) {
			session := map[string]any{"Authentication Verified": true, "QPACK Resumed Encoders": []map[string]any{{"Status": "decoded", "HTTP3 Stream ID": uint64(7), "Instruction Bytes": []byte{0x3f, 0x21}, "Byte Source": map[string]any{"Instruction Bytes": []byte{9, 8, 7}}}}}
			if worker {
				wire, err := json.Marshal(session)
				require.NoError(t, err)
				decoder := json.NewDecoder(bytes.NewReader(wire))
				decoder.UseNumber()
				require.NoError(t, decoder.Decode(&session))
			}
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{{Protocol: "http3", Status: "decoded", Session: session}})
			ranged := 0
			for _, field := range fields {
				if strings.HasPrefix(field.text, "Instruction Bytes:") && field.hasRange {
					ranged++
					require.Equal(t, []byte{0x3f, 0x21}, sources[field.source-1].Data)
				}
			}
			require.Equal(t, 1, ranged, "a same-named value inside a witness is context, not a decoded instruction")
		})
	}
}
