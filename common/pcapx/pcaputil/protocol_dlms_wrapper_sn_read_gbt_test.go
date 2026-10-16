package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

// Completing a GBT carrier must not silently expand a separately selected
// wire-only SN profile. This uses the public API against the unguarded draft.
func TestDLMSWrapperSNReadGBTBoundary(t *testing.T) {
	for _, command := range []byte{5, 12} {
		for _, tr := range []string{"tcp", "udp"} {
			for _, d := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/%s/d%t", command, tr, d), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					defer s.Close("cleanup")
					s.(*captureSession).f.a.config.Deferred = d
					normal := snReadControls(t)[2]
					qwire := wrapperWire(t, normal.Steps[0]["input_hex"].(string))
					q := s.Feed(0, time.Unix(100, 0), qwire)
					require.Nil(t, q.Err)
					require.Len(t, q.Events, 1)
					qe := q.Events[0]
					f, err := qe.GetFields()
					require.NoError(t, err)
					wrapperCloseEqual(t, f, normal.Steps[0]["GetFields"])
					expected := cloneSession(f)
					expected["Association"], expected["Outstanding"] = "observed-request", 1
					wrapperCloseEqual(t, qe.Session, expected)
					require.Equal(t, qe.ID, qe.TransactionID)
					require.Zero(t, qe.ResponseTo)
					flow := qe.FlowID
					require.NotZero(t, flow)
					in := wrapperWire(t, snReadControls(t)[0].Steps[int(command/12)]["input_hex"].(string))[8:]
					p := append([]byte{0xe0, 0x81, 0, 1, 0, 0, byte(len(in))}, in...)
					w := make([]byte, 8+len(p))
					w[1] = 1
					dir := 0
					src, dst := uint16(16), uint16(1)
					if command == 12 {
						dir = 1
						src, dst = dst, src
					}
					binary.BigEndian.PutUint16(w[2:], src)
					binary.BigEndian.PutUint16(w[4:], dst)
					binary.BigEndian.PutUint16(w[6:], uint16(len(p)))
					copy(w[8:], p)
					rejected := s.Feed(dir, time.Unix(101, 0), w)
					require.Len(t, rejected.Events, 1)
					e := rejected.Events[0]
					rocTypedError(t, "UnsupportedFeature", rejected.Err)
					require.Equal(t, flow, e.FlowID)
					require.Equal(t, "dlms-wrapper-v1-gbt", e.Profile)
					require.Equal(t, w, e.Raw)
					require.Nil(t, e.Session)
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.ResponseTo)
					snReadQueryError(t, e, "UnsupportedFeature")
					wrapperClosePoison(e)
					snReadQueryError(t, e, "UnsupportedFeature")
					late := s.Feed(1, time.Unix(102, 0), wrapperWire(t, normal.Steps[1]["input_hex"].(string)))
					require.Nil(t, late.Err)
					require.Len(t, late.Events, 1)
					le := late.Events[0]
					lf, err := le.GetFields()
					require.NoError(t, err)
					wrapperCloseEqual(t, lf, normal.Steps[1]["GetFields"])
					ls := cloneSession(lf)
					ls["Association"], ls["Outstanding"] = "ambiguous-conversation", 0
					wrapperCloseEqual(t, le.Session, ls)
					require.Zero(t, le.TransactionID)
					require.Zero(t, le.ResponseTo)
					require.Equal(t, flow, le.FlowID)
					require.Empty(t, s.Close("GBT-boundary"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}

func snReadGBTSuppControls(t *testing.T) []snReadControl {
	t.Helper()
	raw := finsReadBatch(t, "dlms-wrapper-sn-read", "DLMS_WRAPPER_SN_READ_TEST_ROOT", "gbt-supplement.json")
	require.Equal(t, "bc77182c122011ab4d5aab9c06f962309bf5806a28104590d92733e44db9fbea", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []snReadControl }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 2)
	return d.Cases
}
func TestDLMSWrapperSNReadGBTNative(t *testing.T) {
	for _, c := range snReadGBTSuppControls(t) {
		for _, tr := range []string{"tcp", "udp"} {
			for _, d := range []bool{false, true} {
				chunks := []int{0, 1, 7}
				if tr == "udp" {
					chunks = []int{0}
				}
				for _, chunk := range chunks {
					t.Run(fmt.Sprintf("%s/%s/d%t/chunk%d", c.Name, tr, d, chunk), func(t *testing.T) {
						b := DefaultParserBudget()
						if c.Budget.Collection > 0 {
							b.MaxCollectionElements = c.Budget.Collection
						}
						if c.Budget.Depth > 0 {
							b.MaxRecursionDepth = c.Budget.Depth
						}
						s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(tr), WithSessionPorts(40000, 4059))
						require.NoError(t, err)
						defer s.Close("cleanup")
						s.(*captureSession).f.a.config.Deferred = d
						ids := make([]uint64, len(c.Steps))
						saved := make([]*ProtocolEvent, len(c.Steps))
						var flow uint64
						for i, given := range c.Steps {
							w := snReadStep(t, given, tr)
							if tr == "udp" {
								w["direction"] = w["native_udp_direction"]
							}
							wire := wrapperWire(t, w["input_hex"].(string))
							require.Equal(t, w["input_sha256"], fmt.Sprintf("%x", sha256.Sum256(wire)))
							var es []*ProtocolEvent
							size := chunk
							if size == 0 {
								size = len(wire)
							}
							fatal := false
							if tr == "tcp" {
								for _, o := range c.Overrides {
									if o.Step == i+1 {
										require.Equal(t, "FatalSessionError", o.Error)
										require.Zero(t, o.Events)
										require.Equal(t, len(wire), o.Consumed)
										require.False(t, o.NeedMore)
										fatal = true
									}
								}
							}
							var final *ProtocolError
							for at := 0; at < len(wire); at += size {
								end := min(at+size, len(wire))
								part := append([]byte(nil), wire[at:end]...)
								r := s.Feed(int(w["direction"].(float64)), time.Unix(int64(w["timestamp_unix"].(float64)), 0), part)
								clear(part)
								require.Equal(t, end-at, r.Consumed)
								if fatal {
									require.Empty(t, r.Events)
									require.False(t, r.NeedMore)
									rocTypedError(t, "FatalSessionError", r.Err)
									require.Equal(t, "session stopped", r.Err.Message)
								} else if len(r.Events) == 0 {
									require.Equal(t, "tcp", tr)
									rocTypedError(t, "NeedMore", r.Err)
									require.True(t, r.NeedMore)
								} else {
									require.False(t, r.NeedMore)
									final = r.Err
									es = append(es, r.Events...)
								}
							}
							if fatal {
								continue
							}
							require.Len(t, es, 1)
							if w["error"] == nil {
								require.Nil(t, final)
							} else {
								rocTypedError(t, w["query_error"].(string), final)
							}
							e := es[0]
							expectedFlow := snReadFlow(t, e, w, tr, false, &flow)
							ids[i] = e.ID
							snReadCheckEvent(t, e, w, ids, d, expectedFlow)
							wrapperClosePoison(e)
							clear(wire)
							snReadCheckSaved(t, e, w, ids)
							saved[i] = e
						}
						snReadCheckClose(t, c, s.Close(c.Close.Reason), tr, flow, false)
						require.Empty(t, s.Close("repeat"))
						require.Zero(t, s.Stats().BufferedBytes)
						require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
						want := c.Close
						if tr == "tcp" {
							want = c.TCPClose
						}
						require.EqualValues(t, len(want.Events), s.Stats().Incomplete)
						for i, e := range saved {
							if e != nil {
								snReadCheckSaved(t, e, c.Steps[i], ids)
							}
						}
					})
				}
			}
		}
	}
}
func TestDLMSWrapperSNReadGBTMatrix(t *testing.T) {
	for _, c := range snReadGBTSuppControls(t) {
		for _, tr := range []string{"tcp", "udp"} {
			t.Run(c.Name+"/"+tr, func(t *testing.T) {
				var reuse map[string]string
				require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-wrapper-sn-read", "DLMS_WRAPPER_SN_READ_TEST_ROOT", "reuse-inputs.json"), &reuse))
				name := c.Name + "." + tr + ".pcap"
				var raw []byte
				if p, ok := reuse[name]; ok {
					parts := strings.SplitN(p, "/", 2)
					require.Len(t, parts, 2)
					raw = finsReadBatch(t, parts[0], "DLMS_WRAPPER_SN_READ_TEST_ROOT", parts[1])
				} else {
					raw = finsReadBatch(t, "dlms-wrapper-sn-read", "DLMS_WRAPPER_SN_READ_TEST_ROOT", name)
				}
				discoveryMatrix(t, func(t *testing.T, workers int, d, o bool) {
					cfg := hdlcOpCase{}
					cfg.Steps = make([]hdlcOpStep, len(c.Steps))
					cfg.Budget.Collection = c.Budget.Collection
					cfg.Budget.Depth = c.Budget.Depth
					es, stats := selectedReplay(t, cfg, raw, workers, d, o, tr, "dlms-wrapper")
					indices := make([]int, 0, len(c.Steps))
					for i := range c.Steps {
						skip := false
						if tr == "tcp" {
							for _, v := range c.Overrides {
								if v.Step == i+1 {
									skip = true
								}
							}
						}
						if !skip {
							indices = append(indices, i)
						}
					}
					close := c.Close
					if tr == "tcp" {
						close = c.TCPClose
					}
					require.Len(t, es, len(indices)+len(close.Events))
					ids := make([]uint64, len(c.Steps))
					for j, i := range indices {
						ids[i] = es[j].ID
					}
					var flow uint64
					for j, i := range indices {
						e := es[j]
						w := snReadStep(t, c.Steps[i], tr)
						if tr == "udp" {
							w["direction"] = w["capture_udp_direction"]
						}
						expectedFlow := snReadFlow(t, e, w, tr, true, &flow)
						snReadCheckEvent(t, e, w, ids, d, expectedFlow)
						require.Equal(t, CaptureDomain{}, e.Domain)
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						shift := 1
						if tr == "tcp" {
							shift = 4
						}
						require.EqualValues(t, i+shift, e.SourceBytes.PacketRefs[0].Number)
						require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
						wrapperClosePoison(e)
						snReadCheckSaved(t, e, w, ids)
					}
					snReadCheckClose(t, c, es[len(indices):], tr, flow, true)
					require.EqualValues(t, len(indices), stats.Messages)
					require.EqualValues(t, len(close.Events), stats.Incomplete)
				})
			})
		}
	}
}
