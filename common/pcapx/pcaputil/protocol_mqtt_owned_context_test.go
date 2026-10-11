package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func mqttContextWire(t testing.TB, name string) []byte {
	t.Helper()
	wire, err := trafficfixture.ReadFile("testdata/mqtt-owned-context/records/" + name + ".bin")
	require.NoError(t, err)
	return wire
}

func testMQTT5SealedContextMatrix(t *testing.T) {
	var answers struct {
		Profiles map[string]struct {
			MaxElements int `json:"max_elements"`
			Events      []struct {
				Record    string         `json:"record"`
				Direction int            `json:"direction"`
				Session   map[string]any `json:"session"`
				Error     *struct {
					Kind    ProtocolErrorKind `json:"kind"`
					Message string            `json:"message"`
				} `json:"error"`
			} `json:"events"`
		} `json:"profiles"`
	}
	raw, err := trafficfixture.ReadFile("testdata/mqtt-owned-context/answers.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &answers))
	require.Len(t, answers.Profiles, 5)
	for _, name := range []string{"owned", "namespaces", "negative-pubrec", "pending-limit", "alias-limit"} {
		profile := answers.Profiles[name]
		pcap, err := trafficfixture.ReadFile("testdata/mqtt-owned-context/captures/" + name + ".pcap")
		require.NoError(t, err)
		for _, workers := range []int{1, 2, 4} {
			for _, deferred := range []bool{false, true} {
				for _, observer := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/w%d/d%v/o%v", name, workers, deferred, observer), func(t *testing.T) {
						var events []*ProtocolEvent
						var stats ProtocolStats
						var seen atomic.Uint64
						budget := DefaultParserBudget()
						budget.MaxCollectionElements = profile.MaxElements
						opts := []CaptureOption{WithProtocolDeferred(deferred), WithTCPReassemblyWorkers(workers), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
						if observer {
							opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
						}
						analyzer, err := NewPacketAnalyzer(opts...)
						require.NoError(t, err)
						defer analyzer.Close()
						// Capture options expose byte ceilings; set the existing test
						// collection ceiling before any packet reaches workers.
						analyzer.conf.binParser.budget.MaxCollectionElements = budget.MaxCollectionElements
						reader, err := NewCaptureReader(bytes.NewReader(pcap))
						require.NoError(t, err)
						for {
							packet, ci, err := reader.ReadPacketData()
							if err == io.EOF {
								break
							}
							require.NoError(t, err)
							require.NoError(t, analyzer.Feed(packet, ci, reader.LinkType()))
						}
						require.NoError(t, analyzer.Close())
						require.Len(t, events, len(profile.Events), "entire message set including refusals")
						if observer {
							require.Positive(t, seen.Load())
						} else {
							require.Zero(t, seen.Load())
						}
						for i, e := range events {
							want := profile.Events[i]
							wire := mqttContextWire(t, want.Record)
							require.Equal(t, "mqtt", e.Protocol)
							require.Equal(t, "tcp", e.Transport)
							require.Equal(t, want.Direction, e.Direction)
							require.Equal(t, wire, e.Raw)
							require.Equal(t, len(wire), e.Length)
							require.Zero(t, e.ResponseTo)
							require.Zero(t, e.TransactionID)
							require.NotEmpty(t, e.SourceBytes.PacketRefs)
							hash := sha256.Sum256(wire)
							require.Equal(t, hex.EncodeToString(hash[:]), e.SourceBytes.SHA256)
							if want.Error != nil {
								require.Nil(t, e.Session)
								e.Fields = map[string]any{"Matched Request": true}
								e.Structured = map[string]any{"fields": e.Fields}
								fields, err := e.GetFields()
								require.Nil(t, fields)
								var typed *ProtocolError
								require.True(t, errors.As(err, &typed))
								require.Equal(t, want.Error.Kind, typed.Kind)
								require.Equal(t, want.Error.Message, typed.Message)
							} else {
								require.Empty(t, e.Error)
								rocEqualFields(t, want.Session, e.Session)
								tree, err := e.Decode()
								require.NoError(t, err)
								rocEqualFields(t, want.Session, tree["session"].(map[string]any))
								if name == "owned" && want.Record == "owned-publish" {
									testLegacyRuleQueryOwnership(t, e)
								}
							}
						}
						// Refusal clears pending context; normal exchanges complete.
						require.Zero(t, stats.BufferedBytes)
					})
				}
			}
		}
	}
}

// Default limits accept all three identifiers. Closing with no acknowledgments
// must expose the outstanding set, unlike the limit=2 fatal refusal profile.
func testMQTT5DefaultBudgetCloseInvariant(t *testing.T) {
	pcap, err := trafficfixture.ReadFile("testdata/mqtt-owned-context/captures/pending-limit.pcap")
	require.NoError(t, err)
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			for _, observer := range []bool{false, true} {
				t.Run(fmt.Sprintf("w%d/d%v/o%v", workers, deferred, observer), func(t *testing.T) {
					var events []*ProtocolEvent
					var stats ProtocolStats
					var seen atomic.Uint64
					opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
					if observer {
						opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
					}
					require.NoError(t, ReplayPcap(bytes.NewReader(pcap), opts...))
					require.Len(t, events, 6)
					for i, name := range []string{"connect", "connack0", "pending1", "pending2", "pending3"} {
						e := events[i]
						require.Equal(t, "mqtt", e.Protocol)
						require.Equal(t, mqttContextWire(t, name), e.Raw)
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
						_, err := e.Decode()
						require.NoError(t, err)
						if i >= 2 {
							require.Equal(t, 0, e.Direction)
							want := map[string]any{"Packet Type": float64(3), "Packet Name": "PUBLISH", "QoS": float64(1), "DUP": false, "Context Level": "observed", "Topic Name": "t", "Packet Identifier": float64(i - 1), "Outstanding": "PUBACK"}
							rocEqualFields(t, want, e.Session)
						}
					}
					last := events[5]
					require.Equal(t, "mqtt", last.Protocol)
					require.Equal(t, "incomplete", last.Status)
					require.Contains(t, last.Summary, "MQTT exchange ended with unmatched packet identifiers")
					require.Empty(t, last.Error, "Close diagnostics preserve the existing incomplete-event API")
					require.Equal(t, 0, last.Direction)
					require.Empty(t, last.Raw)
					require.Zero(t, last.Length)
					require.Zero(t, last.ResponseTo)
					require.Zero(t, last.TransactionID)
					rocEqualFields(t, map[string]any{"Outstanding": float64(3)}, last.Session)
					require.Zero(t, stats.BufferedBytes)
					if observer {
						require.Positive(t, seen.Load())
					} else {
						require.Zero(t, seen.Load())
					}
				})
			}
		}
	}
}

func testMQTT5SealedSessionChunks(t *testing.T) {
	var answers struct {
		Profiles map[string]struct {
			MaxElements int `json:"max_elements"`
			Events      []struct {
				Record    string         `json:"record"`
				Direction int            `json:"direction"`
				Session   map[string]any `json:"session"`
				Error     *struct {
					Kind    ProtocolErrorKind `json:"kind"`
					Message string            `json:"message"`
				} `json:"error"`
			} `json:"events"`
		} `json:"profiles"`
	}
	raw, err := trafficfixture.ReadFile("testdata/mqtt-owned-context/answers.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &answers))
	for _, name := range []string{"owned", "namespaces", "negative-pubrec", "pending-limit", "alias-limit"} {
		p := answers.Profiles[name]
		for _, chunk := range []int{0, 1, 7} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/chunk%d/d%v", name, chunk, deferred), func(t *testing.T) {
					budget := DefaultParserBudget()
					budget.MaxCollectionElements = p.MaxElements
					s, err := NewProtocolSession(budget)
					require.NoError(t, err)
					defer s.Close("done")
					s.(*captureSession).f.a.config.Deferred = deferred
					offsets := [2]uint64{}
					for i, want := range p.Events {
						wire := mqttContextWire(t, want.Record)
						var events []*ProtocolEvent
						var last *ProtocolError
						for at := 0; at < len(wire); {
							n := len(wire) - at
							if chunk != 0 {
								n = min(n, chunk)
							}
							r := s.Feed(want.Direction, time.Unix(10, int64(i)*int64(time.Millisecond)), wire[at:at+n])
							if r.Err != nil && r.Err.Kind != ErrNeedMore {
								last = r.Err
							}
							events = append(events, r.Events...)
							at += n
						}
						require.Len(t, events, 1)
						e := events[0]
						require.Equal(t, wire, e.Raw)
						require.Equal(t, uint64(i+1), e.ID)
						require.Equal(t, uint64(1), e.FlowID)
						require.Equal(t, offsets[want.Direction], e.Offset)
						offsets[want.Direction] += uint64(len(wire))
						clear(wire)
						require.Equal(t, mqttContextWire(t, want.Record), e.Raw, "caller input mutation cannot change event")
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
						if want.Error != nil {
							require.NotNil(t, last)
							require.Equal(t, want.Error.Kind, last.Kind)
							require.Equal(t, want.Error.Message, last.Message)
						} else {
							require.Nil(t, last)
							rocEqualFields(t, want.Session, e.Session)
							tree, err := e.Decode()
							require.NoError(t, err)
							rocEqualFields(t, want.Session, tree["session"].(map[string]any))
						}
					}
					s.Close("done")
					require.Nil(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}

func mqttRefusedQuery(t *testing.T, s ProtocolSession, r FeedResult, want string) {
	t.Helper()
	require.NotNil(t, r.Err)
	require.Equal(t, ErrResourceExceeded, r.Err.Kind)
	require.Equal(t, want, r.Err.Message)
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	require.Equal(t, uint64(5), e.ID)
	require.Equal(t, uint64(1), e.FlowID)
	require.Zero(t, e.ResponseTo)
	require.Zero(t, e.TransactionID)
	e.Session, e.Fields = map[string]any{"Matched Request": true}, map[string]any{"success": true}
	e.Structured = map[string]any{"fields": e.Fields}
	e.Status, e.Error = "decoded", ""
	for repeat := 0; repeat < 2; repeat++ {
		fields, err := e.GetFields()
		require.Nil(t, fields)
		var typed *ProtocolError
		require.True(t, errors.As(err, &typed))
		require.Equal(t, ErrResourceExceeded, typed.Kind)
		require.Equal(t, want, typed.Message)
		typed.Kind, typed.Message = ErrMalformedMessage, "caller-forged"
	}
	late := s.Feed(1, time.Unix(11, 0), mqttPkt(0x40, []byte{0, 1}))
	require.NotNil(t, late.Err)
	require.Equal(t, ErrFatalSessionError, late.Err.Kind)
	require.Empty(t, late.Events)
	s.Close("done")
	require.Nil(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}

// MQTT 5.0 OASIS OS, sections 2.2.1 and 3.3.2.3.4: packet identifiers
// are directional and Topic Alias storage is scoped to the network connection.
// The local parser's collection/retention limits must bound those contexts.
func testMQTT5RetainedContextBoundsAndQueries(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprintf("owned-deferred%v", deferred), func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			defer s.Close("done")
			s.(*captureSession).f.a.config.Deferred = deferred
			ts := time.Unix(10, 0)
			require.Nil(t, s.Feed(0, ts, mqttContextWire(t, "connect")).Err)
			require.Nil(t, s.Feed(1, ts, mqttContextWire(t, "connack1")).Err)
			wire := mqttContextWire(t, "owned-publish")
			r := s.Feed(0, ts, wire)
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			want := map[string]any{"Packet Type": uint64(3), "Packet Name": "PUBLISH", "QoS": uint64(1), "DUP": false, "Context Level": "observed", "Topic Name": "a/b", "Packet Identifier": uint64(7), "Outstanding": "PUBACK"}
			require.Equal(t, want, e.Session)
			for _, public := range []map[string]any{e.Session, {"Matched Request": true, "Packet Identifier": uint64(999)}, nil} {
				e.Session = public
				decoded, err := e.Decode()
				require.NoError(t, err)
				require.Equal(t, want, decoded["session"])
				mutateRTPSQuery(decoded["session"])
			}
			testLegacyRuleQueryOwnership(t, e)
			r = s.Feed(1, ts.Add(time.Millisecond), mqttContextWire(t, "puback"))
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			require.Equal(t, map[string]any{"Packet Type": uint64(4), "Packet Name": "PUBACK", "QoS": uint64(0), "DUP": false, "Context Level": "observed", "Packet Identifier": uint64(7), "Matched Request": true}, r.Events[0].Session)
			s.Close("done")
			s.Close("again")
			require.Zero(t, s.Stats().BufferedBytes)
		})
		t.Run(fmt.Sprintf("pending-deferred%v", deferred), func(t *testing.T) {
			budget := DefaultParserBudget()
			budget.MaxCollectionElements = 2
			s, err := NewProtocolSession(budget)
			require.NoError(t, err)
			defer s.Close("done")
			s.(*captureSession).f.a.config.Deferred = deferred
			ts := time.Unix(10, 0)
			require.Nil(t, s.Feed(0, ts, mqttContextWire(t, "connect")).Err)
			require.Nil(t, s.Feed(1, ts, mqttContextWire(t, "connack0")).Err)
			for i := byte(1); i <= 3; i++ {
				wire := mqttContextWire(t, fmt.Sprintf("pending%d", i))
				r := s.Feed(0, ts, wire)
				if i < 3 {
					require.Nil(t, r.Err)
					require.Len(t, r.Events, 1)
					require.Equal(t, uint64(i), r.Events[0].Session["Packet Identifier"])
				} else {
					mqttRefusedQuery(t, s, r, "MQTT pending packet identifiers")
				}
			}
			s.Close("done")
			s.Close("again")
			require.Zero(t, s.Stats().BufferedBytes)
		})
		t.Run(fmt.Sprintf("aliases-deferred%v", deferred), func(t *testing.T) {
			budget := DefaultParserBudget()
			budget.MaxCollectionElements = 2
			s, err := NewProtocolSession(budget)
			require.NoError(t, err)
			defer s.Close("done")
			s.(*captureSession).f.a.config.Deferred = deferred
			ts := time.Unix(10, 0)
			require.Nil(t, s.Feed(0, ts, mqttContextWire(t, "connect")).Err)
			require.Nil(t, s.Feed(1, ts, mqttContextWire(t, "connack3")).Err)
			for i := byte(1); i <= 3; i++ {
				wire := mqttContextWire(t, fmt.Sprintf("alias%d", i))
				r := s.Feed(0, ts, wire)
				if i < 3 {
					require.Nil(t, r.Err)
					require.Len(t, r.Events, 1)
					require.Equal(t, uint64(i), r.Events[0].Session["Topic Alias"])
				} else {
					mqttRefusedQuery(t, s, r, "MQTT topic aliases")
				}
			}
			s.Close("done")
			s.Close("again")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func testMQTT5IndependentPacketIdentifierNamespaces(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprint(deferred), func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			defer s.Close("done")
			s.(*captureSession).f.a.config.Deferred = deferred
			ts := time.Unix(10, 0)
			require.Nil(t, s.Feed(0, ts, mqttContextWire(t, "connect")).Err)
			require.Nil(t, s.Feed(1, ts, mqttContextWire(t, "connack0")).Err)
			// Client and server independently own identifier7. PUBREC and
			// PUBACK arrive from different owners even during the QoS2 handshake.
			require.Nil(t, s.Feed(0, ts, mqttContextWire(t, "client-qos2")).Err)
			require.Nil(t, s.Feed(1, ts, mqttContextWire(t, "server-qos1")).Err)
			for _, step := range []struct {
				dir  int
				hdr  byte
				name string
				qos  uint64
			}{{1, 0x50, "PUBREC", 0}, {0, 0x40, "PUBACK", 0}, {0, 0x62, "PUBREL", 1}, {1, 0x70, "PUBCOMP", 0}} {
				r := s.Feed(step.dir, ts, mqttContextWire(t, map[byte]string{0x50: "pubrec", 0x40: "puback", 0x62: "pubrel", 0x70: "pubcomp"}[step.hdr]))
				require.Nil(t, r.Err, step.name)
				require.Len(t, r.Events, 1)
				require.Equal(t, map[string]any{"Packet Type": uint64(step.hdr >> 4), "Packet Name": step.name, "QoS": step.qos, "DUP": false, "Context Level": "observed", "Packet Identifier": uint64(7), "Matched Request": true}, r.Events[0].Session)
			}
			require.Empty(t, s.Close("complete"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func testMQTT5NegativePUBRECDoesNotInventPUBRELContext(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("done")
	ts := time.Unix(10, 0)
	require.Nil(t, s.Feed(0, ts, mqttContextWire(t, "connect")).Err)
	require.Nil(t, s.Feed(1, ts, mqttContextWire(t, "connack0")).Err)
	publish := mqttContextWire(t, "client-qos2")
	require.Nil(t, s.Feed(0, ts, publish).Err)
	r := s.Feed(1, ts, mqttContextWire(t, "pubrec-negative"))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, map[string]any{"Packet Type": uint64(5), "Packet Name": "PUBREC", "QoS": uint64(0), "DUP": false, "Context Level": "observed", "Packet Identifier": uint64(7), "Matched Request": true, "Reason Code": uint64(0x80)}, r.Events[0].Session)
	r = s.Feed(0, ts, mqttContextWire(t, "pubrel"))
	require.Nil(t, r.Err) // Capture can observe unmatched control packets.
	require.Len(t, r.Events, 1)
	require.Equal(t, map[string]any{"Packet Type": uint64(6), "Packet Name": "PUBREL", "QoS": uint64(1), "DUP": false, "Context Level": "observed", "Packet Identifier": uint64(7)}, r.Events[0].Session)
	require.Empty(t, s.Close("complete"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func testMQTTScopedNativeVersionAndConnect(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, level := range []byte{4, 5} {
			t.Run(fmt.Sprintf("level%d/deferred%v", level, deferred), func(t *testing.T) {
				s, err := NewProtocolSession(DefaultParserBudget())
				require.NoError(t, err)
				defer s.Close("done")
				s.(*captureSession).f.a.config.Deferred = deferred
				var wire []byte
				if level == 5 {
					wire = mqttContextWire(t, "connect")
				} else {
					wire = mqtt311Wire(t, "connect")
				}
				r := s.Feed(0, time.Unix(10, 0), wire)
				require.Nil(t, r.Err)
				require.Len(t, r.Events, 1, "complete CONNECT needs native context")
				require.Equal(t, map[string]any{"Packet Type": uint64(1), "Packet Name": "CONNECT", "QoS": uint64(0), "DUP": false, "Context Level": "observed"}, r.Events[0].Session)
				r = s.Feed(0, time.Unix(11, 0), wire)
				require.NotNil(t, r.Err, "second CONNECT cannot replace connection context")
				require.Equal(t, ErrMalformedMessage, r.Err.Kind)
				require.Contains(t, r.Err.Message, "second CONNECT")
				require.Len(t, r.Events, 1)
				require.Nil(t, r.Events[0].Session)
				_, err = r.Events[0].GetFields()
				require.Error(t, err)
				late := s.Feed(1, time.Unix(12, 0), mqttContextWire(t, "puback"))
				require.NotNil(t, late.Err)
				require.Equal(t, ErrFatalSessionError, late.Err.Kind)
				require.Empty(t, late.Events)
				require.Nil(t, s.Close("done"))
				require.Nil(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}

func mqtt311Wire(t testing.TB, name string) []byte {
	t.Helper()
	if name == "ack" {
		return mqttContextWire(t, "puback")
	}
	wire, err := trafficfixture.ReadFile("testdata/mqtt311-native/records/" + name + ".bin")
	require.NoError(t, err)
	return wire
}

type mqtt311AnswerEvent struct {
	Record    string         `json:"record"`
	Direction int            `json:"direction"`
	Session   map[string]any `json:"session"`
	Error     *struct {
		Kind    ProtocolErrorKind `json:"kind"`
		Message string            `json:"message"`
	} `json:"error"`
}

func mqtt311CheckEvent(t *testing.T, e *ProtocolEvent, want mqtt311AnswerEvent) {
	t.Helper()
	wire := mqtt311Wire(t, want.Record)
	require.Equal(t, "mqtt", e.Protocol)
	require.Equal(t, "tcp", e.Transport)
	require.Equal(t, want.Direction, e.Direction)
	require.Equal(t, wire, e.Raw)
	require.Equal(t, len(wire), e.Length)
	require.Zero(t, e.TransactionID)
	require.Zero(t, e.ResponseTo)
	hash := sha256.Sum256(wire)
	require.Equal(t, hex.EncodeToString(hash[:]), e.SourceBytes.SHA256)
	if want.Error != nil {
		require.Nil(t, e.Session)
		require.Equal(t, "malformed", e.Status)
		_, err := e.GetFields()
		var typed *ProtocolError
		require.True(t, errors.As(err, &typed))
		require.Equal(t, want.Error.Kind, typed.Kind)
		require.Contains(t, typed.Message, want.Error.Message)
		return
	}
	require.Empty(t, e.Error)
	rocEqualFields(t, want.Session, e.Session)
	decoded, err := e.Decode()
	require.NoError(t, err)
	rocEqualFields(t, want.Session, decoded["session"].(map[string]any))
	fields, err := e.GetFields()
	require.NoError(t, err)
	require.EqualValues(t, uint64(wire[0]), findSessionField(fields, "Fixed Header"))
	switch want.Record {
	case "connect":
		require.Equal(t, "MQTT", findSessionField(fields, "Protocol Name"))
		require.EqualValues(t, uint64(4), findSessionField(fields, "Protocol Level"))
		require.EqualValues(t, uint64(2), findSessionField(fields, "Connect Flags"))
		require.EqualValues(t, uint64(60), findSessionField(fields, "Keep Alive"))
		require.Equal(t, "dev", findSessionField(fields, "Client Identifier"))
	case "connack":
		require.EqualValues(t, uint64(0), findSessionField(fields, "Acknowledgment Flags"))
		require.EqualValues(t, uint64(0), findSessionField(fields, "Return Code"))
	case "binary", "empty", "client", "server":
		topic := "t"
		payload := []byte{0x80, 0, 1}
		if want.Record == "empty" {
			payload = []byte{}
		}
		if want.Record == "client" {
			topic = "client"
		}
		if want.Record == "server" {
			topic = "server"
			payload = []byte{0xff}
		}
		require.Equal(t, topic, findSessionField(fields, "Topic Name"))
		got, ok := findSessionField(fields, "Application Message").([]byte)
		require.True(t, ok, "including the zero-width payload")
		require.Equal(t, payload, got)
		if want.Record == "client" || want.Record == "server" {
			require.EqualValues(t, uint64(7), findSessionField(fields, "Packet Identifier"))
		}
	case "ack":
		require.EqualValues(t, uint64(7), findSessionField(fields, "Packet Identifier"))
	}
	// Mutating any public or returned projection must not alter accepted fields.
	mutateRTPSQuery(fields)
	e.Session = map[string]any{"Matched Request": false, "Packet Identifier": uint64(999)}
	again, err := e.Decode()
	require.NoError(t, err)
	require.Equal(t, decoded, again)
}

func testMQTT311SealedNativeProfiles(t *testing.T) {
	var answers struct {
		Profiles map[string]struct {
			Events []mqtt311AnswerEvent `json:"events"`
		} `json:"profiles"`
	}
	raw, err := trafficfixture.ReadFile("testdata/mqtt311-native/answers.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &answers))
	require.Len(t, answers.Profiles, 4)
	for _, name := range []string{"binary-qos", "empty-payload", "second-connect", "wrong-connack-direction"} {
		profile := answers.Profiles[name]
		pcap, err := trafficfixture.ReadFile("testdata/mqtt311-native/captures/" + name + ".pcap")
		require.NoError(t, err)
		for _, workers := range []int{1, 2, 4} {
			for _, deferred := range []bool{false, true} {
				for _, observer := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/capture/w%d/d%v/o%v", name, workers, deferred, observer), func(t *testing.T) {
						var events []*ProtocolEvent
						var stats ProtocolStats
						var seen atomic.Uint64
						opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
						if observer {
							opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
						}
						require.NoError(t, ReplayPcap(bytes.NewReader(pcap), opts...))
						require.Len(t, events, len(profile.Events), "entire ordered event set")
						for i, e := range events {
							require.Equal(t, uint64(i+1), e.ID)
							require.Equal(t, uint64(1), e.FlowID)
							require.NotEmpty(t, e.SourceBytes.PacketRefs)
							mqtt311CheckEvent(t, e, profile.Events[i])
						}
						require.Zero(t, stats.BufferedBytes)
						if observer {
							require.EqualValues(t, len(profile.Events)+4, seen.Load())
						} else {
							require.Zero(t, seen.Load())
						}
					})
				}
			}
		}
		for _, chunk := range []int{0, 1, 7} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/session/chunk%d/d%v", name, chunk, deferred), func(t *testing.T) {
					s, err := NewProtocolSession(DefaultParserBudget())
					require.NoError(t, err)
					defer s.Close("done")
					s.(*captureSession).f.a.config.Deferred = deferred
					offsets := [2]uint64{}
					for i, want := range profile.Events {
						wire := mqtt311Wire(t, want.Record)
						var events []*ProtocolEvent
						var last *ProtocolError
						for at := 0; at < len(wire); {
							n := len(wire) - at
							if chunk > 0 {
								n = min(n, chunk)
							}
							r := s.Feed(want.Direction, time.Unix(10, int64(i)*int64(time.Millisecond)), wire[at:at+n])
							at += n
							if r.Err != nil && r.Err.Kind != ErrNeedMore {
								last = r.Err
							}
							events = append(events, r.Events...)
						}
						require.Len(t, events, 1)
						e := events[0]
						require.Equal(t, uint64(i+1), e.ID)
						require.Equal(t, uint64(1), e.FlowID)
						require.Equal(t, offsets[want.Direction], e.Offset)
						offsets[want.Direction] += uint64(len(wire))
						clear(wire)
						mqtt311CheckEvent(t, e, want)
						if want.Error == nil {
							require.Nil(t, last)
						} else {
							require.NotNil(t, last)
							require.Equal(t, want.Error.Kind, last.Kind)
							require.Contains(t, last.Message, want.Error.Message)
							late := s.Feed(1, time.Unix(12, 0), mqtt311Wire(t, "ack"))
							require.NotNil(t, late.Err)
							require.Equal(t, ErrFatalSessionError, late.Err.Kind)
							require.Empty(t, late.Events)
						}
					}
					require.Empty(t, s.Close("complete"))
					require.Nil(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}

// OASIS MQTT 3.1.1 / 5: new QoS>0 publications allocate an unused
// identifier. The opposite endpoint owns a separate namespace; a completed
// exchange releases its identifier, but a fresh DUP=0 PUBLISH cannot erase it.
func testMQTTActiveIdentifierCannotBeOverwritten(t *testing.T) {
	for _, level := range []byte{4, 5} {
		for _, deferred := range []bool{false, true} {
			for _, qos := range []byte{1, 2} {
				t.Run(fmt.Sprintf("level%d/d%v/qos%d", level, deferred, qos), func(t *testing.T) {
					connect := mqtt311Wire(t, "connect")
					publish := mqtt311Wire(t, "client")
					if level == 5 {
						connect = mqttContextWire(t, "connect")
						publish = mqttContextWire(t, "client-qos2")
					}
					publish[0] = 0x30 | qos<<1
					session := func() ProtocolSession {
						s, err := NewProtocolSession(DefaultParserBudget())
						require.NoError(t, err)
						s.(*captureSession).f.a.config.Deferred = deferred
						require.Nil(t, s.Feed(0, time.Unix(1, 0), connect).Err)
						return s
					}
					s := session()
					defer s.Close("done")
					require.Nil(t, s.Feed(0, time.Unix(2, 0), publish).Err)
					// QoS2 retains the identifier through PUBCOMP, including after PUBREL.
					if qos == 2 {
						require.Nil(t, s.Feed(1, time.Unix(3, 0), mqttContextWire(t, "pubrec")).Err)
						require.Nil(t, s.Feed(0, time.Unix(4, 0), mqttContextWire(t, "pubrel")).Err)
					}
					r := s.Feed(0, time.Unix(5, 0), publish)
					require.NotNil(t, r.Err, "DUP=0 cannot replace an active exchange")
					require.Equal(t, ErrMalformedMessage, r.Err.Kind)
					require.Contains(t, r.Err.Message, "packet identifier 7 is already in use")
					require.Len(t, r.Events, 1)
					require.Nil(t, r.Events[0].Session)
					require.Equal(t, publish, r.Events[0].Raw)
					require.Zero(t, r.Events[0].ResponseTo)
					require.Zero(t, r.Events[0].TransactionID)
					_, err := r.Events[0].GetFields()
					var typed *ProtocolError
					require.ErrorAs(t, err, &typed)
					require.Equal(t, ErrMalformedMessage, typed.Kind)
					late := s.Feed(1, time.Unix(6, 0), mqttContextWire(t, "puback"))
					require.NotNil(t, late.Err)
					require.Equal(t, ErrFatalSessionError, late.Err.Kind)
					require.Empty(t, late.Events)
					require.Empty(t, s.Close("bad exchange"))
					require.Nil(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
					// Adjacent valid reuse after terminal acknowledgment must remain accepted.
					good := session()
					defer good.Close("done")
					for repeat := 0; repeat < 2; repeat++ {
						require.Nil(t, good.Feed(0, time.Unix(7, 0), publish).Err)
						ack := "puback"
						if qos == 2 {
							require.Nil(t, good.Feed(1, time.Unix(8, 0), mqttContextWire(t, "pubrec")).Err)
							require.Nil(t, good.Feed(0, time.Unix(9, 0), mqttContextWire(t, "pubrel")).Err)
							ack = "pubcomp"
						}
						a := good.Feed(1, time.Unix(10, 0), mqttContextWire(t, ack))
						require.Nil(t, a.Err)
						require.Len(t, a.Events, 1)
						require.Equal(t, true, a.Events[0].Session["Matched Request"])
						require.Equal(t, uint64(7), a.Events[0].Session["Packet Identifier"])
					}
					require.Empty(t, good.Close("complete"))
					require.Zero(t, good.Stats().BufferedBytes)
				})
			}
		}
	}
}

func testMQTTSealedIdentifierReuse(t *testing.T) {
	var answers struct {
		Profiles map[string]struct {
			Events []struct {
				Wire      string         `json:"wire_hex"`
				Direction int            `json:"direction"`
				Session   map[string]any `json:"session"`
				Error     *struct {
					Kind    ProtocolErrorKind `json:"kind"`
					Message string            `json:"message"`
				} `json:"error"`
			} `json:"events"`
		} `json:"profiles"`
	}
	b, err := trafficfixture.ReadFile("testdata/mqtt-identifier/answers.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &answers))
	require.Len(t, answers.Profiles, 8)
	for name, p := range answers.Profiles {
		check := func(t *testing.T, i int, e *ProtocolEvent) {
			t.Helper()
			want := p.Events[i]
			wire, err := hex.DecodeString(want.Wire)
			require.NoError(t, err)
			require.Equal(t, "mqtt", e.Protocol)
			require.Equal(t, "tcp", e.Transport)
			require.Equal(t, want.Direction, e.Direction)
			require.Equal(t, wire, e.Raw)
			require.Equal(t, len(wire), e.Length)
			require.Equal(t, uint64(i+1), e.ID)
			require.Equal(t, uint64(1), e.FlowID)
			require.Zero(t, e.ResponseTo)
			require.Zero(t, e.TransactionID)
			hash := sha256.Sum256(wire)
			require.Equal(t, hex.EncodeToString(hash[:]), e.SourceBytes.SHA256)
			if want.Error != nil {
				require.Nil(t, e.Session)
				require.Equal(t, "malformed", e.Status)
				_, err := e.GetFields()
				var typed *ProtocolError
				require.ErrorAs(t, err, &typed)
				require.Equal(t, want.Error.Kind, typed.Kind)
				require.Equal(t, want.Error.Message, typed.Message)
				return
			}
			require.Empty(t, e.Error)
			rocEqualFields(t, want.Session, e.Session)
			decoded, err := e.Decode()
			require.NoError(t, err)
			rocEqualFields(t, want.Session, decoded["session"].(map[string]any))
			fields, err := e.GetFields()
			require.NoError(t, err)
			if wire[0]>>4 >= 3 && wire[0]>>4 <= 7 {
				require.EqualValues(t, 7, findSessionField(fields, "Packet Identifier"))
			}
			if wire[0]>>4 == 3 {
				topic := "client"
				payload := []byte{0x80, 0, 1}
				if name[5] == '5' {
					topic = "a/b"
					payload = []byte("pay")
				}
				require.Equal(t, topic, findSessionField(fields, "Topic Name"))
				require.Equal(t, payload, findSessionField(fields, "Application Message"))
			}
			mutateRTPSQuery(fields)
			mutateRTPSQuery(e.Session)
			again, err := e.Decode()
			require.NoError(t, err)
			require.Equal(t, decoded, again)
		}
		pcap, err := trafficfixture.ReadFile("testdata/mqtt-identifier/captures/" + name + ".pcap")
		require.NoError(t, err)
		for _, workers := range []int{1, 2, 4} {
			for _, deferred := range []bool{false, true} {
				for _, observer := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/capture/w%d/d%v/o%v", name, workers, deferred, observer), func(t *testing.T) {
						var events []*ProtocolEvent
						var stats ProtocolStats
						var seen atomic.Uint64
						opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
						if observer {
							opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
						}
						require.NoError(t, ReplayPcap(bytes.NewReader(pcap), opts...))
						require.Len(t, events, len(p.Events))
						for i, e := range events {
							require.NotEmpty(t, e.SourceBytes.PacketRefs)
							check(t, i, e)
						}
						require.Zero(t, stats.BufferedBytes)
						if observer {
							require.EqualValues(t, len(p.Events)+4, seen.Load())
						} else {
							require.Zero(t, seen.Load())
						}
					})
				}
			}
		}
		for _, chunk := range []int{0, 1, 7} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/session/chunk%d/d%v", name, chunk, deferred), func(t *testing.T) {
					s, err := NewProtocolSession(DefaultParserBudget())
					require.NoError(t, err)
					defer s.Close("done")
					s.(*captureSession).f.a.config.Deferred = deferred
					offsets := [2]uint64{}
					for i, want := range p.Events {
						wire, err := hex.DecodeString(want.Wire)
						require.NoError(t, err)
						var events []*ProtocolEvent
						var last *ProtocolError
						for at := 0; at < len(wire); {
							n := len(wire) - at
							if chunk > 0 {
								n = min(n, chunk)
							}
							r := s.Feed(want.Direction, time.Unix(10, int64(i)*int64(time.Millisecond)), wire[at:at+n])
							at += n
							events = append(events, r.Events...)
							if r.Err != nil && r.Err.Kind != ErrNeedMore {
								last = r.Err
							}
						}
						require.Len(t, events, 1)
						require.Equal(t, offsets[want.Direction], events[0].Offset)
						offsets[want.Direction] += uint64(len(wire))
						clear(wire)
						check(t, i, events[0])
						if want.Error == nil {
							require.Nil(t, last)
						} else {
							require.NotNil(t, last)
							require.Equal(t, want.Error.Kind, last.Kind)
							require.Equal(t, want.Error.Message, last.Message)
						}
					}
					require.Empty(t, s.Close("done"))
					require.Nil(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}
