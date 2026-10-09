package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type discoveryControl struct {
	Name, Protocol, File, SHA256 string
	PayloadHex                   string                         `json:"payload_hex"`
	ExpectedFields               map[string]any                 `json:"expected_fields"`
	ExpectedError                *struct{ Kind, Detail string } `json:"expected_validation_error"`
	Port                         uint16
	Steps                        []struct {
		Dir                 int
		Hex                 string
		Source, Destination string
	}
	Packets               int
	ExpectedDecodedFields []struct{ Fields map[string]any } `json:"expected_decoded_fields"`
	Budgets               *struct {
		Message  int `json:"max_message_bytes"`
		Buffered int `json:"max_buffered_bytes"`
	} `json:"budgets"`
	Association struct {
		Pairs []struct {
			Request  []int `json:"request_packetRefs"`
			Response []int `json:"response_packetRefs"`
			Sequence uint32
		}
		Outstanding *int
		Unmatched   []int   `json:"unmatched_response_packetRefs"`
		Ambiguous   *uint32 `json:"ambiguous_sequence"`
	} `json:"expected_conservative_association"`
}

func discoveryControls(t *testing.T, file string) []discoveryControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("knx-pfcp/" + file)
	require.NoError(t, err)
	var m struct{ Cases []discoveryControl }
	require.NoError(t, json.Unmarshal(b, &m))
	// Every scenario must bind its capture and independent answer through the
	// immutable inventory, including cases that share identical capture bytes.
	inventories, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	var original struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(b, &original))
	for i, c := range m.Cases {
		answerPath := "answers/" + c.Name + ".json"
		answer, err := trafficfixture.ReadFile("knx-pfcp/" + answerPath)
		require.NoError(t, err)
		require.JSONEq(t, string(original.Cases[i]), string(answer))
		bound := 0
		for _, inventory := range inventories {
			for _, item := range inventory.Cases {
				if item.Input.OriginalPath != "common/pcapx/pcaputil/knx-pfcp/"+c.File {
					continue
				}
				require.Equal(t, c.SHA256, item.Input.SHA256)
				for _, expectation := range item.Expectations {
					var binding struct {
						AnswerFile string `json:"answer_file"`
						AnswerSHA  string `json:"answer_sha256"`
						Packets    int    `json:"packet_count"`
					}
					require.NoError(t, json.Unmarshal(expectation.PayloadConstraints, &binding))
					if binding.AnswerFile != answerPath {
						continue
					}
					require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(answer)), binding.AnswerSHA)
					require.Equal(t, c.Packets, binding.Packets)
					bound++
				}
			}
		}
		require.Equal(t, 1, bound, "unbound or duplicated scenario answer: %s", c.Name)
	}
	for i := range m.Cases {
		if m.Cases[i].Name == "knx-extended-search-request" {
			upgrade, err := trafficfixture.ReadFile("knx-extended/legacy-upgrade.json")
			require.NoError(t, err)
			var current struct {
				PayloadHex string `json:"payload_hex"`
				Fields     map[string]any
			}
			require.NoError(t, json.Unmarshal(upgrade, &current))
			require.Equal(t, m.Cases[i].PayloadHex, current.PayloadHex)
			require.NotEmpty(t, current.Fields)
			m.Cases[i].ExpectedError = nil
			m.Cases[i].ExpectedFields = current.Fields
		}
	}
	return m.Cases
}
func discoveryCapture(t *testing.T, c discoveryControl) []byte {
	t.Helper()
	b, err := trafficfixture.ReadFile("knx-pfcp/" + c.File)
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(b)))
	return b
}
func discoveryMatrix(t *testing.T, f func(*testing.T, int, bool, bool)) {
	t.Helper()
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			for _, observe := range []bool{false, true} {
				t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) { f(t, workers, deferred, observe) })
			}
		}
	}
}
func discoveryReplay(t *testing.T, raw []byte, workers int, deferred, observe bool, extra ...CaptureOption) ([]*ProtocolEvent, ProtocolStats) {
	t.Helper()
	var events []*ProtocolEvent
	var stats ProtocolStats
	var seen atomic.Int64
	var assembly TCPReassemblyStats
	opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s }), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
	if observe {
		opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.NotEmpty(t, p.Data()); seen.Add(1) }))
	}
	opts = append(extra, opts...)
	owned := bytes.Clone(raw)
	require.NoError(t, ReplayPcap(bytes.NewReader(owned), opts...))
	clear(owned)
	require.Zero(t, stats.BufferedBytes)
	require.Zero(t, stats.CallbackPanics)
	if observe {
		require.EqualValues(t, assembly.CapturedPackets, seen.Load())
	}
	return events, stats
}
func TestKNXPFCPSealedByteOracle(t *testing.T) {
	cs := discoveryControls(t, "controls.json")
	require.Len(t, cs, 41)
	for _, c := range cs {
		t.Run(c.Name, func(t *testing.T) {
			w, err := hex.DecodeString(c.PayloadHex)
			require.NoError(t, err)
			var fields map[string]any
			if c.Port == 3671 {
				fields, err = decodeKNXSearch(w, 4096)
			} else {
				fields, err = decodePFCPHeartbeat(w, 4096)
			}
			if c.ExpectedError != nil {
				rocTypedError(t, c.ExpectedError.Kind, err)
				require.Nil(t, fields)
				require.Contains(t, err.Error(), c.ExpectedError.Detail)
			} else {
				require.NoError(t, err)
				rocEqualFields(t, c.ExpectedFields, fields)
			}
		})
	}
}
func TestKNXPFCPSealedNativeMatrix(t *testing.T) {
	for _, c := range discoveryControls(t, "controls.json") {
		t.Run(c.Name, func(t *testing.T) {
			raw := discoveryCapture(t, c)
			w, _ := hex.DecodeString(c.PayloadHex)
			protocol, profile := "knx", "knx-basic-search"
			if c.Name == "knx-extended-search-request" {
				profile = "knx-extended-search"
			}
			if c.Port == 8805 {
				protocol, profile = "pfcp", "pfcp-v1-heartbeat"
			}
			expectedError := c.ExpectedError
			if c.Name == "pfcp-other-message-type" {
				// Preserve the original Heartbeat-only decoder oracle above.
				// Native admission now implements Setup; its independently
				// pinned mandatory-IE answer is a separate immutable overlay.
				raw, err := trafficfixture.ReadFile("pfcp-setup/native-profile-upgrade/heartbeat-upgrade.json")
				require.NoError(t, err)
				var upgrade struct {
					Name       string
					CaptureSHA string                        `json:"capture_sha256"`
					Payload    string                        `json:"payload_hex"`
					PayloadSHA string                        `json:"payload_sha256"`
					Profile    string                        `json:"native_profile"`
					Error      struct{ Kind, Detail string } `json:"expected_error"`
				}
				require.NoError(t, json.Unmarshal(raw, &upgrade))
				require.Equal(t, c.Name, upgrade.Name)
				require.Equal(t, c.SHA256, upgrade.CaptureSHA)
				require.Equal(t, c.PayloadHex, upgrade.Payload)
				require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(w)), upgrade.PayloadSHA)
				profile = upgrade.Profile
				expectedError = &struct{ Kind, Detail string }{upgrade.Error.Kind, upgrade.Error.Detail}
			}
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := discoveryReplay(t, raw, workers, deferred, observe, WithProtocolDecodeAs("udp", c.Port, protocol))
				require.Len(t, events, 1)
				require.EqualValues(t, 1, len(events[0].SourceBytes.PacketRefs))
				require.EqualValues(t, 1, stats.Messages)
				e := events[0]
				require.Equal(t, protocol, e.Protocol)
				require.Equal(t, profile, e.Profile)
				require.Equal(t, "explicit-decode-as", e.Admission)
				require.Equal(t, w, e.Raw)
				require.Len(t, e.SourceBytes.PacketRefs, 1)
				require.EqualValues(t, 1, e.SourceBytes.PacketRefs[0].Number)
				require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
				require.Zero(t, e.ResponseTo)
				fields, err := e.GetFields()
				if expectedError != nil {
					rocTypedError(t, expectedError.Kind, err)
					require.Contains(t, err.Error(), expectedError.Detail)
					require.Nil(t, fields)
					require.Nil(t, e.Session)
					return
				}
				require.NoError(t, err)
				require.Empty(t, e.Error)
				rocEqualFields(t, c.ExpectedFields, fields)
				if protocol == "knx" {
					require.Zero(t, e.FlowID)
					require.Zero(t, e.TransactionID)
				}
				fields["kind"] = "caller change"
				if rows, ok := fields["ies"].([]map[string]any); ok && len(rows) > 0 {
					rows[0]["value_hex"] = "changed"
				}
				clear(e.Raw)
				e.Session["kind"] = "public projection change"
				fields, err = e.GetFields()
				require.NoError(t, err)
				rocEqualFields(t, c.ExpectedFields, fields)
			})
		})
	}
}
func TestKNXPFCPPublicNativeIngress(t *testing.T) {
	n := 0
	for _, c := range discoveryControls(t, "controls.json") {
		if c.ExpectedFields == nil {
			continue
		}
		n++
		t.Run(c.Name, func(t *testing.T) {
			events, _ := discoveryReplay(t, discoveryCapture(t, c), 1, false, false)
			require.Len(t, events, 1)
			want := "knx"
			if c.Port == 8805 {
				want = "pfcp"
			}
			require.Equal(t, want, events[0].Protocol)
			require.Empty(t, events[0].Error)
			f, err := events[0].GetFields()
			require.NoError(t, err)
			rocEqualFields(t, c.ExpectedFields, f)
		})
	}
	require.Equal(t, 20, n)
}
func TestPFCPSealedAssociationMatrix(t *testing.T) {
	cs := discoveryControls(t, "association-controls.json")
	require.Len(t, cs, 8)
	for _, c := range cs {
		t.Run(c.Name, func(t *testing.T) {
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				var extra []CaptureOption
				if c.Budgets != nil {
					extra = append(extra, WithBinParserConfig(BinParserConfig{MaxMessageBytes: c.Budgets.Message, MaxBufferedBytes: c.Budgets.Buffered}))
				}
				events, stats := discoveryReplay(t, discoveryCapture(t, c), workers, deferred, observe, extra...)
				require.Len(t, events, c.Packets)
				require.EqualValues(t, c.Packets, stats.Messages)
				ids := map[int]uint64{}
				matched := map[int]int{}
				for i, e := range events {
					require.Equal(t, "pfcp", e.Protocol)
					require.Empty(t, e.Error)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
					wire, err := hex.DecodeString(c.Steps[i].Hex)
					require.NoError(t, err)
					require.Equal(t, wire, e.Raw)
					fields, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, c.ExpectedDecodedFields[i].Fields, fields)
					ids[i+1] = e.ID
					if e.ResponseTo != 0 {
						for j, id := range ids {
							if id == e.ResponseTo {
								matched[i+1] = j
							}
						}
						require.Equal(t, e.ResponseTo, e.TransactionID)
					}
				}
				require.Len(t, matched, len(c.Association.Pairs))
				for _, p := range c.Association.Pairs {
					require.Len(t, p.Response, 1)
					require.Equal(t, p.Request[0], matched[p.Response[0]])
					for _, n := range p.Request {
						require.Equal(t, ids[p.Request[0]], events[n-1].TransactionID)
					}
				}
				for _, n := range c.Association.Unmatched {
					require.Zero(t, events[n-1].ResponseTo)
					require.Equal(t, "unmatched-response", events[n-1].Session["Association"])
				}
				if c.Association.Outstanding != nil {
					require.EqualValues(t, *c.Association.Outstanding, events[len(events)-1].Session["Outstanding"])
				}
				if c.Association.Ambiguous != nil {
					for _, e := range events[1:] {
						require.Equal(t, "ambiguous-sequence", e.Session["Association"])
						require.Zero(t, e.ResponseTo)
					}
				}
			})
		})
	}
}

func discoveryPayload(t *testing.T, name string) []byte {
	t.Helper()
	for _, c := range discoveryControls(t, "controls.json") {
		if c.Name == name {
			w, err := hex.DecodeString(c.PayloadHex)
			require.NoError(t, err)
			return w
		}
	}
	t.Fatalf("missing sealed control %s", name)
	return nil
}
func discoverySession(t *testing.T, b ParserBudget, port uint16) *captureSession {
	t.Helper()
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, port))
	require.NoError(t, err)
	return s.(*captureSession)
}
func TestKNXPFCPDatagramBoundariesAndAdmission(t *testing.T) {
	for _, name := range []string{"knx-basic-search-request", "pfcp-heartbeat-request"} {
		t.Run(name, func(t *testing.T) {
			w := discoveryPayload(t, name)
			port := uint16(3671)
			protocol := "knx"
			if name == "pfcp-heartbeat-request" {
				port = 8805
				protocol = "pfcp"
			}
			s := discoverySession(t, DefaultParserBudget(), port)
			p := s.Probe(w)
			require.Equal(t, ProbeAccept, p.Verdict)
			require.Equal(t, protocol, p.Protocol)
			require.Zero(t, s.Stats().BufferedBytes)
			for cut := 1; cut < len(w); cut++ {
				left, right := s.Feed(0, time.Unix(10, 0), w[:cut]), s.Feed(0, time.Unix(11, 0), w[cut:])
				for _, out := range []FeedResult{left, right} {
					for _, e := range out.Events {
						require.False(t, e.Protocol == protocol && e.Error == "", "no cross-datagram reassembly at cut%d", cut)
					}
				}
			}
			joined := s.Feed(0, time.Unix(12, 0), append(bytes.Clone(w), w...))
			require.NotNil(t, joined.Err)
			valid := s.Feed(0, time.Unix(13, 0), w)
			require.Nil(t, valid.Err)
			require.Len(t, valid.Events, 1)
			require.Equal(t, protocol, valid.Events[0].Protocol)
			s.Close("done")
			require.Zero(t, s.Stats().BufferedBytes)
			require.Empty(t, s.Close("idempotent"))
			closed := s.Feed(0, time.Unix(14, 0), w)
			rocTypedError(t, "FatalSessionError", closed.Err)
			off := discoverySession(t, DefaultParserBudget(), 40001)
			out := off.Feed(0, time.Unix(10, 0), w)
			for _, e := range out.Events {
				require.NotEqual(t, protocol, e.Protocol)
			}
			off.Close("offport")
		})
	}
}
func TestKNXPFCPResourceBoundariesAndOwnership(t *testing.T) {
	q, r := discoveryPayload(t, "pfcp-heartbeat-request"), discoveryPayload(t, "pfcp-heartbeat-response")
	for _, delta := range []int{-1, 0} {
		t.Run(fmt.Sprintf("shared%d", delta), func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxFrameBytes, b.MaxMessageBytes = 64, 64
			b.MaxBufferedBytes = 512 + 128*len(q) + 4352 + len(q) + delta
			s := discoverySession(t, b, 8805)
			out := s.Feed(0, time.Unix(10, 0), q)
			if delta < 0 {
				rocTypedError(t, "ResourceExceeded", out.Err)
				require.Equal(t, "limited", out.Events[0].Status)
				require.Zero(t, s.Stats().ContextRequired)
				require.Zero(t, s.Stats().BufferedBytes)
			} else {
				require.Nil(t, out.Err)
				require.EqualValues(t, 4352+len(q), s.Stats().BufferedBytes)
				reply := s.Feed(1, time.Unix(11, 0), r)
				require.Nil(t, reply.Err)
				require.Equal(t, out.Events[0].ID, reply.Events[0].ResponseTo)
			}
			require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
			s.Close("budget")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	for _, name := range []string{"pfcp-heartbeat-request", "knx-basic-search-response"} {
		t.Run(name, func(t *testing.T) {
			w := discoveryPayload(t, name)
			protocol := "pfcp"
			port := uint16(8805)
			if name[0] == 'k' {
				protocol = "knx"
				port = 3671
			}
			for _, delta := range []int{-1, 0} {
				b := DefaultParserBudget()
				b.MaxFrameBytes = len(w) + delta
				s := discoverySession(t, b, port)
				e := &ProtocolEvent{Source: "a:40000", Destination: fmt.Sprintf("b:%d", port), Transport: "udp", Timestamp: time.Unix(1, 0)}
				require.True(t, s.f.a.decodeDiscoveryDatagram(e, w, 40000, port, protocol))
				if delta < 0 {
					rocTypedError(t, "ResourceExceeded", e.sessionError)
					require.Empty(t, e.Raw)
					require.Nil(t, e.Session)
					require.Zero(t, s.Stats().BufferedBytes)
				} else {
					require.Nil(t, e.sessionError)
					require.Equal(t, w, e.Raw)
				}
				s.Close("frame")
			}
			b := DefaultParserBudget()
			b.MaxRecursionDepth = discoveryFieldDepth(protocol, w) - 1
			s := discoverySession(t, b, port)
			out := s.Feed(0, time.Unix(1, 0), w)
			rocTypedError(t, "ResourceExceeded", out.Err)
			s.Close("depth")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	t.Run("IE-count", func(t *testing.T) {
		w := discoveryPayload(t, "pfcp-duplicate-recovery-first-wins")
		for _, limit := range []int{1, 2} {
			b := DefaultParserBudget()
			b.MaxCollectionElements = limit
			s := discoverySession(t, b, 8805)
			out := s.Feed(0, time.Unix(1, 0), w)
			if limit == 1 {
				rocTypedError(t, "ResourceExceeded", out.Err)
				require.Nil(t, out.Events[0].Session)
			} else {
				require.Nil(t, out.Err)
			}
			s.Close("collection")
			require.Zero(t, s.Stats().BufferedBytes)
		}
	})
}
func TestPFCPIsolationReuseDenialAndIdle(t *testing.T) {
	q, r := discoveryPayload(t, "pfcp-heartbeat-request"), discoveryPayload(t, "pfcp-heartbeat-response")
	ts := time.Unix(100, 0)
	event := func(src, dst string, d CaptureDomain, stamp time.Time) *ProtocolEvent {
		return &ProtocolEvent{Source: src, Destination: dst, Transport: "udp", Domain: d, Timestamp: stamp}
	}
	d0, d1 := CaptureDomain{Section: 1, Interface: 0}, CaptureDomain{Section: 1, Interface: 1}
	s := discoverySession(t, DefaultParserBudget(), 8805)
	a := s.f.a
	first := event("a:40000", "b:8805", d0, ts)
	borrowed := bytes.Clone(q)
	require.True(t, a.decodeDiscoveryDatagram(first, borrowed, 40000, 8805, "pfcp"))
	require.Nil(t, first.sessionError)
	clear(borrowed)
	for _, wrong := range []*ProtocolEvent{event("b:8805", "a:40000", d1, ts), event("c:8805", "a:40000", d0, ts)} {
		require.True(t, a.decodeDiscoveryDatagram(wrong, r, 8805, 40000, "pfcp"))
		require.Nil(t, wrong.sessionError)
		require.Zero(t, wrong.ResponseTo)
		require.Equal(t, "unmatched-response", wrong.Session["Association"])
	}
	good := event("b:8805", "a:40000", d0, ts)
	require.True(t, a.decodeDiscoveryDatagram(good, r, 8805, 40000, "pfcp"))
	require.Equal(t, first.ID, good.ResponseTo)
	duplicate := event("b:8805", "a:40000", d0, ts)
	require.True(t, a.decodeDiscoveryDatagram(duplicate, r, 8805, 40000, "pfcp"))
	require.Zero(t, duplicate.ResponseTo)
	retry := event("a:40000", "b:8805", d0, ts)
	require.True(t, a.decodeDiscoveryDatagram(retry, q, 40000, 8805, "pfcp"))
	require.Equal(t, "completed-sequence-reuse", retry.Session["Association"])
	expired := event("b:8805", "a:40000", d0, ts.Add(pfcpIdleTTL))
	require.True(t, a.decodeDiscoveryDatagram(expired, r, 8805, 40000, "pfcp"))
	require.Zero(t, expired.ResponseTo)
	require.Zero(t, s.Stats().BufferedBytes)
	fresh := event("a:40000", "b:8805", d0, ts.Add(pfcpIdleTTL))
	require.True(t, a.decodeDiscoveryDatagram(fresh, q, 40000, 8805, "pfcp"))
	require.NotEqual(t, first.FlowID, fresh.FlowID)
	s.Close("scope")
	require.Zero(t, s.Stats().BufferedBytes)
	require.Nil(t, a.udpSessions)
	require.Empty(t, s.Close("idempotent"))
	t.Run("denied-distinct-same-sequence", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxFrameBytes = len(q)
		s := discoverySession(t, b, 8805)
		first := s.Feed(0, ts, q)
		require.Nil(t, first.Err)
		large := discoveryPayload(t, "pfcp-recovery-extension")
		out := s.Feed(0, ts.Add(time.Second), large)
		rocTypedError(t, "ResourceExceeded", out.Err)
		late := s.Feed(1, ts.Add(2*time.Second), r)
		require.Nil(t, late.Err)
		require.Zero(t, late.Events[0].ResponseTo)
		require.Equal(t, "ambiguous-sequence", late.Events[0].Session["Association"])
		s.Close("pressure")
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("new-sequence-collection-denial", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxCollectionElements = 1
		s := discoverySession(t, b, 8805)
		first := s.Feed(0, ts, q)
		require.Nil(t, first.Err)
		next := bytes.Clone(q)
		next[6]++
		out := s.Feed(0, ts, next)
		rocTypedError(t, "ResourceExceeded", out.Err)
		late := s.Feed(1, ts, r)
		require.Nil(t, late.Err)
		require.Zero(t, late.Events[0].ResponseTo)
		require.Equal(t, "ambiguous-conversation", late.Events[0].Session["Association"])
		s.Close("bounded")
		require.Zero(t, s.Stats().BufferedBytes)
	})
}

func TestKNXMultipleDiscoveryResponders(t *testing.T) {
	cs := discoveryControls(t, "discovery-observations.json")
	require.Len(t, cs, 1)
	c := cs[0]
	discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
		events, stats := discoveryReplay(t, discoveryCapture(t, c), workers, deferred, observe)
		require.Len(t, events, 3)
		require.EqualValues(t, 3, stats.Messages)
		for i, e := range events {
			require.Equal(t, "knx", e.Protocol)
			require.Empty(t, e.Error)
			require.Equal(t, c.Steps[i].Source, e.Source)
			require.Equal(t, c.Steps[i].Destination, e.Destination)
			require.Zero(t, e.FlowID)
			require.Zero(t, e.TransactionID)
			require.Zero(t, e.ResponseTo)
			require.Len(t, e.SourceBytes.PacketRefs, 1)
			require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
			wire, err := hex.DecodeString(c.Steps[i].Hex)
			require.NoError(t, err)
			require.Equal(t, wire, e.Raw)
			fields, err := e.GetFields()
			require.NoError(t, err)
			rocEqualFields(t, c.ExpectedDecodedFields[i].Fields, fields)
		}
	})
}
