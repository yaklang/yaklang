package sharkcli

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestDCERPCReplySummaryAndProgressiveAssociation(t *testing.T) {
	e := &pcaputil.ProtocolEvent{Protocol: "dcerpc", Status: "decoded", ResponseTo: 42,
		Session: map[string]any{"Packet Name": "Response", "Call ID": uint32(9), "Context ID": uint16(0), "Matched Request": "EPM.ept_lookup", "Association Status": "matched"}}
	require.Equal(t, "Response · call=9 · context=0 · reply to PDU #42 · EPM.ept_lookup", applicationEventSummary(e))
	fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	var lines []string
	for _, field := range fields {
		lines = append(lines, field.text)
		if strings.HasPrefix(field.text, "Response to PDU:") {
			require.False(t, field.hasRange, "generated call links cannot masquerade as bytes")
		}
	}
	require.Contains(t, strings.Join(lines, "\n"), "Response to PDU: #42 (observed request on this connection)")
	e.ResponseTo = 0
	e.Session = map[string]any{"Packet Name": "Response", "Call ID": uint32(9), "Context ID": uint16(0), "Unmatched": true, "Association Status": "missing-request"}
	require.Equal(t, "Response · call=9 · context=0 · unmatched: missing-request", applicationEventSummary(e))
	e.Profile = "dcerpc-security-envelope"
	_, ok := dcerpcEventSummary(e)
	require.False(t, ok, "opaque security envelopes retain their own presentation scope")
}

func TestDCERPCLogicalStubHasItsOwnByteSourceThroughProductionWorker(t *testing.T) {
	stub := bytes.Repeat([]byte{0x5a, 0xa5}, 20)
	fragment := func(flags byte, payload []byte) []byte {
		wire := make([]byte, 24+len(payload))
		wire[0], wire[2], wire[3], wire[4] = 5, 0, flags, 0x10
		binary.LittleEndian.PutUint16(wire[8:10], uint16(len(wire)))
		binary.LittleEndian.PutUint32(wire[12:16], 2)
		binary.LittleEndian.PutUint32(wire[16:20], uint32(len(payload)))
		binary.LittleEndian.PutUint16(wire[22:24], 2)
		copy(wire[24:], payload)
		return wire
	}
	s, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	r := s.Feed(0, time.Unix(1, 0), fragment(1, stub[:20]))
	require.Nil(t, r.Err)
	r = s.Feed(0, time.Unix(2, 0), fragment(2, stub[20:]))
	require.Nil(t, r.Err)
	e := r.Events[0]
	require.Len(t, e.Raw, 44)
	e.SourceBytes = pcaputil.ByteSource{Kind: "reassembled", PacketRefs: []pcaputil.PacketReference{{Number: 12}}, CompletedIn: &pcaputil.PacketReference{Number: 12}}
	encoded, err := marshalDecodeRequest(decodeRequest{Number: 1, Events: []*pcaputil.ProtocolEvent{e}})
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(encoded), &output))
	decoded, err := decodeWorkerOutput(output.Bytes(), &capturedPacket{number: 1})
	require.NoError(t, err)
	found := false
	var lines []string
	for _, field := range decoded.fields {
		lines = append(lines, field.text)
		if !strings.HasPrefix(field.text, "Stub: 40 bytes") || !field.hasRange {
			continue
		}
		source := decoded.sources[field.source-1]
		if !strings.Contains(source.Name, "DCE/RPC message stub") {
			continue
		}
		require.Equal(t, stub, source.Data, "IPC must not leave the logical stub as base64 text or a last-fragment slice")
		require.Equal(t, []int{0, 40}, []int{field.start, field.end})
		require.NotEqual(t, e.Raw, source.Data, "logical offsets are relative to their own owned data, never the last wire PDU")
		found = true
	}
	require.True(t, found)
	require.Contains(t, strings.Join(lines, "\n"), "RPC wire PDU bytes complete in frame #12 · 1 contributing packet(s)")
	require.Contains(t, applicationEventSummary(e), "2 fragments · 40 stub bytes")
}

func TestDCERPCOrderedResultsLeadDetailsAndOwnDistinctRanges(t *testing.T) {
	// A complete unassociated server ACK: no fictitious request IDs are needed
	// to render the three observed result values and their original byte ranges.
	wire := make([]byte, 108)
	wire[0], wire[2], wire[3], wire[4] = 5, 12, 3, 0x10
	binary.LittleEndian.PutUint16(wire[8:10], uint16(len(wire)))
	binary.LittleEndian.PutUint32(wire[12:16], 2)
	binary.LittleEndian.PutUint16(wire[24:26], 4)
	copy(wire[26:30], []byte{'1', '3', '5', 0})
	wire[32], wire[36], wire[38], wire[84], wire[86] = 3, 2, 2, 3, 3
	ndr64, err := hex.DecodeString("33057171babe37498319b5dbef9ccc36")
	require.NoError(t, err)
	copy(wire[64:80], ndr64)
	wire[80] = 1
	s, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	r := s.Feed(1, time.Unix(1, 0), wire)
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	require.Equal(t, "decoded", e.Status, e.Error)
	_, err = e.GetFields()
	require.NoError(t, err)
	for mode := 0; mode < 3; mode++ {
		current := e
		if mode > 0 {
			encoded, err := marshalDecodeRequest(decodeRequest{Number: 1, Events: []*pcaputil.ProtocolEvent{e}})
			require.NoError(t, err)
			var request decodeRequest
			decoder := json.NewDecoder(bytes.NewReader(encoded))
			if mode == 2 {
				decoder.UseNumber()
			}
			require.NoError(t, decoder.Decode(&request))
			require.NoError(t, restoreWorkerByteFields(request.Events, request.ByteFields))
			current = request.Events[0]
		}
		fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{current})
		var lines []string
		first, legacy, syntax := -1, -1, 0
		for i, f := range fields {
			lines = append(lines, f.text)
			if f.text == "Presentation Context Results[0]" {
				first = i
			}
			if f.text == "Other decoded fields" {
				legacy = i
			}
			if strings.HasPrefix(f.text, "Transfer Syntax: ") && syntax < 3 {
				start := 40 + 24*syntax
				require.True(t, f.hasRange)
				require.Equal(t, []int{start, start + 16}, []int{f.start, f.end})
				require.Equal(t, wire, sources[f.source-1].Data, "same-named values must select their own PDU occurrence")
				if syntax == 1 {
					require.Contains(t, f.text, "71710533-beba-4937-8319-b5dbef9ccc36")
				}
				syntax++
			}
		}
		require.Equal(t, 3, syntax)
		require.True(t, first >= 0 && first < 18 && legacy > first, "structured results lead the legacy/raw field tree")
		text := strings.Join(lines, "\n")
		require.Contains(t, text, "Result: 2 (Provider rejection)")
		require.Contains(t, text, "Reason: 2 (transfer syntaxes not supported)")
		require.Contains(t, text, "Result: 3 (Feature negotiation ACK)")
		require.Contains(t, text, "Reason: 0x0003 (observed feature bits)")
		require.Contains(t, text, "Association Status: missing-request")
	}
	encoded, err := marshalDecodeRequest(decodeRequest{Number: 1, Events: []*pcaputil.ProtocolEvent{e}})
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(encoded), &output))
	decoded, err := decodeWorkerOutput(output.Bytes(), &capturedPacket{number: 1})
	require.NoError(t, err)
	selected := 0
	for _, field := range decoded.fields {
		if strings.HasPrefix(field.text, "Transfer Syntax: ") && selected < 3 {
			start := 40 + 24*selected
			require.True(t, field.hasRange)
			require.Equal(t, []int{start, start + 16}, []int{field.start, field.end})
			require.Equal(t, wire, decoded.sources[field.source-1].Data)
			selected++
		}
	}
	require.Equal(t, 3, selected, "the production worker decoder must retain all scoped occurrences")
}

func TestDetailWireRangesKeepExactTypesAndRejectInvalidCoordinates(t *testing.T) {
	values := map[string]any{"a": []uint64{64, 80}, "b": []int{0, 0}, "c": []any{float64(0), float64(8)}, "d": []any{uint16(1), uint32(3)}}
	require.Equal(t, map[string][]int{"a": {64, 80}, "b": {0, 0}, "c": {0, 8}, "d": {1, 3}}, detailWireRanges(values))
	require.Equal(t, map[string][]int{"a": {64, 80}}, detailWireRanges(map[string]any{"a": []any{json.Number("64"), json.Number("80")}}))
	for _, span := range []any{[]int{-1, 3}, []int{3, 1}, []int{0}, []any{0.5, 3.0}, []any{math.NaN(), 3.0}, []any{0.0, math.Inf(1)}, []any{0.0, math.MaxFloat64}, []any{uint64(0), ^uint64(0)}, []any{"0", "3"}, []any{false, true}, []any{json.Number("0.5"), json.Number("3")}, []any{json.Number("0"), json.Number("18446744073709551615")}} {
		require.Empty(t, detailWireRanges(map[string]any{"bad": span}))
	}
	original := map[string][]int{"a": {1, 3}}
	copy := detailWireRanges(original)
	copy["a"][0] = 2
	require.Equal(t, 1, original["a"][0])
	require.Equal(t, "unknown", dcerpcDetailValue("Result", "unknown", "unknown", 0, false))
}
