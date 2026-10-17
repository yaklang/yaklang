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

func gbtSetTypes(t *testing.T, value any) {
	t.Helper()
	switch v := value.(type) {
	case map[string]any:
		for k, x := range v {
			switch k {
			case "wrapper_version", "source_wport", "destination_wport", "block_number", "block_number_ack", "class_id":
				require.IsType(t, uint16(0), x, k)
			case "command", "block_control", "window_size", "service_choice", "invoke_id_and_priority", "invoke_id", "unused_flag_bits_raw", "data_access_result", "access_selection_raw", "request_command", "type":
				require.IsType(t, byte(0), x, k)
			case "attribute_id":
				require.IsType(t, int8(0), x, k)
			case "block_data_length", "list_count", "value_count", "length":
				require.IsType(t, uint64(0), x, k)
			case "payload_length", "assembled_data_length", "Outstanding", "index", "apdu_offset", "encoded_length", "success_item_count", "error_item_count":
				require.IsType(t, int(0), x, k)
			case "transfer_start_event", "acknowledged_block_event", "duplicate_of_event":
				require.IsType(t, uint64(0), x, k)
			}
			if k == "value" && (v["type"] == byte(17) || v["type"] == byte(18)) {
				require.IsType(t, uint64(0), x, k)
			}
			gbtSetTypes(t, x)
		}
	case []map[string]any:
		for _, x := range v {
			gbtSetTypes(t, x)
		}
	case []any:
		for _, x := range v {
			gbtSetTypes(t, x)
		}
	}
}

func TestDLMSWrapperGBTSetExistingAPI(t *testing.T) {
	raw := finsReadBatch(t, "dlms-wrapper-gbt-set", "DLMS_WRAPPER_GBT_SET_TEST_ROOT", "baseline-controls.json")
	require.Equal(t, "284237f46232bdd6d3a0e53a4d62221c4f3d8c1559adcce6357a0341d328275a", fmt.Sprintf("%x", sha256.Sum256(raw)))
	errata := finsReadBatch(t, "dlms-wrapper-gbt-set", "DLMS_WRAPPER_GBT_SET_TEST_ROOT", "TYPE-ERRATUM.json")
	require.Equal(t, "7f87bbff2499d689b81960566ed0dc478e67e05e4fe12c6492735d9987e8eebb", fmt.Sprintf("%x", sha256.Sum256(errata)))
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
							gbtSetTypes(t, f)
							gbtSetTypes(t, ev.Session)
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
