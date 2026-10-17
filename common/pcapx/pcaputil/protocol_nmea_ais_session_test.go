package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"strings"
	"sync"
	"testing"
	"time"
)

func aisSessionFixture(t *testing.T) ([][]byte, map[string]any) {
	b, err := trafficfixture.ReadFile("nmea-ais-type5/answers/voyage-reported.json")
	require.NoError(t, err)
	var a nmeaNavigationAnswer
	require.NoError(t, json.Unmarshal(b, &a))
	require.Len(t, a.Wire, 1)
	w, err := hex.DecodeString(a.Wire[0])
	require.NoError(t, err)
	cut := bytes.IndexByte(w, '\n') + 1
	require.Positive(t, cut)
	want := cloneSession(a.Fields[0])
	want["Assembly Scope"] = "selected-directed-UDP-observation"
	want["Sequence Reuse Policy"] = "quarantined for capture lifetime"
	want["Association Verified"] = false
	return [][]byte{w[:cut], w[cut:]}, want
}
func aisSessionSentence(w []byte, seq, channel string) []byte {
	star := bytes.LastIndexByte(w, '*')
	p := strings.Split(string(w[1:star]), ",")
	p[3], p[4] = seq, channel
	body := strings.Join(p, ",")
	var sum byte
	for _, c := range []byte(body) {
		sum ^= c
	}
	return []byte(fmt.Sprintf("!%s*%02X\r\n", body, sum))
}
func aisTestSession(t *testing.T, budget ParserBudget, deferred bool) ProtocolSession {
	s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"), WithSessionPorts(38000, 10110))
	require.NoError(t, err)
	a := s.(*captureSession).f.a
	a.datagramDecodeAs[10110] = "nmea-ais-multipart"
	a.config.Deferred = deferred
	return s
}
func aisSessionReject(t *testing.T, e *ProtocolEvent, kind string) {
	f, err := e.GetFields()
	require.Nil(t, f)
	rocTypedError(t, kind, err)
	require.Nil(t, e.Session)
	require.Zero(t, e.ResponseTo)
	require.Zero(t, e.TransactionID)
	assertOwnedProtocolRefusal(t, e, kind, err)
}
func aisSessionComplete(t *testing.T, e *ProtocolEvent, want map[string]any, ids []uint64, wire []byte) {
	fields := cloneSession(want)
	fields["Fragment Event IDs"] = ids
	require.Equal(t, aisMultipartProfile, e.Profile)
	require.Equal(t, "message", e.Completeness)
	require.Equal(t, wire, e.Raw)
	require.Zero(t, e.ResponseTo)
	require.Zero(t, e.TransactionID)
	equal := func(got any) {
		a, err := json.Marshal(fields)
		require.NoError(t, err)
		b, err := json.Marshal(got)
		require.NoError(t, err)
		require.JSONEq(t, string(a), string(b))
	}
	equal(e.Session)
	saved := cloneSession(e.Session)
	defer func() { e.Session = saved }()
	query := func() {
		f, err := e.GetFields()
		require.NoError(t, err)
		equal(f)
		x, err := e.Decode()
		require.NoError(t, err)
		equal(x["fields"])
		equal(x["session"])
		mutateRTPSQuery(f)
		mutateRTPSQuery(x)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
	}
	query()
	mutateRTPSQuery(e.Session)
	query()
	e.Session = nil
	query()
	e.Fields = map[string]any{"caller": true}
	e.Structured = map[string]any{"caller": true}
	clear(e.Raw)
	query()
	e.Raw = bytes.Clone(wire)
}
func TestNMEAAISMultipartLifetime(t *testing.T) {
	w, want := aisSessionFixture(t)
	for _, deferred := range []bool{false, true} {
		for _, scenario := range []string{"valid", "retry", "retry-deadline", "wrong-sequence", "wrong-channel", "wrong-direction", "orphan", "reordered", "conflict", "reuse", "expiry", "backdated", "checksum", "cut", "oversize", "assembled-bytes", "projection-memory"} {
			t.Run(fmt.Sprintf("%s/deferred=%v", scenario, deferred), func(t *testing.T) {
				s := aisTestSession(t, ParserBudget{}, deferred)
				a := s.(*captureSession).f.a
				feed := func(dir int, sec int64, wire []byte) *ProtocolEvent {
					raw := bytes.Clone(wire)
					o := s.Feed(dir, time.Unix(sec, 0), raw)
					clear(raw)
					require.Equal(t, len(wire), o.Consumed)
					require.Len(t, o.Events, 1)
					return o.Events[0]
				}
				q := feed(0, 1, w[0])
				aisSessionReject(t, q, "NeedMore")
				require.Positive(t, s.Stats().BufferedBytes)
				sec := int64(3)
				expected := ""
				switch scenario {
				case "retry":
					aisSessionReject(t, feed(0, 2, w[0]), "NeedMore")
				case "retry-deadline":
					aisSessionReject(t, feed(0, 30, w[0]), "NeedMore")
					sec = 31
					expected = "ContextRequired"
				case "wrong-sequence":
					aisSessionReject(t, feed(0, 2, aisSessionSentence(w[1], "9", "A")), "ContextRequired")
				case "wrong-channel":
					aisSessionReject(t, feed(0, 2, aisSessionSentence(w[1], "7", "B")), "ContextRequired")
				case "wrong-direction":
					aisSessionReject(t, feed(1, 2, w[1]), "ContextRequired")
				case "orphan", "reordered":
					require.Len(t, s.Close("reset"), 1)
					s = aisTestSession(t, ParserBudget{}, deferred)
					a = s.(*captureSession).f.a
					aisSessionReject(t, feed(0, 1, w[1]), "ContextRequired")
					aisSessionReject(t, feed(0, 2, w[0]), "ContextRequired")
					expected = "ContextRequired"
				case "conflict":
					other := bytes.Clone(w[0])
					other[25] ^= 1
					star := bytes.LastIndexByte(other, '*')
					var sum byte
					for _, c := range other[1:star] {
						sum ^= c
					}
					copy(other[star+1:], fmt.Sprintf("%02X", sum))
					aisSessionReject(t, feed(0, 2, other), "ContextRequired")
					expected = "ContextRequired"
				case "reuse":
					e := feed(0, 2, w[1])
					aisSessionComplete(t, e, want, []uint64{q.ID, e.ID}, bytes.Join(w, nil))
					aisSessionReject(t, feed(0, 3, w[0]), "ContextRequired")
					sec = 4
					expected = "ContextRequired"
				case "expiry":
					sec = 31
					expected = "ContextRequired"
				case "backdated":
					aisSessionReject(t, feed(0, 3, aisSessionSentence(w[0], "9", "A")), "NeedMore")
					aisSessionReject(t, feed(0, 2, w[1]), "ContextRequired")
					sec = 4
					expected = "ContextRequired"
				case "checksum":
					bad := bytes.Clone(w[1])
					bad[len(bad)-4] = 'F'
					if bytes.Equal(bad, w[1]) {
						bad[len(bad)-4] = 'E'
					}
					aisSessionReject(t, feed(0, 2, bad), "MalformedMessage")
					expected = "ContextRequired"
				case "cut":
					aisSessionReject(t, feed(0, 2, w[1][:len(w[1])-1]), "MalformedMessage")
					expected = "ContextRequired"
				case "oversize":
					a.config.MaxMessageBytes = 20
					aisSessionReject(t, feed(0, 2, w[1]), "ResourceExceeded")
					a.config.MaxMessageBytes = 1 << 20
					expected = "ContextRequired"
				case "assembled-bytes":
					a.config.MaxMessageBytes = len(bytes.Join(w, nil)) - 1
					aisSessionReject(t, feed(0, 2, w[1]), "ResourceExceeded")
					a.config.MaxMessageBytes = 1 << 20
					expected = "ContextRequired"
				case "projection-memory":
					a.config.MaxBufferedBytes = int(a.buffered.Load()) + 100
					aisSessionReject(t, feed(0, 2, w[1]), "ResourceExceeded")
					a.config.MaxBufferedBytes = 32 << 20
					expected = "ContextRequired"
				}
				e := feed(0, sec, w[1])
				if expected != "" {
					aisSessionReject(t, e, expected)
				} else {
					aisSessionComplete(t, e, want, []uint64{q.ID, e.ID}, bytes.Join(w, nil))
				}
				s.Close("done")
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
				require.Nil(t, a.aisMultipart)
			})
		}
	}
}
func TestNMEAAISMultipartCloseBudgetAndOwnership(t *testing.T) {
	w, want := aisSessionFixture(t)
	for _, b := range []ParserBudget{{MaxCollectionElements: 1}, {MaxRecursionDepth: 1}, {MaxMessageBytes: 64, MaxBufferedBytes: 100}} {
		s := aisTestSession(t, b, false)
		o := s.Feed(0, time.Unix(1, 0), w[0])
		require.Len(t, o.Events, 1)
		aisSessionReject(t, o.Events[0], "ResourceExceeded")
		s.Close("budget")
		require.Zero(t, s.Stats().BufferedBytes)
	}
	s := aisTestSession(t, ParserBudget{}, false)
	o := s.Feed(0, time.Unix(1, 0), w[0])
	q := o.Events[0]
	clear(q.Raw)
	q.SourceBytes.PacketRefs = nil
	e := s.Feed(0, time.Unix(2, 0), w[1]).Events[0]
	aisSessionComplete(t, e, want, []uint64{q.ID, e.ID}, bytes.Join(w, nil))
	require.Equal(t, uint64(len(bytes.Join(w, nil))), s.Stats().MessageBytes)
	s.Close("done")
	require.Zero(t, s.Stats().BufferedBytes)
	s = aisTestSession(t, ParserBudget{}, true)
	s.Feed(0, time.Unix(1, 0), w[0])
	closed := s.Close("missing")
	require.Len(t, closed, 1)
	aisSessionReject(t, closed[0], "NeedMore")
	require.Equal(t, w[0], closed[0].Raw)
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestNMEAAISMultipartDomainAndContributorIsolation(t *testing.T) {
	w, want := aisSessionFixture(t)
	for _, change := range []string{"interface", "section", "tunnel", "source", "destination", "direction"} {
		t.Run(change, func(t *testing.T) {
			s := aisTestSession(t, ParserBudget{}, false)
			a := s.(*captureSession).f.a
			domain := CaptureDomain{Section: 1, Interface: 2, Encapsulation: "/vlan:3"}
			makeEvent := func(n uint64, sec int64) *ProtocolEvent {
				return &ProtocolEvent{Timestamp: time.Unix(sec, 0), Transport: "udp", Source: "192.0.2.10:38000", Destination: "192.0.2.20:10110", Domain: domain, SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: n, Domain: domain}}}}
			}
			q := makeEvent(1, 1)
			a.decodeAISMultipartDatagram(q, w[0], nil)
			aisSessionReject(t, q, "NeedMore")
			rejected := makeEvent(2, 100)
			switch change {
			case "interface":
				rejected.Domain.Interface++
			case "section":
				rejected.Domain.Section++
			case "tunnel":
				rejected.Domain.Encapsulation = "/vlan:4"
			case "source":
				rejected.Source = "192.0.2.11:38000"
			case "destination":
				rejected.Destination = "192.0.2.21:10110"
			case "direction":
				rejected.Source, rejected.Destination = rejected.Destination, rejected.Source
			}
			rejected.SourceBytes.PacketRefs[0].Domain = rejected.Domain
			a.decodeAISMultipartDatagram(rejected, w[1], nil)
			aisSessionReject(t, rejected, "ContextRequired")
			// An unrelated domain clock cannot expire or backdate this context.
			e := makeEvent(3, 2)
			a.decodeAISMultipartDatagram(e, w[1], nil)
			aisSessionComplete(t, e, want, []uint64{q.ID, e.ID}, bytes.Join(w, nil))
			require.Equal(t, []PacketReference{{Number: 1, Domain: domain}, {Number: 3, Domain: domain}}, e.SourceBytes.PacketRefs)
			require.Equal(t, []uint64{q.ID, e.ID}, e.SourceBytes.ParentPDUs)
			require.Equal(t, q.FlowID, e.FlowID)
			require.NotEqual(t, q.FlowID, rejected.FlowID)
			e.SourceBytes.PacketRefs[0].Number = 999
			s.Close("done")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	// Public first-event changes cannot mutate the retained physical evidence.
	s := aisTestSession(t, ParserBudget{}, false)
	a := s.(*captureSession).f.a
	q := &ProtocolEvent{Transport: "udp", Source: "a", Destination: "b", Timestamp: time.Unix(1, 0), SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: 1}}}}
	a.decodeAISMultipartDatagram(q, w[0], nil)
	q.SourceBytes.PacketRefs[0].Number = 999
	clear(q.Raw)
	e := &ProtocolEvent{Transport: "udp", Source: "a", Destination: "b", Timestamp: time.Unix(2, 0), SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: 2}}}}
	a.decodeAISMultipartDatagram(e, w[1], nil)
	aisSessionComplete(t, e, want, []uint64{q.ID, e.ID}, bytes.Join(w, nil))
	require.Equal(t, []PacketReference{{Number: 1}, {Number: 2}}, e.SourceBytes.PacketRefs)
	s.Close("done")
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestNMEAAISMultipartRefusalAndHistoryBoundaries(t *testing.T) {
	w, _ := aisSessionFixture(t)
	for _, scenario := range []string{"carrier-cut", "identity-count", "contributors", "unlabelled", "missing-channel", "truncated-sentence", "two-in-datagram"} {
		t.Run(scenario, func(t *testing.T) {
			s := aisTestSession(t, ParserBudget{}, false)
			a := s.(*captureSession).f.a
			q := s.Feed(0, time.Unix(1, 0), w[0]).Events[0]
			aisSessionReject(t, q, "NeedMore")
			switch scenario {
			case "carrier-cut":
				e := &ProtocolEvent{Timestamp: time.Unix(2, 0), Transport: "udp", Source: q.Source, Destination: q.Destination}
				a.decodeAISMultipartDatagram(e, w[1], protocolError(ErrNeedMore, "carrier cut"))
				aisSessionReject(t, e, "NeedMore")
			case "identity-count":
				a.budget.MaxCollectionElements = 2
				aisSessionReject(t, s.Feed(0, time.Unix(2, 0), aisSessionSentence(w[0], "8", "A")).Events[0], "NeedMore")
				aisSessionReject(t, s.Feed(0, time.Unix(3, 0), aisSessionSentence(w[0], "9", "A")).Events[0], "ResourceExceeded")
				a.budget.MaxCollectionElements = 4096
			case "contributors":
				e := &ProtocolEvent{Timestamp: time.Unix(2, 0), Transport: "udp", Source: q.Source, Destination: q.Destination, SourceBytes: ByteSource{PacketRefs: make([]PacketReference, 33)}}
				a.decodeAISMultipartDatagram(e, w[1], nil)
				aisSessionReject(t, e, "ResourceExceeded")
			case "unlabelled":
				aisSessionReject(t, s.Feed(0, time.Unix(2, 0), aisSessionSentence(w[1], "", "A")).Events[0], "ContextRequired")
			case "missing-channel":
				aisSessionReject(t, s.Feed(0, time.Unix(2, 0), aisSessionSentence(w[1], "7", "")).Events[0], "ContextRequired")
			case "truncated-sentence":
				aisSessionReject(t, s.Feed(0, time.Unix(2, 0), w[1][:len(w[1])-1]).Events[0], "MalformedMessage")
			case "two-in-datagram":
				aisSessionReject(t, s.Feed(0, time.Unix(2, 0), bytes.Join(w, nil)).Events[0], "MalformedMessage")
			}
			aisSessionReject(t, s.Feed(0, time.Unix(4, 0), w[1]).Events[0], "ContextRequired")
			s.Close("done")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	for n := 0; n < len(w[0]); n++ {
		s := aisTestSession(t, ParserBudget{}, false)
		o := s.Feed(0, time.Unix(1, 0), w[0][:n])
		require.Len(t, o.Events, 1)
		aisSessionReject(t, o.Events[0], "MalformedMessage")
		aisSessionReject(t, s.Feed(0, time.Unix(2, 0), w[1]).Events[0], "ContextRequired")
		s.Close("cut")
		require.Zero(t, s.Stats().BufferedBytes)
	}
}

func TestNMEAAISMultipartSealedMatrix(t *testing.T) {
	b, err := trafficfixture.ReadFile("nmea-ais-session/controls.json")
	require.NoError(t, err)
	var controls struct {
		Schema string
		Cases  []struct {
			ID, Capture, Answer, SHA256 string
			AnswerSHA                   string `json:"answer_sha256"`
			Packets                     int
		}
	}
	require.NoError(t, json.Unmarshal(b, &controls))
	require.Equal(t, "nmea-ais-type5-udp-session/v1", controls.Schema)
	require.Len(t, controls.Cases, 24)
	// Deduplicated physical inputs retain every explicit configuration answer.
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	facts := map[string]trafficfixture.FrozenCase{}
	for _, batch := range all {
		for _, c := range batch.Cases {
			if strings.HasPrefix(c.ID, "nmea-ais-session/") {
				facts[c.Input.File] = c
			}
		}
	}
	require.Len(t, facts, 23)
	for _, c := range controls.Cases {
		t.Run(c.ID, func(t *testing.T) {
			fact, ok := facts[c.Capture]
			require.True(t, ok)
			require.Equal(t, c.SHA256, fact.Input.SHA256)
			require.Equal(t, c.Packets, fact.Facts.PacketCount)
			bound := 0
			for _, x := range fact.Expectations {
				var b struct {
					Answer string `json:"answer_file"`
					SHA    string `json:"answer_sha256"`
				}
				require.NoError(t, json.Unmarshal(x.PayloadConstraints, &b))
				if b.Answer == c.Answer {
					require.Equal(t, c.AnswerSHA, b.SHA)
					bound++
				}
			}
			require.Equal(t, 1, bound, "each configuration oracle has one binding")
			raw, err := trafficfixture.ReadFile("nmea-ais-session/" + c.Capture)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			b, err := trafficfixture.ReadFile("nmea-ais-session/" + c.Answer)
			require.NoError(t, err)
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(b)))
			var answer struct {
				CaptureError string `json:"capture_error"`
				Events       []struct {
					FlowObserved  bool   `json:"flow_observed"`
					Kind          string `json:"error_kind"`
					ID            uint64
					Raw           string `json:"raw_hex"`
					Refs, Parents []uint64
					Fields        map[string]any
					Direction     int
					Length        int `json:"physical_length"`
				}
				Budget  *struct{ Message, Buffered int }
				Packets int    `json:"packet_count"`
				Input   uint64 `json:"input_bytes"`
			}
			require.NoError(t, json.Unmarshal(b, &answer))
			require.Equal(t, c.Packets, answer.Packets)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observer := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observer), func(t *testing.T) {
							var mu sync.Mutex
							var events []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithProtocolDecodeAs("udp", 10110, "nmea-ais-multipart"), WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); events = append(events, e); mu.Unlock() }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if answer.Budget != nil {
								opts = append(opts, WithProtocolBudget(answer.Budget.Message, answer.Budget.Buffered))
							}
							if observer {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							owned := bytes.Clone(raw)
							captureErr := ReplayPcap(bytes.NewReader(owned), opts...)
							if answer.CaptureError != "" {
								require.ErrorContains(t, captureErr, answer.CaptureError)
							} else {
								require.NoError(t, captureErr)
							}
							clear(owned)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							require.Equal(t, answer.Input, stats.InputBytes)
							require.Equal(t, answer.Input, stats.MessageBytes)
							require.EqualValues(t, c.Packets, stats.Messages)
							if observer {
								require.Equal(t, c.Packets, seen)
							}
							require.Len(t, events, len(answer.Events))
							for i, e := range events {
								x := answer.Events[i]
								require.Equal(t, x.ID, e.ID)
								require.Equal(t, x.Direction, e.Direction)
								require.Equal(t, x.Length, e.Length)
								require.Equal(t, "nmea", e.Protocol)
								require.Equal(t, aisMultipartProfile, e.Profile)
								if x.FlowObserved {
									require.NotZero(t, e.FlowID)
								} else {
									require.Zero(t, e.FlowID)
								}
								require.Zero(t, e.ResponseTo)
								require.Zero(t, e.TransactionID)
								require.Equal(t, x.Raw, hex.EncodeToString(e.Raw))
								require.Equal(t, x.Parents, e.SourceBytes.ParentPDUs)
								refs := make([]uint64, 0, len(e.SourceBytes.PacketRefs))
								for _, r := range e.SourceBytes.PacketRefs {
									require.Equal(t, e.Domain, r.Domain)
									refs = append(refs, r.Number)
								}
								require.Equal(t, x.Refs, refs)
								if x.Direction == 0 {
									require.Equal(t, "192.0.2.10:38000", e.Source)
									require.Equal(t, "192.0.2.20:10110", e.Destination)
								} else {
									require.Equal(t, "192.0.2.20:10110", e.Source)
									require.Equal(t, "192.0.2.10:38000", e.Destination)
								}
								if x.Kind != "" {
									aisSessionReject(t, e, x.Kind)
									require.Equal(t, x.Kind, e.ExpertCode)
								} else {
									wire, err := hex.DecodeString(x.Raw)
									require.NoError(t, err)
									aisSessionComplete(t, e, x.Fields, x.Parents, wire)
								}
							}
						})
					}
				}
			}
		})
	}
}
