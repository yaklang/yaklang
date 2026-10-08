package pcaputil

import (
	"bytes"
	"crypto/sha256"
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
	"sync/atomic"
	"testing"
	"time"
)

type canAnswer struct {
	Wire, Error      string
	Fields           map[string]any
	SelectedFields   map[string]any `json:"selected_fields"`
	SelectedError    string         `json:"selected_error"`
	Selected         bool
	CaptureTruncated bool   `json:"capture_truncated"`
	ReaderError      string `json:"reader_error"`
}

func canControls(t *testing.T) []rocControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("socketcan-j1939/manifest.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []rocControl
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, "pr5013-socketcan-j1939/v1", m.Schema)
	require.Len(t, m.Cases, 55)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	found := map[string]trafficfixture.FrozenCase{}
	for _, batch := range all {
		for _, c := range batch.Cases {
			found[c.ID] = c
		}
	}
	for _, c := range m.Cases {
		v, ok := found["socketcan-j1939/"+c.ID]
		require.True(t, ok)
		require.Equal(t, c.Capture, v.Input.File)
		require.Equal(t, c.SHA256, v.Input.SHA256)
		require.Equal(t, c.Packets, v.Facts.PacketCount)
		require.Len(t, v.Expectations, 1)
		var binding struct {
			AnswerFile string `json:"answer_file"`
			AnswerSHA  string `json:"answer_sha256"`
		}
		require.NoError(t, json.Unmarshal(v.Expectations[0].PayloadConstraints, &binding))
		require.Equal(t, c.Answer, binding.AnswerFile)
		require.Equal(t, c.AnswerSHA, binding.AnswerSHA)
	}
	return m.Cases
}
func canInput(t *testing.T, c rocControl) ([]byte, canAnswer) {
	t.Helper()
	b, err := trafficfixture.ReadFile("socketcan-j1939/" + c.Capture)
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(b)))
	a, err := trafficfixture.ReadFile("socketcan-j1939/" + c.Answer)
	require.NoError(t, err)
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(a)))
	var want canAnswer
	require.NoError(t, json.Unmarshal(a, &want))
	return b, want
}
func canCheck(t *testing.T, a canAnswer, e *ProtocolEvent, deferred, selected bool) {
	t.Helper()
	want, kind, protocol, profile := a.Fields, a.Error, "socketcan", "socketcan-controller-record"
	if selected && a.SelectedFields != nil {
		want = a.SelectedFields
		protocol, profile = "j1939", "j1939-classic-request-name"
	}
	if selected && a.SelectedError != "" {
		want = nil
		kind = a.SelectedError
		protocol, profile = "j1939", "j1939-classic-request-name"
	}
	require.Equal(t, protocol, e.Protocol)
	require.Equal(t, profile, e.Profile)
	require.Equal(t, "can", e.Transport)
	require.Zero(t, e.FlowID)
	require.Zero(t, e.TransactionID)
	require.Zero(t, e.ResponseTo)
	require.Empty(t, e.Source)
	require.Empty(t, e.Destination)
	require.Len(t, e.SourceBytes.PacketRefs, 1)
	require.EqualValues(t, 1, e.SourceBytes.PacketRefs[0].Number)
	require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
	if a.CaptureTruncated {
		require.Empty(t, e.Raw)
	} else {
		require.Equal(t, a.Wire, hex.EncodeToString(e.Raw))
	}
	if want == nil {
		rocTypedError(t, kind, e.sessionError)
		require.NotEmpty(t, e.Error)
		require.Nil(t, e.Session)
		require.Nil(t, e.Structured)
		return
	}
	require.Empty(t, e.Error)
	status := "decoded"
	if deferred {
		status = "deferred"
	}
	require.Equal(t, status, e.Status)
	rocEqualFields(t, want, e.Session)
	f, err := e.GetFields()
	require.NoError(t, err)
	rocEqualFields(t, want, f)
	d, err := e.Decode()
	require.NoError(t, err)
	rocEqualFields(t, want, protocolFields(d))
	f["Header Reserved"].([]byte)[0] ^= 255
	if data := f["Data"].([]byte); len(data) > 0 {
		data[0] ^= 255
	}
	rocEqualFields(t, want, e.Session)
	rocEqualFields(t, want, e.semanticFields)
}
func TestSocketCANSealedReplay(t *testing.T) {
	for _, c := range canControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, want := canInput(t, c)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						for _, selected := range []bool{false, true} {
							t.Run(fmt.Sprintf("w%d/d%v/o%v/s%v", workers, deferred, observe, selected), func(t *testing.T) {
								var events []*ProtocolEvent
								var stats ProtocolStats
								var asm TCPReassemblyStats
								var seen atomic.Int64
								opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { asm = s })}
								if selected {
									opts = append(opts, WithCANDecodeAs(0, "j1939"))
								}
								if observe {
									opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.Equal(t, raw[40:], p.Data()); seen.Add(1) }))
								}
								err := ReplayPcap(bytes.NewReader(raw), opts...)
								if want.ReaderError != "" {
									require.EqualError(t, err, want.ReaderError)
									require.EqualValues(t, 1, asm.TruncatedCaptures)
								} else {
									require.NoError(t, err)
									require.Zero(t, asm.TruncatedCaptures)
								}
								require.Len(t, events, 1)
								canCheck(t, want, events[0], deferred, selected)
								require.Zero(t, stats.BufferedBytes)
								require.Zero(t, asm.UnreassembledBytes)
								require.Zero(t, asm.UnreassembledSegments)
								require.Zero(t, asm.DecodeErrors)
								require.EqualValues(t, 1, asm.CapturedPackets)
								if observe {
									require.EqualValues(t, 1, seen.Load())
								}
							})
						}
					}
				}
			}
		})
	}
}
func TestSocketCANBoundsAndOwnership(t *testing.T) {
	for _, c := range canControls(t) {
		_, a := canInput(t, c)
		if a.Fields == nil {
			continue
		}
		w, err := hex.DecodeString(a.Wire)
		require.NoError(t, err)
		f, err := decodeSocketCAN(w)
		require.NoError(t, err)
		rocEqualFields(t, a.Fields, f)
		if len(w) <= 16 && a.Fields["RTR"] == false {
			for cut := 0; cut < 8+int(a.Fields["Data Length"].(float64)); cut++ {
				got, err := decodeSocketCAN(w[:cut])
				require.Error(t, err)
				require.Nil(t, got)
			}
		}
		for i := range w {
			w[i] ^= 255
		}
		rocEqualFields(t, a.Fields, f)
	}
	for _, class := range []uint32{0, 1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1023, 0x10000} {
		w := make([]byte, 16)
		w[0] = 32
		w[1] = byte(class >> 16)
		w[2] = byte(class >> 8)
		w[3] = byte(class)
		w[4] = 8
		f, err := decodeSocketCAN(w)
		require.NoError(t, err)
		require.Equal(t, class, f["Identifier"])
		require.True(t, f["Error Frame"].(bool))
		require.NotContains(t, f, "J1939")
	}
}
func TestSocketCANResourcesAndSelection(t *testing.T) {
	for _, c := range canControls(t) {
		if c.ID != "header-short-0" {
			continue
		}
		raw, answer := canInput(t, c)
		file := filepath.Join(t.TempDir(), "empty-can.pcap")
		require.NoError(t, os.WriteFile(file, raw, 0600))
		for _, workers := range []int{1, 2, 4} {
			for _, observe := range []bool{false, true} {
				for _, native := range []bool{false, true} {
					var recorded bytes.Buffer
					var events []*ProtocolEvent
					seen := 0
					opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithCaptureWriter(&recorded), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })}
					if observe {
						opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.Empty(t, p.Data()); seen++ }))
					}
					if native {
						require.NoError(t, OpenPcapFile(file, opts...))
					} else {
						require.NoError(t, ReplayPcap(bytes.NewReader(raw), opts...))
					}
					require.Len(t, events, 1)
					rocTypedError(t, answer.Error, events[0].sessionError)
					require.Nil(t, events[0].Session)
					if observe {
						require.Equal(t, 1, seen)
					}
					r, err := pcapgo.NewReader(bytes.NewReader(recorded.Bytes()))
					require.NoError(t, err)
					require.Equal(t, layers.LinkType(227), r.LinkType())
					saved, ci, err := r.ReadPacketData()
					require.NoError(t, err)
					require.Empty(t, saved)
					require.Zero(t, ci.CaptureLength)
					require.Zero(t, ci.Length)
					_, _, err = r.ReadPacketData()
					require.ErrorIs(t, err, io.EOF)
				}
			}
		}
	}
	var wire []byte
	var answer canAnswer
	for _, c := range canControls(t) {
		if c.ID == "j1939-address-claim" {
			_, answer = canInput(t, c)
			wire, _ = hex.DecodeString(answer.Wire)
		}
	}
	require.NotEmpty(t, wire)
	var ng bytes.Buffer
	w, err := pcapgo.NewNgWriter(&ng, layers.LinkType(227))
	require.NoError(t, err)
	id, err := w.AddInterface(pcapgo.NgInterface{LinkType: layers.LinkType(227), SnapLength: 65535})
	require.NoError(t, err)
	require.Equal(t, 1, id)
	for _, iface := range []int{0, 1} {
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(wire), Length: len(wire), InterfaceIndex: iface}, wire))
	}
	require.NoError(t, w.Flush())
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			for _, observe := range []bool{false, true} {
				var events []*ProtocolEvent
				var stats ProtocolStats
				opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithCANDecodeAs(1, "j1939"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
				if observe {
					opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.Equal(t, wire, p.Data()) }))
				}
				require.NoError(t, ReplayPcap(bytes.NewReader(ng.Bytes()), opts...))
				require.Len(t, events, 2)
				rocEqualFields(t, answer.Fields, events[0].Session)
				rocEqualFields(t, answer.SelectedFields, events[1].Session)
				for i, e := range events {
					require.Equal(t, i, e.Domain.Interface)
					require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
					require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.ResponseTo)
				}
				require.Zero(t, stats.BufferedBytes)
			}
		}
	}
	for _, limit := range []int{len(wire) - 1, len(wire)} {
		budget := DefaultParserBudget()
		budget.MaxFrameBytes = limit
		budget.MaxMessageBytes = limit
		var events []*ProtocolEvent
		a := &binParser{budget: budget, canDecodeAs: map[int]string{1: "j1939"}, config: BinParserConfig{OnEvent: func(e *ProtocolEvent) { events = append(events, e) }}}
		for _, iface := range []int{1, 2} {
			ci := gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(wire), Length: len(wire), InterfaceIndex: iface}
			a.decodeCANRecord(wire, captureEvidence{Ref: PacketReference{Number: 1, Domain: CaptureDomain{Section: 3, Interface: iface}}}, ci)
		}
		require.Len(t, events, 2)
		require.NotEqual(t, events[0].Domain, events[1].Domain)
		require.Zero(t, a.stats().BufferedBytes)
		if limit < len(wire) {
			for _, e := range events {
				rocTypedError(t, "ResourceExceeded", e.sessionError)
				require.Empty(t, e.Raw)
				require.Nil(t, e.Session)
			}
		} else {
			rocEqualFields(t, answer.SelectedFields, events[0].Session)
			rocEqualFields(t, answer.Fields, events[1].Session)
			require.Equal(t, "j1939", events[0].Protocol)
			require.Equal(t, "socketcan", events[1].Protocol)
		}
	}
	c := NewDefaultConfig()
	require.Error(t, WithCANDecodeAs(-1, "j1939")(c))
	require.Error(t, WithCANDecodeAs(1, "isotp")(c))
	require.Error(t, WithCANDecodeAs(1, "uds")(c))
	require.NoError(t, WithCANDecodeAs(1, "j1939")(c))
	require.NoError(t, WithCANDecodeAs(1, "j1939")(c))
	for _, order := range []bool{false, true} {
		c = NewDefaultConfig()
		opts := []CaptureOption{WithCANDecodeAs(1, "j1939"), WithBinParserConfig(BinParserConfig{OnEvent: func(*ProtocolEvent) {}})}
		if order {
			opts[0], opts[1] = opts[1], opts[0]
		}
		for _, opt := range opts {
			require.NoError(t, opt(c))
		}
		require.NoError(t, c.prepareBinParser())
		require.Equal(t, "j1939", c.binParser.canDecodeAs[1])
		c.canDecodeAs[1] = "mutated"
		require.Equal(t, "j1939", c.binParser.canDecodeAs[1])
		a := c.binParser
		require.NoError(t, c.finishBinParser())
		require.Zero(t, a.stats().BufferedBytes)
	}
}
