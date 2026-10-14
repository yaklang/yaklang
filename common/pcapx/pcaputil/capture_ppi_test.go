package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func ppiSeed(t *testing.T) ([][]byte, someIPAnswer) {
	t.Helper()
	b, err := trafficfixture.ReadFile("someip-tp/ordered.pcap")
	require.NoError(t, err)
	r, err := pcapgo.NewReader(bytes.NewReader(b))
	require.NoError(t, err)
	var frames [][]byte
	for {
		v, _, err := r.ReadPacketData()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		frames = append(frames, bytes.Clone(v))
	}
	require.Len(t, frames, 2)
	var answer someIPAnswer
	for _, c := range someIPCases(t) {
		if c.ID == "ordered" {
			answer = c
		}
	}
	return frames, answer
}
func ppiFrame(raw []byte, interfaceID *uint32) []byte {
	n := 8
	if interfaceID != nil {
		n = 16
	}
	h := make([]byte, n)
	binary.LittleEndian.PutUint16(h[2:4], uint16(n))
	binary.LittleEndian.PutUint32(h[4:8], 1)
	if interfaceID != nil {
		binary.LittleEndian.PutUint16(h[8:10], 8)
		binary.LittleEndian.PutUint16(h[10:12], 4)
		binary.LittleEndian.PutUint32(h[12:16], *interfaceID)
	}
	return append(h, raw...)
}
func ppiPcap(t *testing.T, frames [][]byte, link layers.LinkType) []byte {
	t.Helper()
	var b bytes.Buffer
	w := pcapgo.NewWriter(&b)
	require.NoError(t, w.WriteFileHeader(65535, link))
	for i, frame := range frames {
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1, int64(i)*1000000), CaptureLength: len(frame), Length: len(frame)}, frame))
	}
	return b.Bytes()
}
func TestPPICaptureDomainAndOwnedTransport(t *testing.T) {
	seed, want := ppiSeed(t)
	for _, aggregation := range []bool{false, true} {
		for _, workers := range []int{1, 2, 4} {
			for _, deferred := range []bool{false, true} {
				for _, observe := range []bool{false, true} {
					t.Run(fmt.Sprintf("aggregation%t/w%d/deferred%t/observe%t", aggregation, workers, deferred, observe), func(t *testing.T) {
						var id *uint32
						zero := uint32(0)
						domain := "/ppi:ethernet"
						if aggregation {
							id = &zero
							domain += "/aggregation:0"
						}
						raw := ppiPcap(t, [][]byte{ppiFrame(seed[0], id), ppiFrame(seed[1], id)}, 192)
						var events []*ProtocolEvent
						var stats ProtocolStats
						seen := 0
						opts := []CaptureOption{WithProtocolDecodeAs("udp", 18193, "someip-tp"), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
						if observe {
							opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { seen++; require.IsType(t, &capturePPILayer{}, p.Layers()[0]) }))
						}
						require.NoError(t, ReplayPcap(bytes.NewReader(raw), opts...))
						clear(raw)
						require.Len(t, events, 2)
						for i, e := range events {
							require.Equal(t, domain, e.Domain.Encapsulation)
							require.Equal(t, 0, e.Domain.Interface)
							require.Equal(t, want.Steps[i].IDs, e.SourceBytes.ParentPDUs)
							require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
							someIPCheck(t, e, want.Steps[i].Fields, "")
						}
						require.Zero(t, stats.BufferedBytes)
						require.Zero(t, stats.CallbackPanics)
						if observe {
							require.Equal(t, 2, seen)
						}
					})
				}
			}
		}
	}
	t.Run("aggregation-interfaces-do-not-mix", func(t *testing.T) {
		left, right := uint32(11), uint32(12)
		frames := [][]byte{ppiFrame(seed[0], &left), ppiFrame(seed[1], &right), ppiFrame(seed[1], &left)}
		var out []*ProtocolEvent
		require.NoError(t, ReplayPcap(bytes.NewReader(ppiPcap(t, frames, 192)), WithProtocolDecodeAs("udp", 18193, "someip-tp"), WithOnProtocolMessage(func(e *ProtocolEvent) { out = append(out, e) })))
		require.Len(t, out, 3)
		someIPCheck(t, out[0], want.Steps[0].Fields, "")
		someIPCheck(t, out[1], nil, "ContextRequired")
		fields := cloneSession(want.Steps[1].Fields)
		fields["assembly"].(map[string]any)["contributor_event_ids"] = []uint64{1, 3}
		someIPCheck(t, out[2], fields, "")
		require.Equal(t, "/ppi:ethernet/aggregation:11", out[0].Domain.Encapsulation)
		require.Equal(t, "/ppi:ethernet/aggregation:12", out[1].Domain.Encapsulation)
		require.Equal(t, out[0].Domain, out[2].Domain)
		require.Equal(t, []uint64{1, 3}, out[2].SourceBytes.ParentPDUs)
	})
	t.Run("declared-Ethernet-does-not-guess-PPI", func(t *testing.T) {
		var events []*ProtocolEvent
		require.EqualError(t, ReplayPcap(bytes.NewReader(ppiPcap(t, [][]byte{ppiFrame(seed[0], nil)}, layers.LinkTypeEthernet)), WithProtocolDecodeAs("udp", 18193, "someip-tp"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })), "LLC header too small")
		for _, e := range events {
			require.NotEqual(t, "someip", e.Protocol)
		}
	})
}

func TestPPIDCPKeepsTheInnerEthernetOracle(t *testing.T) {
	var selected dcpControl
	for _, c := range dcpControls(t) {
		if len(c.Steps) == 1 && len(c.Messages) == 1 && !c.Steps[0].Truncated {
			selected = c
			break
		}
	}
	require.NotEmpty(t, selected.Name)
	frame := doipDiscoveryWire(t, selected.Steps[0].Frame)
	for _, wrapped := range []bool{false, true} {
		t.Run(fmt.Sprint(wrapped), func(t *testing.T) {
			raw := frame
			link := layers.LinkTypeEthernet
			if wrapped {
				zero := uint32(0)
				raw = ppiFrame(frame, &zero)
				link = 192
			}
			var events []*ProtocolEvent
			var stats ProtocolStats
			require.NoError(t, ReplayPcap(bytes.NewReader(ppiPcap(t, [][]byte{raw}, link)), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
			require.Zero(t, stats.BufferedBytes)
			dcpCheckMessages(t, selected, events, false, false)
			if wrapped {
				require.Equal(t, "/ppi:ethernet/aggregation:0", events[0].Domain.Encapsulation)
			} else {
				require.Empty(t, events[0].Domain.Encapsulation)
			}
		})
	}
}

func TestPPIMetadataRefusalsCannotReachTransport(t *testing.T) {
	seed, _ := ppiSeed(t)
	valid := ppiFrame(seed[0], nil)
	zero := uint32(0)
	aggregate := ppiFrame(seed[0], &zero)
	variants := map[string][]byte{"short": bytes.Clone(valid[:7]), "version": bytes.Clone(valid), "flags": bytes.Clone(valid), "length-underflow": bytes.Clone(valid), "length-overrun": bytes.Clone(valid), "inner-DLT": bytes.Clone(valid), "unknown-field": bytes.Clone(aggregate), "short-field": bytes.Clone(aggregate), "duplicate-aggregation": append(bytes.Clone(aggregate[:16]), aggregate[8:]...)}
	variants["version"][0] = 1
	variants["flags"][1] = 128
	binary.LittleEndian.PutUint16(variants["length-underflow"][2:4], 7)
	binary.LittleEndian.PutUint16(variants["length-overrun"][2:4], 4096)
	binary.LittleEndian.PutUint32(variants["inner-DLT"][4:8], 0x10001)
	binary.LittleEndian.PutUint16(variants["unknown-field"][8:10], 99)
	binary.LittleEndian.PutUint16(variants["short-field"][10:12], 5)
	binary.LittleEndian.PutUint16(variants["duplicate-aggregation"][2:4], 24)
	for name, frame := range variants {
		t.Run(name, func(t *testing.T) {
			p := gopacket.NewPacket(frame, captureLinkDecoder(192), gopacket.Default)
			require.NotNil(t, p.ErrorLayer())
			require.Nil(t, p.NetworkLayer())
			require.Nil(t, p.TransportLayer())
			require.ErrorContains(t, p.ErrorLayer().Error(), "Unable to decode LinkType PPI")
			var events []*ProtocolEvent
			var stats ProtocolStats
			require.NoError(t, ReplayPcap(bytes.NewReader(ppiPcap(t, [][]byte{frame}, 192)), WithProtocolDecodeAs("udp", 18193, "someip-tp"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
			require.Len(t, events, 1)
			require.Equal(t, "ip", events[0].Protocol)
			require.Equal(t, "unrecognized", events[0].Status)
			require.Equal(t, "UnsupportedOrMalformedNetworkLayer", events[0].ExpertCode)
			require.Equal(t, "UnsupportedOrMalformedNetworkLayer", events[0].Error)
			require.Zero(t, stats.BufferedBytes)
			require.Zero(t, stats.CallbackPanics)
		})
	}
}

func TestIPDiagnosticQueriesPreserveRejectedOutcome(t *testing.T) {
	seed, err := trafficfixture.ReadFile("someip-tp/ordered.pcap")
	require.NoError(t, err)
	r, err := pcapgo.NewReader(bytes.NewReader(seed))
	require.NoError(t, err)
	frame, _, err := r.ReadPacketData()
	require.NoError(t, err)
	h := make([]byte, 8)
	h[0] = 1
	binary.LittleEndian.PutUint16(h[2:4], 8)
	binary.LittleEndian.PutUint32(h[4:8], 1)
	raw := append(h, frame...)
	var b bytes.Buffer
	w := pcapgo.NewWriter(&b)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkType(192)))
	require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(raw), Length: len(raw)}, raw))
	var out []*ProtocolEvent
	var stats ProtocolStats
	require.NoError(t, ReplayPcap(bytes.NewReader(b.Bytes()), WithOnProtocolMessage(func(e *ProtocolEvent) { out = append(out, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
	require.Len(t, out, 1)
	e := out[0]
	require.Equal(t, "ip", e.Protocol)
	require.Equal(t, "UnsupportedOrMalformedNetworkLayer", e.ExpertCode)
	original, refusal := e.GetFields()
	require.Error(t, refusal)
	require.Nil(t, original)
	saved := refusal.Error()
	e.Fields = map[string]any{"forged": true}
	e.Structured = map[string]any{"fields": map[string]any{"forged": true}}
	e.Session = map[string]any{"forged": true}
	e.Raw = []byte("fake")
	e.Status = "decoded"
	e.Error = ""
	e.Protocol = "snmp"
	e.Profile = "snmp-native"
	f, err := e.GetFields()
	require.Error(t, err, "a rejected capture diagnostic cannot become a successful field query through public cache edits")
	require.Nil(t, f)
	require.EqualError(t, err, saved)
	d, err := e.Decode()
	require.Nil(t, d)
	require.EqualError(t, err, saved)
	require.Zero(t, stats.BufferedBytes)
}

func TestIPDiagnosticStatusesAndManualCacheCompatibility(t *testing.T) {
	for _, status := range []string{"malformed", "incomplete", "limited", "context-required", "decoded", "deferred"} {
		t.Run(status, func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			session := s.(*captureSession)
			session.events = nil
			session.f.a.networkDiagnostic(captureEvidence{}, gopacket.CaptureInfo{}, status, "OriginalNetworkCode")
			require.Len(t, session.events, 1)
			e := session.events[0]
			require.Equal(t, "OriginalNetworkCode", e.ExpertCode)
			_, original := e.GetFields()
			require.Error(t, original)
			e.Fields = map[string]any{"forged": true}
			e.Structured = map[string]any{"fields": map[string]any{"forged": true}}
			e.Status = "decoded"
			e.Rule = "application-layer.dns_fields"
			e.Error = ""
			f, err := e.GetFields()
			require.Nil(t, f)
			require.EqualError(t, err, original.Error())
			require.Equal(t, "OriginalNetworkCode", e.ExpertCode)
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	t.Run("manual-cache", func(t *testing.T) {
		e := &ProtocolEvent{Transport: "network", Protocol: "ip", Status: "unrecognized", Fields: map[string]any{"manual": true}}
		f, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, map[string]any{"manual": true}, f)
	})
}

func ppiSealedRead(t *testing.T, n string) []byte {
	t.Helper()
	var b []byte
	var err error
	if r := os.Getenv("PPI_TEST_ROOT"); r != "" {
		b, err = os.ReadFile(filepath.Join(r, n))
	} else {
		b, err = trafficfixture.ReadFile("ppi-capture/" + n)
	}
	require.NoError(t, err)
	return b
}
func TestPPISealedReplay(t *testing.T) {
	var m struct {
		Schema string
		Cases  []json.RawMessage
	}
	require.NoError(t, json.Unmarshal(ppiSealedRead(t, "manifest.json"), &m))
	require.Equal(t, "owned-ppi-capture/v1", m.Schema)
	require.Len(t, m.Cases, 21)
	if os.Getenv("PPI_TEST_ROOT") == "" {
		bindPPIAndSDAnswers(t, "ppi-capture", ppiSealedRead(t, "manifest.json"))
	}

	for _, row := range m.Cases {
		var c struct {
			ID, Capture, SHA256 string
			CaptureError        string `json:"capture_error"`
			Packets             int
			Events              []struct {
				Protocol, Profile, Error, Domain, Raw, Source, Destination string
				Fields                                                     map[string]any
				ParentIDs                                                  []uint64 `json:"parent_ids"`
				PacketRefs                                                 []uint64 `json:"packet_refs"`
			}
			CloseIDs [][]uint64 `json:"close_ids"`
		}
		require.NoError(t, json.Unmarshal(row, &c))
		t.Run(c.ID, func(t *testing.T) {
			raw := ppiSealedRead(t, c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/deferred%t/observer%t", workers, deferred, observe), func(t *testing.T) {
							var out []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithProtocolDecodeAs("udp", 18193, "someip-tp"), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { out = append(out, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							input := bytes.Clone(raw)
							err := ReplayPcap(bytes.NewReader(input), opts...)
							clear(input)
							if c.CaptureError != "" {
								require.EqualError(t, err, c.CaptureError)
							} else {
								require.NoError(t, err)
							}
							require.Len(t, out, len(c.Events)+len(c.CloseIDs))
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							if observe {
								require.Equal(t, c.Packets, seen)
							}
							for i, want := range c.Events {
								e := out[i]
								require.Equal(t, want.Protocol, e.Protocol)
								require.Equal(t, want.Profile, e.Profile)
								require.Equal(t, want.Domain, e.Domain.Encapsulation)
								require.Equal(t, want.Raw, hex.EncodeToString(e.Raw))
								require.Equal(t, want.Source, e.Source)
								require.Equal(t, want.Destination, e.Destination)
								require.Len(t, e.SourceBytes.PacketRefs, len(want.PacketRefs))
								for j, n := range want.PacketRefs {
									require.Equal(t, n, e.SourceBytes.PacketRefs[j].Number)
									require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[j].Domain)
								}
								require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
								if len(want.ParentIDs) == 0 {
									require.Empty(t, e.SourceBytes.ParentPDUs)
								} else {
									require.Equal(t, want.ParentIDs, e.SourceBytes.ParentPDUs)
								}
								if e.Protocol == "someip" {
									someIPCheck(t, e, want.Fields, want.Error)
								} else if e.Protocol == "ip" {
									require.Equal(t, want.Error, e.ExpertCode)
									_, er := e.GetFields()
									require.Error(t, er)
									saved := er.Error()
									e.Fields = map[string]any{"forged": true}
									e.Structured = map[string]any{"fields": map[string]any{"forged": true}}
									e.Status = "decoded"
									e.Error = ""
									f, er := e.GetFields()
									require.Nil(t, f)
									require.EqualError(t, er, saved)
								} else {
									f, er := e.GetFields()
									require.NoError(t, er)
									rocEqualFields(t, want.Fields, f)
									mutateRTPSQuery(f)
									mutateRTPSQuery(e.Session)
									e.Fields = map[string]any{"forged": true}
									e.Structured = map[string]any{"fields": map[string]any{"forged": true}}
									clear(e.Raw)
									f, er = e.GetFields()
									require.NoError(t, er)
									rocEqualFields(t, want.Fields, f)
									require.Zero(t, e.ResponseTo)
									require.Zero(t, e.TransactionID)
								}
							}
							for i, ids := range c.CloseIDs {
								e := out[len(c.Events)+i]
								require.Equal(t, ids, e.SourceBytes.ParentPDUs)
								someIPCheck(t, e, nil, "NeedMore")
							}
						})
					}
				}
			}
		})
	}
}

func bindPPIAndSDAnswers(t *testing.T, folder string, manifest []byte) {
	t.Helper()
	var doc struct{ Cases []struct{ ID, SHA256 string } }
	require.NoError(t, json.Unmarshal(manifest, &doc))
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	hash := fmt.Sprintf("%x", sha256.Sum256(manifest))
	for _, c := range doc.Cases {
		bound := 0
		for _, batch := range all {
			for _, v := range batch.Cases {
				if v.ID != folder+"/"+c.ID {
					continue
				}
				require.Equal(t, c.SHA256, v.Input.SHA256)
				for _, e := range v.Expectations {
					var a struct {
						File string `json:"answer_file"`
						SHA  string `json:"answer_sha256"`
					}
					require.NoError(t, json.Unmarshal(e.PayloadConstraints, &a))
					if a.File == "manifest.json" {
						require.Equal(t, hash, a.SHA)
						bound++
					}
				}
			}
		}
		require.Equal(t, 1, bound, c.ID)
	}
}
