package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// These maps are the fixed SN abstract grammar and adopted observer state
// contract, never fields captured from the implementation under test.
func snMultiStateFields(t *testing.T, wire string) map[string]any {
	t.Helper()
	w := wrapperWire(t, wire)
	p := w[8:]
	f := map[string]any{"wrapper_version": uint16(1), "source_wport": binary.BigEndian.Uint16(w[2:4]), "destination_wport": binary.BigEndian.Uint16(w[4:6]), "payload_length": len(p), "wrapper_header_hex": wire[:16], "apdu_hex": wire[16:], "command": p[0], "reference_mode": "short-name", "association_observed": false, "authentication_verified": false, "object_model_verified": false, "operation_success_verified": false, "list_count": uint64(p[1]), "list_count_encoding_hex": wire[18:20]}
	switch {
	case p[0] == 5 && p[2] == 2:
		f["kind"] = "ReadRequest"
		f["descriptors"] = []map[string]any{
			{"index": 0, "apdu_offset": 2, "access_choice_raw": byte(2), "access_choice": "variable-name", "variable_name": uint16(4096), "variable_name_hex": "1000", "raw_hex": "021000", "encoded_length": 3},
			{"index": 1, "apdu_offset": 5, "access_choice_raw": byte(2), "access_choice": "variable-name", "variable_name": uint16(64000), "variable_name_hex": "fa00", "raw_hex": "02fa00", "encoded_length": 3},
		}
	case p[0] == 5 && p[2] == 5:
		f["kind"] = "ReadRequest"
		f["descriptors"] = []map[string]any{{"index": 0, "apdu_offset": 2, "access_choice_raw": byte(5), "access_choice": "block-number-access", "block_number": uint16(1), "encoded_length": 3, "raw_hex": "050001"}}
	case p[0] == 12 && p[1] == 1:
		f["kind"] = "ReadResponse"
		f["transfer_complete"] = false
		f["results"] = []map[string]any{{"index": 0, "apdu_offset": 2, "result_choice_raw": byte(2), "result_choice": "data-block-result", "last_block_raw": p[3], "last_block": p[3] == 1, "block_number": binary.BigEndian.Uint16(p[4:6]), "raw_data_length": uint64(p[6]), "length_encoding_hex": hex.EncodeToString(p[6:7]), "raw_data_hex": hex.EncodeToString(p[7:]), "raw_hex": hex.EncodeToString(p[2:]), "encoded_length": len(p) - 2}}
	default:
		t.Fatal("unselected state-vector syntax")
	}
	return f
}

func TestDLMSWrapperSNMultiResultRetiredGeneration(t *testing.T) {
	const q = "0001001000010008050202100002fa00"
	const prefix = "000100010010000a0c0102000001030904de"
	const echo = "00010010000100050501050001"
	const final = "000100010010000a0c010201000203adbeef"
	const multi = "00010001001000100c020200000102110702010002021108"
	vectors := []struct {
		wire                 string
		dir                  int
		association, profile string
		outstanding          int
	}{
		{q, 0, "unverified-sn-read-observation", "dlms-wrapper-v1-sn-read", 0},
		{prefix, 1, "unverified-sn-data-block", "dlms-wrapper-v1-sn-read-block", 1},
		{multi, 1, "unverified-sn-multiresult-observation", "dlms-wrapper-v1-sn-multiresult", 0},
		{echo, 0, "retired-sn-wire-group", "dlms-wrapper-v1-sn-read-block", 0},
		{prefix, 1, "retired-sn-wire-group", "dlms-wrapper-v1-sn-read-block", 0},
		{q, 0, "ordinary-sn-request-retired-wire-group", "dlms-wrapper-v1-sn-read", 0},
		{prefix, 1, "unverified-sn-data-block", "dlms-wrapper-v1-sn-read-block", 1},
		{echo, 0, "unverified-sn-block-number-access", "dlms-wrapper-v1-sn-read-block", 1},
		{final, 1, "unverified-sn-wire-group-complete", "dlms-wrapper-v1-sn-read-block", 0},
	}
	for _, tr := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			chunks := []int{0, 1, 7}
			if tr == "udp" {
				chunks = []int{0}
			}
			for _, chunk := range chunks {
				t.Run(fmt.Sprintf("%s/d%t/chunk%d", tr, deferred, chunk), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					defer s.Close("cleanup")
					s.(*captureSession).f.a.config.Deferred = deferred
					ids := make([]uint64, len(vectors))
					offsets := [2]uint64{}
					var flow uint64
					for i, v := range vectors {
						wire := wrapperWire(t, v.wire)
						size := chunk
						if size == 0 {
							size = len(wire)
						}
						var events []*ProtocolEvent
						for at := 0; at < len(wire); at += size {
							part := append([]byte(nil), wire[at:min(at+size, len(wire))]...)
							r := s.Feed(v.dir, time.Unix(int64(100+i), 0), part)
							clear(part)
							require.Equal(t, min(size, len(wire)-at), r.Consumed)
							if len(r.Events) == 0 {
								rocTypedError(t, "NeedMore", r.Err)
								require.True(t, r.NeedMore)
							} else {
								require.Nil(t, r.Err)
								require.False(t, r.NeedMore)
							}
							events = append(events, r.Events...)
						}
						require.Len(t, events, 1)
						e := events[0]
						ids[i] = e.ID
						require.NotZero(t, e.ID)
						for _, old := range ids[:i] {
							require.NotEqual(t, old, e.ID)
						}
						require.Equal(t, "dlms-wrapper", e.Protocol)
						require.Equal(t, v.profile, e.Profile)
						require.Zero(t, e.TransactionID)
						require.Zero(t, e.ResponseTo)
						require.Equal(t, wire, e.Raw)
						require.Equal(t, len(wire), e.Length)
						require.Equal(t, time.Unix(int64(100+i), 0), e.Timestamp)
						require.Equal(t, CaptureDomain{}, e.Domain)
						endpoints := [2]string{"192.0.2.1:40000", "192.0.2.2:4059"}
						require.Equal(t, endpoints[v.dir], e.Source)
						require.Equal(t, endpoints[1-v.dir], e.Destination)
						require.Empty(t, e.Error)
						if tr == "tcp" {
							require.Equal(t, v.dir, e.Direction)
							require.Equal(t, offsets[v.dir], e.Offset)
							require.NotZero(t, e.FlowID)
							if flow == 0 {
								flow = e.FlowID
							}
							require.Equal(t, flow, e.FlowID)
						} else {
							if i == 0 {
								require.Zero(t, e.FlowID)
							} else {
								require.NotZero(t, e.FlowID)
								if flow == 0 {
									flow = e.FlowID
								}
								require.Equal(t, flow, e.FlowID)
							}
							require.Equal(t, v.dir, e.Direction)
							require.Zero(t, e.Offset)
						}
						offsets[v.dir] += uint64(len(wire))
						status := "decoded"
						if deferred {
							status = "deferred"
						}
						require.Equal(t, status, e.Status)
						require.Equal(t, "message", e.Completeness)
						var expected map[string]any
						if i == 2 {
							expected = map[string]any{"wrapper_version": uint16(1), "source_wport": uint16(1), "destination_wport": uint16(16), "payload_length": 16, "wrapper_header_hex": multi[:16], "apdu_hex": multi[16:], "command": byte(12), "reference_mode": "short-name", "association_observed": false, "authentication_verified": false, "object_model_verified": false, "operation_success_verified": false, "list_count": uint64(2), "list_count_encoding_hex": "02", "kind": "ReadResponse", "error_item_count": 0, "data_item_count": 0, "results": []map[string]any{
								{"index": 0, "apdu_offset": 2, "result_choice_raw": byte(2), "result_choice": "data-block-result", "last_block_raw": byte(0), "last_block": false, "block_number": uint16(1), "raw_data_length": uint64(2), "length_encoding_hex": "02", "raw_data_hex": "1107", "raw_hex": "02000001021107", "encoded_length": 7},
								{"index": 1, "apdu_offset": 9, "result_choice_raw": byte(2), "result_choice": "data-block-result", "last_block_raw": byte(1), "last_block": true, "block_number": uint16(2), "raw_data_length": uint64(2), "length_encoding_hex": "02", "raw_data_hex": "1108", "raw_hex": "02010002021108", "encoded_length": 7},
							}}
						} else {
							expected = snMultiStateFields(t, v.wire)
						}
						if i == 1 || i == 6 {
							expected["transfer_start_event"], expected["observed_block_count"], expected["assembled_data_length"] = ids[i], 1, 3
						}
						if i == 7 {
							expected["acknowledged_block_event"] = ids[6]
						}
						if i == 8 {
							expected["transfer_start_event"], expected["observed_block_count"], expected["assembled_data_length"], expected["transfer_complete"] = ids[6], 2, 6, true
							expected["assembled_data_hex"] = "0904deadbeef"
							expected["assembled_data"] = map[string]any{"type": byte(9), "length": uint64(4), "length_encoding_hex": "04", "value_hex": "deadbeef", "raw_hex": "0904deadbeef"}
						}
						fields, err := e.GetFields()
						require.NoError(t, err)
						require.Equal(t, expected, fields)
						snReadTypes(t, fields)
						session := cloneSession(expected)
						session["Association"], session["Outstanding"] = v.association, v.outstanding
						require.Equal(t, session, e.Session)
						snReadTypes(t, e.Session)
						decoded, err := e.Decode()
						require.NoError(t, err)
						require.Equal(t, expected, decoded["fields"])
						require.Equal(t, session, decoded["session"])
						poisonWrapperStructured(fields)
						poisonWrapperStructured(e.Session)
						poisonWrapperStructured(decoded)
						again, err := e.GetFields()
						require.NoError(t, err)
						require.Equal(t, expected, again)
					}
					require.Empty(t, s.Close("state-vector"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}
