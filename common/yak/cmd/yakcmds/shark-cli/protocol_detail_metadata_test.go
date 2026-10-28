package sharkcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func detailLines(fields []field) string {
	var text strings.Builder
	for _, f := range fields {
		text.WriteString(f.text + "\n")
	}
	return text.String()
}

func workerMetadataDetail(t *testing.T, events []*pcaputil.ProtocolEvent, stream bool) packetDetail {
	t.Helper()
	request := decodeRequest{Number: 1, Events: events}
	if stream {
		request.Events, request.Stream = nil, &streamDecodeRequest{ID: 1, Events: events}
	}
	wire, err := marshalDecodeRequest(request)
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(wire), &output))
	got, err := decodeWorkerOutput(output.Bytes(), &capturedPacket{number: 1})
	require.NoError(t, err)
	return got
}

func TestConnectionObservationPrecedesWireTreesWithoutInventingByteSources(t *testing.T) {
	for _, protocol := range []string{"tls", "quic", "http3", "doq"} {
		t.Run(protocol, func(t *testing.T) {
			context := map[string]any{"Evidence Scope": "current connection hello observations", "Session ID Echo": true, "Session ID Resumption Indicated": true, "PSK Selected": true, "PSK Selection": map[string]any{"Binder Verified": false, "Ticket Association": "unconfirmed", "Selected Identity": uint16(0)}, "Certificate Trust": "not-evaluated", "Record Type": uint8(23), "Authenticated Plaintext": []byte{1, 2, 3}, "GRPC Messages": []map[string]any{{"Payload": []byte{4, 5, 6}}}, "HTTP3 Frames": []map[string]any{{"Frame Type": "DATA", "Frame Header Bytes": []byte{0, 1}, "Payload": []byte{7, 8, 9}}}}
			e := &pcaputil.ProtocolEvent{ID: 1, Protocol: protocol, Status: "decoded", Raw: []byte{23, 3, 3, 0, 1, 42}, Fields: map[string]any{"ZZZ remainder": "still visible"}, Session: map[string]any{"Session Establishment": context, "Authentication Verified": true, "Record Type": uint8(23), "Record Version": uint16(0x303), "Record Length": uint16(1), "Wire Field Ranges": map[string][]int{"Record Type": {0, 1}, "Record Version": {1, 3}, "Record Length": {3, 5}}}}
			before, err := json.Marshal(e)
			require.NoError(t, err)
			localFields, localSources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			views := []packetDetail{{fields: localFields, sources: localSources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)}
			for _, got := range views {
				require.Len(t, got.sources, 1, "metadata keys cannot create gRPC/HTTP3/plaintext byte sources")
				require.Equal(t, e.Raw, got.sources[0].Data)
				start, other := -1, -1
				inContext := false
				for i, f := range got.fields {
					if strings.HasPrefix(f.text, "Session Establishment ·") {
						start, inContext = i, true
					} else if inContext && f.depth == 0 {
						inContext = false
					}
					if inContext {
						require.False(t, f.hasRange, f.text)
						require.Zero(t, f.source, "retained context is not a new wire/cipher byte span")
					}
					if f.text == "Other decoded fields" && other == -1 {
						other = i
					}
				}
				require.GreaterOrEqual(t, start, 0)
				require.Greater(t, other, start)
				text := detailLines(got.fields)
				require.Contains(t, text, "Session ID Resumption Indicated: true")
				require.Contains(t, text, "Binder Verified: false")
				require.Contains(t, text, "Certificate Trust: not-evaluated")
				require.Contains(t, text, "no new handshake or authentication result implied")
				require.Contains(t, text, "ZZZ remainder: still visible")
				if protocol == "tls" {
					wire := -1
					found := false
					for i, f := range got.fields {
						if f.text == "TLS record and observed context" {
							wire = i
							require.Zero(t, f.depth, "wire record is a sibling of retained connection metadata")
						}
						if strings.HasPrefix(f.text, "Record Version:") && f.hasRange {
							found = true
							require.Greater(t, i, wire)
							require.Positive(t, f.depth, "actual record fields are nested below the wire heading")
							require.Equal(t, 1, f.source)
							require.Equal(t, 1, f.start)
							require.Equal(t, 3, f.end)
						}
					}
					require.Greater(t, wire, start)
					require.True(t, found, "actual TLS record header spans remain available")
				}
			}
			after, err := json.Marshal(e)
			require.NoError(t, err)
			require.Equal(t, before, after, "rendering and IPC must not mutate original events/context/types")
			require.IsType(t, uint16(0), context["PSK Selection"].(map[string]any)["Selected Identity"])
		})
	}
}

func TestConnectionObservationWalkerReportsBoundsAndKeepsNonFamilyFields(t *testing.T) {
	context := map[string]any{"Evidence Scope": "connection", "Data": bytes.Repeat([]byte{0xff}, 128), "List": []any{1, 2, 3, 4}}
	for _, budget := range []int{0, 1, 3} {
		fields, limited := connectionObservationFields(context, budget)
		require.True(t, limited)
		require.LessOrEqual(t, len(fields), budget)
	}
	deep := map[string]any{}
	at := deep
	for i := 0; i < 40; i++ {
		child := map[string]any{}
		at["Nested"], at = child, child
	}
	_, limited := connectionObservationFields(deep, 4096)
	require.True(t, limited)
	e := &pcaputil.ProtocolEvent{Protocol: "dns", Status: "decoded", Session: map[string]any{"Session Establishment": context}}
	fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	text := detailLines(fields)
	require.NotContains(t, text, "Session Establishment · observed connection context")
	require.Contains(t, text, "Session Establishment", "an unrelated protocol's value is retained without promoting it to TLS context")
}

func TestProtocolDetailStatusDoesNotDrawMissingEndpointArrows(t *testing.T) {
	for _, tc := range []struct{ source, destination, want string }{
		{"", "", "Status: limited"},
		{"192.0.2.1", "", "Status: limited · source 192.0.2.1"},
		{"", "192.0.2.2", "Status: limited · destination 192.0.2.2"},
		{"192.0.2.1", "192.0.2.2", "Status: limited · source 192.0.2.1 → 192.0.2.2"},
	} {
		e := &pcaputil.ProtocolEvent{Protocol: "ip", Status: "limited", Source: tc.source, Destination: tc.destination}
		require.Equal(t, tc.want, protocolEventStatusText(e, "limited"))
		fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
		require.Contains(t, detailLines(fields), tc.want)
	}
}

func TestESPObservedHeaderFieldsPrecedeOpaqueBodyWithoutAuthenticationClaims(t *testing.T) {
	raw := append([]byte{0, 0, 0, 42, 0, 0, 0, 7}, bytes.Repeat([]byte{0xaa}, 64)...)
	e := &pcaputil.ProtocolEvent{Protocol: "esp", Status: "limited", Raw: raw, Session: map[string]any{"SPI": uint32(42), "Sequence": uint32(7), "Header Complete": true, "Algorithm Context Available": false, "Authentication Verified": false, "Extended Sequence Reconstructed": false, "Observation Scope": "wire header only", "Opaque Body": raw[8:], "Wire Field Ranges": map[string][]int{"SPI": {0, 4}, "Sequence": {4, 8}, "Opaque Body": {8, len(raw)}}}}
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	positions := map[string]int{}
	for i, f := range fields {
		for _, prefix := range []string{"SPI:", "Sequence:", "Header Complete:", "Algorithm Context Available:", "Authentication Verified:", "Extended Sequence Reconstructed:", "Observation Scope:", "Opaque Body:"} {
			if strings.HasPrefix(f.text, prefix) {
				positions[prefix] = i
			}
		}
	}
	require.Len(t, positions, 8)
	for name, at := range positions {
		if name != "Opaque Body:" {
			require.Less(t, at, positions["Opaque Body:"])
		}
	}
	require.Equal(t, raw, sources[0].Data)
	text := detailLines(fields)
	require.Contains(t, text, "Authentication Verified: false")
	require.Contains(t, text, "Extended Sequence Reconstructed: false")
}
