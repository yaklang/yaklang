package pcaputil

import (
	"encoding/hex"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

// Existing ordinary 0/1 responses preserve exact diagnostics as well as fields.
// Neither a count overflow nor a canonical port can prove a new block choice.
func TestDLMSWrapperSNMultiResultOrdinaryBudgetCompatibility(t *testing.T) {
	for _, write := range []bool{false, true} {
		for _, count := range []int{16, 17} {
			for _, tr := range []string{"tcp", "udp"} {
				for _, deferred := range []bool{false, true} {
					t.Run(fmt.Sprintf("write%t/count%d/%s/d%t", write, count, tr, deferred), func(t *testing.T) {
						cmd := byte(12)
						row := "0103"
						name := "Read"
						profile := "dlms-wrapper-v1-sn-read"
						assoc := "unverified-sn-read-observation"
						if write {
							cmd = 13
							row = "00"
							name = "Write"
							profile = "dlms-wrapper-v1-sn-write"
							assoc = "unverified-sn-write-observation"
						}
						apdu := fmt.Sprintf("%02x%02x", cmd, count) + strings.Repeat(row, count)
						hexwire := fmt.Sprintf("000100010010%04x", len(apdu)/2) + apdu
						wire := wrapperWire(t, hexwire)
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(40000, 4059))
						require.NoError(t, err)
						defer s.Close("cleanup")
						s.(*captureSession).f.a.config.Deferred = deferred
						result := s.Feed(1, time.Unix(100, 0), append([]byte(nil), wire...))
						require.Equal(t, len(wire), result.Consumed)
						require.False(t, result.NeedMore)
						require.Len(t, result.Events, 1)
						e := result.Events[0]
						require.Equal(t, profile, e.Profile)
						require.Zero(t, e.TransactionID)
						require.Zero(t, e.ResponseTo)
						if count == 17 {
							message := "DLMS Wrapper SN " + name + " list exceeds selected collection budget"
							require.NotNil(t, result.Err)
							require.Equal(t, ErrResourceExceeded, result.Err.Kind)
							require.Equal(t, message, result.Err.Message)
							require.Equal(t, "ResourceExceeded: "+message, e.Error)
							require.Equal(t, "limited", e.Status)
							require.Equal(t, "limited", e.Completeness)
							require.Nil(t, e.Fields)
							require.Nil(t, e.Session)
							snMultiOwnedDiagnosis(t, e, "ResourceExceeded", message)
						} else {
							require.Nil(t, result.Err)
							require.Equal(t, wire, e.Raw)
							expected := map[string]any{"wrapper_version": uint16(1), "source_wport": uint16(1), "destination_wport": uint16(16), "payload_length": len(apdu) / 2, "wrapper_header_hex": hexwire[:16], "apdu_hex": apdu, "command": cmd, "reference_mode": "short-name", "association_observed": false, "authentication_verified": false, "object_model_verified": false, "operation_success_verified": false, "list_count": uint64(count), "list_count_encoding_hex": "10", "kind": name + "Response", "error_item_count": count}
							rows := make([]map[string]any, count)
							for i := range rows {
								if write {
									rows[i] = map[string]any{"index": i, "apdu_offset": 2 + i, "result_choice_raw": byte(0), "result_choice": "success", "result_success": true, "raw_hex": "00", "encoded_length": 1}
								} else {
									rows[i] = map[string]any{"index": i, "apdu_offset": 2 + 2*i, "result_choice_raw": byte(1), "result_choice": "data-access-error", "data_access_result": byte(3), "raw_hex": "0103", "encoded_length": 2}
								}
							}
							expected["results"] = rows
							if write {
								expected["error_item_count"], expected["success_item_count"] = 0, count
							} else {
								expected["data_item_count"] = 0
							}
							fields, err := e.GetFields()
							require.NoError(t, err)
							require.Equal(t, expected, fields)
							snReadTypes(t, fields)
							session := cloneSession(expected)
							session["Association"], session["Outstanding"] = assoc, 0
							require.Equal(t, session, e.Session)
							poisonWrapperStructured(fields)
							poisonWrapperStructured(e.Session)
							again, err := e.GetFields()
							require.NoError(t, err)
							require.Equal(t, expected, again)
							raw, err := hex.DecodeString(hexwire)
							require.NoError(t, err)
							require.Equal(t, raw, e.Raw)
						}
						require.Empty(t, s.Close("compat"))
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
					})
				}
			}
		}
	}
}
