package sharkcli

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func ospfv3PresentationPacket(t *testing.T, ah bool) *capturedPacket {
	w := []byte{3, 1, 0, 44, 192, 0, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 7, 1, 0, 0, 0x13, 0, 10, 0, 40, 192, 0, 2, 1, 0, 0, 0, 0, 192, 0, 2, 2, 192, 0, 2, 2}
	context := stream_parser.OSPFv3WireContext{Source: netip.MustParseAddr("fe80::1"), Destination: netip.MustParseAddr("ff02::5")}
	binary.BigEndian.PutUint16(w[12:], stream_parser.OSPFv3Checksum(w, context))
	frame := make([]byte, 54)
	copy(frame, []byte{0x33, 0x33, 0, 0, 0, 5, 2, 0, 0, 0, 0, 1, 0x86, 0xdd})
	frame[14], frame[20], frame[21] = 0x60, 89, 1
	copy(frame[22:38], context.Source.AsSlice())
	copy(frame[38:54], context.Destination.AsSlice())
	if ah {
		frame[20] = 51
		extension := make([]byte, 24)
		extension[0], extension[1], extension[7], extension[11] = 89, 4, 7, 9
		frame = append(frame, extension...)
	}
	frame = append(frame, w...)
	binary.BigEndian.PutUint16(frame[18:], uint16(len(frame)-54))
	raw := replayPIMPresentationPacket(t, frame)
	raw.reference = raw.events[0].SourceBytes.PacketRefs[0]
	return raw
}

func TestOSPFv3ProgressiveNativeQuickAndWorkerSources(t *testing.T) {
	for _, ah := range []bool{false, true} {
		raw := ospfv3PresentationPacket(t, ah)
		event := raw.events[0]
		require.Equal(t, "ospfv3_observed", event.Rule)
		require.Equal(t, "OSPFv3ControlFields", event.Entry)
		require.Equal(t, event.SourceBytes.PacketRefs[0], raw.reference)
		expected, decodeErr := stream_parser.DecodeOSPFv3Wire(event.Raw, stream_parser.OSPFv3WireContext{Source: netip.MustParseAddr("fe80::1"), Destination: netip.MustParseAddr("ff02::5")}, 4096)
		require.NoError(t, decodeErr)
		wantJSON, err := json.Marshal(expected)
		require.NoError(t, err)
		gotJSON, err := json.Marshal(event.Session)
		require.NoError(t, err)
		require.JSONEq(t, string(wantJSON), string(gotJSON))
		var prefix packetDetail
		require.True(t, appendOSPFv3CapturedPrefix(&prefix, raw, raw.events), "valid source-reproduced event must suppress duplicate legacy fields")
		row := summarize(raw)
		require.Equal(t, "OSPFv3", row.Protocol)
		require.Contains(t, row.Info, "Hello")
		require.Contains(t, row.Info, "router=192.0.2.1")
		require.Equal(t, 1, strings.Count(strings.Join(row.ProtocolStack, "/"), "OSPFv3"))
		for _, d := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
			text := detailLines(d.fields)
			require.Contains(t, text, "IPv6 header")
			require.Contains(t, text, "Hello")
			require.NotContains(t, text, "Wire Field Ranges")
			require.NotContains(t, text, "YAML dissector incomplete")
			require.NotContains(t, text, "<nil>")
			if ah {
				require.Contains(t, text, "AH header · authentication unverified")
			}
			var offsets []int
			for _, f := range d.fields {
				if f.text == "Neighbor Router ID: 192.0.2.2" {
					require.True(t, f.hasRange)
					require.Equal(t, 1, f.source)
					require.GreaterOrEqual(t, f.depth, 3)
					require.Equal(t, []byte{192, 0, 2, 2}, d.sources[0].Data[f.start:f.end])
					offsets = append(offsets, f.start)
				}
				if strings.HasPrefix(f.text, "Checksum Verified:") || strings.HasPrefix(f.text, "Authentication Verified:") {
					require.False(t, f.hasRange, "derived context has no invented wire octets")
				}
			}
			require.Equal(t, []int{36, 40}, offsets, "equal neighboring values retain distinct occurrences")
			require.True(t, bytes.Equal(raw.events[0].Raw, d.sources[0].Data))
		}
		bad := *raw.events[0]
		bad.SourceBytes.SHA256 = "forged"
		var d packetDetail
		require.False(t, appendOSPFv3CapturedPrefix(&d, raw, []*pcaputil.ProtocolEvent{&bad}))
	}
}

func TestOSPFv3WorkerPreservesExactCaptureDomainAndRejectsWrongNumber(t *testing.T) {
	raw := ospfv3PresentationPacket(t, true)
	raw.reference.Domain.Interface = 7
	raw.reference.Domain.Encapsulation = "source-selected-interface"
	raw.events[0].SourceBytes.PacketRefs[0] = raw.reference
	request := packetDecodeRequest(raw)
	require.Equal(t, raw.reference, request.Reference)
	wire, err := marshalDecodeRequest(request)
	require.NoError(t, err)
	var restored decodeRequest
	require.NoError(t, json.Unmarshal(wire, &restored))
	require.Equal(t, raw.reference, restored.Reference)
	request.Reference.Number++
	wire, err = marshalDecodeRequest(request)
	require.NoError(t, err)
	var output bytes.Buffer
	err = runDecodeWorker(bytes.NewReader(wire), &output)
	require.ErrorContains(t, err, "reference does not match")
}

func TestOSPFv3UnsupportedSiblingRemainsProgressiveAcrossWorker(t *testing.T) {
	// Literal Fletcher bytes independently checked over LSAs excluding Age:
	// unknown0xa123=0x943e; Router-LSA0x2001=0x4924. No full-body decoder is
	// needed to determine the exact24-byte boundary of the unknown LSA.
	w := []byte{3, 4, 0, 68, 192, 0, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
	w = append(w, []byte{0, 1, 0xa1, 0x23, 0, 0, 0, 1, 192, 0, 2, 1, 0x80, 0, 0, 1, 0x94, 0x3e, 0, 24, 1, 2, 3, 4}...)
	w = append(w, []byte{0, 1, 0x20, 1, 0, 0, 0, 1, 192, 0, 2, 1, 0x80, 0, 0, 1, 0x49, 0x24, 0, 24, 0, 0, 0, 0x13}...)
	context := stream_parser.OSPFv3WireContext{Source: netip.MustParseAddr("fe80::1"), Destination: netip.MustParseAddr("ff02::5")}
	binary.BigEndian.PutUint16(w[12:], stream_parser.OSPFv3Checksum(w, context))
	base := ospfv3PresentationPacket(t, false)
	for _, corrupt := range []bool{false, true} {
		wire := bytes.Clone(w)
		wantStatus := "context-required"
		if corrupt {
			wire[12] ^= 1
			wantStatus = "malformed"
		}
		frame := append(bytes.Clone(base.data[:54]), wire...)
		binary.BigEndian.PutUint16(frame[18:], uint16(len(wire)))
		raw := replayPIMPresentationPacket(t, frame)
		require.Len(t, raw.events, 1)
		raw.reference = raw.events[0].SourceBytes.PacketRefs[0]
		require.Equal(t, wantStatus, raw.events[0].Status)
		require.Zero(t, raw.events[0].FlowID)
		require.Zero(t, raw.events[0].ResponseTo)
		require.Equal(t, "OSPFv3", summarize(raw).Protocol)
		for _, d := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
			text := detailLines(d.fields)
			require.Contains(t, text, "LSAs[1]")
			require.Contains(t, text, "LS Type: 0xa123 (unselected LSA type)")
			require.Contains(t, text, "LS Type: 0x2001 (Router-LSA)")
			require.Contains(t, text, "Options: 19")
			require.NotContains(t, text, "YAML dissector incomplete")
			require.NotContains(t, text, "enclosing IPv6 pseudo-header not supplied")
			found := 0
			for _, f := range d.fields {
				if f.text == "LS Type: 0x2001 (Router-LSA)" {
					require.True(t, f.hasRange)
					require.Equal(t, 1, f.source)
					require.Equal(t, 46, f.start)
					require.Equal(t, 48, f.end)
					require.Equal(t, []byte{0x20, 1}, d.sources[0].Data[f.start:f.end])
					found++
				}
			}
			require.Equal(t, 1, found, "no duplicate legacy tree or lost known sibling")
		}
	}
}

func TestOSPFv3ReadableTypeNamesPreserveIntegerFieldsAndWireOccurrences(t *testing.T) {
	w := make([]byte, 56)
	w[0], w[1] = 3, 5
	binary.BigEndian.PutUint16(w[2:], 56)
	for i, typ := range []uint16{0x2009, 0xa123} {
		at := 16 + 20*i
		binary.BigEndian.PutUint16(w[at+2:], typ)
		binary.BigEndian.PutUint16(w[at+18:], 20)
	}
	m, err := stream_parser.DecodeOSPFv3Wire(w, stream_parser.OSPFv3WireContext{}, 32)
	require.NoError(t, err)
	before, err := json.Marshal(m)
	require.NoError(t, err)
	e := &pcaputil.ProtocolEvent{ID: 1, Protocol: "ospf", Profile: "ospfv3-bounded-control", Status: "decoded", Raw: w, Session: m}
	fs, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	seen := 0
	for _, f := range fs {
		if f.text == "LS Type: 0x2009 (Intra-Area-Prefix-LSA)" {
			require.True(t, f.hasRange)
			require.Equal(t, 18, f.start)
			require.Equal(t, 20, f.end)
			require.Equal(t, []byte{0x20, 9}, sources[f.source-1].Data[f.start:f.end])
			seen++
		}
		if f.text == "LS Type: 0xa123 (unselected LSA type)" {
			require.True(t, f.hasRange)
			require.Equal(t, 38, f.start)
			require.Equal(t, 40, f.end)
			seen++
		}
	}
	require.Equal(t, 2, seen)
	after, err := json.Marshal(m)
	require.NoError(t, err)
	require.Equal(t, before, after)
	for _, c := range []struct {
		name string
		v    any
		want string
	}{
		{"Type", uint64(4), "4 (Link State Update)"},
		{"Referenced LS Type", uint64(0), "0x0000 (no referenced LSA)"},
		{"Referenced LS Type", uint64(0x2001), "0x2001 (Router-LSA)"},
	} {
		v, ok := ospfv3FieldValue(c.name, c.v)
		require.True(t, ok)
		require.Equal(t, c.want, v)
	}
	for _, v := range []any{"bogus", -1, uint64(65536), 1.5} {
		_, ok := ospfv3FieldValue("LS Type", v)
		require.False(t, ok)
	}
	other := *e
	other.Profile = "unselected-ospf-layout"
	plain, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{&other})
	require.Contains(t, detailLines(plain), "LS Type: 8201")
	require.NotContains(t, detailLines(plain), "Intra-Area-Prefix-LSA", "a shared protocol label cannot select the v3 field layout")
}
