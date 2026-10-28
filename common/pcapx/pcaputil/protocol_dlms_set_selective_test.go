package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"io"
	"strings"
	"testing"
	"time"
)

type selectedMeta struct {
	FlowID      any
	TCPOverride *struct {
		Consumed int
		Error    string
		Message  string `json:"error_message"`
		Events   []any
	} `json:"native_tcp_override"`
	EventOverride json.RawMessage `json:"tcp_event_override"`
}
type selectedWrapper struct {
	methodListCase
	Meta     []selectedMeta
	TCPClose *struct{ Events []any } `json:"native_tcp_Close_override"`
}

func selectedControls(t *testing.T) ([]selectedWrapper, []hdlcOpCase) {
	t.Helper()
	var x struct {
		Wrapper []json.RawMessage `json:"wrapper_cases"`
		HDLC    []hdlcOpCase      `json:"hdlc_cases"`
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-set-selective", "DLMS_SET_SELECTIVE_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Wrapper, 37)
	require.Len(t, x.HDLC, 37)
	out := make([]selectedWrapper, len(x.Wrapper))
	for i, raw := range x.Wrapper {
		var c actionCase
		require.NoError(t, json.Unmarshal(raw, &c))
		var m struct {
			Steps []selectedMeta
			Close *struct{ Events []any } `json:"native_tcp_Close_override"`
		}
		require.NoError(t, json.Unmarshal(raw, &m))
		out[i] = selectedWrapper{methodListCase: methodListCase{actionCase: c}, Meta: m.Steps, TCPClose: m.Close}
		for _, s := range m.Steps {
			out[i].FlowIDs = append(out[i].FlowIDs, s.FlowID)
		}
	}
	return out, x.HDLC
}
func selectFieldTypes(t *testing.T, f map[string]any) {
	t.Helper()
	check := func(m map[string]any) {
		if v, ok := m["selective_access"]; ok && v == true {
			require.IsType(t, byte(0), m["access_selection_raw"])
			require.Equal(t, byte(1), m["access_selection_raw"])
			require.IsType(t, byte(0), m["access_selector"])
			require.Equal(t, false, m["selector_semantics_verified"])
			parameter, ok := m["access_parameters"].(map[string]any)
			require.True(t, ok)
			actionDataTypes(t, parameter)
		}
	}
	check(f)
	if a, ok := f["descriptors"].([]map[string]any); ok {
		for _, d := range a {
			check(d)
		}
	}
}
func selectedVisible(t *testing.T, c selectedWrapper, transport string) methodListCase {
	t.Helper()
	v := c.methodListCase
	v.Steps = nil
	v.FlowIDs = nil
	for i, s := range c.Steps {
		m := c.Meta[i]
		if transport == "tcp" && m.TCPOverride != nil {
			continue
		}
		if transport == "tcp" && len(m.EventOverride) > 0 {
			require.NoError(t, json.Unmarshal(m.EventOverride, &s))
		}
		v.Steps = append(v.Steps, s)
		v.FlowIDs = append(v.FlowIDs, m.FlowID)
	}
	return v
}
func selectedWrapperClose(t *testing.T, c selectedWrapper, es []*ProtocolEvent, transport, reason string) {
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

	methodListCloseCheck(t, c.methodListCase, es, transport, reason)
}
func selectedWrapperNative(t *testing.T, cases []selectedWrapper, source, destination uint16) {
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
						selectedWrapperWireCheck(t, selectedVisible(t, c, transport), es, transport, false, deferred)
						selectedWrapperClose(t, c, s.Close("selected-test"), transport, "selected-test")
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
						require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
					})
				}
			}
		}
	}
}
func TestDLMSSetSelectiveNative(t *testing.T) {
	w, h := selectedControls(t)
	t.Run("wrapper", func(t *testing.T) { selectedWrapperNative(t, w, 41000, 4059) })
	t.Run("hdlc", func(t *testing.T) { hdlcOpNative(t, h, 41000, 4059) })
}
func TestDLMSSetSelectiveOffPort(t *testing.T) {
	w, h := selectedControls(t)
	t.Run("wrapper", func(t *testing.T) { selectedWrapperNative(t, []selectedWrapper{w[0], w[17]}, 45000, 45001) })
	t.Run("hdlc", func(t *testing.T) { hdlcOpNative(t, []hdlcOpCase{h[0], h[17]}, 45000, 45001) })
}
func selectedWrapperWireCheck(t *testing.T, c methodListCase, es []*ProtocolEvent, transport string, captured, deferred bool) {
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
		if transport == "tcp" {
			if captured {
				require.NotZero(t, e.FlowID)
				require.Equal(t, es[0].FlowID, e.FlowID)
			} else {
				require.EqualValues(t, 1, e.FlowID)
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
			shift := 1
			if transport == "tcp" {
				shift = 4
			}
			require.EqualValues(t, i+shift, e.SourceBytes.PacketRefs[0].Number)
			require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
			ends := []string{"192.0.2.1:41000", "192.0.2.2:4059"}
			require.Equal(t, ends[a.Direction], e.Source)
			require.Equal(t, ends[1-a.Direction], e.Destination)
		}
		if a.Error == "" {
			f, err := e.GetFields()
			require.NoError(t, err)
			selectFieldTypes(t, f)
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

// Preserve original sealed answers. This exact-input, source-derived sidecar
// changes only the typed classification of a historically truncated value.
func setSelectiveLegacy(t *testing.T, original []byte, cases []actionCase) []actionCase {
	t.Helper()
	var update struct {
		SHA  string          `json:"original_input_sha256"`
		Case json.RawMessage `json:"case"`
		Step int             `json:"changed_step_1based"`
		Old  string          `json:"old_error"`
		New  string          `json:"new_error"`
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-set-selective", "DLMS_SET_SELECTIVE_TEST_ROOT", "legacy-normal-update.json"), &update))
	require.Equal(t, "0e84716bf92b093eb3259ee273c21abc430053077cc172aa09f6f5fc2734fe9b", update.SHA)
	require.Equal(t, "UnsupportedFeature", update.Old)
	require.Equal(t, "MalformedMessage", update.New)
	var replacement actionCase
	require.NoError(t, json.Unmarshal(update.Case, &replacement))
	var old struct{ Cases []map[string]any }
	require.NoError(t, json.Unmarshal(original, &old))
	var want map[string]any
	require.NoError(t, json.Unmarshal(update.Case, &want))
	found := 0
	for i, c := range cases {
		if c.Name != replacement.Name {
			continue
		}
		found++
		require.Len(t, replacement.Steps, len(c.Steps))
		at := update.Step - 1
		require.GreaterOrEqual(t, at, 0)
		require.Less(t, at, len(c.Steps))
		require.Equal(t, update.SHA, c.Steps[at].SHA)
		require.Equal(t, update.Old, c.Steps[at].Error)
		require.Equal(t, update.New, replacement.Steps[at].Error)
		before := old.Cases[i]
		require.Equal(t, c.Name, before["name"])
		steps := before["steps"].([]any)
		s := steps[at].(map[string]any)
		require.Equal(t, update.Old, s["error"])
		require.Equal(t, update.Old, s["query_error"])
		for _, k := range []string{"error", "query_error"} {
			s[k] = update.New
		}
		s["status"], s["completeness"] = "malformed", "malformed"
		require.Equal(t, want, before, "Only four classification keys for this exact SHA may differ; all inputs, fields, owners and neighboring outcomes stay exact")
		cases[i] = replacement
	}
	require.Equal(t, 1, found)
	return cases
}

func selectedReplay(t *testing.T, c hdlcOpCase, raw []byte, workers int, deferred, observe bool, transport, profile string) ([]*ProtocolEvent, ProtocolStats) {
	t.Helper()
	var es []*ProtocolEvent
	var stats ProtocolStats
	seen := 0
	opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDecodeAs("udp", 4059, profile), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) {
		if observe {
			es = append(es, actionSnapshot(e))
			actionPoison(e)
		} else {
			es = append(es, e)
		}
	}), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
	if observe {
		opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.NotEmpty(t, p.Data()); seen++ }))
	}
	a, err := NewPacketAnalyzer(opts...)
	require.NoError(t, err)
	if c.Budget.Collection > 0 {
		a.conf.binParser.budget.MaxCollectionElements = c.Budget.Collection
	}
	if c.Budget.Depth > 0 {
		a.conf.binParser.budget.MaxRecursionDepth = c.Budget.Depth
	}
	r, err := NewCaptureReader(bytes.NewReader(raw))
	require.NoError(t, err)
	for {
		b, ci, e := r.ReadPacketData()
		if e == io.EOF {
			break
		}
		require.NoError(t, e)
		require.NoError(t, a.Feed(b, ci, r.LinkType()))
		clear(b)
	}
	require.NoError(t, a.Close())
	require.NoError(t, a.Close())
	require.Zero(t, stats.BufferedBytes)
	require.Zero(t, stats.CallbackPanics)
	if observe {
		expected := len(c.Steps)
		if transport == "tcp" {
			expected += 3
		}
		require.Equal(t, expected, seen)
	}
	return es, stats
}

func TestDLMSSetSelectiveMatrix(t *testing.T) {
	wr, hd := selectedControls(t)
	for _, kind := range []string{"wrapper", "hdlc"} {
		count := len(wr)
		if kind == "hdlc" {
			count = len(hd)
		}
		for index := 0; index < count; index++ {
			for _, transport := range []string{"udp", "tcp"} {
				name := wr[index].Name
				cfg := hdlcOpCase{Budget: wr[index].Budget}
				steps := len(wr[index].Steps)
				cfg.Steps = make([]hdlcOpStep, steps)
				closeCount := len(wr[index].Close.Events)
				profile := "dlms-wrapper"
				if kind == "hdlc" {
					name = hd[index].Name
					cfg = hd[index]
					steps = len(cfg.Steps)
					closeCount = len(cfg.Close.Events)
					profile = "dlms"
				}
				t.Run(kind+"/"+name+"/"+transport, func(t *testing.T) {
					var reuse map[string]string
					require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-set-selective", "DLMS_SET_SELECTIVE_TEST_ROOT", "reuse-inputs.json"), &reuse))
					file := kind + "/" + name + "." + transport + ".pcap"
					var raw []byte
					if p, ok := reuse[file]; ok {
						v := strings.SplitN(p, "/", 2)
						require.Len(t, v, 2)
						raw = finsReadBatch(t, v[0], "DLMS_SET_SELECTIVE_TEST_ROOT", v[1])
					} else {
						raw = finsReadBatch(t, "dlms-set-selective", "DLMS_SET_SELECTIVE_TEST_ROOT", file)
					}
					discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
						es, stats := selectedReplay(t, cfg, raw, workers, deferred, observe, transport, profile)
						visible := steps
						closes := closeCount
						if transport == "tcp" {
							if kind == "wrapper" {
								visible = len(selectedVisible(t, wr[index], transport).Steps)
								if wr[index].TCPClose != nil {
									closes = 0
								}
							} else {
								for _, step := range cfg.Steps {
									if step.TCPOverride != nil {
										visible--
									}
								}
								if cfg.TCPCloseOverride != nil {
									closes = 0
								}
							}
						}
						require.Len(t, es, visible+closes)
						if kind == "wrapper" {
							selectedWrapperWireCheck(t, selectedVisible(t, wr[index], transport), es[:visible], transport, true, deferred)
							selectedWrapperClose(t, wr[index], es[visible:], transport, "ctx-canceled")
						} else {
							hdlcOpCheck(t, cfg, es[:visible], transport, true, deferred)
							hdlcOpCloseCheck(t, cfg, es[visible:], transport, "ctx-canceled")
						}
						require.EqualValues(t, visible, stats.Messages)
						require.EqualValues(t, closes, stats.Incomplete)
					})
				})
			}
		}
	}
}

// Exact source-derived logical reservation boundaries, independently checked in
// projection.json. These are owned-graph accounting contracts, not heap sizes.
func TestDLMSSetSelectiveProjectionRefusalLifecycle(t *testing.T) {
	wr, hd := selectedControls(t)
	var expected struct {
		Rows []struct {
			Carrier string
			Index   int
			Need    int
		} `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-set-selective", "DLMS_SET_SELECTIVE_TEST_ROOT", "projection.json"), &expected))
	require.Len(t, expected.Rows, 16)
	for _, v := range expected.Rows {
		name := wr[v.Index].Name
		for _, delta := range []int{-1, 0} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/delta%d/d%t", v.Carrier, name, delta, deferred), func(t *testing.T) {
					b := DefaultParserBudget()
					b.MaxFrameBytes = 4096
					b.MaxMessageBytes = 4096
					b.MaxBufferedBytes = v.Need + delta
					s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41000, 4059))
					require.NoError(t, err)
					s.(*captureSession).f.a.config.Deferred = deferred
					var es []*ProtocolEvent
					if v.Carrier == "wrapper" {
						c := wr[v.Index]
						c.Steps = append([]actionStep(nil), c.Steps...)
						if delta < 0 {
							q := &c.Steps[2]
							q.Error = "ResourceExceeded"
							q.Status = "limited"
							q.Completeness = "limited"
							q.Session = nil
							q.Fields = nil
							q.Transaction = 0
							q.Response = 0
							q.Raw = nil
							refused := 36
							if v.Index >= 17 {
								refused = 21
							}
							c.Steps[3] = wr[refused].Steps[3]
						}
						for _, st := range c.Steps {
							w := wrapperWire(t, st.Wire)
							input := bytes.Clone(w)
							o := s.Feed(st.Direction, time.Unix(st.At, 0), input)
							clear(input)
							require.Equal(t, len(w), o.Consumed)
							require.Len(t, o.Events, 1)
							if st.Error == "" {
								require.Nil(t, o.Err)
							} else {
								rocTypedError(t, st.Error, o.Err)
							}
							es = append(es, actionSnapshot(o.Events[0]))
							actionPoison(o.Events[0])
						}
						selectedWrapperWireCheck(t, c.methodListCase, es, "udp", false, deferred)
						selectedWrapperClose(t, c, s.Close("projection"), "udp", "projection")
					} else {
						require.Equal(t, "hdlc", v.Carrier)
						c := hd[v.Index]
						c.Steps = append([]hdlcOpStep(nil), c.Steps...)
						if delta < 0 {
							for i := 2; i < len(c.Steps); i++ {
								q := &c.Steps[i]
								q.Error = "ResourceExceeded"
								if i == 3 {
									q.Error = "ContextRequired"
								}
								q.Status = "context-required"
								q.Completeness = "context-required"
								q.Session = nil
								q.Fields = nil
								q.Tx = 0
								q.Response = 0
							}
						}
						for _, st := range c.Steps {
							w := wrapperWire(t, st.Wire)
							input := bytes.Clone(w)
							o := s.Feed(st.Direction, time.Unix(st.At, 0), input)
							clear(input)
							require.Equal(t, len(w), o.Consumed)
							require.Len(t, o.Events, 1)
							if st.Error == "" {
								require.Nil(t, o.Err)
							} else {
								rocTypedError(t, st.Error, o.Err)
							}
							es = append(es, actionSnapshot(o.Events[0]))
							actionPoison(o.Events[0])
						}
						hdlcOpCheck(t, c, es, "udp", false, deferred)
						hdlcOpCloseCheck(t, c, s.Close("projection"), "udp", "projection")
					}
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
					require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
				})
			}
		}
	}
}
