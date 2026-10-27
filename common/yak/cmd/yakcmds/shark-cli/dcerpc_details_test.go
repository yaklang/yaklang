package sharkcli

import (
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
	for _, ipc := range []bool{false, true} {
		current := e
		if ipc {
			encoded, err := marshalDecodeRequest(decodeRequest{Number: 1, Events: []*pcaputil.ProtocolEvent{e}})
			require.NoError(t, err)
			var request decodeRequest
			require.NoError(t, json.Unmarshal(encoded, &request))
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
}

func TestDetailWireRangesKeepExactTypesAndRejectInvalidCoordinates(t *testing.T) {
	values := map[string]any{"a": []uint64{64, 80}, "b": []int{0, 0}, "c": []any{float64(0), float64(8)}, "d": []any{uint16(1), uint32(3)}}
	require.Equal(t, map[string][]int{"a": {64, 80}, "b": {0, 0}, "c": {0, 8}, "d": {1, 3}}, detailWireRanges(values))
	for _, span := range []any{[]int{-1, 3}, []int{3, 1}, []int{0}, []any{0.5, 3.0}, []any{math.NaN(), 3.0}, []any{0.0, math.Inf(1)}, []any{0.0, math.MaxFloat64}, []any{uint64(0), ^uint64(0)}, []any{"0", "3"}, []any{false, true}} {
		require.Empty(t, detailWireRanges(map[string]any{"bad": span}))
	}
	original := map[string][]int{"a": {1, 3}}
	copy := detailWireRanges(original)
	copy["a"][0] = 2
	require.Equal(t, 1, original["a"][0])
	require.Equal(t, "unknown", dcerpcDetailValue("Result", "unknown", "unknown", 0, false))
}
