package sharkcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestWorkerPreservesByteTypesWithoutGuessingTextOrMapMarkers(t *testing.T) {
	wire := []byte{0xff, 0, 0xc3, 0x28, 0xff, 0}
	fields := map[string]any{"Public Key": wire, "Text": "none", "Looks Encoded": "AAH/", "Invalid UTF8 String": string(wire), "Marker-shaped map": map[string]any{"_shark_bytes": "AAH/"}, "Names/with[brackets]": []map[string]any{{"0": wire}}, "Empty": []byte{}, "Nil Bytes": []byte(nil), "Missing value": nil, "Scalar List": []any{1, 2, 3}}
	e := &pcaputil.ProtocolEvent{ID: 1, Protocol: "ssh", Status: "decoded", Raw: []byte{1, 2, 3}, Fields: fields, Session: map[string]any{"Nested": map[string]any{"Bytes": wire}}}
	for _, stream := range []bool{false, true} {
		r := decodeRequest{Number: 7, Events: []*pcaputil.ProtocolEvent{e}}
		if stream {
			r.Events, r.Stream = nil, &streamDecodeRequest{ID: 7, Events: []*pcaputil.ProtocolEvent{e}}
		}
		encoded, err := marshalDecodeRequest(r)
		require.NoError(t, err)
		var request decodeRequest
		require.NoError(t, json.Unmarshal(encoded, &request))
		events := request.Events
		if stream {
			events = request.Stream.Events
		}
		require.NoError(t, restoreWorkerByteFields(events, request.ByteFields))
		got := events[0].Fields
		require.Equal(t, wire, got["Public Key"])
		require.Equal(t, wire, got["Names/with[brackets]"].([]any)[0].(map[string]any)["0"])
		require.Equal(t, "none", got["Text"])
		require.Equal(t, "AAH/", got["Looks Encoded"])
		require.Equal(t, string(wire), got["Invalid UTF8 String"])
		require.Equal(t, map[string]any{"_shark_bytes": "AAH/"}, got["Marker-shaped map"])
		require.IsType(t, []any{}, got["Scalar List"])
		require.Equal(t, []byte{}, got["Empty"])
		require.Equal(t, []byte(nil), got["Nil Bytes"])
		require.Nil(t, got["Missing value"])
		var out bytes.Buffer
		require.NoError(t, runDecodeWorker(bytes.NewReader(encoded), &out))
		d, err := decodeWorkerOutput(out.Bytes(), &capturedPacket{number: 7})
		require.NoError(t, err)
		var lines []string
		for _, f := range d.fields {
			lines = append(lines, f.text)
			if strings.HasPrefix(f.text, "Public Key:") {
				require.Contains(t, f.text, "6 bytes · HEX ff 00 c3 28 ff 00")
				require.True(t, f.hasRange)
				require.Positive(t, f.source, "the field's extracted bytes cannot masquerade as captured-frame offsets")
				require.Contains(t, d.sources[f.source-1].Name, "frame offsets unavailable")
				require.Equal(t, wire, d.sources[f.source-1].Data[f.start:f.end])
			}
			if strings.HasPrefix(f.text, "Invalid UTF8 String:") {
				require.Contains(t, f.text, "6 bytes · HEX ff 00 c3 28 ff 00")
				require.True(t, f.hasRange)
				require.Positive(t, f.source)
				require.Equal(t, wire, d.sources[f.source-1].Data[f.start:f.end])
			}
		}
		require.Contains(t, strings.Join(lines, "\n"), "Text: none")
		require.Contains(t, strings.Join(lines, "\n"), "Looks Encoded: AAH/")
		clear(d.sources[len(d.sources)-1].Data)
		require.Equal(t, []byte{0xff, 0, 0xc3, 0x28, 0xff, 0}, wire, "worker and source snapshots own their bytes")
	}
}

func FuzzWorkerByteFieldTypesRemainLiteral(f *testing.F) {
	f.Add([]byte{0xff, 0, 0xc3, 0x28}, "name/with[0]", "AAH/")
	f.Add([]byte{}, "0", "none")
	f.Add([]byte{1}, string([]byte{0xff}), "ordinary text")
	f.Fuzz(func(t *testing.T, data []byte, key, text string) {
		if len(data) > 8192 || len(key) > 128 || len(text) > 256 {
			return
		}
		path := "wire:" + key
		e := &pcaputil.ProtocolEvent{Protocol: "ssh", Status: "decoded", Fields: map[string]any{"Values": []map[string]any{{path: data, "Plain text": text}}, "Original string bytes": string(data)}}
		encoded, err := marshalDecodeRequest(decodeRequest{Events: []*pcaputil.ProtocolEvent{e}})
		require.NoError(t, err)
		require.LessOrEqual(t, len(encoded), decoderInputLimit)
		var request decodeRequest
		require.NoError(t, json.Unmarshal(encoded, &request))
		require.NoError(t, restoreWorkerByteFields(request.Events, request.ByteFields))
		if !utf8.ValidString(key) {
			require.Equal(t, "limited", request.Events[0].Status, "unrepresentable map-key identity cannot silently select a different field")
			return
		}
		require.Equal(t, "decoded", request.Events[0].Status)
		values := request.Events[0].Fields["Values"].([]any)[0].(map[string]any)
		require.Equal(t, data, values[path])
		require.Equal(t, text, values["Plain text"])
		require.Equal(t, string(data), request.Events[0].Fields["Original string bytes"])
		require.Equal(t, data, e.Fields["Values"].([]map[string]any)[0][path], "projection never rewrites the input history")
	})
}

func TestWorkerRejectsInconsistentOrRepeatedByteDescriptors(t *testing.T) {
	e := &pcaputil.ProtocolEvent{Protocol: "ssh", Fields: map[string]any{"Bytes": []byte{1, 2, 3}}}
	encoded, err := marshalDecodeRequest(decodeRequest{Events: []*pcaputil.ProtocolEvent{e}})
	require.NoError(t, err)
	for _, mutate := range []func(*decodeRequest){
		func(r *decodeRequest) { r.ByteFields[0].Length++ },
		func(r *decodeRequest) { r.ByteFields[0].Length = -1 },
		func(r *decodeRequest) { r.ByteFields[0].Event = 1 },
		func(r *decodeRequest) { r.ByteFields[0].Root = "Raw" },
		func(r *decodeRequest) { r.ByteFields[0].Path = []string{"absent"} },
		func(r *decodeRequest) { r.ByteFields[0].Nil = true },
		func(r *decodeRequest) { r.ByteFields[0].StringBytes = []byte{0xff, 0xff, 0xff} },
		func(r *decodeRequest) { r.ByteFields[0].StringBytes = []byte("abc") },
		func(r *decodeRequest) { r.ByteFields = append(r.ByteFields, r.ByteFields[0]) },
		func(r *decodeRequest) { r.Events[0].Fields["Bytes"] = "AQID\n" },
		func(r *decodeRequest) { r.Events[0].Fields["Bytes"] = "not-base64" },
		func(r *decodeRequest) { r.Events[0].Fields["Bytes"] = []any{1, 2, 3} },
	} {
		var r decodeRequest
		require.NoError(t, json.Unmarshal(encoded, &r))
		mutate(&r)
		data, err := json.Marshal(r)
		require.NoError(t, err)
		var out bytes.Buffer
		require.Error(t, runDecodeWorker(bytes.NewReader(data), &out))
		require.Empty(t, out.Bytes(), "an invalid type projection cannot publish a successful detail")
	}
}

func TestByteTypeIPCDoesNotBypassProtectedFrameSourceAdmission(t *testing.T) {
	for _, verified := range []bool{false, true} {
		e := &pcaputil.ProtocolEvent{Protocol: "http3", Status: "decoded", Raw: []byte{9, 9, 9}, Session: map[string]any{"Authentication Verified": verified, "HTTP3 Stream ID": uint64(7), "HTTP3 Frames": []map[string]any{{"Frame Type": "CANCEL_PUSH", "Payload": []byte{1, 2, 3}}, {"Frame Type": "DATA", "Payload": []byte{4, 5, 6}}}}}
		input, err := marshalDecodeRequest(decodeRequest{Number: 11, Events: []*pcaputil.ProtocolEvent{e}})
		require.NoError(t, err)
		var output bytes.Buffer
		require.NoError(t, runDecodeWorker(bytes.NewReader(input), &output))
		detail, err := decodeWorkerOutput(output.Bytes(), &capturedPacket{number: 11})
		require.NoError(t, err)
		selected := 0
		for _, f := range detail.fields {
			if strings.HasPrefix(f.text, "Payload:") && f.hasRange {
				selected++
				require.True(t, verified)
				require.Equal(t, "HTTP/3 DATA · reassembled · stream 7", detail.sources[f.source-1].Name)
				require.Equal(t, []byte{4, 5, 6}, detail.sources[f.source-1].Data)
			}
		}
		if verified {
			require.Equal(t, 1, selected)
		} else {
			require.Zero(t, selected)
		}
	}
}

func TestTypedByteSourcesKeepKnownWireRangesAndReportSourceBudget(t *testing.T) {
	e := &pcaputil.ProtocolEvent{Protocol: "ntp", Status: "decoded", Raw: []byte{1, 2, 3}, Session: map[string]any{"Wire Field Ranges": map[string][]int{"Control Data": {1, 3}}, "Control Data": []byte{2, 3}, "Unmapped": bytes.Repeat([]byte{0xff}, (1<<20)+1)}}
	fs, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	require.Contains(t, fs[0].text, "Byte-value source budget reached")
	for _, f := range fs {
		if strings.HasPrefix(f.text, "Control Data:") {
			require.True(t, f.hasRange)
			require.Equal(t, 1, f.source)
			require.Equal(t, []byte{2, 3}, sources[f.source-1].Data[f.start:f.end])
		}
		if strings.HasPrefix(f.text, "Unmapped:") {
			require.False(t, f.hasRange)
			require.Contains(t, f.text, "full byte source unavailable")
		}
	}
}
