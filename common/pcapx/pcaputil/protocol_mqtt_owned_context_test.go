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

func TestMQTT5SealedContextMatrix(t *testing.T) { testMQTT5SealedContextMatrix(t) }

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
func TestMQTT5RetainedContextBoundsAndQueries(t *testing.T) {
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

func TestMQTT5IndependentPacketIdentifierNamespaces(t *testing.T) {
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

func TestMQTT5NegativePUBRECDoesNotInventPUBRELContext(t *testing.T) {
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
