package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func uploadControls(t *testing.T) []selectedWrapper {
	t.Helper()
	var x struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-action-upload", "DLMS_ACTION_UPLOAD_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 18)
	out := make([]selectedWrapper, len(x.Cases))
	for i, raw := range x.Cases {
		var c actionCase
		require.NoError(t, json.Unmarshal(raw, &c))
		var m struct {
			Steps []selectedMeta
			Close *struct{ Events []any } `json:"native_tcp_Close_override"`
		}
		require.NoError(t, json.Unmarshal(uploadNativeMeta(t, raw, c), &m))
		out[i] = selectedWrapper{methodListCase: methodListCase{actionCase: c}, Meta: m.Steps, TCPClose: m.Close}
		for _, step := range m.Steps {
			out[i].FlowIDs = append(out[i].FlowIDs, step.FlowID)
		}
	}
	return out
}
func uploadWireCheck(t *testing.T, c methodListCase, es []*ProtocolEvent, transport string, captured, deferred bool) {
	t.Helper()
	require.Len(t, es, len(c.Steps))
	for i, e := range es {
		if c.Steps[i].Error != "" {
			continue
		}
		f, err := e.GetFields()
		require.NoError(t, err)
		if v, ok := f["block_number"]; ok {
			require.IsType(t, uint32(0), v)
		}
		if v, ok := f["raw_data_length"]; ok {
			require.IsType(t, uint64(0), v)
		}
		if v, ok := f["transfer_complete"]; ok {
			require.IsType(t, false, v)
		}
		if v, ok := f["last_block"]; ok {
			require.IsType(t, false, v)
		}
		if v, ok := f["observed_block_count"]; ok {
			require.IsType(t, int(0), v)
		}
		if v, ok := f["assembled_data"].(map[string]any); ok {
			actionDataTypes(t, v)
		}
		for _, key := range []string{"return_data", "parameter"} {
			if v, ok := f[key].(map[string]any); ok {
				actionDataTypes(t, v)
			}
		}
		for _, key := range []string{"command", "service_choice", "invoke_id_and_priority", "invoke_id", "action_result", "data_access_result"} {
			if v, ok := f[key]; ok {
				require.IsType(t, byte(0), v)
			}
		}
		if v, ok := f["method_id"]; ok {
			require.IsType(t, int8(0), v)
		}
		if v, ok := f["class_id"]; ok {
			require.IsType(t, uint16(0), v)
		}
		selectFieldTypes(t, f)
	}
	// Without a retained UDP flow, each captured datagram has flow-local
	// direction zero. Check that public fact before adapting a shallow view to
	// the common checker's wire-global endpoint convention. No field is skipped.
	checked := es
	if captured && transport == "udp" {
		checked = append([]*ProtocolEvent(nil), es...)
		for i, e := range es {
			if want, ok := c.FlowIDs[i].(float64); ok && want == 0 {
				require.Zero(t, e.FlowID)
				require.Zero(t, e.Direction)
				view := *e
				view.Direction = c.Steps[i].Direction ^ c.Steps[0].Direction
				checked[i] = &view
			}
		}
	}
	selectedWrapperWireCheck(t, c, checked, transport, captured, deferred)
}
func uploadNative(t *testing.T, cases []selectedWrapper, source, destination uint16) {
	t.Helper()
	for _, c := range cases {
		for _, transport := range []string{"udp", "tcp"} {
			for _, deferred := range []bool{false, true} {
				for _, chunk := range []int{0, 1, 7} {
					if transport == "udp" && chunk != 0 {
						continue
					}
					t.Run(fmt.Sprintf("%s/%s/d%t/chunk%d", c.Name, transport, deferred, chunk), func(t *testing.T) {
						b := DefaultParserBudget()
						if c.Budget.Collection > 0 {
							b.MaxCollectionElements = c.Budget.Collection
						}
						if c.Budget.Depth > 0 {
							b.MaxRecursionDepth = c.Budget.Depth
						}
						s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(transport), WithSessionPorts(source, destination))
						require.NoError(t, err)
						s.(*captureSession).f.a.config.Deferred = deferred
						var es []*ProtocolEvent
						for i, step := range c.Steps {
							w := wrapperWire(t, step.Wire)
							before := len(es)
							var final *ProtocolError
							meta := c.Meta[i]
							for at := 0; at < len(w); {
								end := len(w)
								if chunk > 0 {
									end = min(end, at+chunk)
								}
								input := bytes.Clone(w[at:end])
								o := s.Feed(step.Direction, time.Unix(step.At, 0), input)
								clear(input)
								require.Equal(t, end-at, o.Consumed)
								if transport == "tcp" && meta.TCPOverride != nil {
									require.Empty(t, o.Events)
									rocTypedError(t, meta.TCPOverride.Error, o.Err)
									require.Equal(t, meta.TCPOverride.Message, o.Err.Message)
									require.False(t, o.NeedMore)
								} else if len(o.Events) == 0 {
									if transport == "tcp" {
										rocTypedError(t, "NeedMore", o.Err)
									}
								} else {
									final = o.Err
								}
								for _, e := range o.Events {
									es = append(es, actionSnapshot(e))
									actionPoison(e)
								}
								at = end
							}
							if transport == "tcp" && meta.TCPOverride != nil {
								require.Equal(t, before, len(es))
								require.Zero(t, s.Stats().BufferedBytes)
							} else {
								require.Equal(t, before+1, len(es))
								if step.Error == "" {
									require.Nil(t, final)
								} else {
									rocTypedError(t, step.Error, final)
								}
							}
						}
						uploadWireCheck(t, selectedVisible(t, c, transport), es, transport, false, deferred)
						uploadClose(t, c, s.Close("selected-test"), transport, "selected-test")
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
						require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
					})
				}
			}
		}
	}
}
func uploadClose(t *testing.T, c selectedWrapper, es []*ProtocolEvent, transport, reason string) {
	t.Helper()
	if transport == "tcp" && c.TCPClose != nil {
		require.Empty(t, es)
		return
	}

	for i, e := range es {
		require.IsType(t, int(0), e.Session["Outstanding"])
		require.IsType(t, uint16(0), e.Session["source_wport"])
		require.IsType(t, uint16(0), e.Session["destination_wport"])
		require.IsType(t, byte(0), e.Session["invoke_id"])
		require.IsType(t, byte(0), e.Session["request_command"])
		if v, ok := e.Session["service_choice"]; ok {
			require.IsType(t, byte(0), v)
		}
		at := c.Close.Events[i].At
		if transport == "tcp" {
			for _, step := range c.Steps {
				if step.Direction == e.Direction {
					at = step.At
				}
			}
		}
		require.Equal(t, at, e.Timestamp.Unix())
		if transport == "tcp" {
			var offset uint64
			for _, step := range c.Steps {
				if step.Direction == e.Direction {
					offset += uint64(len(wrapperWire(t, step.Wire)))
				}
			}
			require.Equal(t, offset, e.Offset)
		} else {
			require.Zero(t, e.Offset)
		}
	}

	uploadCloseBase(t, c.methodListCase, es, transport, reason)
}
func uploadCloseBase(t *testing.T, c methodListCase, es []*ProtocolEvent, transport, reason string) {
	t.Helper()
	require.Len(t, es, len(c.Close.Events))
	for i, e := range es {
		a := c.Close.Events[i]
		require.Equal(t, "dlms-wrapper", e.Protocol)
		profile := "dlms-wrapper-v1-action-block"
		require.Equal(t, profile, e.Profile)
		require.Equal(t, "incomplete", e.Status)
		require.Equal(t, "incomplete", e.Completeness)
		require.Nil(t, e.Raw)
		require.Zero(t, e.Length)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
		require.Equal(t, a.Direction, e.Direction)
		rocEqualFields(t, a.Session, e.Session)
		want := a.UDP
		if transport == "tcp" {
			want = strings.Replace(a.TCP, "<reason>", reason, 1)
		} else {
			require.Equal(t, a.At, e.Timestamp.Unix())
		}
		require.Equal(t, want, e.Summary)
		require.Equal(t, "NeedMore: "+want, e.Error)
		actionQueries(t, e, nil, nil, "NeedMore")
		if e.Session != nil {
			poisonWrapperStructured(e.Session)
		}
		e.Protocol = "redis"
		e.Profile = "redis"
		e.Fields = map[string]any{"caller": true}
		e.Structured = map[string]any{"caller": true}
		e.Raw = []byte{0}
		actionQueries(t, e, nil, nil, "NeedMore")
	}
}

func TestDLMSActionUploadNative(t *testing.T) { uploadNative(t, uploadControls(t), 41000, 4059) }
func TestDLMSActionUploadOffPort(t *testing.T) {
	c := uploadControls(t)
	uploadNative(t, []selectedWrapper{c[0], c[2]}, 45000, 45001)
}
func TestDLMSActionUploadMatrix(t *testing.T) {
	for _, c := range uploadControls(t) {
		for _, transport := range []string{"udp", "tcp"} {
			t.Run(c.Name+"/"+transport, func(t *testing.T) {
				var reuse map[string]string
				require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-action-upload", "DLMS_ACTION_UPLOAD_TEST_ROOT", "reuse-inputs.json"), &reuse))
				file := c.Name + "." + transport + ".pcap"
				var raw []byte
				if p, ok := reuse[file]; ok {
					parts := strings.SplitN(p, "/", 2)
					require.Len(t, parts, 2)
					raw = finsReadBatch(t, parts[0], "DLMS_ACTION_UPLOAD_TEST_ROOT", parts[1])
				} else {
					raw = finsReadBatch(t, "dlms-action-upload", "DLMS_ACTION_UPLOAD_TEST_ROOT", file)
				}
				discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
					cfg := hdlcOpCase{}
					cfg.Steps = make([]hdlcOpStep, len(c.Steps))
					cfg.Budget.Collection = c.Budget.Collection
					cfg.Budget.Depth = c.Budget.Depth
					es, stats := selectedReplay(t, cfg, raw, w, d, o, transport, "dlms-wrapper")
					v := selectedVisible(t, c, transport)
					closeCount := len(c.Close.Events)
					if transport == "tcp" && c.TCPClose != nil {
						closeCount = 0
					}
					require.Len(t, es, len(v.Steps)+closeCount)
					uploadWireCheck(t, v, es[:len(v.Steps)], transport, true, d)
					uploadClose(t, c, es[len(v.Steps):], transport, "ctx-canceled")
					require.EqualValues(t, len(v.Steps), stats.Messages)
					require.EqualValues(t, closeCount, stats.Incomplete)
				})
			})
		}
	}
}

// Independently derived pre-decode reservation boundaries. Both traces retain
// a completed GET conversation, so refusal cannot be masked by flow admission.
func TestDLMSActionUploadProjectionLifecycle(t *testing.T) {
	var raw struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-action-upload", "DLMS_ACTION_UPLOAD_TEST_ROOT", "projection.json"), &raw))
	require.Len(t, raw.Cases, 2)
	for _, v := range raw.Cases {
		var c actionCase
		var bound struct {
			Budget struct{ MaxBufferedBytes int }
			Steps  []struct{ FlowID any }
		}
		require.NoError(t, json.Unmarshal(v, &c))
		require.NoError(t, json.Unmarshal(v, &bound))
		mc := methodListCase{actionCase: c}
		for _, step := range bound.Steps {
			mc.FlowIDs = append(mc.FlowIDs, step.FlowID)
		}
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/d%t", c.Name, deferred), func(t *testing.T) {
				b := DefaultParserBudget()
				b.MaxBufferedBytes = bound.Budget.MaxBufferedBytes
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41000, 4059))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				var events []*ProtocolEvent
				for _, step := range c.Steps {
					wire := wrapperWire(t, step.Wire)
					input := bytes.Clone(wire)
					o := s.Feed(step.Direction, time.Unix(step.At, 0), input)
					clear(input)
					require.Equal(t, len(wire), o.Consumed)
					require.Len(t, o.Events, 1)
					require.False(t, o.NeedMore)
					require.Equal(t, "dlms-wrapper", o.State)
					if step.Error == "" {
						require.Nil(t, o.Err)
					} else {
						rocTypedError(t, step.Error, o.Err)
					}
					events = append(events, actionSnapshot(o.Events[0]))
					actionPoison(o.Events[0])
				}
				uploadWireCheck(t, mc, events, "udp", false, deferred)
				require.Empty(t, s.Close("projection"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
				require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
			})
		}
	}
}

// The independent codec model records zero events as a count and calls its
// error key error_type. Validate every native-feed expectation before adapting
// that representation to the existing shared test helper's typed envelope.
func uploadNativeMeta(t *testing.T, raw json.RawMessage, c actionCase) []byte {
	t.Helper()
	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &top))
	var steps []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(top["steps"], &steps))
	for i, step := range steps {
		if v, ok := step["native_tcp_override"]; ok {
			var meta struct {
				ErrorType string `json:"error_type"`
				Message   string `json:"error_message"`
				Events    int    `json:"events"`
				Consumed  int    `json:"consumed"`
				NeedMore  bool   `json:"need_more"`
			}
			require.NoError(t, json.Unmarshal(v, &meta))
			var keys map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(v, &keys))
			require.Len(t, keys, 5)
			require.Equal(t, "FatalSessionError", meta.ErrorType)
			require.Zero(t, meta.Events)
			require.False(t, meta.NeedMore)
			require.Equal(t, len(wrapperWire(t, c.Steps[i].Wire)), meta.Consumed)
			require.Equal(t, "session stopped", meta.Message)
			normalized, err := json.Marshal(map[string]any{"Error": meta.ErrorType, "error_message": meta.Message, "Events": []any{}, "Consumed": meta.Consumed})
			require.NoError(t, err)
			step["native_tcp_override"] = normalized
		}
	}
	encoded, err := json.Marshal(steps)
	require.NoError(t, err)
	top["steps"] = encoded
	out, err := json.Marshal(top)
	require.NoError(t, err)
	return out
}
