package pcaputil

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDLMSHDLCSNReadExistingAPI(t *testing.T) {
	q := "7ea014032110f051e6e600050202100002fa00ee187e"
	r := "7ea013210330d381e6e7000c020011070103cb2a7e"
	qsn := map[string]any{"payload_length": 8, "apdu_hex": "050202100002fa00", "command": byte(5), "reference_mode": "short-name", "association_observed": false, "authentication_verified": false, "object_model_verified": false, "operation_success_verified": false, "list_count": uint64(2), "list_count_encoding_hex": "02", "kind": "ReadRequest", "descriptors": []map[string]any{{"index": 0, "apdu_offset": 2, "access_choice_raw": byte(2), "access_choice": "variable-name", "variable_name": uint16(4096), "variable_name_hex": "1000", "raw_hex": "021000", "encoded_length": 3}, {"index": 1, "apdu_offset": 5, "access_choice_raw": byte(2), "access_choice": "variable-name", "variable_name": uint16(64000), "variable_name_hex": "fa00", "raw_hex": "02fa00", "encoded_length": 3}}}
	rsn := map[string]any{"payload_length": 7, "apdu_hex": "0c020011070103", "command": byte(12), "reference_mode": "short-name", "association_observed": false, "authentication_verified": false, "object_model_verified": false, "operation_success_verified": false, "list_count": uint64(2), "list_count_encoding_hex": "02", "kind": "ReadResponse", "data_item_count": 1, "error_item_count": 1, "results": []map[string]any{{"index": 0, "apdu_offset": 2, "result_choice_raw": byte(0), "result_choice": "data", "data": map[string]any{"type": byte(17), "raw_hex": "1107", "value": uint64(7)}, "raw_hex": "001107", "encoded_length": 3}, {"index": 1, "apdu_offset": 5, "result_choice_raw": byte(1), "result_choice": "data-access-error", "data_access_result": byte(3), "raw_hex": "0103", "encoded_length": 2}}}
	common := func(request bool) map[string]any {
		f := map[string]any{"Observation": "unverified-dlms-hdlc-wire-values", "Frame Format": uint16(40980), "Frame Length": 20, "Destination Address": uint32(1), "Source Address": uint32(16), "Destination Address Octets": 1, "Source Address Octets": 1, "Control": byte(16), "Poll/Final": true, "FCS": uint16(6382), "HCS": uint16(20976), "Send Sequence": byte(0), "Receive Sequence": byte(0), "LLC": []byte{0xe6, 0xe6, 0}, "Frame Kind": "SN Read Request", "SN Read": qsn, "Association": "unverified-sn-read-observation"}
		if !request {
			f["Frame Format"] = uint16(40979)
			f["Frame Length"] = 19
			f["Destination Address"] = uint32(16)
			f["Source Address"] = uint32(1)
			f["Control"] = byte(48)
			f["FCS"] = uint16(10955)
			f["HCS"] = uint16(33235)
			f["Receive Sequence"] = byte(1)
			f["LLC"] = []byte{0xe6, 0xe7, 0}
			f["Frame Kind"] = "SN Read Response"
			f["SN Read"] = rsn
		}
		return f
	}
	for _, tc := range []struct {
		name, wire string
		dir        int
		fields     map[string]any
	}{{"SDK-variable-name-addresses", q, 0, common(true)}, {"SDK-generic-response-mixed-result", r, 1, common(false)}, {"adjacent-normal-complete", "", 0, nil}} {
		for _, tr := range []string{"tcp", "udp"} {
			for _, deferred := range []bool{false, true} {
				for _, chunk := range []int{0, 1, 7} {
					if tr == "udp" && chunk != 0 {
						continue
					}
					t.Run(fmt.Sprintf("%s/%s/d%v/chunk%d", tc.name, tr, deferred, chunk), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(41000, 4059))
						require.NoError(t, err)
						defer s.Close("cleanup")
						s.(*captureSession).f.a.config.Deferred = deferred
						feed := func(dir int, w string) []*ProtocolEvent {
							wire := wrapperWire(t, w)
							var es []*ProtocolEvent
							for at := 0; at < len(wire); {
								end := len(wire)
								if chunk > 0 {
									end = min(end, at+chunk)
								}
								in := bytes.Clone(wire[at:end])
								out := s.Feed(dir, time.Unix(100, 0), in)
								clear(in)
								require.Equal(t, end-at, out.Consumed)
								if len(out.Events) == 0 {
									rocTypedError(t, "NeedMore", out.Err)
									require.True(t, out.NeedMore)
								} else {
									require.Nil(t, out.Err)
									require.False(t, out.NeedMore)
								}
								es = append(es, out.Events...)
								at = end
							}
							return es
						}
						if tc.wire == "" {
							qs := feed(0, "7ea0190321107fdae6e600c001cf000100002a0000ff0200cfc97e")
							rs := feed(1, "7ea012210330689de6e700c401cf001107e6977e")
							require.Len(t, qs, 1)
							require.Len(t, rs, 1)
							require.Equal(t, qs[0].ID, rs[0].ResponseTo)
							require.Zero(t, rs[0].TransactionID)
							f, err := rs[0].GetFields()
							require.NoError(t, err)
							require.Equal(t, "Get Response Normal", f["Frame Kind"])
							require.Equal(t, byte(7), f["Data Value"])
						} else {
							es := feed(tc.dir, tc.wire)
							require.Len(t, es, 1)
							e := es[0]
							require.Equal(t, "dlms", e.Protocol)
							require.Equal(t, "dlms-hdlc-sn-read", e.Profile)
							require.Zero(t, e.TransactionID)
							require.Zero(t, e.ResponseTo)
							require.Equal(t, tc.dir, e.Direction)
							require.Equal(t, wrapperWire(t, tc.wire), e.Raw)
							if tr == "udp" {
								require.Zero(t, e.FlowID)
							} else {
								require.NotZero(t, e.FlowID)
							}
							rocEqualFields(t, tc.fields, e.Session)
							f, err := e.GetFields()
							require.NoError(t, err)
							rocEqualFields(t, tc.fields, f)
							hdlcOpTypes(t, f)
							sn := f["SN Read"].(map[string]any)
							require.IsType(t, uint64(0), sn["list_count"])
							for _, k := range []string{"Invoke ID", "Invoke ID and Priority", "Confirmed Service", "High Priority", "Outstanding"} {
								require.NotContains(t, f, k)
							}
							poisonWrapperStructured(f)
							actionPoison(e)
							f, err = e.GetFields()
							require.NoError(t, err)
							rocEqualFields(t, tc.fields, f)
						}
						require.Empty(t, s.Close("done"))
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
					})
				}
			}
		}
	}
}
