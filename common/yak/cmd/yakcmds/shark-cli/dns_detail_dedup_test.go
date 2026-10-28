package sharkcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func dnsDetailMap() map[string]any {
	return map[string]any{"ID": uint16(17), "Flags": uint16(256), "Name Wire": []byte{3, 'w', 'w', 'w', 0}, "Answers": []map[string]any(nil), "Empty Bytes": []byte{}}
}

func dnsDetailEvent() *pcaputil.ProtocolEvent {
	return &pcaputil.ProtocolEvent{ID: 1, Protocol: "dns", Status: "decoded", Raw: []byte{0, 17, 1, 0}, Fields: dnsDetailMap(), Session: map[string]any{"DNS": dnsDetailMap(), "Association Status": "observed"}, SourceBytes: pcaputil.ByteSource{Kind: "captured"}}
}

func dnsDetailBranches(fields []field) int {
	count := 0
	for _, f := range fields {
		if f.branch && f.text == "DNS" {
			count++
		}
	}
	return count
}

func TestDNSSamePDUDisplayDedupRequiresCompleteTypedEquality(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*pcaputil.ProtocolEvent)
	}{
		{"equal", func(*pcaputil.ProtocolEvent) {}},
		{"integer-width", func(e *pcaputil.ProtocolEvent) { e.Session["DNS"].(map[string]any)["ID"] = uint32(17) }},
		{"integer-float", func(e *pcaputil.ProtocolEvent) { e.Session["DNS"].(map[string]any)["ID"] = float64(17) }},
		{"integer-text", func(e *pcaputil.ProtocolEvent) { e.Session["DNS"].(map[string]any)["ID"] = "17" }},
		{"nil-empty", func(e *pcaputil.ProtocolEvent) { e.Session["DNS"].(map[string]any)["Answers"] = []map[string]any{} }},
		{"nil-empty-octets", func(e *pcaputil.ProtocolEvent) { e.Session["DNS"].(map[string]any)["Empty Bytes"] = []byte(nil) }},
		{"octets-text", func(e *pcaputil.ProtocolEvent) { e.Session["DNS"].(map[string]any)["Name Wire"] = "A3d3dwA=" }},
		{"extra-context", func(e *pcaputil.ProtocolEvent) { e.Session["DNS"].(map[string]any)["Additional Context"] = "distinct" }},
		{"different-clipped-tail", func(e *pcaputil.ProtocolEvent) {
			left, right := bytes.Repeat([]byte{1}, 96), bytes.Repeat([]byte{1}, 96)
			right[95] = 2
			e.Fields["Name Wire"], e.Session["DNS"].(map[string]any)["Name Wire"] = left, right
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := dnsDetailEvent()
			tc.mutate(e)
			before, err := json.Marshal(e)
			require.NoError(t, err)
			duplicate := tc.name == "equal"
			require.Equal(t, duplicate, duplicateDNSSessionFields(e))
			fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			expected := 1
			if duplicate {
				expected = 0
			}
			require.Equal(t, expected, dnsDetailBranches(fields))
			require.Contains(t, detailLines(fields), "Association Status: observed", "distinct retained context survives display dedup")
			after, err := json.Marshal(e)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestDNSDisplayCopyPreservesTypeDifferencesAcrossPacketAndStreamIPC(t *testing.T) {
	for _, distinct := range []string{"equal", "integer-width", "nil-empty", "octets-text"} {
		for _, stream := range []bool{false, true} {
			e := dnsDetailEvent()
			dns := e.Session["DNS"].(map[string]any)
			switch distinct {
			case "integer-width":
				dns["ID"] = uint32(17)
			case "nil-empty":
				dns["Answers"] = []map[string]any{}
			case "octets-text":
				dns["Name Wire"] = "A3d3dwA="
			}
			request := decodeRequest{Number: 1, Events: []*pcaputil.ProtocolEvent{e}}
			if stream {
				request.Events, request.Stream = nil, &streamDecodeRequest{ID: 1, Events: []*pcaputil.ProtocolEvent{e}}
			}
			before, err := json.Marshal(e)
			require.NoError(t, err)
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
			require.NoError(t, restoreWorkerByteFields(events, projected.ByteFields))
			_, retained := events[0].Session["DNS"]
			require.Equal(t, distinct != "equal", retained)
			require.Equal(t, []byte{3, 'w', 'w', 'w', 0}, events[0].Fields["Name Wire"])
			if distinct != "equal" && distinct != "octets-text" {
				require.Equal(t, []byte{3, 'w', 'w', 'w', 0}, events[0].Session["DNS"].(map[string]any)["Name Wire"])
			}
			got := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, stream)
			expected := 1
			if distinct == "equal" {
				expected = 0
			}
			require.Equal(t, expected, dnsDetailBranches(got.fields), "child must not repeat equality after JSON folds numeric widths")
			require.Equal(t, e.Raw, got.sources[0].Data)
			after, err := json.Marshal(e)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.IsType(t, uint16(0), e.Fields["ID"])
		}
	}
}

func TestManualWorkerRequestsKeepUnprovedDNSMapEquality(t *testing.T) {
	e := dnsDetailEvent()
	for _, stream := range []bool{false, true} {
		request := decodeRequest{Number: 1, Events: []*pcaputil.ProtocolEvent{e}}
		if stream {
			request.Events, request.Stream = nil, &streamDecodeRequest{ID: 1, Events: []*pcaputil.ProtocolEvent{e}}
		}
		// Deliberately bypass the parent's exact-typed display projection. A
		// plain JSON request cannot prove the original integer/byte map types.
		wire, err := json.Marshal(request)
		require.NoError(t, err)
		var output bytes.Buffer
		require.NoError(t, runDecodeWorker(bytes.NewReader(wire), &output))
		got, err := decodeWorkerOutput(output.Bytes(), &capturedPacket{number: 1})
		require.NoError(t, err)
		require.Equal(t, 1, dnsDetailBranches(got.fields))
	}
}

func TestDNSDisplayDedupDoesNotCrossPDUOrCapturedByteSources(t *testing.T) {
	first, second := dnsDetailEvent(), dnsDetailEvent()
	second.ID, second.SourceBytes.Kind, second.Raw = 2, "reassembled", []byte{0, 17, 1, 0, 1}
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{first, second})
	text := detailLines(fields)
	require.Contains(t, text, "PDU #1 · captured")
	require.Contains(t, text, "PDU #2 · reassembled")
	require.Equal(t, 2, strings.Count(text, "ID: 17"), "one complete message tree remains for each distinct PDU")
	for _, raw := range [][]byte{first.Raw, second.Raw} {
		found := false
		for _, source := range sources {
			found = found || bytes.Equal(raw, source.Data)
		}
		require.True(t, found)
	}
	packet := vxlanHeaderDetailPacket(t, layers.LinkTypeEthernet, 0x88)
	for _, e := range packet.events {
		f, err := e.GetFields()
		require.NoError(t, err)
		e.Fields = f // Hydrate the same owned public detail snapshot used by the inspector.
	}
	for _, got := range []packetDetail{detail(packet), workerMetadataDetailForPacket(t, packet)} {
		captured, logical := false, false
		for _, f := range got.fields {
			if f.text == "DNS" && f.hasRange {
				require.Zero(t, f.source)
				require.Equal(t, 92, f.start)
				require.Equal(t, 121, f.end)
				captured = true
			}
			if strings.HasPrefix(f.text, "DNS · PDU #") && f.hasRange {
				require.Positive(t, f.source)
				logical = true
			}
		}
		require.True(t, captured, "captured-frame YAML fields retain their actual Source0 bytes")
		require.True(t, logical, "the logical PDU byte source is preserved independently")
	}
}

func workerMetadataDetailForPacket(t *testing.T, packet *capturedPacket) packetDetail {
	t.Helper()
	wire, err := marshalDecodeRequest(packetDecodeRequest(packet))
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(wire), &output))
	got, err := decodeWorkerOutput(output.Bytes(), packet)
	require.NoError(t, err)
	return got
}

func TestDNSDisplayCopyRetainsResourceMarkersAndEventIndices(t *testing.T) {
	oversized := &pcaputil.ProtocolEvent{ID: 9, Protocol: "ssh", Status: "decoded", Session: map[string]any{"Detail": strings.Repeat("x", decoderInputLimit)}}
	dns := dnsDetailEvent()
	wire, err := marshalDecodeRequest(decodeRequest{Number: 1, Events: []*pcaputil.ProtocolEvent{nil, oversized, dns}})
	require.NoError(t, err)
	var request decodeRequest
	require.NoError(t, json.Unmarshal(wire, &request))
	require.Len(t, request.Events, 3)
	require.Nil(t, request.Events[0])
	require.Equal(t, uint64(9), request.Events[1].ID)
	require.Equal(t, "limited", request.Events[1].Status)
	require.Contains(t, request.Events[1].Summary, "IPC byte/type projection limit")
	require.Equal(t, uint64(1), request.Events[2].ID)
	for _, descriptor := range request.ByteFields {
		require.Equal(t, 2, descriptor.Event)
	}
	require.NoError(t, restoreWorkerByteFields(request.Events, request.ByteFields))
	require.Equal(t, []byte{3, 'w', 'w', 'w', 0}, request.Events[2].Fields["Name Wire"])
	require.NotNil(t, dns.Session["DNS"], "the original native snapshot still owns its complete context")
}
