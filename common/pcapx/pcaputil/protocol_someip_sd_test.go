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
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type someIPSDAnswer struct {
	ID, Wire, Error, Capture, SHA256 string
	Fields                           map[string]any
}

func someIPSDRead(t *testing.T, n string) []byte {
	t.Helper()
	var b []byte
	var err error
	if root := os.Getenv("SOMEIP_SD_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, n))
	} else {
		b, err = trafficfixture.ReadFile("someip-sd/" + n)
	}
	require.NoError(t, err)
	return b
}
func someIPSDCases(t *testing.T) []someIPSDAnswer {
	t.Helper()
	var m struct {
		Schema string
		Cases  []someIPSDAnswer
	}
	require.NoError(t, json.Unmarshal(someIPSDRead(t, "manifest.json"), &m))
	require.Equal(t, "owned-someip-sd/v1", m.Schema)
	require.Len(t, m.Cases, 45)
	if os.Getenv("SOMEIP_SD_TEST_ROOT") == "" {
		bindPPIAndSDAnswers(t, "someip-sd", someIPSDRead(t, "manifest.json"))
	}

	return m.Cases
}
func someIPSDCheck(t *testing.T, e *ProtocolEvent, want map[string]any, kind string) {
	t.Helper()
	require.Equal(t, "someip", e.Protocol)
	require.Equal(t, someIPSDProfile, e.Profile)
	require.Zero(t, e.TransactionID)
	require.Zero(t, e.ResponseTo)
	f, err := e.GetFields()
	if kind != "" {
		require.Nil(t, f)
		assertOwnedProtocolRefusal(t, e, kind, err)
		require.Nil(t, e.Session)
		return
	}
	require.NoError(t, err)
	rocEqualFields(t, want, f)
	rocEqualFields(t, want, e.Session)
	mutateRTPSQuery(f)
	mutateRTPSQuery(e.Session)
	d, err := e.Decode()
	require.NoError(t, err)
	rocEqualFields(t, want, d["fields"].(map[string]any))
	rocEqualFields(t, want, d["session"].(map[string]any))
	mutateRTPSQuery(d)
	e.Fields = map[string]any{"forged": true}
	e.Structured = map[string]any{"forged": true}
	clear(e.Raw)
	f, err = e.GetFields()
	require.NoError(t, err)
	rocEqualFields(t, want, f)
}
func TestSomeIPSDSealedReplay(t *testing.T) {
	for _, c := range someIPSDCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw := someIPSDRead(t, c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/deferred%t/observer%t", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var seen atomic.Int64
							opts := []CaptureOption{WithProtocolDecodeAs("udp", 30490, "someip-sd"), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
							}
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							require.Len(t, events, 1)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							if observe {
								require.EqualValues(t, 1, seen.Load())
							}
							e := events[0]
							require.EqualValues(t, 1, e.ID)
							require.Equal(t, "udp", e.Transport)
							require.Equal(t, "192.0.2.1:40000", e.Source)
							require.Equal(t, "192.0.2.2:30490", e.Destination)
							require.Equal(t, 0, e.Direction)
							require.Equal(t, c.Wire, hex.EncodeToString(e.Raw))
							require.Empty(t, e.SourceBytes.ParentPDUs)
							require.Len(t, e.SourceBytes.PacketRefs, 1)
							require.EqualValues(t, 1, e.SourceBytes.PacketRefs[0].Number)
							require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
							someIPSDCheck(t, e, c.Fields, c.Error)
						})
					}
				}
			}
		})
	}
}
func TestSomeIPSDSealedSessionOwnership(t *testing.T) {
	for _, c := range someIPSDCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			for _, deferred := range []bool{false, true} {
				for _, direction := range []int{0, 1} {
					t.Run(fmt.Sprintf("deferred%t/d%d", deferred, direction), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 30490), WithSessionSomeIPSD(30490), func(s *captureSession) error { s.f.a.config.Deferred = deferred; return nil })
						require.NoError(t, err)
						wire, err := hex.DecodeString(c.Wire)
						require.NoError(t, err)
						before := s.Stats()
						s.Probe(wire)
						require.Equal(t, before, s.Stats())
						o := s.Feed(direction, time.Unix(1, 0), wire)
						require.Len(t, o.Events, 1)
						require.Equal(t, direction, o.Events[0].Direction)
						clear(wire)
						someIPSDCheck(t, o.Events[0], c.Fields, c.Error)
						require.Empty(t, s.Close("end"))
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
					})
				}
			}
		})
	}
}

func TestSomeIPSDLiveBudgetsAndAtomicIsolation(t *testing.T) {
	var selected someIPSDAnswer
	for _, c := range someIPSDCases(t) {
		if c.ID == "subscribe-ipv4" {
			selected = c
		}
	}
	wire, err := hex.DecodeString(selected.Wire)
	require.NoError(t, err)
	for _, tc := range []struct {
		name                   string
		depth, elements, frame int
		ok                     bool
	}{{"depth-low", 4, 4096, 1 << 20, false}, {"depth-exact", 5, 4096, 1 << 20, true}, {"count-low", 64, 2, 1 << 20, false}, {"count-exact", 64, 3, 1 << 20, true}, {"frame-low", 64, 4096, len(wire) - 1, false}, {"frame-exact", 64, 4096, len(wire), true}} {
		t.Run(tc.name, func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxRecursionDepth = tc.depth
			b.MaxCollectionElements = tc.elements
			b.MaxFrameBytes = tc.frame
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 30490), WithSessionSomeIPSD(30490))
			require.NoError(t, err)
			o := s.Feed(0, time.Unix(1, 0), bytes.Clone(wire))
			require.Len(t, o.Events, 1)
			if tc.ok {
				someIPSDCheck(t, o.Events[0], selected.Fields, "")
			} else {
				someIPSDCheck(t, o.Events[0], nil, "ResourceExceeded")
			}
			require.Empty(t, s.Close("end"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	t.Run("pool-refusal-is-stateless", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 30490), WithSessionSomeIPSD(30490))
		require.NoError(t, err)
		a := s.(*captureSession).f.a
		a.config.MaxBufferedBytes = 32768
		o := s.Feed(0, time.Unix(1, 0), wire)
		require.Len(t, o.Events, 1)
		someIPSDCheck(t, o.Events[0], nil, "ResourceExceeded")
		require.Zero(t, s.Stats().BufferedBytes)
		a.config.MaxBufferedBytes = DefaultParserBudget().MaxBufferedBytes
		o = s.Feed(1, time.Unix(2, 0), wire)
		require.Len(t, o.Events, 1)
		someIPSDCheck(t, o.Events[0], selected.Fields, "")
		require.Zero(t, o.Events[0].ResponseTo)
		require.Empty(t, s.Close("end"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("datagram-boundary", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 30490), WithSessionSomeIPSD(30490))
		require.NoError(t, err)
		for _, part := range [][]byte{wire[:16], wire[16:]} {
			o := s.Feed(0, time.Unix(1, 0), part)
			require.Len(t, o.Events, 1)
			f, err := o.Events[0].GetFields()
			require.Error(t, err)
			require.Nil(t, f)
		}
		o := s.Feed(0, time.Unix(2, 0), wire)
		require.Len(t, o.Events, 1)
		someIPSDCheck(t, o.Events[0], selected.Fields, "")
		require.Empty(t, s.Close("end"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("carrier-and-defaults", func(t *testing.T) {
		_, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionSomeIPSD(30490))
		require.Error(t, err)
		require.Error(t, WithProtocolDecodeAs("tcp", 30490, "someip-sd")(NewDefaultConfig()))
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 30490))
		require.NoError(t, err)
		o := s.Feed(0, time.Unix(1, 0), wire)
		for _, e := range o.Events {
			require.NotEqual(t, someIPSDProfile, e.Profile)
		}
		s.Close("end")
	})
}
