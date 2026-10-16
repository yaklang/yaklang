package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

type methodListCase struct {
	actionCase
	FlowIDs []any
}

func methodListControls(t *testing.T) []methodListCase {
	var x struct{ Cases []actionCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-action-list", "DLMS_ACTION_LIST_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 59)
	var flow struct {
		Cases []struct{ Steps []struct{ FlowID any } }
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-action-list", "DLMS_ACTION_LIST_TEST_ROOT", "controls.json"), &flow))
	require.Len(t, flow.Cases, len(x.Cases))
	out := make([]methodListCase, len(x.Cases))
	for i, c := range x.Cases {
		out[i].actionCase = c
		for _, v := range flow.Cases[i].Steps {
			out[i].FlowIDs = append(out[i].FlowIDs, v.FlowID)
		}
	}
	return out
}
func methodListCloseCheck(t *testing.T, c methodListCase, es []*ProtocolEvent, transport, reason string) {
	t.Helper()
	require.Len(t, es, len(c.Close.Events))
	for i, e := range es {
		a := c.Close.Events[i]
		require.Equal(t, "dlms-wrapper", e.Protocol)
		profile := "dlms-wrapper-v1-set-normal"
		if a.Session["service_choice"] == float64(4) {
			profile = "dlms-wrapper-v1-set-list"
		}
		if a.Session["request_command"] == float64(195) {
			profile = "dlms-wrapper-v1-action-normal"
			if a.Session["service_choice"] == float64(3) {
				profile = "dlms-wrapper-v1-action-list"
			}
		}
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
func TestDLMSActionListNative(t *testing.T) {
	for _, c := range methodListControls(t) {
		for _, transport := range []string{"udp", "tcp"} {
			if transport == "tcp" {
				bad := false
				for _, a := range c.Steps {
					bad = bad || a.Error != ""
				}
				if bad {
					continue
				}
			}
			for _, d := range []bool{false, true} {
				for _, size := range []int{0, 1, 7} {
					if transport == "udp" && size != 0 {
						continue
					}
					t.Run(fmt.Sprintf("%s/%s/d%t/chunk%d", c.Name, transport, d, size), func(t *testing.T) {
						b := DefaultParserBudget()
						if c.Budget.Depth > 0 {
							b.MaxRecursionDepth = c.Budget.Depth
						}
						if c.Budget.Collection > 0 {
							b.MaxCollectionElements = c.Budget.Collection
						}
						s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(transport), WithSessionPorts(41000, 4059))
						require.NoError(t, err)
						s.(*captureSession).f.a.config.Deferred = d
						var es []*ProtocolEvent
						for _, a := range c.Steps {
							w := wrapperWire(t, a.Wire)
							for at := 0; at < len(w); {
								end := len(w)
								if size > 0 {
									end = min(at+size, end)
								}
								in := bytes.Clone(w[at:end])
								o := s.Feed(a.Direction, time.Unix(a.At, 0), in)
								clear(in)
								if at == 0 && end == len(w) {
									if a.Error == "" {
										require.Nil(t, o.Err)
									} else {
										rocTypedError(t, a.Error, o.Err)
									}
								} else if o.Err != nil {
									require.Equal(t, ErrNeedMore, o.Err.Kind)
								}
								for _, e := range o.Events {
									snap := actionSnapshot(e)
									es = append(es, snap)
									actionPoison(e)
								}
								at = end
							}
						}
						methodListCheck(t, c, es, transport, false, d)
						methodListCloseCheck(t, c, s.Close("action-test"), transport, "action-test")
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
						require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
					})
				}
			}
		}
	}
}
func TestDLMSActionListMatrix(t *testing.T) {
	for _, c := range methodListControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			var reuse map[string]string
			require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-action-list", "DLMS_ACTION_LIST_TEST_ROOT", "reuse-inputs.json"), &reuse))
			raw := []byte(nil)
			if path, ok := reuse[c.Name]; ok {
				parts := strings.SplitN(path, "/", 2)
				require.Len(t, parts, 2)
				raw = finsReadBatch(t, parts[0], "DLMS_ACTION_LIST_TEST_ROOT", parts[1])
			} else {
				raw = finsReadBatch(t, "dlms-action-list", "DLMS_ACTION_LIST_TEST_ROOT", c.Name+".pcap")
			}
			discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
				es, stats := actionReplay(t, c.actionCase, raw, w, d, o)
				require.Len(t, es, len(c.Steps)+len(c.Close.Events))
				methodListCheck(t, c, es[:len(c.Steps)], "udp", true, d)
				methodListCloseCheck(t, c, es[len(c.Steps):], "udp", "")
				require.EqualValues(t, len(c.Steps), stats.Messages)
				require.EqualValues(t, len(c.Close.Events), stats.Incomplete)
			})
		})
	}
}

func methodListCheck(t *testing.T, c methodListCase, es []*ProtocolEvent, transport string, captured, deferred bool) {
	methodListWireCheck(t, c, es, transport, captured, deferred)
	for i, e := range es {
		if c.Steps[i].Error != "" {
			continue
		}
		f, err := e.GetFields()
		require.NoError(t, err)
		for _, k := range []string{"list_count", "parameter_count"} {
			if _, ok := f[k]; ok {
				require.IsType(t, uint64(0), f[k])
			}
		}
		for _, k := range []string{"success_item_count", "error_item_count"} {
			if _, ok := f[k]; ok {
				require.IsType(t, int(0), f[k])
			}
		}
		for _, k := range []string{"descriptors", "parameters", "results"} {
			if a, ok := f[k]; ok {
				require.IsType(t, []map[string]any{}, a)
				for _, v := range a.([]map[string]any) {
					for _, k := range []string{"index", "apdu_offset", "encoded_length"} {
						require.IsType(t, int(0), v[k])
					}
					if _, ok := v["class_id"]; ok {
						require.IsType(t, uint16(0), v["class_id"])
						if _, ok := v["method_id"]; ok {
							require.IsType(t, int8(0), v["method_id"])
						} else {
							require.IsType(t, int8(0), v["attribute_id"])
							require.IsType(t, byte(0), v["access_selection_raw"])
						}
					}
					if _, ok := v["action_result"]; ok {
						require.IsType(t, byte(0), v["action_result"])
						require.IsType(t, false, v["action_result_success"])
						require.IsType(t, false, v["return_parameters_present"])
					}
					if _, ok := v["return_choice_raw"]; ok {
						require.IsType(t, byte(0), v["return_choice_raw"])
					}
					if _, ok := v["data_access_result"]; ok {
						require.IsType(t, byte(0), v["data_access_result"])
					}
					if d, ok := v["data"].(map[string]any); ok {
						actionDataTypes(t, d)
					}
				}
			}
		}
	}
}

func methodListWireCheck(t *testing.T, c methodListCase, es []*ProtocolEvent, transport string, captured, deferred bool) {
	t.Helper()
	require.Len(t, es, len(c.Steps))
	ids := make([]uint64, len(es))
	for i, e := range es {
		ids[i] = e.ID
		require.NotZero(t, e.ID)
		for j := 0; j < i; j++ {
			require.NotEqual(t, ids[j], e.ID)
		}
	}
	owner := func(n int) uint64 {
		if n == 0 {
			return 0
		}
		require.LessOrEqual(t, n, len(ids))
		return ids[n-1]
	}
	offsets := [2]uint64{}
	for i, e := range es {
		a := c.Steps[i]
		w := wrapperWire(t, a.Wire)
		require.Equal(t, a.SHA, fmt.Sprintf("%x", sha256.Sum256(w)))
		require.Equal(t, "dlms-wrapper", e.Protocol)
		require.Equal(t, a.Profile, e.Profile)
		require.Equal(t, transport, e.Transport)
		status := a.Status
		if deferred && a.Error == "" {
			status = "deferred"
		}
		require.Equal(t, status, e.Status)
		require.Equal(t, a.Completeness, e.Completeness)
		direction := a.Direction
		if captured {
			direction ^= c.Steps[0].Direction
		}
		require.Equal(t, direction, e.Direction)
		require.Equal(t, a.At, e.Timestamp.Unix())
		require.Equal(t, a.Length, e.Length)
		if transport == "udp" {
			switch want := c.FlowIDs[i].(type) {
			case float64:
				require.Zero(t, want)
				require.Zero(t, e.FlowID)
			case string:
				require.Equal(t, "same retained flow", want)
				require.NotZero(t, e.FlowID)
				require.Equal(t, es[0].FlowID, e.FlowID)
			default:
				t.Fatalf("missing independent FlowID expectation for %s step%d", c.Name, i)
			}
		}
		require.Equal(t, owner(a.Transaction), e.TransactionID)
		require.Equal(t, owner(a.Response), e.ResponseTo)
		if transport == "tcp" {
			require.Equal(t, offsets[a.Direction], e.Offset)
			offsets[a.Direction] += uint64(len(w))
		} else {
			require.Zero(t, e.Offset)
		}
		if a.Raw == nil {
			require.Nil(t, e.Raw)
		} else {
			require.Equal(t, wrapperWire(t, *a.Raw), e.Raw)
		}
		if a.Session == nil {
			require.Nil(t, e.Session)
		} else {
			rocEqualFields(t, a.Session, e.Session)
		}
		if captured {
			require.Len(t, e.SourceBytes.PacketRefs, 1)
			require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
			require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
			ends := []string{"192.0.2.1:41000", "192.0.2.2:4059"}
			require.Equal(t, ends[a.Direction], e.Source)
			require.Equal(t, ends[1-a.Direction], e.Destination)
		}
		actionQueries(t, e, a.Fields, a.Session, a.Error)
		if a.Error != "" {
			require.Contains(t, e.Error, a.Error)
		} else {
			require.Empty(t, e.Error)
		}
		// Mutable views and aliases cannot authorize an alternate protocol decoder.
		if e.Session != nil {
			poisonWrapperStructured(e.Session)
		}
		clear(e.Raw)
		e.Session = map[string]any{"caller": true}
		e.Protocol = "tls"
		e.Profile = "tls"
		e.Raw = []byte{0}
		e.Fields = map[string]any{"caller": true}
		e.Structured = map[string]any{"caller": true}
		e.Metadata = "caller"
		e.TransactionID = 999999
		e.ResponseTo = 999999
		actionQueries(t, e, a.Fields, a.Session, a.Error)
	}
}

func TestDLMSActionListProjectionRefusalLifecycle(t *testing.T) {
	q := wrapperWire(t, "000100100001001dc303c102000f0000280000ff01000100002a0000ff0202090301020300")
	r := wrapperWire(t, "0001000100100008c703c10200000300")
	need := 512 + 32768 + 512*len(q) + 2048*min(256, len(q)-12) + 8192*min(64, (len(q)-12)/9) + 256
	for _, delta := range []int{-1, 0} {
		for _, d := range []bool{false, true} {
			t.Run(fmt.Sprintf("delta%d/d%t", delta, d), func(t *testing.T) {
				b := DefaultParserBudget()
				b.MaxFrameBytes = 4096
				b.MaxMessageBytes = 4096
				b.MaxBufferedBytes = need + delta
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41000, 4059))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = d
				first := s.Feed(0, time.Unix(1, 0), bytes.Clone(q))
				require.Len(t, first.Events, 1)
				if delta == 0 {
					require.Nil(t, first.Err)
					f, err := first.Events[0].GetFields()
					require.NoError(t, err)
					rocEqualFields(t, map[string]any{"type": float64(9), "raw_hex": "0903010203", "length": float64(3), "length_encoding_hex": "03", "value_hex": "010203"}, f["parameters"].([]map[string]any)[0]["data"].(map[string]any))
					require.Equal(t, first.Events[0].ID, first.Events[0].TransactionID)
				} else {
					rocTypedError(t, "ResourceExceeded", first.Err)
					require.Nil(t, first.Events[0].Raw)
					require.Nil(t, first.Events[0].Session)
					require.Zero(t, first.Events[0].ResponseTo)
					require.Zero(t, first.Events[0].TransactionID)
					actionQueries(t, first.Events[0], nil, nil, "ResourceExceeded")
				}
				late := s.Feed(1, time.Unix(2, 0), bytes.Clone(r))
				require.Nil(t, late.Err)
				require.Len(t, late.Events, 1)
				if delta == 0 {
					require.Equal(t, first.Events[0].ID, late.Events[0].ResponseTo)
					require.Equal(t, first.Events[0].ID, late.Events[0].TransactionID)
					require.Equal(t, "observed-response", late.Events[0].Session["Association"])
				} else {
					require.Zero(t, late.Events[0].ResponseTo)
					require.Zero(t, late.Events[0].TransactionID)
					require.Equal(t, "ambiguous-conversation", late.Events[0].Session["Association"])
				}
				require.Equal(t, 0, late.Events[0].Session["Outstanding"])
				require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
				require.Empty(t, s.Close("projection"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
func TestDLMSActionListIdleRetiresOwner(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 4059))
	require.NoError(t, err)
	q := wrapperWire(t, "000100100001001dc303c102000f0000280000ff01000100002a0000ff0202090301020300")
	r := wrapperWire(t, "0001000100100008c703c10200000300")
	first := s.Feed(0, time.Unix(1, 0), bytes.Clone(q))
	require.Nil(t, first.Err)
	require.Len(t, first.Events, 1)
	late := s.Feed(1, time.Unix(31, 0), bytes.Clone(r))
	require.Nil(t, late.Err)
	require.Len(t, late.Events, 1)
	require.Zero(t, late.Events[0].ResponseTo)
	require.Zero(t, late.Events[0].TransactionID)
	require.Equal(t, "ambiguous-conversation", late.Events[0].Session["Association"])
	next := s.Feed(0, time.Unix(32, 0), bytes.Clone(q))
	require.Nil(t, next.Err)
	require.Len(t, next.Events, 1)
	require.Zero(t, next.Events[0].TransactionID)
	require.Equal(t, "ambiguous-conversation", next.Events[0].Session["Association"])
	require.Empty(t, s.Close("idle"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestDLMSActionListOffPortChunks(t *testing.T) {
	c := methodListControls(t)[0]
	for _, transport := range []string{"udp", "tcp"} {
		for _, d := range []bool{false, true} {
			for _, chunk := range []int{0, 1, 7} {
				if transport == "udp" && chunk != 0 {
					continue
				}
				t.Run(fmt.Sprintf("%s/d%t/chunk%d", transport, d, chunk), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(45000, 45001))
					require.NoError(t, err)
					s.(*captureSession).f.a.config.Deferred = d
					var es []*ProtocolEvent
					for _, a := range c.Steps {
						w := wrapperWire(t, a.Wire)
						for at := 0; at < len(w); {
							end := len(w)
							if chunk > 0 {
								end = min(at+chunk, end)
							}
							input := bytes.Clone(w[at:end])
							o := s.Feed(a.Direction, time.Unix(a.At, 0), input)
							clear(input)
							if o.Err != nil {
								require.Equal(t, ErrNeedMore, o.Err.Kind)
							}
							for _, e := range o.Events {
								es = append(es, actionSnapshot(e))
								actionPoison(e)
							}
							at = end
						}
					}
					methodListCheck(t, c, es, transport, false, d)
					require.Empty(t, s.Close("offport"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}
