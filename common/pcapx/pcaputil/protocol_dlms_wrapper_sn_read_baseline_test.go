package pcaputil

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// Existing public API witnesses: no new symbol is required for the old code to
// compile. Exact fixed-source wire fields distinguish this from name detection.
func TestDLMSWrapperSNReadExistingAPI(t *testing.T) {
	q := "0001001000010008050202100002fa00"
	r := "00010001001000070c020011070103"
	base := func(w string, cmd byte, src, dst uint16) map[string]any {
		return map[string]any{"wrapper_version": uint16(1), "source_wport": src, "destination_wport": dst, "payload_length": len(w)/2 - 8, "wrapper_header_hex": w[:16], "apdu_hex": w[16:], "command": cmd, "reference_mode": "short-name", "association_observed": false, "authentication_verified": false, "object_model_verified": false, "operation_success_verified": false, "list_count": uint64(2), "list_count_encoding_hex": "02"}
	}
	qf := base(q, 5, 16, 1)
	qf["kind"] = "ReadRequest"
	qf["descriptors"] = []map[string]any{
		{"index": 0, "apdu_offset": 2, "access_choice_raw": byte(2), "access_choice": "variable-name", "variable_name": uint16(4096), "variable_name_hex": "1000", "raw_hex": "021000", "encoded_length": 3},
		{"index": 1, "apdu_offset": 5, "access_choice_raw": byte(2), "access_choice": "variable-name", "variable_name": uint16(64000), "variable_name_hex": "fa00", "raw_hex": "02fa00", "encoded_length": 3},
	}
	rf := base(r, 12, 1, 16)
	rf["kind"] = "ReadResponse"
	rf["data_item_count"], rf["error_item_count"] = 1, 1
	rf["results"] = []map[string]any{
		{"index": 0, "apdu_offset": 2, "result_choice_raw": byte(0), "result_choice": "data", "data": map[string]any{"type": byte(17), "raw_hex": "1107", "value": uint64(7)}, "raw_hex": "001107", "encoded_length": 3},
		{"index": 1, "apdu_offset": 5, "result_choice_raw": byte(1), "result_choice": "data-access-error", "data_access_result": byte(3), "raw_hex": "0103", "encoded_length": 2},
	}
	for _, tc := range []struct {
		name, wire string
		dir        int
		fields     map[string]any
	}{
		{"short-name-address-list", q, 0, qf}, {"short-name-mixed-result", r, 1, rf}, {"adjacent-LN-owner", "", 0, nil},
	} {
		for _, tr := range []string{"tcp", "udp"} {
			for _, deferred := range []bool{false, true} {
				for _, chunk := range []int{1, 7, 65543} {
					if tr == "udp" && chunk != 65543 {
						continue
					}
					t.Run(fmt.Sprintf("%s/%s/%v/%d", tc.name, tr, deferred, chunk), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(40000, 4059))
						require.NoError(t, err)
						defer s.Close("defer")
						s.(*captureSession).f.a.config.Deferred = deferred
						feed := func(dir int, w string) []*ProtocolEvent {
							wire := wrapperWire(t, w)
							var es []*ProtocolEvent
							for at := 0; at < len(wire); {
								end := min(at+chunk, len(wire))
								in := append([]byte(nil), wire[at:end]...)
								out := s.Feed(dir, time.Unix(100, 0), in)
								for j := range in {
									in[j] ^= 255
								}
								if out.Err != nil {
									require.Equal(t, ErrNeedMore, out.Err.Kind)
								}
								es = append(es, out.Events...)
								at = end
							}
							return es
						}
						if tc.wire == "" {
							qs := feed(0, "000100100001000dc001c1000100002a0000ff0200")
							require.Len(t, qs, 1)
							_, err = qs[0].GetFields()
							require.NoError(t, err)
							rs := feed(1, "0001000100100006c401c1001107")
							require.Len(t, rs, 1)
							e := rs[0]
							f, err := e.GetFields()
							require.NoError(t, err)
							require.Equal(t, "dlms-wrapper-v1-get-normal", e.Profile)
							require.Equal(t, qs[0].ID, e.TransactionID)
							require.Equal(t, qs[0].ID, e.ResponseTo)
							require.Equal(t, "observed-response", e.Session["Association"])
							require.Equal(t, 0, e.Session["Outstanding"])
							rocEqualFields(t, map[string]any{"type": float64(17), "raw_hex": "1107", "value": float64(7)}, f["data"].(map[string]any))
						} else {
							es := feed(tc.dir, tc.wire)
							require.Len(t, es, 1)
							e := es[0]
							f, err := e.GetFields()
							require.NoError(t, err)
							require.Equal(t, "dlms-wrapper", e.Protocol)
							require.Equal(t, "dlms-wrapper-v1-sn-read", e.Profile)
							require.Equal(t, wrapperWire(t, tc.wire), e.Raw)
							require.Zero(t, e.TransactionID)
							require.Zero(t, e.ResponseTo)
							rocEqualFields(t, tc.fields, f)
							expected := cloneSession(tc.fields)
							expected["Association"], expected["Outstanding"] = "unverified-sn-read-observation", 0
							rocEqualFields(t, expected, e.Session)
							require.IsType(t, uint16(0), f["wrapper_version"])
							require.IsType(t, byte(0), f["command"])
							require.IsType(t, uint64(0), f["list_count"])
							for _, key := range []string{"invoke_id", "invoke_id_and_priority", "confirmed_service", "high_priority", "service_choice"} {
								require.NotContains(t, f, key)
							}
							poisonWrapperStructured(e.Session)
							poisonWrapperStructured(f)
							again, err := e.GetFields()
							require.NoError(t, err)
							rocEqualFields(t, tc.fields, again)
						}
						require.Empty(t, s.Close("baseline"))
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
					})
				}
			}
		}
	}
}
