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

func setListBlocksControls(t *testing.T) []selectedWrapper {
	t.Helper()
	var x struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-wrapper-set-list-blocks", "DLMS_WRAPPER_SET_LIST_BLOCKS_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 19)
	out := make([]selectedWrapper, len(x.Cases))
	for i, raw := range x.Cases {
		var c actionCase
		require.NoError(t, json.Unmarshal(raw, &c))
		var m struct {
			Steps []selectedMeta
			Close *struct{ Events []any } `json:"native_tcp_Close_override"`
		}
		require.NoError(t, json.Unmarshal(raw, &m))
		out[i] = selectedWrapper{methodListCase: methodListCase{actionCase: c}, Meta: m.Steps, TCPClose: m.Close}
		var timing struct {
			Close struct {
				Events []struct {
					Direction int
					NativeTCP int64 `json:"native_tcp_timestamp_unix"`
				}
			}
		}
		require.NoError(t, json.Unmarshal(raw, &timing))
		require.Len(t, timing.Close.Events, len(c.Close.Events))
		for j, ce := range timing.Close.Events {
			var last int64
			for _, st := range c.Steps {
				if st.Direction == ce.Direction {
					last = st.At
				}
			}
			require.Equal(t, last, ce.NativeTCP)
			require.Equal(t, ce.Direction, c.Close.Events[j].Direction)
		}
		for _, step := range m.Steps {
			out[i].FlowIDs = append(out[i].FlowIDs, step.FlowID)
		}
	}
	return out
}
func setListBlocksWireCheck(t *testing.T, c methodListCase, es []*ProtocolEvent, transport string, captured, deferred bool) {
	t.Helper()
	require.Len(t, es, len(c.Steps))
	for i, e := range es {
		if c.Steps[i].Error != "" {
			continue
		}
		f, err := e.GetFields()
		require.NoError(t, err)
		for k, want := range map[string]any{"block_number": uint32(0), "raw_data_length": uint64(0), "list_count": uint64(0), "value_count": uint64(0), "assembled_value_count": uint64(0), "last_block": false, "transfer_complete": false, "observed_block_count": int(0), "success_item_count": int(0), "error_item_count": int(0)} {
			if v, ok := f[k]; ok {
				require.IsType(t, want, v, k)
			}
		}
		for _, key := range []string{"descriptors", "values", "results", "assembled_values"} {
			if v, ok := f[key]; ok {
				require.IsType(t, []map[string]any{}, v, key)
				for _, row := range v.([]map[string]any) {
					require.IsType(t, int(0), row["index"])
					require.IsType(t, int(0), row["encoded_length"])
					require.IsType(t, "", row["raw_hex"])
					if key == "assembled_values" {
						require.IsType(t, int(0), row["encoded_offset"])
						require.NotContains(t, row, "apdu_offset")
					} else {
						require.IsType(t, int(0), row["apdu_offset"])
					}
					if key == "descriptors" {
						require.IsType(t, uint16(0), row["class_id"])
						require.IsType(t, int8(0), row["attribute_id"])
						require.IsType(t, byte(0), row["access_selection_raw"])
						require.IsType(t, false, row["selective_access"])
					}
					if key == "results" {
						require.IsType(t, byte(0), row["data_access_result"])
						require.IsType(t, false, row["result_success"])
					}
					if key == "values" || key == "assembled_values" {
						d, ok := row["data"].(map[string]any)
						require.True(t, ok)
						actionDataTypes(t, d)
					}
				}
			}
		}
		selectFieldTypes(t, f)
	}
	checked := es
	if captured && transport == "udp" {
		checked = append([]*ProtocolEvent(nil), es...)
		for i, e := range es {
			if want, ok := c.FlowIDs[i].(float64); ok && want == 0 {
				require.Zero(t, e.FlowID)
				require.Zero(t, e.Direction)
				v := *e
				v.Direction = c.Steps[i].Direction ^ c.Steps[0].Direction
				checked[i] = &v
			}
		}
	}
	selectedWrapperWireCheck(t, c, checked, transport, captured, deferred)
}
func setListBlocksNative(t *testing.T, cases []selectedWrapper, source, destination uint16) {
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
						setListBlocksWireCheck(t, selectedVisible(t, c, transport), es, transport, false, deferred)
						setListBlocksClose(t, c, s.Close("selected-test"), transport, "selected-test")
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
						require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
					})
				}
			}
		}
	}
}
func setListBlocksClose(t *testing.T, c selectedWrapper, es []*ProtocolEvent, transport, reason string) {
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

	setListBlocksCloseBase(t, c.methodListCase, es, transport, reason)
}
func setListBlocksCloseBase(t *testing.T, c methodListCase, es []*ProtocolEvent, transport, reason string) {
	t.Helper()
	require.Len(t, es, len(c.Close.Events))
	for i, e := range es {
		a := c.Close.Events[i]
		require.Equal(t, "dlms-wrapper", e.Protocol)
		profile := "dlms-wrapper-v1-set-list-block"
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

func TestDLMSWrapperSetListBlocksNative(t *testing.T) {
	setListBlocksNative(t, setListBlocksControls(t), 41000, 4059)
}
func TestDLMSWrapperSetListBlocksOffPort(t *testing.T) {
	c := setListBlocksControls(t)
	setListBlocksNative(t, c[:3], 45000, 45001)
}
func TestDLMSWrapperSetListBlocksMatrix(t *testing.T) {
	for _, c := range setListBlocksControls(t) {
		for _, transport := range []string{"udp", "tcp"} {
			t.Run(c.Name+"/"+transport, func(t *testing.T) {
				var reuse map[string]string
				require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-wrapper-set-list-blocks", "DLMS_WRAPPER_SET_LIST_BLOCKS_TEST_ROOT", "reuse-inputs.json"), &reuse))
				file := c.Name + "." + transport + ".pcap"
				var raw []byte
				if p, ok := reuse[file]; ok {
					parts := strings.SplitN(p, "/", 2)
					require.Len(t, parts, 2)
					raw = finsReadBatch(t, parts[0], "DLMS_WRAPPER_SET_LIST_BLOCKS_TEST_ROOT", parts[1])
				} else {
					raw = finsReadBatch(t, "dlms-wrapper-set-list-blocks", "DLMS_WRAPPER_SET_LIST_BLOCKS_TEST_ROOT", file)
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
					setListBlocksWireCheck(t, v, es[:len(v.Steps)], transport, true, d)
					setListBlocksClose(t, c, es[len(v.Steps):], transport, "ctx-canceled")
					require.EqualValues(t, len(v.Steps), stats.Messages)
					require.EqualValues(t, closeCount, stats.Incomplete)
				})
			})
		}
	}
}
