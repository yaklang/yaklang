package pcaputil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func gbtActionListTypes(t *testing.T, value any) {
	t.Helper()
	switch v := value.(type) {
	case map[string]any:
		for k, x := range v {
			switch k {
			case "wrapper_version", "source_wport", "destination_wport", "block_number", "block_number_ack", "class_id":
				require.IsType(t, uint16(0), x, k)
			case "command", "block_control", "window_size", "service_choice", "invoke_id_and_priority", "invoke_id", "unused_flag_bits_raw", "data_access_result", "access_selection_raw", "request_command", "type", "action_result", "return_choice_raw":
				require.IsType(t, byte(0), x, k)
			case "attribute_id", "method_id":
				require.IsType(t, int8(0), x, k)
			case "block_data_length", "list_count", "value_count", "parameter_count", "length":
				require.IsType(t, uint64(0), x, k)
			case "payload_length", "assembled_data_length", "Outstanding", "index", "apdu_offset", "encoded_length", "success_item_count", "error_item_count":
				require.IsType(t, int(0), x, k)
			case "transfer_start_event", "acknowledged_block_event", "duplicate_of_event":
				require.IsType(t, uint64(0), x, k)
			}
			if k == "value" && (v["type"] == byte(17) || v["type"] == byte(18)) {
				require.IsType(t, uint64(0), x, k)
			}
			gbtActionListTypes(t, x)
		}
	case []map[string]any:
		for _, x := range v {
			gbtActionListTypes(t, x)
		}
	case []any:
		for _, x := range v {
			gbtActionListTypes(t, x)
		}
	}
}

func TestDLMSWrapperGBTActionListExistingAPI(t *testing.T) {
	raw := finsReadBatch(t, "dlms-wrapper-gbt-action-list", "DLMS_WRAPPER_GBT_ACTION_LIST_TEST_ROOT", "baseline-controls.json")
	require.Equal(t, "35fef57f82fff186c5c66a186c67013624242fdfa994bf8a64f42c7d8fb03dc2", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct{ Cases []wrapperCloseCase }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Cases, 3)
	for _, original := range doc.Cases {
		for _, tr := range []string{"tcp", "udp"} {
			for _, d := range []bool{false, true} {
				chunks := []int{0, 1, 7}
				if tr == "udp" {
					chunks = []int{0}
				}
				for _, chunk := range chunks {
					t.Run(fmt.Sprintf("%s/%s/d%t/chunk%d", original.Name, tr, d, chunk), func(t *testing.T) {
						c := original
						c.Transport = tr
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(40000, 4059))
						require.NoError(t, err)
						defer s.Close("cleanup")
						s.(*captureSession).f.a.config.Deferred = d
						var ids []uint64
						var saved []*ProtocolEvent
						var flow uint64
						for _, given := range c.Steps {
							encoded, e := json.Marshal(given)
							require.NoError(t, e)
							var w map[string]any
							require.NoError(t, json.Unmarshal(encoded, &w))
							if tr == "tcp" {
								w["offset"] = w["native_tcp_offset"]
							}
							wire, e := hex.DecodeString(w["input_hex"].(string))
							require.NoError(t, e)
							var es []*ProtocolEvent
							size := chunk
							if size == 0 {
								size = len(wire)
							}
							for at := 0; at < len(wire); at += size {
								part := wire[at:min(at+size, len(wire))]
								r := s.Feed(int(w["direction"].(float64)), time.Unix(int64(w["timestamp_unix"].(float64)), 0), part)
								require.Equal(t, len(part), r.Consumed)
								if r.Err != nil {
									require.Equal(t, ErrNeedMore, r.Err.Kind)
									require.True(t, r.NeedMore)
									require.Empty(t, r.Events)
								}
								if len(r.Events) > 0 {
									require.False(t, r.NeedMore)
								}
								es = append(es, r.Events...)
							}
							require.Len(t, es, 1)
							ev := es[0]
							if flow == 0 {
								flow = ev.FlowID
								require.NotZero(t, flow)
							}
							ids = append(ids, ev.ID)
							wrapperCloseMessage(t, ev, w, c, ids, d, false, flow)
							f, e := ev.GetFields()
							require.NoError(t, e)
							gbtActionListTypes(t, f)
							gbtActionListTypes(t, ev.Session)
							wrapperClosePoison(ev)
							clear(wire)
							wrapperCloseQueries(t, ev, w, ids)
							saved = append(saved, ev)
						}
						require.Empty(t, s.Close("baseline"))
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
						require.Zero(t, s.Stats().Incomplete)
						for i, e := range saved {
							wrapperCloseQueries(t, e, c.Steps[i], ids)
						}
					})
				}
			}
		}
	}
}
