package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type nmeaNavigationControl struct {
	ID, Capture, Answer, SHA256 string
	AnswerSHA                   string `json:"answer_sha256"`
	Packets                     int
}
type nmeaNavigationAnswer struct {
	Fields    []map[string]any
	PureError string   `json:"pure_error"`
	Wire      []string `json:"wire_hex"`
}

func testNMEANavigationSealedReplay(t *testing.T, root, schema string, count int) {
	b, err := trafficfixture.ReadFile(root + "/controls.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []nmeaNavigationControl
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, schema, m.Schema)
	require.Len(t, m.Cases, count)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	facts := map[string]trafficfixture.FrozenCase{}
	for _, batch := range all {
		for _, c := range batch.Cases {
			if strings.HasPrefix(c.ID, root+"/") {
				facts[c.ID] = c
			}
		}
	}
	require.Len(t, facts, count)
	for _, c := range m.Cases {
		t.Run(c.ID, func(t *testing.T) {
			raw, err := trafficfixture.ReadFile(root + "/" + c.Capture)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			b, err := trafficfixture.ReadFile(root + "/" + c.Answer)
			require.NoError(t, err)
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(b)))
			fact, ok := facts[root+"/"+c.ID]
			require.True(t, ok)
			require.Equal(t, c.Capture, fact.Input.File)
			require.Equal(t, c.SHA256, fact.Input.SHA256)
			require.EqualValues(t, len(raw), fact.Input.Bytes)
			require.Equal(t, c.Packets, fact.Facts.PacketCount)
			require.Len(t, fact.Expectations, 1)
			var binding struct {
				File string `json:"answer_file"`
				SHA  string `json:"answer_sha256"`
			}
			require.NoError(t, json.Unmarshal(fact.Expectations[0].PayloadConstraints, &binding))
			require.Equal(t, c.Answer, binding.File)
			require.Equal(t, c.AnswerSHA, binding.SHA)
			var a nmeaNavigationAnswer
			require.NoError(t, json.Unmarshal(b, &a))
			for i, s := range a.Wire {
				w, err := hex.DecodeString(s)
				require.NoError(t, err)
				fields, err := decodeNMEADatagram(w, 4)
				if a.PureError != "" {
					require.NotNil(t, err)
					rocTypedError(t, a.PureError, err)
					require.Nil(t, fields)
				} else {
					require.NoError(t, err)
					rocEqualFields(t, a.Fields[i], fields)
				}
			}
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observer := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observer), func(t *testing.T) {
							var mu sync.Mutex
							var events []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); events = append(events, e); mu.Unlock() }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observer {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							owned := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(owned), opts...))
							clear(owned)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							if observer {
								require.Equal(t, c.Packets, seen)
							}
							require.Len(t, events, c.Packets)
							for i, e := range events {
								require.NotZero(t, e.ID)
								require.Zero(t, e.FlowID)
								require.Zero(t, e.ResponseTo)
								require.Zero(t, e.TransactionID)
								require.Equal(t, "192.0.2.10:38000", e.Source)
								require.Equal(t, "192.0.2.20:10110", e.Destination)
								require.Len(t, e.SourceBytes.PacketRefs, 1)
								require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
								require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
								if a.PureError != "" {
									require.Empty(t, e.Protocol)
									require.Equal(t, "unrecognized", e.Status)
									require.Nil(t, e.Session)
									f, err := e.Decode()
									require.Error(t, err)
									require.Nil(t, f)
									continue
								}
								require.Equal(t, "nmea", e.Protocol)
								require.Equal(t, "nmea-native", e.Profile)
								require.Equal(t, a.Wire[i], hex.EncodeToString(e.Raw))
								f, err := e.GetFields()
								require.NoError(t, err)
								rocEqualFields(t, a.Fields[i], f)
								testNativeObservationSessionQueries(t, e, a.Fields[i])
							}
						})
					}
				}
			}
		})
	}
}

func testNMEAVTGSealedReplay(t *testing.T) {
	testNMEANavigationSealedReplay(t, "nmea-vtg", "nmea-vtg-units/v1", 51)
	// The complete historical GGA answer supplies the old depth regression,
	// avoiding a baseline that fails solely because VTG was not implemented.
	b, err := trafficfixture.ReadFile("incremental-mvp/nmea/controls.json")
	require.NoError(t, err)
	var historical struct {
		Cases []struct {
			Name     string
			Steps    []struct{ Hex string }
			Expected []map[string]any `json:"expected_messages"`
		}
	}
	require.NoError(t, json.Unmarshal(b, &historical))
	var wire []byte
	var one map[string]any
	for _, c := range historical.Cases {
		if c.Name == "nmea-gga-positive" {
			require.Len(t, c.Steps, 1)
			wire, err = hex.DecodeString(c.Steps[0].Hex)
			require.NoError(t, err)
			require.Len(t, c.Expected, 1)
			one = cloneSession(c.Expected[0])
			delete(one, "Message Count")
		}
	}
	require.NotEmpty(t, wire)
	require.NotEmpty(t, one)
	two := append(bytes.Clone(wire), wire...)
	want := map[string]any{"Packet Name": "NMEA 0183 sentences", "Message Count": float64(2), "Messages": []any{one, one}, "Context Level": "observed"}
	for _, depth := range []int{1, 2} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("depth%d/deferred%t", depth, deferred), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(ParserBudget{MaxRecursionDepth: depth}, WithSessionTransport("udp"), WithSessionPorts(38000, 10110))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				input := bytes.Clone(two)
				out := s.Feed(0, time.Unix(1, 0), input)
				clear(input)
				require.Equal(t, len(two), out.Consumed)
				require.Len(t, out.Events, 1)
				if depth == 1 {
					require.NotNil(t, out.Err)
					rocTypedError(t, "ResourceExceeded", out.Err)
					require.Nil(t, out.Events[0].Session)
					f, err := out.Events[0].GetFields()
					rocTypedError(t, "ResourceExceeded", err)
					require.Nil(t, f)
				} else {
					require.Nil(t, out.Err)
					f, err := out.Events[0].GetFields()
					require.NoError(t, err)
					rocEqualFields(t, want, f)
					testNativeObservationSessionQueries(t, out.Events[0], want)
				}
				// Single flat fields fit depth1 after a rejected two-level datagram.
				next := s.Feed(1, time.Unix(2, 0), wire)
				require.Nil(t, next.Err)
				require.Len(t, next.Events, 1)
				f, err := next.Events[0].GetFields()
				require.NoError(t, err)
				normal := cloneSession(one)
				normal["Message Count"] = float64(1)
				rocEqualFields(t, normal, f)
				require.Zero(t, next.Events[0].ResponseTo)
				require.Zero(t, next.Events[0].TransactionID)
				require.Empty(t, s.Close("done"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
