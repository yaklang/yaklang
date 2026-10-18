package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// All expected outer fields and Data bytes are independently derived from the
// fixed reference grammar. This witness uses only pre-existing public APIs;
// missing internal symbols or an overlay/vet failure is not a baseline failure.
func TestDLMSWrapperSNBlocksExistingAPI(t *testing.T) {
	raw := finsReadBatch(t, "dlms-sn-read-service-block", "DLMS_SN_READ_BLOCK_TEST_ROOT", "source-candidates.json")
	require.Equal(t, "a6f7f7199b6d30d405318938eb349c58bd8e2a0cc50e5caaf88e5889c731315b", fmt.Sprintf("%x", sha256.Sum256(raw)))
	type vector struct {
		Wire   string         `json:"wrapper_wire_hex"`
		Dir    int            `json:"caller_direction"`
		Fields map[string]any `json:"outer_fields"`
	}
	var gold struct {
		Trace []vector `json:"minimal_two_block_trace"`
		Last  vector   `json:"single_last_response"`
	}
	require.NoError(t, json.Unmarshal(raw, &gold))
	require.Len(t, gold.Trace, 4)
	for _, mode := range []string{"two-block-single-Data", "single-last-Data", "adjacent-LN-owner"} {
		for _, tr := range []string{"tcp", "udp"} {
			for _, deferred := range []bool{false, true} {
				for _, chunk := range []int{1, 7, 65543} {
					if tr == "udp" && chunk != 65543 {
						continue
					}
					t.Run(fmt.Sprintf("%s/%s/%v/%d", mode, tr, deferred, chunk), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(40000, 4059))
						require.NoError(t, err)
						defer s.Close("cleanup")
						s.(*captureSession).f.a.config.Deferred = deferred
						var offsets [2]uint64
						var retainedFlow uint64
						feed := func(v vector, index int) *ProtocolEvent {
							wire := wrapperWire(t, v.Wire)
							var es []*ProtocolEvent
							for at := 0; at < len(wire); {
								end := min(at+chunk, len(wire))
								in := append([]byte(nil), wire[at:end]...)
								r := s.Feed(v.Dir, time.Unix(100+int64(index), 0), in)
								clear(in)
								require.Equal(t, end-at, r.Consumed)
								if len(r.Events) == 0 {
									require.Equal(t, "tcp", tr)
									require.True(t, r.NeedMore)
									rocTypedError(t, "NeedMore", r.Err)
								} else {
									require.Nil(t, r.Err)
									require.False(t, r.NeedMore)
								}
								es = append(es, r.Events...)
								at = end
							}
							require.Len(t, es, 1)
							e := es[0]
							require.NotZero(t, e.ID)
							require.Equal(t, "dlms-wrapper", e.Protocol)
							require.Equal(t, v.Dir, e.Direction)
							require.Equal(t, time.Unix(100+int64(index), 0), e.Timestamp)
							require.Equal(t, len(wire), e.Length)
							require.Equal(t, wire, e.Raw)
							if tr == "tcp" {
								require.Equal(t, offsets[v.Dir], e.Offset)
							} else {
								require.Zero(t, e.Offset)
							}
							offsets[v.Dir] += uint64(len(wire))
							return e
						}
						if mode == "adjacent-LN-owner" {
							q := feed(vector{Wire: "000100100001000dc001c1000100002a0000ff0200", Dir: 0}, 0)
							qf, err := q.GetFields()
							require.NoError(t, err)
							require.Equal(t, "GetRequestNormal", qf["kind"])
							r := feed(vector{Wire: "0001000100100006c401c1001107", Dir: 1}, 1)
							rf, err := r.GetFields()
							require.NoError(t, err)
							require.Equal(t, "GetResponseNormal", rf["kind"])
							require.Equal(t, q.ID, r.ResponseTo)
							require.Equal(t, q.ID, r.TransactionID)
							require.Equal(t, "observed-response", r.Session["Association"])
							require.Equal(t, 0, r.Session["Outstanding"])
							require.Equal(t, map[string]any{"type": byte(17), "value": uint64(7), "raw_hex": "1107"}, rf["data"])
						} else {
							trace := gold.Trace
							if mode == "single-last-Data" {
								trace = []vector{gold.Last}
							}
							var saved []*ProtocolEvent
							var wants []map[string]any
							var start, previous uint64
							for i, v := range trace {
								e := feed(v, i)
								f, err := e.GetFields()
								require.NoError(t, err)
								wire := wrapperWire(t, v.Wire)
								want := cloneSession(v.Fields)
								want["wrapper_version"], want["source_wport"], want["destination_wport"] = uint16(1), binary.BigEndian.Uint16(wire[2:4]), binary.BigEndian.Uint16(wire[4:6])
								want["wrapper_header_hex"] = hex.EncodeToString(wire[:8])
								assoc, outstanding := "unverified-sn-read-observation", 0
								profile := "dlms-wrapper-v1-sn-read-block"
								switch {
								case len(trace) == 1:
									start = e.ID
									want["transfer_complete"] = true
									want["transfer_start_event"], want["observed_block_count"], want["assembled_data_length"] = start, 1, 5
									want["assembled_data_hex"], want["assembled_data"] = "0600000007", map[string]any{"type": byte(6), "value": uint64(7), "raw_hex": "0600000007"}
									assoc = "unverified-sn-wire-group-complete"
								case i == 0:
									profile = "dlms-wrapper-v1-sn-read"
								case i == 1:
									start = e.ID
									want["transfer_complete"] = false
									want["transfer_start_event"], want["observed_block_count"], want["assembled_data_length"] = start, 1, 2
									assoc, outstanding = "unverified-sn-data-block", 1
								case i == 2:
									want["acknowledged_block_event"] = previous
									assoc, outstanding = "unverified-sn-block-number-access", 1
								case i == 3:
									want["transfer_complete"] = true
									want["transfer_start_event"], want["observed_block_count"], want["assembled_data_length"] = start, 2, 5
									want["assembled_data_hex"], want["assembled_data"] = "0600000007", map[string]any{"type": byte(6), "value": uint64(7), "raw_hex": "0600000007"}
									assoc = "unverified-sn-wire-group-complete"
								}
								require.Equal(t, profile, e.Profile)
								rocEqualFields(t, want, f)
								session := cloneSession(want)
								session["Association"], session["Outstanding"] = assoc, outstanding
								rocEqualFields(t, session, e.Session)
								require.Zero(t, e.TransactionID)
								require.Zero(t, e.ResponseTo)
								require.IsType(t, uint16(0), f["source_wport"])
								require.IsType(t, byte(0), f["command"])
								if assembled, ok := f["assembled_data"]; ok {
									m, ok := assembled.(map[string]any)
									require.True(t, ok)
									require.IsType(t, byte(0), m["type"])
									require.IsType(t, uint64(0), m["value"])
								}
								if i > 0 || len(trace) == 1 {
									require.NotZero(t, e.FlowID)
									if retainedFlow == 0 {
										retainedFlow = e.FlowID
									}
									require.Equal(t, retainedFlow, e.FlowID)
								}
								poisonWrapperStructured(f)
								poisonWrapperStructured(e.Session)
								again, err := e.GetFields()
								require.NoError(t, err)
								rocEqualFields(t, want, again)
								d, err := e.Decode()
								require.NoError(t, err)
								dm := d
								require.NotNil(t, dm)
								df, ok := dm["fields"].(map[string]any)
								require.True(t, ok)
								rocEqualFields(t, want, df)
								ds, ok := dm["session"].(map[string]any)
								require.True(t, ok)
								rocEqualFields(t, session, ds)
								poisonWrapperStructured(dm)
								saved = append(saved, e)
								wants = append(wants, want)
								previous = e.ID
							}
							for i, e := range saved {
								f, err := e.GetFields()
								require.NoError(t, err)
								rocEqualFields(t, wants[i], f)
							}
						}
						require.Empty(t, s.Close("baseline"))
						require.Empty(t, s.Close("repeat"))
						require.Zero(t, s.Stats().BufferedBytes)
					})
				}
			}
		}
	}
}
