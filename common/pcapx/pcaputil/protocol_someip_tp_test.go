package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
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

type someIPAnswer struct {
	ID, Capture, SHA256 string
	Steps               []struct {
		Direction   int
		Wire, Error string
		Fields      map[string]any
		IDs         []uint64
	}
	CloseIDs [][]uint64 `json:"close_ids"`
}

func someIPRead(t *testing.T, n string) []byte {
	t.Helper()
	var b []byte
	var e error
	if r := os.Getenv("SOMEIP_TP_TEST_ROOT"); r != "" {
		b, e = os.ReadFile(filepath.Join(r, n))
	} else {
		b, e = trafficfixture.ReadFile("someip-tp/" + n)
	}
	require.NoError(t, e)
	return b
}
func someIPCases(t *testing.T) []someIPAnswer {
	t.Helper()
	var m struct {
		Schema string
		Cases  []someIPAnswer
	}
	require.NoError(t, json.Unmarshal(someIPRead(t, "manifest.json"), &m))
	require.Equal(t, "owned-someip-tp/v1", m.Schema)
	require.Len(t, m.Cases, 39)
	if os.Getenv("SOMEIP_TP_TEST_ROOT") == "" {
		all, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		answerSHA := fmt.Sprintf("%x", sha256.Sum256(someIPRead(t, "manifest.json")))
		for _, c := range m.Cases {
			bound := 0
			for _, batch := range all {
				for _, v := range batch.Cases {
					if v.ID != "someip-tp/"+c.ID {
						continue
					}
					require.Equal(t, c.SHA256, v.Input.SHA256)
					for _, e := range v.Expectations {
						var x struct {
							File string `json:"answer_file"`
							SHA  string `json:"answer_sha256"`
						}
						require.NoError(t, json.Unmarshal(e.PayloadConstraints, &x))
						if x.File == "manifest.json" {
							require.Equal(t, answerSHA, x.SHA)
							bound++
						}
					}
				}
			}
			require.Equal(t, 1, bound, c.ID)
		}
	}

	return m.Cases
}
func someIPCheck(t *testing.T, e *ProtocolEvent, want map[string]any, kind string) {
	t.Helper()
	require.Equal(t, "someip", e.Protocol)
	require.Equal(t, someIPTPProfile, e.Profile)
	require.Zero(t, e.ResponseTo)
	require.Zero(t, e.TransactionID)
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
func TestSomeIPTPSealedReplay(t *testing.T) {
	for _, c := range someIPCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw := someIPRead(t, c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			for _, workers := range []int{1, 2, 4} {
				for _, deferDecode := range []bool{false, true} {
					for _, observer := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/deferred%t/observe%t", workers, deferDecode, observer), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var seen atomic.Int64
							opts := []CaptureOption{WithProtocolDecodeAs("udp", 18193, "someip-tp"), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferDecode), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observer {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
							}
							data := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(data), opts...))
							clear(data)
							require.Len(t, events, len(c.Steps)+len(c.CloseIDs))
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							if observer {
								require.EqualValues(t, len(c.Steps), seen.Load())
							}
							flows := map[int]uint64{}
							for i, s := range c.Steps {
								e := events[i]
								require.EqualValues(t, i+1, e.ID)
								require.Equal(t, "udp", e.Transport)
								source, dest := "192.0.2.1:40000", "192.0.2.2:18193"
								if s.Direction == 1 {
									source, dest = dest, source
								}
								require.Equal(t, source, e.Source)
								require.Equal(t, dest, e.Destination)
								if n, ok := flows[s.Direction]; ok {
									require.Equal(t, n, e.FlowID)
								} else if len(s.Wire) >= 32 {
									flows[s.Direction] = e.FlowID
								}
								require.Equal(t, s.Wire, hex.EncodeToString(e.Raw))
								require.Equal(t, s.IDs, e.SourceBytes.ParentPDUs)
								require.Len(t, e.SourceBytes.PacketRefs, 1)
								require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
								require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
								w, err := hex.DecodeString(s.Wire)
								require.NoError(t, err)
								require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(w)), e.SourceBytes.SHA256)
								someIPCheck(t, e, s.Fields, s.Error)
							}
							for i, ids := range c.CloseIDs {
								e := events[len(c.Steps)+i]
								require.Equal(t, ids, e.SourceBytes.ParentPDUs)
								require.Len(t, e.SourceBytes.PacketRefs, len(ids))
								for j, n := range ids {
									require.Equal(t, n, e.SourceBytes.PacketRefs[j].Number)
								}
								require.Empty(t, e.Raw)
								someIPCheck(t, e, nil, "NeedMore")
							}
						})
					}
				}
			}
		})
	}
}
func TestSomeIPTPSealedSessions(t *testing.T) {
	for _, c := range someIPCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprint(deferred), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 18193), func(s *captureSession) error { s.f.a.config.Deferred = deferred; return nil }, WithSessionSomeIPTP(18193))
					require.NoError(t, err)
					var events []*ProtocolEvent
					for i, step := range c.Steps {
						w, err := hex.DecodeString(step.Wire)
						require.NoError(t, err)
						before := s.Stats()
						s.Probe(w)
						require.Equal(t, before, s.Stats())
						out := s.Feed(step.Direction, time.Unix(1, int64(i)*1000000), w)
						require.Len(t, out.Events, 1)
						events = append(events, out.Events[0])
						clear(w)
					}
					for i, e := range events {
						require.Equal(t, c.Steps[i].Direction, e.Direction)
						someIPCheck(t, e, c.Steps[i].Fields, c.Steps[i].Error)
					}
					closed := s.Close("fixture")
					require.Len(t, closed, len(c.CloseIDs))
					for i, e := range closed {
						require.Equal(t, c.CloseIDs[i], e.SourceBytes.ParentPDUs)
						require.Equal(t, c.Steps[int(c.CloseIDs[i][0])-1].Direction, e.Direction)
						someIPCheck(t, e, nil, "NeedMore")
					}
					require.Empty(t, s.Close("twice"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		})
	}
}

func TestSomeIPTPBoundsDomainAndRetirement(t *testing.T) {
	cases := someIPCases(t)
	var ordered someIPAnswer
	for _, c := range cases {
		if c.ID == "ordered" {
			ordered = c
		}
	}
	first, err := hex.DecodeString(ordered.Steps[0].Wire)
	require.NoError(t, err)
	last, err := hex.DecodeString(ordered.Steps[1].Wire)
	require.NoError(t, err)
	t.Run("explicit-only-and-carrier", func(t *testing.T) {
		require.Error(t, WithProtocolDecodeAs("tcp", 18193, "someip-tp")(NewDefaultConfig()))
		_, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionSomeIPTP(18193))
		require.Error(t, err)
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 18193))
		require.NoError(t, err)
		o := s.Feed(0, time.Unix(1, 0), first)
		for _, e := range o.Events {
			require.NotEqual(t, someIPTPProfile, e.Profile)
		}
		s.Close("default")
	})
	for _, limit := range []string{"bytes", "depth", "segments", "projection"} {
		t.Run(limit, func(t *testing.T) {
			b := DefaultParserBudget()
			if limit == "bytes" {
				b.MaxFrameBytes = 64
				b.MaxMessageBytes = 64
			}
			if limit == "depth" {
				b.MaxRecursionDepth = 2
			}
			if limit == "segments" {
				b.MaxCollectionElements = 1
			}
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 18193), WithSessionSomeIPTP(18193))
			require.NoError(t, err)
			if limit == "projection" {
				s.(*captureSession).f.a.config.MaxBufferedBytes = int(someIPTPBase + 256 + 100)
			}
			input := bytes.Clone(first)
			if limit == "bytes" {
				input = append(input, make([]byte, 48)...)
				input[7] = 76
			}
			o := s.Feed(0, time.Unix(1, 0), input)
			require.Len(t, o.Events, 1)
			if limit == "segments" {
				someIPCheck(t, o.Events[0], ordered.Steps[0].Fields, "")
				o = s.Feed(0, time.Unix(2, 0), last)
				require.Len(t, o.Events, 1)
			}
			someIPCheck(t, o.Events[0], nil, "ResourceExceeded")
			a := s.(*captureSession).f.a
			a.budget = DefaultParserBudget()
			a.config.MaxMessageBytes = a.budget.MaxMessageBytes
			a.config.MaxBufferedBytes = DefaultParserBudget().MaxBufferedBytes
			o = s.Feed(0, time.Unix(3, 0), first)
			require.Len(t, o.Events, 1)
			someIPCheck(t, o.Events[0], nil, "ContextRequired")
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	for _, seconds := range []int64{31, 0} {
		t.Run(fmt.Sprintf("time-%d", seconds), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 18193), WithSessionSomeIPTP(18193))
			require.NoError(t, err)
			s.Feed(0, time.Unix(1, 0), first)
			o := s.Feed(0, time.Unix(seconds, 0), last)
			require.Len(t, o.Events, 1)
			someIPCheck(t, o.Events[0], nil, "ContextRequired")
			o = s.Feed(0, time.Unix(32, 0), first)
			require.Len(t, o.Events, 1)
			someIPCheck(t, o.Events[0], nil, "ContextRequired")
			require.Empty(t, s.Close("end"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	t.Run("capture-domain", func(t *testing.T) {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		a := s.(*captureSession).f.a
		d := CaptureDomain{Section: 1, Interface: 2, Encapsulation: "vlan7/tunnel1"}
		other := d
		other.Interface = 3
		feed := func(w []byte, domain CaptureDomain, n int) *ProtocolEvent {
			e := &ProtocolEvent{Timestamp: time.Unix(int64(n), 0), Transport: "udp", Source: "192.0.2.1:40000", Destination: "192.0.2.2:18193", Domain: domain, SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: uint64(n), Domain: domain}}}}
			a.decodeSomeIPTPDatagram(e, w, nil)
			return e
		}
		e := feed(first, d, 1)
		someIPCheck(t, e, ordered.Steps[0].Fields, "")
		e = feed(last, other, 2)
		someIPCheck(t, e, nil, "ContextRequired")
		e = feed(last, d, 3)
		want := cloneSession(ordered.Steps[1].Fields)
		assembly := want["assembly"].(map[string]any)
		assembly["contributor_event_ids"] = []uint64{1, 3}
		someIPCheck(t, e, want, "")
		require.Equal(t, []uint64{1, 3}, e.SourceBytes.ParentPDUs)
		s.Close("end")
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("truncated-carrier-retires", func(t *testing.T) {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		a := s.(*captureSession).f.a
		e := &ProtocolEvent{Timestamp: time.Unix(1, 0), Source: "a", Destination: "b"}
		a.decodeSomeIPTPDatagram(e, first, nil)
		e = &ProtocolEvent{Timestamp: time.Unix(2, 0), Source: "a", Destination: "b"}
		a.decodeSomeIPTPDatagram(e, last, protocolError(ErrNeedMore, "truncated carrier"))
		someIPCheck(t, e, nil, "NeedMore")
		require.Empty(t, e.Raw)
		e = &ProtocolEvent{Timestamp: time.Unix(3, 0), Source: "a", Destination: "b"}
		a.decodeSomeIPTPDatagram(e, last, nil)
		someIPCheck(t, e, nil, "ContextRequired")
		s.Close("end")
		require.Zero(t, s.Stats().BufferedBytes)
	})
}

func TestSomeIPTPExistingDecodeAsAPI(t *testing.T) {
	t.Run("missing-selected-profile", func(t *testing.T) {
		cfg := NewDefaultConfig()
		require.NoError(t, WithProtocolDecodeAs("udp", 18193, "someip-tp")(cfg), "selected SOME/IP-TP codec must be available through existing DecodeAs API")
	})
	t.Run("adjacent-dlms-option", func(t *testing.T) {
		cfg := NewDefaultConfig()
		require.NoError(t, WithProtocolDecodeAs("udp", 4059, "dlms-wrapper")(cfg))
	})
}

func TestSomeIPTPBackdatedNewIdentityCannotRestart(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 18193), WithSessionSomeIPTP(18193))
	require.NoError(t, err)
	var c someIPAnswer
	for _, x := range someIPCases(t) {
		if x.ID == "ordered" {
			c = x
		}
	}
	w, err := hex.DecodeString(c.Steps[0].Wire)
	require.NoError(t, err)
	s.Feed(0, time.Unix(20, 0), w)
	w[11] = 6
	o := s.Feed(0, time.Unix(10, 0), w)
	require.Len(t, o.Events, 1)
	someIPCheck(t, o.Events[0], nil, "ContextRequired")
	o = s.Feed(0, time.Unix(21, 0), w)
	require.Len(t, o.Events, 1)
	someIPCheck(t, o.Events[0], nil, "ContextRequired")
	s.Close("end")
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestSomeIPTPExactFrameAndPayloadLimits(t *testing.T) {
	var c someIPAnswer
	for _, x := range someIPCases(t) {
		if x.ID == "ordered" {
			c = x
		}
	}
	head, err := hex.DecodeString(c.Steps[0].Wire)
	require.NoError(t, err)
	makeWire := func(offset int, more bool, data []byte) []byte {
		w := append(bytes.Clone(head[:20]), data...)
		binary.BigEndian.PutUint32(w[4:8], uint32(len(w)-8))
		v := uint32(offset)
		if more {
			v |= 1
		}
		binary.BigEndian.PutUint32(w[16:20], v)
		return w
	}
	for _, parts := range []int{64, 65} {
		t.Run(fmt.Sprintf("segments-%d", parts), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 18193), WithSessionSomeIPTP(18193))
			require.NoError(t, err)
			data := []byte("ABCDEFGHIJKLMNOP")
			for i := 0; i < parts; i++ {
				o := s.Feed(0, time.Unix(1, int64(i)), makeWire(i*16, i+1 < parts, data))
				require.Len(t, o.Events, 1)
				if i == 64 {
					someIPCheck(t, o.Events[0], nil, "ResourceExceeded")
				} else {
					f, err := o.Events[0].GetFields()
					require.NoError(t, err)
					a := f["assembly"].(map[string]any)
					require.Equal(t, i+1, a["observed_segments"])
					require.Equal(t, (i+1)*16, a["observed_payload_bytes"])
					require.Equal(t, i+1 == parts, a["complete"])
					if i+1 == parts {
						require.Equal(t, hex.EncodeToString(bytes.Repeat(data, parts)), a["assembled_payload_hex"])
					}
				}
			}
			require.Empty(t, s.Close("end"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	for _, extra := range []int{0, 1} {
		t.Run(fmt.Sprintf("payload-65536-plus-%d", extra), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 18193), WithSessionSomeIPTP(18193))
			require.NoError(t, err)
			a := bytes.Repeat([]byte{0x51}, 32768)
			b := bytes.Repeat([]byte{0x72}, 32768+extra)
			s.Feed(0, time.Unix(1, 0), makeWire(0, true, a))
			o := s.Feed(0, time.Unix(2, 0), makeWire(32768, false, b))
			require.Len(t, o.Events, 1)
			if extra > 0 {
				someIPCheck(t, o.Events[0], nil, "ResourceExceeded")
			} else {
				f, err := o.Events[0].GetFields()
				require.NoError(t, err)
				assembly := f["assembly"].(map[string]any)
				require.Equal(t, true, assembly["complete"])
				require.Equal(t, 65536, assembly["observed_payload_bytes"])
				require.Equal(t, hex.EncodeToString(append(a, b...)), assembly["assembled_payload_hex"])
				require.Equal(t, []uint64{1, 2}, assembly["contributor_event_ids"])
				require.Zero(t, o.Events[0].ResponseTo)
				require.Zero(t, o.Events[0].TransactionID)
			}
			require.Empty(t, s.Close("end"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
