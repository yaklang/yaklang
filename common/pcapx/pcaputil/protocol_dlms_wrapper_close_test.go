package pcaputil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"strings"
	"testing"
	"time"
)

type wrapperCloseCase struct {
	Name, Transport string
	Steps           []map[string]any
	Close           struct {
		Reason string
		Events []map[string]any
	}
}

func wrapperCloseCases(t *testing.T) []wrapperCloseCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-wrapper-close", "DLMS_WRAPPER_CLOSE_TEST_ROOT", "controls.json")
	require.Equal(t, "26f9f32f7f9228230d6da9cf547d5791974e1fdbe4896c938aecdc8e25638824", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []wrapperCloseCase }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 12)
	return d.Cases
}
func wrapperCloseEqual(t *testing.T, a, b any) {
	t.Helper()
	aa, e := json.Marshal(a)
	require.NoError(t, e)
	bb, e := json.Marshal(b)
	require.NoError(t, e)
	require.JSONEq(t, string(bb), string(aa))
}
func wrapperCloseRemap(v any, ids []uint64) any {
	switch x := v.(type) {
	case map[string]any:
		o := make(map[string]any, len(x))
		for k, v := range x {
			if k == "acknowledged_block_event" || k == "transfer_start_event" || k == "duplicate_of_event" {
				n := int(v.(float64))
				if n > 0 {
					o[k] = ids[n-1]
				} else {
					o[k] = uint64(0)
				}
			} else {
				o[k] = wrapperCloseRemap(v, ids)
			}
		}
		return o
	case []any:
		o := make([]any, len(x))
		for i, v := range x {
			o[i] = wrapperCloseRemap(v, ids)
		}
		return o
	default:
		return v
	}
}
func wrapperCloseQueries(t *testing.T, e *ProtocolEvent, w map[string]any, ids []uint64) {
	t.Helper()
	f, err := e.GetFields()
	require.NoError(t, err)
	wrapperCloseEqual(t, f, wrapperCloseRemap(w["GetFields"], ids))
	d, err := e.Decode()
	require.NoError(t, err)
	wrapperCloseEqual(t, d, wrapperCloseRemap(w["Decode"], ids))
	poisonWrapperStructured(f)
	poisonWrapperStructured(d)
	f, err = e.GetFields()
	require.NoError(t, err)
	wrapperCloseEqual(t, f, wrapperCloseRemap(w["GetFields"], ids))
}
func wrapperClosePoison(e *ProtocolEvent) {
	if e.Fields != nil {
		poisonWrapperStructured(e.Fields)
	}
	if e.Structured != nil {
		poisonWrapperStructured(e.Structured)
	}
	if e.Session != nil {
		poisonWrapperStructured(e.Session)
	}
	for i := range e.Raw {
		e.Raw[i] = 255
	}
	e.Protocol, e.Profile, e.Status, e.Error = "caller", "caller", "decoded", ""
	e.Fields = map[string]any{"forged": true}
	e.Structured = map[string]any{"fields": map[string]any{"forged": true}}
	e.Session = map[string]any{"Outstanding": 999999}
	e.ID, e.FlowID, e.TransactionID, e.ResponseTo = 999999, 999999, 999999, 999999
}
func wrapperCloseNeedMore(t *testing.T, e *ProtocolEvent, message string) {
	t.Helper()
	for n := 0; n < 2; n++ {
		var f map[string]any
		var d map[string]any
		var err error
		if n == 0 {
			f, err = e.GetFields()
			require.Nil(t, f)
		} else {
			d, err = e.Decode()
			require.Nil(t, d)
		}
		var pe *ProtocolError
		require.True(t, errors.As(err, &pe))
		require.Equal(t, ErrNeedMore, pe.Kind)
		require.Equal(t, message, pe.Message)
		pe.Kind, pe.Message = ErrMalformedMessage, "caller"
	}
}
func wrapperCloseDirection(w map[string]any, c wrapperCloseCase, replay bool) int {
	dir := int(w["direction"].(float64))
	if replay && c.Transport == "udp" && int(c.Steps[0]["direction"].(float64)) == 1 {
		return 1 - dir
	}
	return dir
}
func wrapperCloseMessage(t *testing.T, e *ProtocolEvent, w map[string]any, c wrapperCloseCase, ids []uint64, deferred, replay bool, flow uint64) {
	t.Helper()
	require.Equal(t, flow, e.FlowID)
	require.Equal(t, "dlms-wrapper", e.Protocol)
	require.Equal(t, w["profile"], e.Profile)
	status := "decoded"
	if deferred {
		status = "deferred"
	}
	require.Equal(t, status, e.Status)
	require.Equal(t, "message", e.Completeness)
	require.Empty(t, e.Error)
	require.Equal(t, wrapperCloseDirection(w, c, replay), e.Direction)
	require.Equal(t, uint64(w["offset"].(float64)), e.Offset)
	require.Equal(t, int64(w["timestamp_unix"].(float64)), e.Timestamp.Unix())
	require.Equal(t, w["source"], e.Source)
	require.Equal(t, w["destination"], e.Destination)
	wire, err := hex.DecodeString(w["input_hex"].(string))
	require.NoError(t, err)
	require.Equal(t, wire, e.Raw)
	require.Equal(t, len(wire), e.Length)
	h := sha256.Sum256(wire)
	require.Equal(t, w["input_sha256"], hex.EncodeToString(h[:]))
	owner := func(k string) uint64 {
		n := int(w[k].(float64))
		if n == 0 {
			return 0
		}
		return ids[n-1]
	}
	require.Equal(t, owner("Transaction_index"), e.TransactionID)
	require.Equal(t, owner("Response_index"), e.ResponseTo)
	wrapperCloseEqual(t, e.Session, wrapperCloseRemap(w["session"], ids))
	wrapperCloseQueries(t, e, w, ids)
}
func wrapperCloseEnd(t *testing.T, es []*ProtocolEvent, c wrapperCloseCase, flow uint64, replay bool) {
	t.Helper()
	require.Len(t, es, len(c.Close.Events))
	for i, w := range c.Close.Events {
		e := es[i]
		require.Equal(t, flow, e.FlowID)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.Length)
		require.Nil(t, e.Raw)
		require.Equal(t, "dlms-wrapper", e.Protocol)
		require.Equal(t, w["profile"], e.Profile)
		require.Equal(t, "incomplete", e.Status)
		require.Equal(t, "incomplete", e.Completeness)
		require.Equal(t, wrapperCloseDirection(w, c, replay), e.Direction)
		require.Equal(t, uint64(w["offset"].(float64)), e.Offset)
		require.Equal(t, int64(w["timestamp_unix"].(float64)), e.Timestamp.Unix())
		require.Equal(t, w["source"], e.Source)
		require.Equal(t, w["destination"], e.Destination)
		summary := w["Summary"].(string)
		message := w["error"].(map[string]any)["message"].(string)
		if replay && c.Transport == "tcp" {
			summary = strings.Replace(summary, c.Close.Reason+":", "ctx-canceled:", 1)
			message = strings.Replace(message, c.Close.Reason+":", "ctx-canceled:", 1)
		}
		require.Equal(t, summary, e.Summary)
		require.Equal(t, "NeedMore: "+message, e.Error)
		wrapperCloseEqual(t, e.Session, w["session"])
		require.Len(t, e.Session, 1)
		require.IsType(t, int(0), e.Session["Outstanding"])
		wrapperCloseNeedMore(t, e, message)
		wrapperClosePoison(e)
		wrapperCloseNeedMore(t, e, message)
	}
}
func TestDLMSWrapperCloseNative(t *testing.T) {
	for _, c := range wrapperCloseCases(t) {
		for _, d := range []bool{false, true} {
			chunks := []int{0, 1, 7}
			if c.Transport == "udp" {
				chunks = []int{0}
			}
			for _, chunk := range chunks {
				t.Run(fmt.Sprintf("%s/d%t/chunk%d", c.Name, d, chunk), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(c.Transport), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					defer s.Close("cleanup")
					s.(*captureSession).f.a.config.Deferred = d
					var ids []uint64
					var saved []*ProtocolEvent
					var flow uint64
					for _, w := range c.Steps {
						wire, err := hex.DecodeString(w["input_hex"].(string))
						require.NoError(t, err)
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
							es = append(es, r.Events...)
						}
						require.Len(t, es, 1)
						e := es[0]
						if flow == 0 {
							flow = e.FlowID
							require.NotZero(t, flow)
						}
						ids = append(ids, e.ID)
						wrapperCloseMessage(t, e, w, c, ids, d, false, flow)
						wrapperClosePoison(e)
						clear(wire)
						wrapperCloseQueries(t, e, w, ids)
						saved = append(saved, e)
					}
					wrapperCloseEnd(t, s.Close(c.Close.Reason), c, flow, false)
					require.Empty(t, s.Close("repeat"))
					require.Zero(t, s.Stats().BufferedBytes)
					require.EqualValues(t, len(c.Close.Events), s.Stats().Incomplete)
					for i, e := range saved {
						wrapperCloseQueries(t, e, c.Steps[i], ids)
					}
				})
			}
		}
	}
}
func TestDLMSWrapperCloseMatrix(t *testing.T) {
	for _, c := range wrapperCloseCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			var reuse map[string]string
			require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-wrapper-close", "DLMS_WRAPPER_CLOSE_TEST_ROOT", "reuse-inputs.json"), &reuse))
			name := c.Name + ".pcap"
			var raw []byte
			if path, ok := reuse[name]; ok {
				parts := strings.SplitN(path, "/", 2)
				require.Len(t, parts, 2)
				raw = finsReadBatch(t, parts[0], "DLMS_WRAPPER_CLOSE_TEST_ROOT", parts[1])
			} else {
				raw = finsReadBatch(t, "dlms-wrapper-close", "DLMS_WRAPPER_CLOSE_TEST_ROOT", name)
			}
			discoveryMatrix(t, func(t *testing.T, workers int, d, o bool) {
				var opts []CaptureOption
				if c.Transport == "udp" {
					opts = append(opts, WithProtocolDecodeAs("udp", 4059, "dlms-wrapper"))
				}
				es, stats := discoveryReplay(t, raw, workers, d, o, opts...)
				require.Len(t, es, len(c.Steps)+len(c.Close.Events))
				ids := make([]uint64, len(c.Steps))
				for i := range c.Steps {
					ids[i] = es[i].ID
				}
				flow := es[0].FlowID
				require.NotZero(t, flow)
				for i, w := range c.Steps {
					wrapperCloseMessage(t, es[i], w, c, ids, d, true, flow)
				}
				wrapperCloseEnd(t, es[len(c.Steps):], c, flow, true)
				require.EqualValues(t, len(c.Steps), stats.Messages)
				require.EqualValues(t, len(c.Close.Events), stats.Incomplete)
				for i, e := range es[:len(c.Steps)] {
					wrapperClosePoison(e)
					wrapperCloseQueries(t, e, c.Steps[i], ids)
				}
			})
		})
	}
}

type wrapperCloseHistory struct {
	Name                                    string
	InputAlias                              string `json:"input_alias"`
	MemberAlias                             string `json:"member_alias"`
	MemberSHA                               string `json:"member_sha256"`
	Index                                   int    `json:"index0"`
	CaptureSHA                              string `json:"capture_sha256"`
	Profile, Transport, Source, Destination string
	Direction                               int
	NativeTime                              int64  `json:"native_time_ns"`
	NativeOffset                            uint64 `json:"native_offset"`
	CaptureTime                             int64  `json:"capture_time_ns"`
	CaptureOffset                           uint64 `json:"capture_offset"`
}

func wrapperCloseHistoryItems(t *testing.T) []wrapperCloseHistory {
	t.Helper()
	raw := finsReadBatch(t, "dlms-wrapper-close", "DLMS_WRAPPER_CLOSE_TEST_ROOT", "historical-close-upgrades.json")
	require.Equal(t, "2ed6cfbc412029f88780a75e1ea8e847efc0c457b949630601e6dfcc87dc6421", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct{ Items []wrapperCloseHistory }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Items, 4)
	return doc.Items
}
func wrapperCloseHistorical(t *testing.T, es []*ProtocolEvent, name, alias, transport string, replay bool, reason string) {
	t.Helper()
	var w *wrapperCloseHistory
	for _, v := range wrapperCloseHistoryItems(t) {
		if v.Name == name {
			copy := v
			w = &copy
			break
		}
	}
	require.NotNil(t, w)
	require.Equal(t, w.InputAlias, alias)
	require.Equal(t, w.Transport, transport)
	require.Len(t, es, 1)
	e := es[0]
	require.NotZero(t, e.FlowID)
	require.Zero(t, e.TransactionID)
	require.Zero(t, e.ResponseTo)
	require.Nil(t, e.Raw)
	require.Zero(t, e.Length)
	require.Equal(t, "dlms-wrapper", e.Protocol)
	require.Equal(t, w.Profile, e.Profile)
	require.Equal(t, "incomplete", e.Status)
	require.Equal(t, "incomplete", e.Completeness)
	require.Equal(t, w.Direction, e.Direction)
	require.Equal(t, w.Source, e.Source)
	require.Equal(t, w.Destination, e.Destination)
	at, off := w.NativeTime, w.NativeOffset
	if replay {
		at, off = w.CaptureTime, w.CaptureOffset
	}
	require.Equal(t, at, e.Timestamp.UnixNano())
	require.Equal(t, off, e.Offset)
	wrapperCloseEqual(t, e.Session, map[string]any{"Outstanding": 1})
	require.Len(t, e.Session, 1)
	require.IsType(t, int(0), e.Session["Outstanding"])
	summary := "DLMS Wrapper exchange ended with unmatched observed requests"
	if transport == "tcp" {
		summary = reason + ": " + summary
	}
	require.Equal(t, summary, e.Summary)
	require.Equal(t, "NeedMore: "+summary, e.Error)
	wrapperCloseNeedMore(t, e, summary)
	wrapperClosePoison(e)
	wrapperCloseNeedMore(t, e, summary)
}
func TestDLMSWrapperCloseHistoricalBinding(t *testing.T) {
	for _, w := range wrapperCloseHistoryItems(t) {
		raw, err := trafficfixture.ReadFile(w.MemberAlias)
		require.NoError(t, err)
		require.Equal(t, w.MemberSHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
		var doc struct{ Cases []map[string]any }
		require.NoError(t, json.Unmarshal(raw, &doc))
		c := doc.Cases[w.Index]
		require.Equal(t, w.Name, c["name"])
		require.Equal(t, w.InputAlias, c["input_alias"])
		cap := wrapperStructuredInput(t, w.InputAlias)
		require.Equal(t, w.CaptureSHA, fmt.Sprintf("%x", sha256.Sum256(cap)))
	}
}
