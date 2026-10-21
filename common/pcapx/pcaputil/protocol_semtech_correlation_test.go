package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"strings"
	"testing"
	"time"
)

// Fixed packet_forwarder d0226eae PROTOCOL.TXT 5.4/5.5: TX_ACK copies
// the PULL_RESP token and reverses the observed UDP endpoints. This is
// passive association only, not gateway authentication or proof of RF delivery.
func TestSemtechDownlinkCorrelationExistingAPI(t *testing.T) {
	request := append([]byte{2, 0x12, 0x34, 3}, []byte(`{"txpk":{"imme":true,"freq":868.1,"rfch":0,"powe":14,"modu":"LORA","datr":"SF7BW125","codr":"4/5","ipol":true,"size":3,"data":"AQID"}}`)...)
	feedback := []byte{2, 0x12, 0x34, 5, 0, 1, 2, 3, 4, 5, 6, 7}
	raw := sessionDatagramPCAP(t, []sessionStep{{1, request}, {0, feedback}}, 1700)
	var events []*ProtocolEvent
	err := ReplayPcap(bytes.NewReader(raw), WithProtocolDecodeAs("udp", 1700, "semtech-downlink-session"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }))
	require.NoError(t, err)
	require.Len(t, events, 2)
	for _, e := range events {
		f, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, "semtech-udp", e.Protocol)
		require.Equal(t, "semtech-v2-downlink-session", e.Profile)
		require.Equal(t, false, e.Session["Authentication Verified"])
		require.Equal(t, false, e.Session["RF Delivery Proven"])
		require.Equal(t, "1234", f["Token"])
	}
	require.Zero(t, events[0].ResponseTo)
	require.Equal(t, events[0].ID, events[0].TransactionID)
	require.Equal(t, events[0].ID, events[1].ResponseTo)
	require.Equal(t, events[0].ID, events[1].TransactionID)
	require.Equal(t, "observed-request", events[0].Session["Association"])
	require.Equal(t, "observed-feedback", events[1].Session["Association"])
}

func semtechCorrelationWire(kind byte, token uint16, body string) []byte {
	w := []byte{2, byte(token >> 8), byte(token), kind}
	if kind == 5 {
		w = append(w, 0, 1, 2, 3, 4, 5, 6, 7)
	}
	return append(w, []byte(body)...)
}

const semtechCorrelationTX = `{"txpk":{"imme":true,"freq":868.1,"rfch":0,"powe":14,"modu":"LORA","datr":"SF7BW125","codr":"4/5","ipol":true,"size":3,"data":"AQID"}}`

func TestSemtechCorrelationTokenLifetimeAndRefusals(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, scenario := range []string{"valid", "interleaved", "duplicate", "wrong-token", "wrong-direction", "conflict", "reuse", "expired", "backdated", "malformed", "byte-budget"} {
			t.Run(fmt.Sprintf("%s/deferred=%v", scenario, deferred), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 1700))
				require.NoError(t, err)
				cs := s.(*captureSession)
				cs.f.a.datagramDecodeAs[1700] = "semtech-downlink-session"
				cs.f.a.config.Deferred = deferred
				feed := func(dir int, sec int64, kind byte, token uint16, body string) *ProtocolEvent {
					out := s.Feed(dir, time.Unix(sec, 0), semtechCorrelationWire(kind, token, body))
					require.Len(t, out.Events, 1)
					return out.Events[0]
				}
				okay := func(e *ProtocolEvent, association string, request uint64) {
					f, err := e.GetFields()
					require.NoError(t, err)
					require.NotNil(t, f)
					require.Equal(t, "semtech-v2-downlink-session", e.Profile)
					require.Equal(t, semtechAssociation(association), e.Session)
					require.Equal(t, request, e.TransactionID)
					if association == "observed-feedback" {
						require.Equal(t, request, e.ResponseTo)
					} else {
						require.Zero(t, e.ResponseTo)
					}
				}
				reject := func(e *ProtocolEvent, kind string) {
					f, err := e.GetFields()
					rocTypedError(t, kind, err)
					require.Nil(t, f)
					require.Nil(t, e.Session)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
				}
				q := feed(1, 1, 3, 0x1234, semtechCorrelationTX)
				okay(q, "observed-request", q.ID)
				expected, feedbackAt := "", int64(3)
				switch scenario {
				case "interleaved":
					other := feed(1, 2, 3, 0x4321, semtechCorrelationTX)
					okay(other, "observed-request", other.ID)
					okay(feed(0, 3, 5, 0x4321, ""), "observed-feedback", other.ID)
					feedbackAt = 4
				case "duplicate":
					okay(feed(1, 2, 3, 0x1234, semtechCorrelationTX), "observed-duplicate-request", q.ID)
				case "wrong-token":
					reject(feed(0, 2, 5, 0x5678, ""), "ContextRequired")
				case "wrong-direction":
					reject(feed(1, 2, 5, 0x1234, ""), "ContextRequired")
				case "conflict":
					reject(feed(1, 2, 3, 0x1234, strings.Replace(semtechCorrelationTX, "868.1", "868.3", 1)), "ContextRequired")
					expected = "ContextRequired"
				case "reuse":
					okay(feed(0, 2, 5, 0x1234, ""), "observed-feedback", q.ID)
					reject(feed(1, 3, 3, 0x1234, semtechCorrelationTX), "ContextRequired")
					expected = "ContextRequired"
					feedbackAt = 4
				case "expired":
					reject(feed(1, 31, 3, 0x1234, semtechCorrelationTX), "ContextRequired")
					expected = "ContextRequired"
					feedbackAt = 32
				case "backdated":
					other := feed(1, 3, 3, 0x4321, semtechCorrelationTX)
					okay(other, "observed-request", other.ID)
					reject(feed(0, 2, 5, 0x1234, ""), "ContextRequired")
					expected = "ContextRequired"
					feedbackAt = 4
				case "malformed":
					reject(feed(0, 2, 5, 0x1234, `{"txpk_ack":{"error":1}}`), "MalformedMessage")
					expected = "ContextRequired"
				case "byte-budget":
					cs.f.a.config.MaxMessageBytes = 64
					reject(feed(0, 2, 5, 0x1234, strings.Repeat(" ", 65)), "ResourceExceeded")
					cs.f.a.config.MaxMessageBytes = DefaultParserBudget().MaxMessageBytes
					expected = "ContextRequired"
				}
				feedback := feed(0, feedbackAt, 5, 0x1234, "")
				if expected != "" {
					reject(feedback, expected)
				} else {
					okay(feedback, "observed-feedback", q.ID)
				}
				// Unrelated fresh identity remains usable after local context refusal.
				adjacent := feed(1, feedbackAt+1, 3, 0xabcd, semtechCorrelationTX)
				okay(adjacent, "observed-request", adjacent.ID)
				okay(feed(0, feedbackAt+2, 5, 0xabcd, ""), "observed-feedback", adjacent.ID)
				require.Empty(t, s.Close("correlation"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
				require.Nil(t, cs.f.a.semtechSessions)
			})
		}
	}
}

func TestSemtechCorrelationQuarantineIsolationAndOwnership(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 1700))
	require.NoError(t, err)
	cs := s.(*captureSession)
	a := cs.f.a
	a.datagramDecodeAs[1700] = "semtech-downlink-session"
	wire := semtechCorrelationWire(3, 0x1234, semtechCorrelationTX)
	q := s.Feed(1, time.Unix(1, 0), wire).Events[0]
	clear(wire)
	f, err := q.GetFields()
	require.NoError(t, err)
	f["Downlink Observation"].(map[string]any)["kind"] = "changed"
	q.Session["Association"] = "changed"
	clear(q.Raw)
	feedback := s.Feed(0, time.Unix(2, 0), semtechCorrelationWire(5, 0x1234, "")).Events[0]
	require.Equal(t, q.ID, feedback.ResponseTo)
	require.Equal(t, q.ID, feedback.TransactionID)
	// An unrelated STUN cleanup beyond ten minutes must not delete token history.
	a.decodeSTUNDatagram(&ProtocolEvent{Timestamp: time.Unix(700, 0), Source: "192.0.2.1:41000", Destination: "192.0.2.2:3478"}, stunTestMessage(1, 0, 1))
	reuse := s.Feed(1, time.Unix(701, 0), semtechCorrelationWire(3, 0x1234, semtechCorrelationTX)).Events[0]
	_, err = reuse.GetFields()
	rocTypedError(t, "ContextRequired", err)
	require.Zero(t, reuse.ResponseTo)
	require.Zero(t, reuse.TransactionID)
	// Exact bytes on another interface/tunnel and tuple have independent state.
	for i, d := range []CaptureDomain{{Interface: 1}, {Interface: 0, Encapsulation: "vlan:7"}} {
		e := &ProtocolEvent{Timestamp: time.Unix(702+int64(i), 0), Source: "192.0.2.2:1700", Destination: "192.0.2.1:40000", Domain: d}
		require.True(t, a.decodeSemtechCorrelatedDatagram(e, semtechCorrelationWire(3, 0x1234, semtechCorrelationTX)))
		_, err = e.GetFields()
		require.NoError(t, err)
		require.Equal(t, e.ID, e.TransactionID)
		r := &ProtocolEvent{Timestamp: e.Timestamp, Source: e.Destination, Destination: e.Source, Domain: d}
		a.decodeSemtechCorrelatedDatagram(r, semtechCorrelationWire(5, 0x1234, ""))
		_, err = r.GetFields()
		require.NoError(t, err)
		require.Equal(t, e.ID, r.ResponseTo)
	}
	require.Empty(t, s.Close("ownership"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestSemtechCorrelationHistoryBoundFailsClosed(t *testing.T) {
	t.Run("global-budget", testSemtechCorrelationGlobalBudgetRefusal)
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 1700))
	require.NoError(t, err)
	cs := s.(*captureSession)
	cs.f.a.datagramDecodeAs[1700] = "semtech-downlink-session"
	for i := 0; i < 129; i++ {
		out := s.Feed(1, time.Unix(int64(i+1), 0), semtechCorrelationWire(3, uint16(i), semtechCorrelationTX))
		require.Len(t, out.Events, 1)
		_, err = out.Events[0].GetFields()
		if i < 128 {
			require.NoError(t, err)
		} else {
			rocTypedError(t, "ResourceExceeded", err)
		}
	}
	out := s.Feed(0, time.Unix(130, 0), semtechCorrelationWire(5, 127, ""))
	require.Len(t, out.Events, 1)
	_, err = out.Events[0].GetFields()
	rocTypedError(t, "ResourceExceeded", err)
	require.Zero(t, out.Events[0].ResponseTo)
	require.Zero(t, out.Events[0].TransactionID)
	require.Len(t, cs.f.a.semtechSessions.entries, 1)
	for _, el := range cs.f.a.semtechSessions.entries {
		require.Len(t, el.Value.(*binUDPEntry).flow.semtech.seen, 128)
	}
	require.Empty(t, s.Close("bound"))
	require.Zero(t, s.Stats().BufferedBytes)
}

type semtechCorrelationCase struct {
	Name, File, SHA256, InputAlias, Answer, AnswerSHA256 string
	MaxMessageBytes                                      int `json:"max_message_bytes"`
	Events                                               []struct {
		Position             int
		Src, Dst, Wire, Code string
		At                   int64
		Dir                  int
		Fields, Session      map[string]any
		RequestPosition      int `json:"request_position"`
		ResponsePosition     int `json:"response_position"`
	}
}

func TestSemtechCorrelationSealedMatrix(t *testing.T) {
	raw, err := trafficfixture.ReadFile("semtech-correlation/controls.json")
	require.NoError(t, err)
	var doc struct{ Cases []semtechCorrelationCase }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Cases, 13)
	// Use explicit wire/answer binding from the immutable corpus inventory.
	var binding struct {
		Cases []struct {
			Name         string
			InputAlias   string `json:"input_alias"`
			Answer       string
			AnswerSHA256 string `json:"answer_sha256"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &binding))
	for i, c := range doc.Cases {
		t.Run(c.Name, func(t *testing.T) {
			input, err := trafficfixture.ReadFile("semtech-correlation/" + c.File)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(input)))
			answer, err := trafficfixture.ReadFile("semtech-correlation/" + binding.Cases[i].Answer)
			require.NoError(t, err)
			require.Equal(t, binding.Cases[i].AnswerSHA256, fmt.Sprintf("%x", sha256.Sum256(answer)))
			var owned semtechCorrelationCase
			require.NoError(t, json.Unmarshal(answer, &owned))
			require.Equal(t, c.Events, owned.Events)
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				es, stats := discoveryReplay(t, input, workers, deferred, observe, WithProtocolDecodeAs("udp", 1700, "semtech-downlink-session"), WithProtocolBudget(c.MaxMessageBytes, DefaultParserBudget().MaxBufferedBytes))
				require.Len(t, es, len(c.Events))
				require.Zero(t, stats.BufferedBytes)
				for n, want := range c.Events {
					e := es[n]
					require.Equal(t, "semtech-udp", e.Protocol)
					require.Equal(t, semtechCorrelationProfile, e.Profile)
					require.Equal(t, "explicit-decode-as", e.Admission)
					require.Equal(t, "udp", e.Transport)
					require.Equal(t, want.Src, e.Source)
					require.Equal(t, want.Dst, e.Destination)
					require.Equal(t, 1-want.Dir, e.Direction)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.EqualValues(t, want.Position, e.SourceBytes.PacketRefs[0].Number)
					require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
					require.Equal(t, time.Unix(1700000000+want.At, 0), e.Timestamp)
					f, err := e.GetFields()
					if want.Code != "" {
						rocTypedError(t, want.Code, err)
						require.Nil(t, f)
						require.Nil(t, e.Session)
						require.Zero(t, e.TransactionID)
						require.Zero(t, e.ResponseTo)
						if want.Code == "ResourceExceeded" {
							require.Equal(t, "limited", e.Status)
						} else if want.Code == "ContextRequired" {
							require.Equal(t, "context-required", e.Status)
						} else {
							require.Equal(t, "malformed", e.Status)
						}
					} else {
						require.NoError(t, err)
						rocEqualFields(t, want.Fields, f)
						rocEqualFields(t, want.Session, e.Session)
						require.Equal(t, es[want.RequestPosition-1].ID, e.TransactionID)
						if want.ResponsePosition == 0 {
							require.Zero(t, e.ResponseTo)
						} else {
							require.Equal(t, es[want.ResponsePosition-1].ID, e.ResponseTo)
						}
						if deferred {
							require.Equal(t, "deferred", e.Status)
						} else {
							require.Equal(t, "decoded", e.Status)
						}
						f["Downlink Observation"].(map[string]any)["kind"] = "caller mutation"
						again, err := e.GetFields()
						require.NoError(t, err)
						rocEqualFields(t, want.Fields, again)
					}
					if len(doipDiscoveryWire(t, want.Wire)) > c.MaxMessageBytes {
						require.Empty(t, e.Raw)
					} else {
						require.Equal(t, doipDiscoveryWire(t, want.Wire), e.Raw)
					}
					if n > 0 {
						require.Greater(t, e.ID, es[n-1].ID)
					}
				}
			})
		})
	}
}

func testSemtechCorrelationGlobalBudgetRefusal(t *testing.T) {
	for _, bytes := range []int{100, 512} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 1700))
		require.NoError(t, err)
		cs := s.(*captureSession)
		a := cs.f.a
		a.datagramDecodeAs[1700] = "semtech-downlink-session"
		a.config.MaxBufferedBytes = bytes
		rejected := s.Feed(1, time.Unix(1, 0), semtechCorrelationWire(3, 0x1234, semtechCorrelationTX))
		require.Len(t, rejected.Events, 1)
		e := rejected.Events[0]
		_, err = e.GetFields()
		rocTypedError(t, "ResourceExceeded", err)
		require.Equal(t, "limited", e.Status)
		require.Empty(t, e.Raw)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
		a.config.MaxBufferedBytes = DefaultParserBudget().MaxBufferedBytes
		for _, kind := range []byte{5, 3} {
			late := s.Feed(func() int {
				if kind == 3 {
					return 1
				}
				return 0
			}(), time.Unix(2, 0), semtechCorrelationWire(kind, 0x1234, func() string {
				if kind == 3 {
					return semtechCorrelationTX
				}
				return ""
			}()))
			require.Len(t, late.Events, 1)
			_, err = late.Events[0].GetFields()
			rocTypedError(t, "ResourceExceeded", err)
			require.Zero(t, late.Events[0].ResponseTo)
			require.Zero(t, late.Events[0].TransactionID)
		}
		require.Empty(t, s.Close("global-refusal"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
}
