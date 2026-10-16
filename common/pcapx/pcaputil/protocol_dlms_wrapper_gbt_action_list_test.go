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

type gbtActionListCloseSpec struct {
	Reason string
	Events []map[string]any
}
type gbtActionListControl struct {
	Name   string
	Budget struct {
		Collection int `json:"MaxCollectionElements"`
		Depth      int `json:"MaxRecursionDepth"`
	}
	Steps     []map[string]any
	Close     gbtActionListCloseSpec
	TCPClose  gbtActionListCloseSpec `json:"native_tcp_Close"`
	Overrides []struct {
		Step             int    `json:"step_1based"`
		Error            string `json:"error_type"`
		Message          string `json:"error_message"`
		Events, Consumed int
		NeedMore         bool
	} `json:"native_tcp_overrides"`
}

func gbtActionListControls(t *testing.T) []gbtActionListControl {
	t.Helper()
	raw := finsReadBatch(t, "dlms-wrapper-gbt-action-list", "DLMS_WRAPPER_GBT_ACTION_LIST_TEST_ROOT", "controls.json")
	require.Equal(t, "7fbd49b7a8e07abe933955597a236cd6c2ae89574acbfb06397d9b973690aa66", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []gbtActionListControl }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 13)
	return d.Cases
}
func gbtActionListStep(t *testing.T, given map[string]any, tr string) map[string]any {
	t.Helper()
	raw, err := json.Marshal(given)
	require.NoError(t, err)
	var w map[string]any
	require.NoError(t, json.Unmarshal(raw, &w))
	if tr == "tcp" {
		w["offset"] = w["native_tcp_offset"]
		// Frozen Wrapper UDP resource events release Raw; admitted TCP events own it.
		if w["query_error"] == "ResourceExceeded" {
			w["Raw_hex"] = w["input_hex"]
		}
	}
	return w
}
func gbtActionListQueryError(t *testing.T, e *ProtocolEvent, kind string) {
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
func gbtActionListCheckEvent(t *testing.T, e *ProtocolEvent, w map[string]any, ids []uint64, d bool, flow uint64) {
	t.Helper()
	if w["error"] == nil {
		c := wrapperCloseCase{Transport: "udp"}
		wrapperCloseMessage(t, e, w, c, ids, d, false, flow)
		f, err := e.GetFields()
		require.NoError(t, err)
		gbtActionListTypes(t, f)
		gbtActionListTypes(t, e.Session)
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
		gbtActionListQueryError(t, e, kind)
	}
}
func gbtActionListCheckSaved(t *testing.T, e *ProtocolEvent, w map[string]any, ids []uint64) {
	t.Helper()
	if w["error"] == nil {
		wrapperCloseQueries(t, e, w, ids)
	} else {
		gbtActionListQueryError(t, e, w["query_error"].(string))
	}
}
func gbtActionListCheckClose(t *testing.T, c gbtActionListControl, es []*ProtocolEvent, tr string, flow uint64, captured bool) {
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
		gbtActionListTypes(t, e.Session)
		wrapperCloseNeedMore(t, e, message)
		wrapperClosePoison(e)
		wrapperCloseNeedMore(t, e, message)
	}
}
func TestDLMSWrapperGBTActionListNative(t *testing.T) {
	for _, c := range gbtActionListControls(t) {
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
							w := gbtActionListStep(t, given, tr)
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
								rocTypedError(t, w["error"].(string), final)
							}
							e := es[0]
							if flow == 0 {
								flow = e.FlowID
								require.NotZero(t, flow)
							}
							ids[i] = e.ID
							gbtActionListCheckEvent(t, e, w, ids, d, flow)
							wrapperClosePoison(e)
							clear(wire)
							gbtActionListCheckSaved(t, e, w, ids)
							saved[i] = e
						}
						gbtActionListCheckClose(t, c, s.Close(c.Close.Reason), tr, flow, false)
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
								gbtActionListCheckSaved(t, e, c.Steps[i], ids)
							}
						}
					})
				}
			}
		}
	}
}
func TestDLMSWrapperGBTActionListMatrix(t *testing.T) {
	for _, c := range gbtActionListControls(t) {
		for _, tr := range []string{"tcp", "udp"} {
			t.Run(c.Name+"/"+tr, func(t *testing.T) {
				var reuse map[string]string
				require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-wrapper-gbt-action-list", "DLMS_WRAPPER_GBT_ACTION_LIST_TEST_ROOT", "reuse-inputs.json"), &reuse))
				name := c.Name + "." + tr + ".pcap"
				var raw []byte
				if p, ok := reuse[name]; ok {
					parts := strings.SplitN(p, "/", 2)
					require.Len(t, parts, 2)
					raw = finsReadBatch(t, parts[0], "DLMS_WRAPPER_GBT_ACTION_LIST_TEST_ROOT", parts[1])
				} else {
					raw = finsReadBatch(t, "dlms-wrapper-gbt-action-list", "DLMS_WRAPPER_GBT_ACTION_LIST_TEST_ROOT", name)
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
					flow := es[0].FlowID
					require.NotZero(t, flow)
					for j, i := range indices {
						e := es[j]
						w := gbtActionListStep(t, c.Steps[i], tr)
						gbtActionListCheckEvent(t, e, w, ids, d, flow)
						require.Equal(t, CaptureDomain{}, e.Domain)
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						shift := 1
						if tr == "tcp" {
							shift = 4
						}
						require.EqualValues(t, i+shift, e.SourceBytes.PacketRefs[0].Number)
						require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
						wrapperClosePoison(e)
						gbtActionListCheckSaved(t, e, w, ids)
					}
					gbtActionListCheckClose(t, c, es[len(indices):], tr, flow, true)
					require.EqualValues(t, len(indices), stats.Messages)
					require.EqualValues(t, len(close.Events), stats.Incomplete)
				})
			})
		}
	}
}
