package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

type snReadCloseSpec struct {
	Reason string
	Events []map[string]any
}
type snReadControl struct {
	Name   string
	Budget struct {
		Collection int `json:"MaxCollectionElements"`
		Depth      int `json:"MaxRecursionDepth"`
	}
	Steps     []map[string]any `json:"metadata"`
	Close     snReadCloseSpec
	TCPClose  snReadCloseSpec `json:"native_tcp_Close"`
	Overrides []struct {
		Step             int    `json:"step_1based"`
		Error            string `json:"error_type"`
		Message          string `json:"error_message"`
		Events, Consumed int
		NeedMore         bool
	} `json:"native_tcp_overrides"`
}

func snReadControls(t *testing.T) []snReadControl {
	t.Helper()
	raw := finsReadBatch(t, "dlms-wrapper-sn-read", "DLMS_WRAPPER_SN_READ_TEST_ROOT", "controls.json")
	require.Equal(t, "7351c3e218e7add499689d78f4f7cc4103f74fdfe0e39cf8b1b46fdd7aeb10b1", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []snReadControl }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 12)
	return snBlocksUpgradeHistorical(t, raw, snParameterizedUpgradeHistoricalWrapper(t, raw, d.Cases))
}
func snReadStep(t *testing.T, given map[string]any, tr string) map[string]any {
	t.Helper()
	raw, err := json.Marshal(given)
	require.NoError(t, err)
	var w map[string]any
	require.NoError(t, json.Unmarshal(raw, &w))
	if tr == "tcp" {
		w["direction"] = w["caller_direction"]
		w["offset"] = w["native_tcp_offset"]
		// Frozen Wrapper UDP resource events release Raw; admitted TCP events own it.
		if w["query_error"] == "ResourceExceeded" {
			w["Raw_hex"] = w["input_hex"]
		}
	}
	return w
}
func snReadQueryError(t *testing.T, e *ProtocolEvent, kind string) {
	t.Helper()
	for pass := 0; pass < 2; pass++ {
		f, err := e.GetFields()
		require.Nil(t, f)
		var pe *ProtocolError
		require.True(t, errors.As(err, &pe))
		require.Equal(t, kind, string(pe.Kind))
		pe.Kind, pe.Message = ErrMalformedMessage, "caller"
		d, err := e.Decode()
		require.Nil(t, d)
		require.True(t, errors.As(err, &pe))
		require.Equal(t, kind, string(pe.Kind))
		pe.Kind, pe.Message = ErrUnsupportedFeature, "caller"
	}
}
func snReadCheckEvent(t *testing.T, e *ProtocolEvent, w map[string]any, ids []uint64, d bool, flow uint64) {
	t.Helper()
	if w["error"] == nil {
		c := wrapperCloseCase{Transport: "udp"}
		wrapperCloseMessage(t, e, w, c, ids, d, false, flow)
		f, err := e.GetFields()
		require.NoError(t, err)
		snReadTypes(t, f)
		snReadTypes(t, e.Session)
	} else {
		require.Equal(t, flow, e.FlowID)
		require.Equal(t, "dlms-wrapper", e.Protocol)
		require.Equal(t, w["profile"], e.Profile)
		require.Equal(t, w["status"], e.Status)
		require.Equal(t, w["completeness"], e.Completeness)
		require.Equal(t, int(w["direction"].(float64)), e.Direction)
		require.Equal(t, int64(w["timestamp_unix"].(float64)), e.Timestamp.Unix())
		require.Equal(t, uint64(w["offset"].(float64)), e.Offset)
		require.Equal(t, w["source"], e.Source)
		require.Equal(t, w["destination"], e.Destination)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
		require.Nil(t, e.Session)
		require.Nil(t, e.Fields)
		require.Nil(t, e.Structured)
		require.Equal(t, int(w["Length"].(float64)), e.Length)
		if w["Raw_hex"] == nil {
			require.Nil(t, e.Raw)
		} else {
			require.Equal(t, wrapperWire(t, w["Raw_hex"].(string)), e.Raw)
		}
		kind := w["query_error"].(string)
		require.Contains(t, e.Error, kind)
		if message, ok := w["error_message"].(string); ok {
			require.Equal(t, kind+": "+message, e.Error)
			_, queryErr := e.GetFields()
			var pe *ProtocolError
			require.True(t, errors.As(queryErr, &pe))
			require.Equal(t, message, pe.Message)
		}
		snReadQueryError(t, e, kind)
	}
}
func snReadCheckSaved(t *testing.T, e *ProtocolEvent, w map[string]any, ids []uint64) {
	t.Helper()
	if w["error"] == nil {
		wrapperCloseQueries(t, e, w, ids)
	} else {
		snReadQueryError(t, e, w["query_error"].(string))
	}
}
func snReadCheckClose(t *testing.T, c snReadControl, es []*ProtocolEvent, tr string, flow uint64, captured bool) {
	t.Helper()
	want := c.Close
	if tr == "tcp" {
		want = c.TCPClose
	}
	require.Len(t, es, len(want.Events))
	for i, w := range want.Events {
		e := es[i]
		require.Equal(t, flow, e.FlowID)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
		require.Nil(t, e.Raw)
		require.Zero(t, e.Length)
		require.Equal(t, "dlms-wrapper", e.Protocol)
		require.Equal(t, w["profile"], e.Profile)
		require.Equal(t, "incomplete", e.Status)
		require.Equal(t, "incomplete", e.Completeness)
		require.Equal(t, CaptureDomain{}, e.Domain)
		require.Equal(t, int(w["direction"].(float64)), e.Direction)
		require.Equal(t, uint64(w["offset"].(float64)), e.Offset)
		require.Equal(t, int64(w["timestamp_unix"].(float64)), e.Timestamp.Unix())
		require.Equal(t, w["source"], e.Source)
		require.Equal(t, w["destination"], e.Destination)
		summary := w["Summary"].(string)
		message := strings.TrimPrefix(w["Error"].(string), "NeedMore: ")
		if captured && tr == "tcp" {
			summary = strings.Replace(summary, want.Reason+":", "ctx-canceled:", 1)
			message = strings.Replace(message, want.Reason+":", "ctx-canceled:", 1)
		}
		require.Equal(t, summary, e.Summary)
		require.Equal(t, "NeedMore: "+message, e.Error)
		wrapperCloseEqual(t, e.Session, w["session"])
		snReadTypes(t, e.Session)
		wrapperCloseNeedMore(t, e, message)
		wrapperClosePoison(e)
		wrapperCloseNeedMore(t, e, message)
	}
}
func TestDLMSWrapperSNReadNative(t *testing.T) {
	for _, c := range snReadControls(t) {
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
func TestDLMSWrapperSNReadMatrix(t *testing.T) {
	for _, c := range snReadControls(t) {
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

// Zero is a concrete SN-only UDP expectation, not a wildcard for any flow.
// Retained LN/association state and every TCP byte flow instead bind one actual
// nonzero ID, which all later events in that context must preserve.
func snReadFlow(t *testing.T, e *ProtocolEvent, w map[string]any, tr string, captured bool, flow *uint64) uint64 {
	t.Helper()
	key := "native_udp_flow_nonzero"
	if tr == "tcp" {
		key = "native_tcp_flow_nonzero"
	} else if captured {
		key = "captured_udp_flow_nonzero"
	}
	nonzero, ok := w[key].(bool)
	require.True(t, ok, key)
	if !nonzero {
		require.Zero(t, e.FlowID)
		return 0
	}
	require.NotZero(t, e.FlowID)
	if *flow == 0 {
		*flow = e.FlowID
	}
	require.Equal(t, *flow, e.FlowID)
	return *flow
}
