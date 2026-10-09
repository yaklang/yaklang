package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func c37118Controls(t *testing.T) []bsapControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("c37118-cfg2-data/manifest.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []bsapControl
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, "pr5013-c37118-selected-session/v1", m.Schema)
	require.Len(t, m.Cases, 55)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	bound := map[string]trafficfixture.FrozenCase{}
	for _, batch := range all {
		for _, c := range batch.Cases {
			bound[c.ID] = c
		}
	}
	for _, c := range m.Cases {
		v, ok := bound["c37118-cfg2-data/"+c.ID]
		require.True(t, ok)
		require.Equal(t, c.Capture, v.Input.File)
		require.Equal(t, c.SHA256, v.Input.SHA256)
		require.Equal(t, c.Packets, v.Facts.PacketCount)
		require.Len(t, v.Expectations, 1)
		var a struct {
			File string `json:"answer_file"`
			SHA  string `json:"answer_sha256"`
		}
		require.NoError(t, json.Unmarshal(v.Expectations[0].PayloadConstraints, &a))
		require.Equal(t, c.Answer, a.File)
		require.Equal(t, c.AnswerSHA, a.SHA)
	}
	return m.Cases
}
func c37118Input(t *testing.T, c bsapControl) ([]byte, bsapAnswer) {
	t.Helper()
	b, err := trafficfixture.ReadFile("c37118-cfg2-data/" + c.Capture)
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(b)))
	ab, err := trafficfixture.ReadFile("c37118-cfg2-data/" + c.Answer)
	require.NoError(t, err)
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(ab)))
	var a bsapAnswer
	require.NoError(t, json.Unmarshal(ab, &a))
	require.Equal(t, c.Packets, a.Packets)
	require.Len(t, a.Events, len(a.Wires))
	r, err := NewCaptureReader(bytes.NewReader(b))
	require.NoError(t, err)
	at, packets := 0, 0
	for {
		wire, ci, err := r.ReadPacketData()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		packets++
		require.Equal(t, len(wire), ci.CaptureLength)
		require.Equal(t, ci.Length, ci.CaptureLength)
		p := gopacket.NewPacket(wire, r.LinkType(), gopacket.Default)
		var w []byte
		if u, ok := p.Layer(layers.LayerTypeUDP).(*layers.UDP); ok {
			w = u.Payload
		} else {
			tcp, ok := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
			require.True(t, ok)
			if len(tcp.Payload) == 0 {
				continue
			}
			w = tcp.Payload
		}
		require.Less(t, at, len(a.Wires))
		require.Equal(t, a.Wires[at], hex.EncodeToString(w))
		at++
	}
	require.Equal(t, c.Packets, packets)
	require.Equal(t, len(a.Events), at)
	return b, a
}
func TestC37118SealedReplay(t *testing.T) {
	for _, c := range c37118Controls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, want := c37118Input(t, c)
			tcp := strings.HasPrefix(c.ID, "tcp-")
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var assembly TCPReassemblyStats
							var seen atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s })}
							if !tcp {
								opts = append(opts, WithProtocolDecodeAs("udp", 28400, "c37118"))
							}
							if observe {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.NotEmpty(t, p.Data()); seen.Add(1) }))
							}
							owned := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(owned), opts...))
							clear(owned)
							require.Len(t, events, len(want.Events))
							require.EqualValues(t, len(events), stats.Messages)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							require.Zero(t, assembly.UnreassembledBytes)
							require.EqualValues(t, c.Packets, assembly.CapturedPackets)
							if observe {
								require.EqualValues(t, c.Packets, seen.Load())
							}
							for i, e := range events {
								w := want.Events[i]
								require.Equal(t, "c37118", e.Protocol)
								require.Equal(t, "c37118-v1-v2-cfg2-data", e.Profile)
								transport := "udp"
								if tcp {
									transport = "tcp"
								}
								require.Equal(t, transport, e.Transport)
								require.Zero(t, e.ResponseTo)
								require.Zero(t, e.TransactionID)
								src, dst := "192.0.2.10:39300", "192.0.2.20:28400"
								if w.Direction == 1 {
									src, dst = dst, src
								}
								require.Equal(t, src, e.Source)
								require.Equal(t, dst, e.Destination)
								require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
								require.Len(t, e.SourceBytes.PacketRefs, 1)
								num := i + 1
								if tcp {
									num += 3
								}
								require.EqualValues(t, num, e.SourceBytes.PacketRefs[0].Number)
								require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
								if w.Error != "" {
									rocTypedError(t, w.Error, e.sessionError)
								} else {
									require.Nil(t, e.sessionError)
								}
								if w.Error != "" {
									require.NotEmpty(t, e.Error)
									if w.Fields == nil {
										require.Nil(t, e.Session)
									} else {
										rocEqualFields(t, w.Fields, e.Session)
									}
									require.Nil(t, e.Fields)
									_, err := e.GetFields()
									rocTypedError(t, w.Error, err)
									continue
								}
								status := "decoded"
								if deferred {
									status = "deferred"
								}
								require.Equal(t, status, e.Status)
								require.Empty(t, e.Error)
								rocEqualFields(t, w.Fields, e.Session)
								f, err := e.GetFields()
								require.NoError(t, err)
								rocEqualFields(t, w.Fields, f)
								f["Frame Size"] = 999
								d, err := e.Decode()
								require.NoError(t, err)
								rocEqualFields(t, w.Fields, protocolFields(d))
								rocEqualFields(t, w.Fields, e.Session)
							}
						})
					}
				}
			}
		})
	}
}
func TestC37118SessionChunksAndOwnership(t *testing.T) {
	for _, c := range c37118Controls(t) {
		if c.ID != "format-0" && c.ID != "format-15" && c.ID != "version2-full" {
			continue
		}
		_, a := c37118Input(t, c)
		for _, chunk := range []int{1, 2, 3, 7, 31, 4096} {
			t.Run(fmt.Sprintf("%s/chunk%d", c.ID, chunk), func(t *testing.T) {
				s, err := NewProtocolSession(DefaultParserBudget())
				require.NoError(t, err)
				var events []*ProtocolEvent
				joined := []byte{}
				for _, e := range a.Events {
					w, err := hex.DecodeString(e.Raw)
					require.NoError(t, err)
					joined = append(joined, w...)
				}
				for at := 0; at < len(joined); at += chunk {
					end := min(len(joined), at+chunk)
					w := bytes.Clone(joined[at:end])
					r := s.Feed(0, time.Unix(1700043000, 0), w)
					require.True(t, r.Err == nil || r.Err.Kind == ErrNeedMore, "%v", r.Err)
					events = append(events, r.Events...)
					clear(w)
				}
				require.Len(t, events, len(a.Events))
				for i, e := range events {
					rocEqualFields(t, a.Events[i].Fields, e.Session)
					f, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, a.Events[i].Fields, f)
					if cfg, ok := f["Configuration"].(map[string]any); ok {
						ps := cfg["PMUs"].([]map[string]any)
						ps[0]["Phasor Names"].([]string)[0] = "modified"
					}
					f, err = e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, a.Events[i].Fields, f)
				}
				s.Close("finished")
				s.Close("idempotent")
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
func TestC37118ResourceDomainAndIdle(t *testing.T) {
	var cfg, data []byte
	for _, c := range c37118Controls(t) {
		if c.ID != "format-0" {
			continue
		}
		_, a := c37118Input(t, c)
		cfg, _ = hex.DecodeString(a.Events[0].Raw)
		data, _ = hex.DecodeString(a.Events[1].Raw)
	}
	require.NotEmpty(t, cfg)
	require.NotEmpty(t, data)
	b := DefaultParserBudget()
	b.MaxBufferedBytes = 15000
	b.MaxFrameBytes = 400
	b.MaxMessageBytes = 400
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(39300, 28400))
	require.NoError(t, err)
	a := s.(*captureSession).f.a
	emit := func(w []byte, iface int, sec int64) *ProtocolEvent {
		e := &ProtocolEvent{Source: "192.0.2.10:39300", Destination: "192.0.2.20:28400", Transport: "udp", Timestamp: time.Unix(sec, 0), Domain: CaptureDomain{Interface: iface}}
		require.True(t, a.decodeC37118Datagram(e, w, true))
		return e
	}
	e := emit(cfg, 0, 1)
	require.Empty(t, e.Error)
	require.NotZero(t, e.FlowID)
	flow := e.FlowID
	e = emit(data, 1, 2)
	rocTypedError(t, "ContextRequired", e.sessionError)
	require.Zero(t, e.FlowID)
	require.Nil(t, e.Session["Data"])
	e = emit(data, 0, 3)
	require.Empty(t, e.Error)
	require.Equal(t, flow, e.FlowID)
	require.NotNil(t, e.Session["Data"])
	hold := &binFlow{a: a}
	require.NoError(t, hold.reserveSession(1400))
	e = emit(cfg, 0, 4)
	rocTypedError(t, "ResourceExceeded", e.sessionError)
	require.Nil(t, e.Session)
	hold.closeSession()
	e = emit(data, 0, 5)
	rocTypedError(t, "ContextRequired", e.sessionError)
	require.Nil(t, e.Session["Data"])
	e = emit(cfg, 0, 6)
	require.Empty(t, e.Error)
	e = emit(data, 0, 7)
	require.Empty(t, e.Error)
	e = emit(data, 0, 38)
	rocTypedError(t, "ContextRequired", e.sessionError)
	require.Zero(t, e.FlowID)
	require.Nil(t, e.Session["Data"])
	e = emit(cfg, 0, 39)
	require.Empty(t, e.Error)
	require.NotEqual(t, flow, e.FlowID)
	require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
	s.Close("finished")
	s.Close("idempotent")
	require.Zero(t, s.Stats().BufferedBytes)
	b.MaxCollectionElements = 18
	s, err = NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(39300, 28400))
	require.NoError(t, err)
	a = s.(*captureSession).f.a
	e = emit(cfg, 0, 1)
	rocTypedError(t, "ResourceExceeded", e.sessionError)
	require.Nil(t, e.Session)
	s.Close("budget")
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestC37118TCPBudgetRecovery(t *testing.T) {
	var cfg, data []byte
	for _, c := range c37118Controls(t) {
		if c.ID == "format-0" {
			_, a := c37118Input(t, c)
			cfg, _ = hex.DecodeString(a.Events[0].Raw)
			data, _ = hex.DecodeString(a.Events[1].Raw)
		}
	}
	b := DefaultParserBudget()
	b.MaxBufferedBytes = 17000
	b.MaxFrameBytes = 400
	b.MaxMessageBytes = 400
	s, err := NewProtocolSession(b)
	require.NoError(t, err)
	r := s.Feed(1, time.Unix(1, 0), cfg)
	require.Nil(t, r.Err)
	hold := &binFlow{a: s.(*captureSession).f.a}
	require.NoError(t, hold.reserveSession(4000))
	r = s.Feed(1, time.Unix(2, 0), cfg)
	require.NotNil(t, r.Err)
	require.Equal(t, ErrResourceExceeded, r.Err.Kind)
	hold.closeSession()
	r = s.Feed(1, time.Unix(3, 0), data)
	require.NotNil(t, r.Err)
	require.Equal(t, ErrContextRequired, r.Err.Kind)
	require.NotContains(t, r.Events[0].Session, "Data")
	r = s.Feed(1, time.Unix(4, 0), append(bytes.Clone(cfg), data...))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 2)
	require.NotNil(t, r.Events[1].Session["Data"])
	require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
	s.Close("test")
	s.Close("idempotent")
	require.Zero(t, s.Stats().BufferedBytes)
}
