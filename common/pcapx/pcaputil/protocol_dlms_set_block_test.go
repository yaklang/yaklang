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

func blockControls(t *testing.T) []selectedWrapper {
	t.Helper()
	var x struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-set-block", "DLMS_SET_BLOCK_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 18)
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
		for _, step := range m.Steps {
			out[i].FlowIDs = append(out[i].FlowIDs, step.FlowID)
		}
	}
	return out
}
func blockWireCheck(t *testing.T, c methodListCase, es []*ProtocolEvent, transport string, captured, deferred bool) {
	t.Helper()
	require.Len(t, es, len(c.Steps))
	for i, e := range es {
		if c.Steps[i].Error != "" {
			continue
		}
		f, err := e.GetFields()
		require.NoError(t, err)
		require.IsType(t, uint32(0), f["block_number"])
		if v, ok := f["raw_data_length"]; ok {
			require.IsType(t, uint64(0), v)
		}
		require.IsType(t, false, f["transfer_complete"])
		if v, ok := f["last_block"]; ok {
			require.IsType(t, false, v)
		}
		if v, ok := f["observed_block_count"]; ok {
			require.IsType(t, int(0), v)
		}
		if v, ok := f["assembled_data"].(map[string]any); ok {
			actionDataTypes(t, v)
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
func blockNative(t *testing.T, cases []selectedWrapper, source, destination uint16) {
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
						blockWireCheck(t, selectedVisible(t, c, transport), es, transport, false, deferred)
						blockClose(t, c, s.Close("selected-test"), transport, "selected-test")
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
						require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
					})
				}
			}
		}
	}
}
func blockClose(t *testing.T, c selectedWrapper, es []*ProtocolEvent, transport, reason string) {
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
		require.Equal(t, c.Close.Events[i].At, e.Timestamp.Unix())
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

	blockCloseBase(t, c.methodListCase, es, transport, reason)
}
func blockCloseBase(t *testing.T, c methodListCase, es []*ProtocolEvent, transport, reason string) {
	t.Helper()
	require.Len(t, es, len(c.Close.Events))
	for i, e := range es {
		a := c.Close.Events[i]
		require.Equal(t, "dlms-wrapper", e.Protocol)
		profile := "dlms-wrapper-v1-set-block"
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

func TestDLMSSetBlockNative(t *testing.T) { blockNative(t, blockControls(t), 41000, 4059) }
func TestDLMSSetBlockOffPort(t *testing.T) {
	c := blockControls(t)
	blockNative(t, []selectedWrapper{c[0], c[2]}, 45000, 45001)
}
func TestDLMSSetBlockMatrix(t *testing.T) {
	for _, c := range blockControls(t) {
		for _, transport := range []string{"udp", "tcp"} {
			t.Run(c.Name+"/"+transport, func(t *testing.T) {
				var reuse map[string]string
				require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-set-block", "DLMS_SET_BLOCK_TEST_ROOT", "reuse-inputs.json"), &reuse))
				file := c.Name + "." + transport + ".pcap"
				var raw []byte
				if p, ok := reuse[file]; ok {
					parts := strings.SplitN(p, "/", 2)
					require.Len(t, parts, 2)
					raw = finsReadBatch(t, parts[0], "DLMS_SET_BLOCK_TEST_ROOT", parts[1])
				} else {
					raw = finsReadBatch(t, "dlms-set-block", "DLMS_SET_BLOCK_TEST_ROOT", file)
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
					blockWireCheck(t, v, es[:len(v.Steps)], transport, true, d)
					blockClose(t, c, es[len(v.Steps):], transport, "ctx-canceled")
					require.EqualValues(t, len(v.Steps), stats.Messages)
					require.EqualValues(t, closeCount, stats.Incomplete)
				})
			})
		}
	}
}

// This is a full feature-upgrade answer for two identical legal archived
// inputs. Original sealed files are immutable; every input/context key and
// every non-target case remains unchanged, rather than suppressing its test.
func setBlockHistoricalUpgrade(t *testing.T, original []byte, cases []actionCase) []actionCase {
	t.Helper()
	var x struct {
		Records []struct {
			SHA  string          `json:"original_input_sha256"`
			Name string          `json:"original_case_name"`
			Case json.RawMessage `json:"case"`
		}
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-set-block", "DLMS_SET_BLOCK_TEST_ROOT", "historical-upgrades.json"), &x))
	require.Len(t, x.Records, 2)
	var old struct{ Cases []map[string]any }
	require.NoError(t, json.Unmarshal(original, &old))
	require.Len(t, old.Cases, len(cases))
	found := 0
	for _, r := range x.Records {
		for i, c := range cases {
			if c.Name != r.Name {
				continue
			}
			found++
			require.Equal(t, "25905ab05db3b95e556088d2172abd5e87e461f5ef083dc29dc69ae221c26d85", r.SHA)
			require.Len(t, c.Steps, 1)
			require.Equal(t, r.SHA, c.Steps[0].SHA)
			require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wrapperWire(t, c.Steps[0].Wire))), r.SHA)
			require.Equal(t, "UnsupportedFeature", c.Steps[0].Error)
			var next map[string]any
			require.NoError(t, json.Unmarshal(r.Case, &next))
			before := old.Cases[i]
			bs := before["steps"].([]any)
			ns := next["steps"].([]any)
			require.Len(t, bs, 1)
			require.Len(t, ns, 1)
			b := bs[0].(map[string]any)
			n := ns[0].(map[string]any)
			for _, key := range []string{"profile", "status", "completeness", "error", "fields", "session", "Transaction_index", "FlowID", "query_error", "GetFields", "Decode"} {
				delete(b, key)
				delete(n, key)
			}
			require.Equal(t, n, b, "All original input/time/direction/offset/Raw/Length/Response and unknown keys must remain exact")
			delete(before, "Close")
			delete(next, "Close")
			require.Equal(t, next, before, "Only new outcomes and Close may differ; names, budgets, ports and ownership contract stay exact")
			var replacement actionCase
			require.NoError(t, json.Unmarshal(r.Case, &replacement))
			require.Empty(t, replacement.Steps[0].Error)
			require.Equal(t, "dlms-wrapper-v1-set-block", replacement.Steps[0].Profile)
			require.Len(t, replacement.Close.Events, 1)
			cases[i] = replacement
		}
	}
	require.Equal(t, 1, found)
	return cases
}
func setBlockHistoricalCloseProfile(c actionCase, fallback string) string {
	if len(c.Steps) == 1 && c.Steps[0].SHA == "25905ab05db3b95e556088d2172abd5e87e461f5ef083dc29dc69ae221c26d85" && c.Steps[0].Profile == "dlms-wrapper-v1-set-block" {
		return "dlms-wrapper-v1-set-block"
	}
	return fallback
}

func TestDLMSSetBlockProjectionRefusalLifecycle(t *testing.T) {
	var x struct {
		Need   int
		Cases  []json.RawMessage
		Leaves []struct {
			Budget   int
			Deferred bool
		}
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-set-block", "DLMS_SET_BLOCK_TEST_ROOT", "projection.json"), &x))
	require.Equal(t, 1103872, x.Need)
	require.Len(t, x.Cases, 2)
	require.Len(t, x.Leaves, 4)
	for _, leaf := range x.Leaves {
		t.Run(fmt.Sprintf("budget%d/d%t", leaf.Budget, leaf.Deferred), func(t *testing.T) {
			index := 0
			if leaf.Budget < x.Need {
				index = 1
			}
			var c actionCase
			require.NoError(t, json.Unmarshal(x.Cases[index], &c))
			var meta struct{ Steps []struct{ FlowID any } }
			require.NoError(t, json.Unmarshal(x.Cases[index], &meta))
			m := methodListCase{actionCase: c}
			for _, v := range meta.Steps {
				m.FlowIDs = append(m.FlowIDs, v.FlowID)
			}
			b := DefaultParserBudget()
			b.MaxFrameBytes = 4096
			b.MaxMessageBytes = 4096
			b.MaxBufferedBytes = leaf.Budget
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41000, 4059))
			require.NoError(t, err)
			s.(*captureSession).f.a.config.Deferred = leaf.Deferred
			var es []*ProtocolEvent
			for _, step := range c.Steps {
				wire := wrapperWire(t, step.Wire)
				input := bytes.Clone(wire)
				o := s.Feed(step.Direction, time.Unix(step.At, 0), input)
				clear(input)
				require.Equal(t, len(wire), o.Consumed)
				require.Len(t, o.Events, 1)
				if step.Error == "" {
					require.Nil(t, o.Err)
				} else {
					rocTypedError(t, step.Error, o.Err)
				}
				es = append(es, actionSnapshot(o.Events[0]))
				actionPoison(o.Events[0])
			}
			selectedWrapperWireCheck(t, m, es, "udp", false, leaf.Deferred)
			require.Empty(t, s.Close("projection"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
			require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
		})
	}
}
