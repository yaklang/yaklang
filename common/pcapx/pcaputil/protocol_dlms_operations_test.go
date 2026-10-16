package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"io"
	"strings"
	"testing"
	"time"
)

type hdlcOpStep struct {
	Wire                                 string `json:"input_hex"`
	SHA                                  string `json:"input_sha256"`
	Direction                            int
	At                                   int64  `json:"timestamp_unix"`
	OffsetTCP                            uint64 `json:"offset_tcp"`
	OffsetUDP                            uint64 `json:"offset_udp"`
	Profile, Status, Completeness, Error string
	Fields, Session                      map[string]any
	Tx                                   uint64 `json:"TransactionID"`
	Response                             int    `json:"Response_index"`
	FlowID                               any
	Length                               int
	Raw                                  *string `json:"Raw_hex"`
	TCPOverride                          *struct {
		Consumed int
		Error    string
		Message  string `json:"error_message"`
		Events   []any
	} `json:"native_tcp_override"`
}
type hdlcOpClose struct {
	Profile, Status, Completeness string
	Direction                     int
	TimeTCP                       int64  `json:"timestamp_tcp"`
	TimeUDP                       int64  `json:"timestamp_udp"`
	OffsetTCP                     uint64 `json:"offset_tcp"`
	OffsetUDP                     uint64 `json:"offset_udp"`
	Session                       map[string]any
	SummaryTCP                    string `json:"summary_tcp"`
	SummaryUDP                    string `json:"summary_udp"`
}
type hdlcOpCase struct {
	Name   string
	Steps  []hdlcOpStep
	Budget struct {
		Collection int `json:"MaxCollectionElements"`
		Depth      int `json:"MaxRecursionDepth"`
	}
	Close            struct{ Events []hdlcOpClose }
	TCPCloseOverride *struct{ Events []any } `json:"native_tcp_Close_override"`
}

func hdlcOpControls(t *testing.T) []hdlcOpCase {
	var x struct{ Cases []hdlcOpCase }
	raw := finsReadBatch(t, "dlms-hdlc-operations", "DLMS_HDLC_OPERATIONS_TEST_ROOT", "controls.json")
	require.NoError(t, json.Unmarshal(raw, &x))
	require.Len(t, x.Cases, 47)
	return hdlcOperationSegmentHistoricalUpgrade(t, raw, x.Cases)
}
func hdlcOpCanonical(v any) any {
	switch x := v.(type) {
	case []byte:
		return map[string]any{"hexbytes": hex.EncodeToString(x)}
	case map[string]any:
		r := map[string]any{}
		for k, v := range x {
			r[k] = hdlcOpCanonical(v)
		}
		return r
	case []map[string]any:
		r := make([]any, len(x))
		for i, v := range x {
			r[i] = hdlcOpCanonical(v)
		}
		return r
	case []any:
		r := make([]any, len(x))
		for i, v := range x {
			r[i] = hdlcOpCanonical(v)
		}
		return r
	}
	return v
}
func hdlcOpEqual(t *testing.T, want, got map[string]any) {
	t.Helper()
	if want == nil {
		require.Nil(t, got)
		return
	}
	rocEqualFields(t, want, hdlcOpCanonical(got).(map[string]any))
}
func hdlcOpTypes(t *testing.T, f map[string]any) {
	t.Helper()
	for _, k := range []string{"Frame Format", "FCS", "HCS"} {
		if _, ok := f[k]; ok {
			require.IsType(t, uint16(0), f[k])
		}
	}
	for _, k := range []string{"Source Address", "Destination Address"} {
		if _, ok := f[k]; ok {
			require.IsType(t, uint32(0), f[k])
		}
	}
	for _, k := range []string{"Frame Length", "Source Address Octets", "Destination Address Octets"} {
		if _, ok := f[k]; ok {
			require.IsType(t, int(0), f[k])
		}
	}
	for _, k := range []string{"Control", "Send Sequence", "Receive Sequence", "Invoke ID and Priority", "Invoke ID"} {
		if _, ok := f[k]; ok {
			require.IsType(t, byte(0), f[k])
		}
	}
	if v, ok := f["LLC"]; ok {
		require.IsType(t, []byte{}, v)
		require.Len(t, v, 3)
	}
	for _, name := range []string{"Set", "Action"} {
		if s, ok := f[name].(map[string]any); ok {
			selectFieldTypes(t, s)
			if v, ok := s["block_number"]; ok {
				require.IsType(t, uint32(0), v)
			}
			if v, ok := s["raw_data_length"]; ok {
				require.IsType(t, uint64(0), v)
			}
			if v, ok := s["observed_block_count"]; ok {
				require.IsType(t, int(0), v)
			}
			for _, key := range []string{"last_block", "transfer_complete"} {
				if v, ok := s[key]; ok {
					require.IsType(t, false, v)
				}
			}
			if v, ok := s["assembled_data"].(map[string]any); ok {
				actionDataTypes(t, v)
			}

			for _, k := range []string{"wrapper_version", "source_wport", "destination_wport", "wrapper_header_hex"} {
				require.NotContains(t, s, k)
			}
			for _, k := range []string{"command", "service_choice", "invoke_id_and_priority", "invoke_id", "unused_flag_bits_raw", "action_result", "return_choice_raw", "data_access_result"} {
				if _, ok := s[k]; ok {
					require.IsType(t, byte(0), s[k])
				}
			}
			if _, ok := s["class_id"]; ok {
				require.IsType(t, uint16(0), s["class_id"])
			}
			for _, k := range []string{"attribute_id", "method_id"} {
				if _, ok := s[k]; ok {
					require.IsType(t, int8(0), s[k])
				}
			}
			require.Equal(t, false, s["operation_success_verified"])
			require.Equal(t, false, s["authentication_verified"])

			for _, k := range []string{"list_count", "value_count", "parameter_count", "assembled_value_count"} {
				if v, ok := s[k]; ok {
					require.IsType(t, uint64(0), v)
				}
			}
			for _, key := range []string{"descriptors", "values", "parameters", "results", "assembled_values"} {
				if v, ok := s[key]; ok {
					items, ok := v.([]map[string]any)
					require.True(t, ok)
					for _, item := range items {
						if key == "assembled_values" {
							require.IsType(t, int(0), item["encoded_offset"])
							require.NotContains(t, item, "apdu_offset")
						}
						for _, k := range []string{"index", "apdu_offset", "encoded_offset", "encoded_length"} {
							if v, ok := item[k]; ok {
								require.IsType(t, int(0), v)
							}
						}
						if v, ok := item["class_id"]; ok {
							require.IsType(t, uint16(0), v)
						}
						for _, k := range []string{"method_id", "attribute_id"} {
							if v, ok := item[k]; ok {
								require.IsType(t, int8(0), v)
							}
						}
						for _, k := range []string{"action_result", "data_access_result", "return_choice_raw", "access_selection_raw"} {
							if v, ok := item[k]; ok {
								require.IsType(t, byte(0), v)
							}
						}
						if d, ok := item["data"].(map[string]any); ok {
							actionDataTypes(t, d)
						}
					}
				}
			}
			for _, k := range []string{"parameter", "data"} {
				if d, ok := s[k].(map[string]any); ok {
					actionDataTypes(t, d)
				}
			}
		}
	}
}
func hdlcOpQueries(t *testing.T, e *ProtocolEvent, want map[string]any, problem string) {
	t.Helper()
	for repeat := 0; repeat < 2; repeat++ {
		f, err := e.GetFields()
		d, de := e.Decode()
		if problem != "" {
			rocTypedError(t, problem, err)
			rocTypedError(t, problem, de)
			require.Nil(t, f)
			require.Nil(t, d)
			continue
		}
		require.NoError(t, err)
		require.NoError(t, de)
		hdlcOpEqual(t, want, f)
		hdlcOpTypes(t, f)
		hdlcOpEqual(t, map[string]any{"fields": want, "session": want}, d)
		if llc, ok := f["LLC"].([]byte); ok {
			clear(llc)
		}
		poisonWrapperStructured(f)
		poisonWrapperStructured(d)
	}
}
func hdlcOpCheck(t *testing.T, c hdlcOpCase, es []*ProtocolEvent, transport string, captured, deferred bool) {
	t.Helper()
	want := []hdlcOpStep{}
	for _, s := range c.Steps {
		if transport == "tcp" && s.TCPOverride != nil {
			continue
		}
		want = append(want, s)
	}
	require.Len(t, es, len(want))
	ids := make([]uint64, len(es))
	for i, e := range es {
		ids[i] = e.ID
		require.NotZero(t, e.ID)
		for j := 0; j < i; j++ {
			require.NotEqual(t, ids[j], e.ID)
		}
	}
	for i, e := range es {
		s := want[i]
		require.Equal(t, "dlms", e.Protocol)
		require.Equal(t, s.Profile, e.Profile)
		require.Equal(t, s.Completeness, e.Completeness)
		status := s.Status
		if deferred && s.Error == "" {
			status = "deferred"
		}
		require.Equal(t, status, e.Status)
		require.Equal(t, s.Direction, e.Direction)
		require.Equal(t, transport, e.Transport)
		require.Equal(t, s.At, e.Timestamp.Unix())
		require.Equal(t, s.Length, e.Length)
		require.Zero(t, s.Tx)
		require.Zero(t, e.TransactionID)
		if s.Response == 0 {
			require.Zero(t, e.ResponseTo)
		} else {
			require.Equal(t, ids[s.Response-1], e.ResponseTo)
		}
		offset := s.OffsetUDP
		if transport == "tcp" {
			offset = s.OffsetTCP
		}
		require.Equal(t, offset, e.Offset)
		if s.Raw == nil {
			require.Nil(t, e.Raw)
		} else {
			require.Equal(t, wrapperWire(t, *s.Raw), e.Raw)
		}
		if transport == "udp" {
			switch v := s.FlowID.(type) {
			case float64:
				require.Zero(t, v)
				require.Zero(t, e.FlowID)
			case string:
				require.Equal(t, "same retained flow", v)
				require.NotZero(t, e.FlowID)
				require.Equal(t, es[0].FlowID, e.FlowID)
			default:
				t.Fatalf("missing independent FlowID in %s", c.Name)
			}
		}
		hdlcOpEqual(t, s.Session, e.Session)
		hdlcOpQueries(t, e, s.Fields, s.Error)
		if s.Error == "" {
			require.Empty(t, e.Error)
		} else {
			require.Contains(t, e.Error, s.Error)
		}
		if captured {
			require.Len(t, e.SourceBytes.PacketRefs, 1)
			shift := 1
			if transport == "tcp" {
				shift = 4
			}
			require.EqualValues(t, i+shift, e.SourceBytes.PacketRefs[0].Number)
			require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
			endpoints := []string{"192.0.2.1:41000", "192.0.2.2:4059"}
			require.Equal(t, endpoints[s.Direction], e.Source)
			require.Equal(t, endpoints[1-s.Direction], e.Destination)
		}
		if llc, ok := e.Session["LLC"].([]byte); ok {
			clear(llc)
		}
		actionPoison(e)
		hdlcOpQueries(t, e, s.Fields, s.Error)
	}
}
func hdlcOpCloseCheck(t *testing.T, c hdlcOpCase, es []*ProtocolEvent, transport, reason string) {
	t.Helper()
	if transport == "tcp" && c.TCPCloseOverride != nil {
		require.Empty(t, es)
		return
	}
	require.Len(t, es, len(c.Close.Events))
	for i, e := range es {
		w := c.Close.Events[i]
		require.Equal(t, "dlms", e.Protocol)
		require.Equal(t, w.Profile, e.Profile)
		require.Equal(t, w.Status, e.Status)
		require.Equal(t, w.Completeness, e.Completeness)
		require.Equal(t, w.Direction, e.Direction)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
		require.Nil(t, e.Raw)
		require.Zero(t, e.Length)
		summary, at, offset := w.SummaryUDP, w.TimeUDP, w.OffsetUDP
		if transport == "tcp" {
			summary, at, offset = strings.Replace(w.SummaryTCP, "<reason>", reason, 1), w.TimeTCP, w.OffsetTCP
		}
		require.Equal(t, summary, e.Summary)
		require.Equal(t, summary, e.Error)
		require.Equal(t, at, e.Timestamp.Unix())
		require.Equal(t, offset, e.Offset)
		hdlcOpEqual(t, w.Session, e.Session)
		if w.Profile == "dlms-hdlc-ln-operations-segmented" || w.Profile == "dlms-hdlc-ln-operation-lists-segmented" || w.Profile == "dlms-hdlc-set-list-block-request-segmented" || w.Profile == "dlms-hdlc-set-list-block-response-segmented" || w.Profile == "dlms-hdlc-set-block-request-segmented" || w.Profile == "dlms-hdlc-set-block-response-segmented" {
			require.Len(t, e.Session, 3)
			for _, key := range []string{"Outstanding", "ObservedFrames", "InformationBytes"} {
				require.IsType(t, int(0), e.Session[key])
			}
		} else {
			require.IsType(t, byte(0), e.Session["Invoke ID"])
			if v, ok := e.Session["Service Choice"]; ok {
				require.IsType(t, byte(0), v)
				require.IsType(t, uint64(0), e.Session["List Count"])
			}
			require.IsType(t, uint32(0), e.Session["Source Address"])
		}

		hdlcOpQueries(t, e, nil, "NeedMore")
		actionPoison(e)
		hdlcOpQueries(t, e, nil, "NeedMore")
	}
}
func TestDLMSHDLCOperationsNative(t *testing.T) { hdlcOpNative(t, hdlcOpControls(t), 41000, 4059) }
func TestDLMSHDLCOperationsOffPort(t *testing.T) {
	hdlcOpNative(t, hdlcOpControls(t)[:1], 45000, 45001)
}
func hdlcOpNative(t *testing.T, cases []hdlcOpCase, source, destination uint16) {
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
						for _, step := range c.Steps {
							w := wrapperWire(t, step.Wire)
							require.Equal(t, step.SHA, fmt.Sprintf("%x", sha256.Sum256(w)))
							before := len(es)
							var final *ProtocolError
							for at := 0; at < len(w); {
								end := len(w)
								if chunk > 0 {
									end = min(end, at+chunk)
								}
								input := bytes.Clone(w[at:end])
								o := s.Feed(step.Direction, time.Unix(step.At, 0), input)
								clear(input)
								require.Equal(t, end-at, o.Consumed)
								if transport == "tcp" && step.TCPOverride != nil {
									require.Empty(t, o.Events)
									rocTypedError(t, step.TCPOverride.Error, o.Err)
									require.Equal(t, step.TCPOverride.Message, o.Err.Message)
									require.False(t, o.NeedMore)
								} else if len(o.Events) == 0 {
									if transport == "tcp" {
										require.NotNil(t, o.Err)
										require.Equal(t, ErrNeedMore, o.Err.Kind)
									}
								} else {
									final = o.Err
								}
								for _, e := range o.Events {
									es = append(es, actionSnapshot(e))
									if llc, ok := e.Session["LLC"].([]byte); ok {
										clear(llc)
									}
									actionPoison(e)
								}
								at = end
							}
							if transport == "tcp" && step.TCPOverride != nil {
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
						hdlcOpCheck(t, c, es, transport, false, deferred)
						hdlcOpCloseCheck(t, c, s.Close("hdlc-test"), transport, "hdlc-test")
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
						require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
					})
				}
			}
		}
	}
}

func TestDLMSHDLCOperationsPublicAPIMissingBaseline(t *testing.T) {
	for _, tc := range []struct{ name, wire, kind string }{{"Set", "7ea01e0321105e8de6e600c101c1000100002a0000ff0200090301020368977e", "Set Request Normal"}, {"Action", "7ea01e0321105e8de6e600c301c1000f0000280000ff01010903010203de887e", "Action Request Normal"}, {"AdjacentGet", "7ea0190321107fdae6e600c001c1000100002a0000ff020012807e", "Get Request Normal"}} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 4059))
			require.NoError(t, err)
			defer s.Close("baseline")
			w := wrapperWire(t, tc.wire)
			input := bytes.Clone(w)
			o := s.Feed(0, time.Unix(100, 0), input)
			clear(input)
			fmt.Printf("HDLC_EXISTING_API_MISSING %s err=%v events=%d\n", tc.name, o.Err, len(o.Events))
			require.Nil(t, o.Err)
			require.Len(t, o.Events, 1)
			f, err := o.Events[0].GetFields()
			require.NoError(t, err)
			require.Equal(t, tc.kind, f["Frame Kind"])
			require.Zero(t, o.Events[0].TransactionID)
			require.Zero(t, o.Events[0].ResponseTo)
			require.Equal(t, w, o.Events[0].Raw)
		})
	}
}

func hdlcOpReplay(t *testing.T, c hdlcOpCase, raw []byte, workers int, deferred, observe bool, transport string) ([]*ProtocolEvent, ProtocolStats) {
	t.Helper()
	var es []*ProtocolEvent
	var stats ProtocolStats
	seen := 0
	opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDecodeAs("udp", 4059, "dlms"), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) {
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
func TestDLMSHDLCOperationsMatrix(t *testing.T) {
	for _, c := range hdlcOpControls(t) {
		for _, transport := range []string{"udp", "tcp"} {
			t.Run(c.Name+"/"+transport, func(t *testing.T) {
				var reuse map[string]string
				require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-operations", "DLMS_HDLC_OPERATIONS_TEST_ROOT", "reuse-inputs.json"), &reuse))
				name := c.Name + "." + transport + ".pcap"
				raw := []byte(nil)
				if path, ok := reuse[name]; ok {
					parts := strings.SplitN(path, "/", 2)
					require.Len(t, parts, 2)
					raw = finsReadBatch(t, parts[0], "DLMS_HDLC_OPERATIONS_TEST_ROOT", parts[1])
				} else {
					raw = finsReadBatch(t, "dlms-hdlc-operations", "DLMS_HDLC_OPERATIONS_TEST_ROOT", name)
				}
				discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
					es, stats := hdlcOpReplay(t, c, raw, w, d, o, transport)
					count := len(c.Steps)
					if transport == "tcp" {
						for _, s := range c.Steps {
							if s.TCPOverride != nil {
								count--
							}
						}
					}
					closeCount := len(c.Close.Events)
					if transport == "tcp" && c.TCPCloseOverride != nil {
						closeCount = 0
					}
					require.Len(t, es, count+closeCount)
					hdlcOpCheck(t, c, es[:count], transport, true, d)
					hdlcOpCloseCheck(t, c, es[count:], transport, "ctx-canceled")
					require.EqualValues(t, count, stats.Messages)
					require.EqualValues(t, closeCount, stats.Incomplete)
				})
			})
		}
	}
}

func TestDLMSHDLCOperationsProjectionRefusalLifecycle(t *testing.T) {
	controls := hdlcOpControls(t)
	original := controls[len(controls)-2]
	q := wrapperWire(t, original.Steps[2].Wire)
	// Existing HDLC conversation storage512 + bounded operation projection.
	// After a complete Get exchange,21-byte Set APDU has8 Data wirebytes.
	need := 512 + 32768 + 512*len(q) + 2048*8
	require.Equal(t, 67584, need)
	for _, delta := range []int{-1, 0} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("delta%d/d%t", delta, deferred), func(t *testing.T) {
				b := DefaultParserBudget()
				b.MaxFrameBytes = 4096
				b.MaxMessageBytes = 4096
				b.MaxBufferedBytes = need + delta
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41000, 4059))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				c := original
				c.Steps = append([]hdlcOpStep(nil), original.Steps...)
				if delta < 0 {
					for i := 2; i < len(c.Steps); i++ {
						v := &c.Steps[i]
						v.Session = nil
						v.Fields = nil
						v.Response = 0
						v.Tx = 0
						v.Status = "context-required"
						v.Completeness = "context-required"
						v.Error = "ResourceExceeded"
						if i == 3 {
							v.Error = "ContextRequired"
							v.Completeness = "context-required"
						}
					}
				}
				var events []*ProtocolEvent
				for _, step := range c.Steps {
					w := wrapperWire(t, step.Wire)
					input := bytes.Clone(w)
					o := s.Feed(step.Direction, time.Unix(step.At, 0), input)
					clear(input)
					require.Equal(t, len(w), o.Consumed)
					require.Len(t, o.Events, 1)
					if step.Error == "" {
						require.Nil(t, o.Err)
					} else {
						rocTypedError(t, step.Error, o.Err)
					}
					events = append(events, actionSnapshot(o.Events[0]))
					actionPoison(o.Events[0])
				}
				hdlcOpCheck(t, c, events, "udp", false, deferred)
				require.Empty(t, s.Close("projection"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
				require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
			})
		}
	}
}
