package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
	"time"
)

type actionStep struct {
	Wire                                 string `json:"input_hex"`
	SHA                                  string `json:"input_sha256"`
	Direction                            int
	At                                   int64 `json:"timestamp_unix"`
	Profile, Status, Completeness, Error string
	Fields, Session                      map[string]any
	Transaction                          int `json:"Transaction_index"`
	Response                             int `json:"Response_index"`
	Length                               int
	Raw                                  *string `json:"Raw_hex"`
}
type actionClose struct {
	Direction int
	At        int64 `json:"timestamp_unix"`
	Session   map[string]any
	UDP       string `json:"summary_udp"`
	TCP       string `json:"summary_tcp"`
}
type actionCase struct {
	Name   string
	Steps  []actionStep
	Budget struct {
		Collection int `json:"MaxCollectionElements"`
	}
	Close struct{ Events []actionClose }
}

func actionControls(t *testing.T) []actionCase {
	var x struct{ Cases []actionCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-action-normal", "DLMS_ACTION_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 20)
	var extra struct{ Cases []actionCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-action-normal", "DLMS_ACTION_TEST_ROOT", "supplemental.json"), &extra))
	require.Len(t, extra.Cases, 8)
	return append(x.Cases, extra.Cases...)
}
func actionQueries(t *testing.T, e *ProtocolEvent, f, s map[string]any, problem string) {
	t.Helper()
	fields, err := e.GetFields()
	d, de := e.Decode()
	if problem != "" {
		rocTypedError(t, problem, err)
		rocTypedError(t, problem, de)
		require.Nil(t, fields)
		require.Nil(t, d)
		return
	}
	require.NoError(t, err)
	require.NoError(t, de)
	rocEqualFields(t, f, fields)
	rocEqualFields(t, f, d["fields"].(map[string]any))
	rocEqualFields(t, s, d["session"].(map[string]any))
	require.Nil(t, d["metadata"])
	for _, k := range []string{"parameter", "data"} {
		if v, ok := fields[k].(map[string]any); ok {
			actionDataTypes(t, v)
		}
	}
	poisonWrapperStructured(fields)
	poisonWrapperStructured(d)
	// Verify the public field scalar types, not only JSON-numeric equality.
	for k, v := range f {
		switch k {
		case "wrapper_version", "source_wport", "destination_wport", "class_id":
			require.IsType(t, uint16(0), fields[k])
		case "command", "service_choice", "invoke_id_and_priority", "invoke_id", "unused_flag_bits_raw", "action_result", "return_choice_raw", "data_access_result":
			require.IsType(t, byte(0), fields[k])
		case "method_id", "attribute_id":
			require.IsType(t, int8(0), fields[k])
		case "payload_length":
			require.IsType(t, int(0), fields[k])
		default:
			_ = v
		}
	}
}
func actionCheck(t *testing.T, c actionCase, es []*ProtocolEvent, transport string, captured, deferred bool) {
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
			if a.Session == nil || a.Session["Association"] == "unmatched-response" && len(c.Steps) == 1 {
				require.Zero(t, e.FlowID)
			} else {
				require.NotZero(t, e.FlowID)
				require.Equal(t, es[0].FlowID, e.FlowID)
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
func actionCloseCheck(t *testing.T, c actionCase, es []*ProtocolEvent, transport, reason string) {
	t.Helper()
	require.Len(t, es, len(c.Close.Events))
	for i, e := range es {
		a := c.Close.Events[i]
		require.Equal(t, "dlms-wrapper", e.Protocol)
		require.Equal(t, "dlms-wrapper-v1-action-normal", e.Profile)
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
			want = reason + ": DLMS Wrapper Action ended with an unmatched request"
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
func TestDLMSActionNormalNative(t *testing.T) {
	for _, c := range actionControls(t) {
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
						actionCheck(t, c, es, transport, false, d)
						actionCloseCheck(t, c, s.Close("action-test"), transport, "action-test")
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
						require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
					})
				}
			}
		}
	}
}
func TestDLMSActionGetBlockServiceBoundary(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 4059))
	require.NoError(t, err)
	defer s.Close("test")
	q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "0001001000010012c301c1000f0000280000ff01010903010203"))
	require.Nil(t, q.Err)
	require.Len(t, q.Events, 1)
	r := s.Feed(1, time.Unix(2, 0), wrapperWire(t, "000100010010000ac402c101000000010103"))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.Zero(t, r.Events[0].ResponseTo, "Get-block must never consume an Action owner")
	require.Zero(t, r.Events[0].TransactionID)
	require.Equal(t, "ambiguous-service", r.Events[0].Session["Association"])
	require.Equal(t, 0, r.Events[0].Session["Outstanding"])
	late := s.Feed(1, time.Unix(3, 0), wrapperWire(t, "0001000100100005c701c10000"))
	require.Nil(t, late.Err)
	require.Len(t, late.Events, 1)
	require.Zero(t, late.Events[0].ResponseTo)
	require.Zero(t, late.Events[0].TransactionID)
	require.Equal(t, "ambiguous-invoke", late.Events[0].Session["Association"])
}

func TestDLMSActionGBTBoundary(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 4059))
	require.NoError(t, err)
	defer s.Close("test")
	o := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "0001001000010019e0810001000012c301c1000f0000280000ff01010903010203"))
	require.Len(t, o.Events, 1)
	rocTypedError(t, "UnsupportedFeature", o.Err)
	require.Zero(t, o.Events[0].ResponseTo)
	require.Zero(t, o.Events[0].TransactionID)
}

func actionReplay(t *testing.T, c actionCase, raw []byte, workers int, deferred, observe bool) ([]*ProtocolEvent, ProtocolStats) {
	t.Helper()
	var es []*ProtocolEvent
	var stats ProtocolStats
	seen := 0
	opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDecodeAs("udp", 4059, "dlms-wrapper"), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) {
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
		require.Equal(t, len(c.Steps), seen)
	}
	return es, stats
}
func TestDLMSActionNormalMatrix(t *testing.T) {
	for _, c := range actionControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			raw := finsReadBatch(t, "dlms-action-normal", "DLMS_ACTION_TEST_ROOT", c.Name+".pcap")
			discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
				es, stats := actionReplay(t, c, raw, w, d, o)
				require.Len(t, es, len(c.Steps)+len(c.Close.Events))
				actionCheck(t, c, es[:len(c.Steps)], "udp", true, d)
				actionCloseCheck(t, c, es[len(c.Steps):], "udp", "")
				require.EqualValues(t, len(c.Steps), stats.Messages)
				require.EqualValues(t, len(c.Close.Events), stats.Incomplete)
			})
		})
	}
}

func TestDLMSActionNormalProjectionRefusalLifecycle(t *testing.T) {
	q := wrapperWire(t, "000100100001000dc301c1000f0000280000ff0100")
	r := wrapperWire(t, "000100010010000bc701c10001000903010203")
	need := 512 + 256 + 32768 + 512*len(r) + 2048*min(256, len(r)-14) + 256
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
				request := s.Feed(0, time.Unix(1, 0), bytes.Clone(q))
				require.Nil(t, request.Err)
				require.Len(t, request.Events, 1)
				o := s.Feed(1, time.Unix(2, 0), bytes.Clone(r))
				require.Len(t, o.Events, 1)
				e := o.Events[0]
				if delta == 0 {
					require.Nil(t, o.Err)
					f, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, map[string]any{"type": float64(9), "raw_hex": "0903010203", "length": float64(3), "length_encoding_hex": "03", "value_hex": "010203"}, f["data"].(map[string]any))
					require.Equal(t, request.Events[0].ID, e.ResponseTo)
					require.Equal(t, request.Events[0].ID, e.TransactionID)
					require.Equal(t, "observed-response", e.Session["Association"])
				} else {
					rocTypedError(t, "ResourceExceeded", o.Err)
					actionQueries(t, e, nil, nil, "ResourceExceeded")
					require.Nil(t, e.Raw)
					require.Nil(t, e.Session)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
					late := s.Feed(1, time.Unix(3, 0), wrapperWire(t, "0001000100100005c701c10000"))
					require.Nil(t, late.Err)
					require.Len(t, late.Events, 1)
					require.Equal(t, "ambiguous-conversation", late.Events[0].Session["Association"])
					require.Equal(t, 0, late.Events[0].Session["Outstanding"])
					require.Zero(t, late.Events[0].ResponseTo)
					require.Zero(t, late.Events[0].TransactionID)
				}
				require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
				require.Empty(t, s.Close("projection"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
func TestDLMSActionNormalIdleRetiresOwner(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 4059))
	require.NoError(t, err)
	q := wrapperWire(t, "000100100001000dc301c1000f0000280000ff0100")
	r := wrapperWire(t, "0001000100100005c701c10000")
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

func actionSnapshot(e *ProtocolEvent) *ProtocolEvent {
	v := *e
	v.Raw = bytes.Clone(e.Raw)
	v.Session = cloneSession(e.Session)
	v.Fields = cloneSession(e.Fields)
	v.Structured = cloneSession(e.Structured)
	v.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
	return &v
}
func actionPoison(e *ProtocolEvent) {
	if e.Session != nil {
		poisonWrapperStructured(e.Session)
	}
	clear(e.Raw)
	e.Protocol = "tls"
	e.Profile = "tls"
	e.Fields = map[string]any{"caller": true}
	e.Structured = map[string]any{"caller": true}
	e.Metadata = "caller"
	e.ID = 999999
	e.TransactionID = 999999
	e.ResponseTo = 999999
	if len(e.SourceBytes.PacketRefs) > 0 {
		e.SourceBytes.PacketRefs[0].Number = 999999
	}
}

func actionDataTypes(t *testing.T, m map[string]any) {
	t.Helper()
	require.IsType(t, byte(0), m["type"])
	if _, ok := m["length"]; ok {
		require.IsType(t, uint64(0), m["length"])
	}
	if m["type"] == byte(17) {
		require.IsType(t, uint64(0), m["value"])
	}
	if a, ok := m["elements"]; ok {
		require.IsType(t, []map[string]any{}, a)
		for _, v := range a.([]map[string]any) {
			actionDataTypes(t, v)
		}
	}
}
