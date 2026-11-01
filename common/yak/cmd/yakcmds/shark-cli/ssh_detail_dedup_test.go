package sharkcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// These owned maps are presentation fixtures. Their equality and ranges are
// tested independently of packet identification and SSH method validation.
func sshDedupDetailFields() map[string]any {
	return map[string]any{
		"Packet Length": uint32(124), "Padding Length": byte(8),
		"Payload": map[string]any{
			"Message Number": byte(31),
			"Host Key": map[string]any{
				"Host Key Format":   "ssh-ed25519",
				"Wire Field Ranges": map[string]any{"Host Key Format": []any{uint64(14), uint64(25)}},
			},
			"Signature": map[string]any{
				"Signature Format":  "ssh-ed25519",
				"Wire Field Ranges": map[string]any{"Signature Format": []any{uint64(105), uint64(116)}},
			},
			"Wire Field Ranges": map[string]any{"Message Number": []any{uint64(5), uint64(6)}},
		},
		"Padding": bytes.Repeat([]byte{0xa5}, 8),
		"Wire Field Ranges": map[string]any{
			"Packet Length":  []any{uint64(0), uint64(4)},
			"Padding Length": []any{uint64(4), uint64(5)},
			"Padding":        []any{uint64(120), uint64(128)},
		},
	}
}

func sshDedupDetailEvent() *pcaputil.ProtocolEvent {
	raw := bytes.Repeat([]byte{0xa5}, 128)
	raw[0], raw[1], raw[2], raw[3], raw[4], raw[5] = 0, 0, 0, 124, 8, 31
	copy(raw[14:25], "ssh-ed25519")
	copy(raw[105:116], "ssh-ed25519")
	return &pcaputil.ProtocolEvent{
		ID: 1, Protocol: "ssh", Transport: "tcp", Status: "decoded", Raw: raw, Length: len(raw),
		Fields: sshDedupDetailFields(),
		Session: map[string]any{
			"Packet Name": "KEX_ECDH_REPLY", "Message Number": byte(31),
			"Identification": "SSH-2.0-retained-context", "Kex Algorithm": "curve25519-sha256",
			"Host Key Format": "ssh-ed25519", "Host Key Fingerprint": "SHA256:presentation-fixture",
			"Initial Packet Fields": sshDedupDetailFields(),
			"Wire Field Ranges": map[string]any{
				"Message Number":  []any{uint64(5), uint64(6)},
				"Host Key Format": []any{uint64(14), uint64(25)},
			},
		},
		SourceBytes: pcaputil.ByteSource{Kind: "reassembled"},
	}
}

func sshDedupCloneValue(v any) any {
	switch v := v.(type) {
	case map[string]any:
		if v == nil {
			return map[string]any(nil)
		}
		out := make(map[string]any, len(v))
		for k, child := range v {
			out[k] = sshDedupCloneValue(child)
		}
		return out
	case []any:
		if v == nil {
			return []any(nil)
		}
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = sshDedupCloneValue(child)
		}
		return out
	case []byte:
		return bytes.Clone(v)
	default:
		return v
	}
}

func sshDedupCloneEvent(e *pcaputil.ProtocolEvent) *pcaputil.ProtocolEvent {
	out := *e
	out.Raw = bytes.Clone(e.Raw)
	out.Fields = sshDedupCloneValue(e.Fields).(map[string]any)
	out.Session = sshDedupCloneValue(e.Session).(map[string]any)
	return &out
}

func sshDedupBranchCount(fields []field, name string) int {
	count := 0
	for _, f := range fields {
		if f.branch && f.text == name {
			count++
		}
	}
	return count
}

func sshDedupLeafCount(fields []field, prefix string) int {
	count := 0
	for _, f := range fields {
		if !f.branch && strings.HasPrefix(f.text, prefix) {
			count++
		}
	}
	return count
}

func sshDedupInitial(e *pcaputil.ProtocolEvent) map[string]any {
	return e.Session["Initial Packet Fields"].(map[string]any)
}

func sshDedupHostKey(values map[string]any) map[string]any {
	return values["Payload"].(map[string]any)["Host Key"].(map[string]any)
}

func TestSSHSamePDUDedupRequiresCompleteTypedEquality(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*pcaputil.ProtocolEvent)
	}{
		{"equal", func(*pcaputil.ProtocolEvent) {}},
		{"integer-width", func(e *pcaputil.ProtocolEvent) { sshDedupInitial(e)["Packet Length"] = uint64(124) }},
		{"integer-float", func(e *pcaputil.ProtocolEvent) { sshDedupInitial(e)["Packet Length"] = float64(124) }},
		{"literal-value", func(e *pcaputil.ProtocolEvent) { sshDedupHostKey(sshDedupInitial(e))["Host Key Format"] = "ssh-ed448" }},
		{"nested-range-position", func(e *pcaputil.ProtocolEvent) {
			sshDedupHostKey(sshDedupInitial(e))["Wire Field Ranges"].(map[string]any)["Host Key Format"] = []any{uint64(15), uint64(26)}
		}},
		{"nested-range-integer-width", func(e *pcaputil.ProtocolEvent) {
			sshDedupHostKey(sshDedupInitial(e))["Wire Field Ranges"].(map[string]any)["Host Key Format"] = []any{uint32(14), uint64(25)}
		}},
		{"missing-range-map", func(e *pcaputil.ProtocolEvent) { delete(sshDedupInitial(e), "Wire Field Ranges") }},
		{"extra-context-in-initial", func(e *pcaputil.ProtocolEvent) { sshDedupInitial(e)["Distinct Context"] = "preserve this subtree" }},
		{"octets-text", func(e *pcaputil.ProtocolEvent) {
			value, err := json.Marshal(sshDedupInitial(e)["Padding"])
			if err != nil {
				panic(err)
			}
			var text string
			if err := json.Unmarshal(value, &text); err != nil {
				panic(err)
			}
			sshDedupInitial(e)["Padding"] = text
		}},
		{"different-clipped-tail", func(e *pcaputil.ProtocolEvent) {
			left, right := bytes.Repeat([]byte{1}, 96), bytes.Repeat([]byte{1}, 96)
			right[95] = 2
			e.Fields["Padding"], sshDedupInitial(e)["Padding"] = left, right
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := sshDedupDetailEvent()
			tc.mutate(e)
			before := sshDedupCloneEvent(e)
			duplicate := tc.name == "equal"
			require.Equal(t, duplicate, duplicateSSHInitialFields(e))
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			wantBodies, wantInitial := 2, 1
			if duplicate {
				wantBodies, wantInitial = 1, 0
			}
			require.Equal(t, wantBodies, sshDedupBranchCount(fields, "Payload"))
			require.Equal(t, wantBodies, sshDedupBranchCount(fields, "Host Key"))
			require.Equal(t, wantBodies, sshDedupLeafCount(fields, "Packet Length:"))
			require.Equal(t, wantInitial, sshDedupBranchCount(fields, "Initial Packet Fields"))
			require.Contains(t, detailLines(fields), "Kex Algorithm: curve25519-sha256")
			require.Contains(t, detailLines(fields), "Host Key Fingerprint: SHA256:presentation-fixture")
			if tc.name == "missing-range-map" {
				require.Len(t, sources, 2)
				require.Contains(t, sources[1].Name, "frame offsets unavailable")
				require.Equal(t, bytes.Repeat([]byte{0xa5}, 8), sources[1].Data, "an unlocated value stays a separate extracted source")
			} else {
				require.Len(t, sources, 1)
			}
			require.Equal(t, e.Raw, sources[0].Data)
			require.True(t, reflect.DeepEqual(before, e), "rendering may not mutate original typed fields/session/bytes")
		})
	}
}

func sshDedupRequireSingleTreeAndAliases(t *testing.T, fields []field) {
	t.Helper()
	require.Equal(t, 1, sshDedupBranchCount(fields, "Payload"))
	require.Equal(t, 1, sshDedupBranchCount(fields, "Host Key"))
	require.Equal(t, 1, sshDedupBranchCount(fields, "Signature"))
	require.Zero(t, sshDedupBranchCount(fields, "Initial Packet Fields"))
	seen := map[string]int{}
	for _, f := range fields {
		for label, span := range map[string][2]int{
			"Packet Length:": {0, 4}, "Message Number:": {5, 6},
			"Host Key Format:": {14, 25}, "Signature Format:": {105, 116},
		} {
			if !strings.HasPrefix(f.text, label) {
				continue
			}
			seen[label]++
			require.True(t, f.hasRange, f.text)
			require.Equal(t, 1, f.source, f.text)
			require.Equal(t, span[0], f.start, f.text)
			require.Equal(t, span[1], f.end, f.text)
		}
		if strings.HasPrefix(f.text, "Identification:") || strings.HasPrefix(f.text, "Kex Algorithm:") || strings.HasPrefix(f.text, "Host Key Fingerprint:") {
			require.False(t, f.hasRange, "retained/derived aliases must not borrow a current-PDU occurrence")
		}
		require.False(t, strings.HasPrefix(f.text, "Wire Field Ranges"), "range metadata must not become user-facing fields")
	}
	require.Equal(t, map[string]int{"Packet Length:": 1, "Message Number:": 2, "Host Key Format:": 2, "Signature Format:": 1}, seen)
}

func TestSSHDetailDedupKeepsCurrentAliasesAndOwnerRanges(t *testing.T) {
	e := sshDedupDetailEvent()
	before := sshDedupCloneEvent(e)
	fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	sshDedupRequireSingleTreeAndAliases(t, fields)
	for _, stream := range []bool{false, true} {
		got := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, stream)
		sshDedupRequireSingleTreeAndAliases(t, got.fields)
		require.Len(t, got.sources, 1)
		require.Equal(t, e.Raw, got.sources[0].Data)
	}
	require.True(t, reflect.DeepEqual(before, e))
	require.IsType(t, uint32(0), e.Fields["Packet Length"])
	require.IsType(t, uint64(0), sshDedupHostKey(e.Fields)["Wire Field Ranges"].(map[string]any)["Host Key Format"].([]any)[0])
}

func TestSSHDisplayCopyCommitsTypedDedupBeforePacketAndStreamJSON(t *testing.T) {
	for _, kind := range []string{"equal", "integer-width", "integer-float", "range-integer-width", "different-value", "different-range"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", kind, stream), func(t *testing.T) {
				e := sshDedupDetailEvent()
				switch kind {
				case "integer-width":
					sshDedupInitial(e)["Packet Length"] = uint64(124)
				case "integer-float":
					sshDedupInitial(e)["Packet Length"] = float64(124)
				case "range-integer-width":
					sshDedupHostKey(sshDedupInitial(e))["Wire Field Ranges"].(map[string]any)["Host Key Format"] = []any{uint32(14), uint64(25)}
				case "different-value":
					sshDedupHostKey(sshDedupInitial(e))["Host Key Format"] = "ssh-ed448"
				case "different-range":
					sshDedupHostKey(sshDedupInitial(e))["Wire Field Ranges"].(map[string]any)["Host Key Format"] = []any{uint64(15), uint64(26)}
				}
				before := sshDedupCloneEvent(e)
				display := protocolDisplayEvent(e)
				_, retained := display.Session["Initial Packet Fields"]
				require.Equal(t, kind != "equal", retained)
				require.NotNil(t, e.Session["Initial Packet Fields"], "only the outgoing display copy may lose a redundant subtree")
				request := decodeRequest{Number: 1, Events: []*pcaputil.ProtocolEvent{e}}
				if stream {
					request.Events, request.Stream = nil, &streamDecodeRequest{ID: 1, Events: []*pcaputil.ProtocolEvent{e}}
				}
				wire, err := marshalDecodeRequest(request)
				require.NoError(t, err)
				var projected decodeRequest
				decoder := json.NewDecoder(bytes.NewReader(wire))
				decoder.UseNumber()
				require.NoError(t, decoder.Decode(&projected))
				events := projected.Events
				if stream {
					events = projected.Stream.Events
				}
				require.Len(t, events, 1)
				require.NoError(t, restoreWorkerByteFields(events, projected.ByteFields))
				_, retained = events[0].Session["Initial Packet Fields"]
				require.Equal(t, kind != "equal", retained)
				if kind == "integer-width" || kind == "integer-float" || kind == "range-integer-width" {
					require.True(t, duplicateSSHInitialFields(events[0]), "JSON has erased the original type difference; a worker must not reconsider the parent's decision")
				}
				workerFields, workerSources := protocolEventDetail(events, false)
				got := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, stream)
				wantBodies := 2
				if kind == "equal" {
					wantBodies = 1
				}
				for _, fs := range [][]field{workerFields, got.fields} {
					require.Equal(t, wantBodies, sshDedupBranchCount(fs, "Payload"))
					require.Equal(t, wantBodies, sshDedupLeafCount(fs, "Packet Length:"))
				}
				require.Len(t, workerSources, 1)
				require.Len(t, got.sources, 1)
				require.Equal(t, e.Raw, workerSources[0].Data)
				require.Equal(t, e.Raw, got.sources[0].Data)
				require.True(t, reflect.DeepEqual(before, e), "projection, byte restoration and workers may not mutate native DTOs")
			})
		}
	}
}

func TestSSHWorkerWithoutParentProofKeepsJSONEqualInitialTree(t *testing.T) {
	e := sshDedupDetailEvent()
	before := sshDedupCloneEvent(e)
	for _, stream := range []bool{false, true} {
		request := decodeRequest{Number: 1, Events: []*pcaputil.ProtocolEvent{e}}
		if stream {
			request.Events, request.Stream = nil, &streamDecodeRequest{ID: 1, Events: []*pcaputil.ProtocolEvent{e}}
		}
		// Deliberately bypass parent projection. JSON equality cannot establish
		// whether the original owned Go maps had the same integer/byte types.
		wire, err := json.Marshal(request)
		require.NoError(t, err)
		var output bytes.Buffer
		require.NoError(t, runDecodeWorker(bytes.NewReader(wire), &output))
		got, err := decodeWorkerOutput(output.Bytes(), &capturedPacket{number: 1})
		require.NoError(t, err)
		require.Equal(t, 2, sshDedupBranchCount(got.fields, "Payload"))
		require.Equal(t, 2, sshDedupLeafCount(got.fields, "Packet Length:"))
	}
	require.True(t, reflect.DeepEqual(before, e))
}

func TestSSHDisplayDedupLeavesNilAndOtherProtocolEvidence(t *testing.T) {
	require.False(t, duplicateSSHInitialFields(nil))
	require.Nil(t, protocolDisplayEvent(nil))
	for _, protocol := range []string{"", "tls", "http3", "dns", "SSH"} {
		e := sshDedupDetailEvent()
		e.Protocol = protocol
		before := sshDedupCloneEvent(e)
		require.False(t, duplicateSSHInitialFields(e))
		require.Contains(t, protocolDisplayEvent(e).Session, "Initial Packet Fields")
		require.True(t, reflect.DeepEqual(before, e))
	}
	for _, kind := range []string{"nil-fields", "empty-fields", "nil-session", "absent-initial", "nil-initial", "wrong-initial-type"} {
		t.Run(kind, func(t *testing.T) {
			e := sshDedupDetailEvent()
			switch kind {
			case "nil-fields":
				e.Fields = nil
			case "empty-fields":
				e.Fields = map[string]any{}
			case "nil-session":
				e.Session = nil
			case "absent-initial":
				delete(e.Session, "Initial Packet Fields")
			case "nil-initial":
				e.Session["Initial Packet Fields"] = map[string]any(nil)
			case "wrong-initial-type":
				e.Session["Initial Packet Fields"] = "owned text, not a field map"
			}
			before := sshDedupCloneEvent(e)
			require.False(t, duplicateSSHInitialFields(e))
			_ = protocolDisplayEvent(e)
			require.True(t, reflect.DeepEqual(before, e))
		})
	}
}

func TestSSHDetailDedupDoesNotMergeDistinctPDUsOrByteSources(t *testing.T) {
	first, second := sshDedupDetailEvent(), sshDedupDetailEvent()
	first.SourceBytes.Kind = "captured"
	second.ID = 2
	second.Raw[127] ^= 1
	second.Fields["Padding"] = bytes.Clone(second.Raw[120:])
	sshDedupInitial(second)["Padding"] = bytes.Clone(second.Raw[120:])
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{first, second})
	require.Equal(t, 2, sshDedupBranchCount(fields, "Payload"), "one body remains for each distinct PDU")
	require.Equal(t, 2, sshDedupLeafCount(fields, "Packet Length:"))
	require.Len(t, sources, 2)
	require.Equal(t, first.Raw, sources[0].Data)
	require.Equal(t, second.Raw, sources[1].Data)
	seen := map[int]int{}
	for _, f := range fields {
		if strings.HasPrefix(f.text, "Signature Format:") {
			require.True(t, f.hasRange)
			require.Equal(t, 105, f.start)
			require.Equal(t, 116, f.end)
			seen[f.source]++
		}
	}
	require.Equal(t, map[int]int{1: 1, 2: 1}, seen, "same named/literal fields keep their own PDU source")
}
