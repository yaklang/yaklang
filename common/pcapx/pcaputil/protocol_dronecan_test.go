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
	"testing"
	"time"
)

func TestDroneCANSealedReplay(t *testing.T) {
	b, err := trafficfixture.ReadFile("dronecan-node-status/manifest.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []rocControl
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Len(t, m.Cases, 53)
	require.Equal(t, "pr5013-dronecan-node-status/v1", m.Schema)
	frozen, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	bound := map[string]trafficfixture.FrozenCase{}
	for _, batch := range frozen {
		for _, entry := range batch.Cases {
			bound[entry.ID] = entry
		}
	}
	for _, c := range m.Cases {
		t.Run(c.ID, func(t *testing.T) {
			raw, err := trafficfixture.ReadFile("dronecan-node-status/" + c.Capture)
			require.NoError(t, err)
			b, err := trafficfixture.ReadFile("dronecan-node-status/" + c.Answer)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(b)))
			v, ok := bound["dronecan-node-status/"+c.ID]
			require.True(t, ok)
			require.Equal(t, c.Capture, v.Input.File)
			require.Equal(t, c.SHA256, v.Input.SHA256)
			require.Equal(t, 1, v.Facts.PacketCount)
			require.Len(t, v.Expectations, 1)
			var binding struct {
				AnswerFile string `json:"answer_file"`
				AnswerSHA  string `json:"answer_sha256"`
			}
			require.NoError(t, json.Unmarshal(v.Expectations[0].PayloadConstraints, &binding))
			require.Equal(t, c.Answer, binding.AnswerFile)
			require.Equal(t, c.AnswerSHA, binding.AnswerSHA)
			var a struct {
				Wire, Error, Protocol string
				BaseFields            map[string]any `json:"base_fields"`
				BaseError             string         `json:"base_error"`
				Fields                map[string]any
				CaptureTruncated      bool   `json:"capture_truncated"`
				ReadError             string `json:"reader_error"`
			}
			require.NoError(t, json.Unmarshal(b, &a))
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						for _, selected := range []bool{false, true} {
							var ev []*ProtocolEvent
							var ps ProtocolStats
							var asm TCPReassemblyStats
							seen := 0
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { ev = append(ev, e) }), WithOnProtocolStats(func(s ProtocolStats) { ps = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { asm = s })}
							if selected {
								opts = append(opts, WithCANDecodeAs(0, "dronecan"))
							}
							if observe {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.Equal(t, a.Wire, hex.EncodeToString(p.Data())); seen++ }))
							}
							err := ReplayPcap(bytes.NewReader(raw), opts...)
							if a.ReadError != "" {
								require.EqualError(t, err, a.ReadError)
							} else {
								require.NoError(t, err)
							}
							require.Len(t, ev, 1)
							e := ev[0]
							want, kind, protocol := a.BaseFields, a.BaseError, "socketcan"
							if selected {
								want, kind, protocol = a.Fields, a.Error, a.Protocol
							}
							require.Equal(t, protocol, e.Protocol)
							profile := "socketcan-controller-record"
							if protocol == "dronecan" {
								profile = "dronecan-v0-node-status"
							}
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
							if kind != "" {
								require.NotEmpty(t, e.Error)
								rocTypedError(t, kind, e.sessionError)
								require.Nil(t, e.Session)
								require.Nil(t, e.Structured)
							} else {
								require.Empty(t, e.Error)
								status := "decoded"
								if deferred {
									status = "deferred"
								}
								require.Equal(t, status, e.Status)
								rocEqualFields(t, want, e.Session)
								fields, err := e.GetFields()
								require.NoError(t, err)
								rocEqualFields(t, want, fields)
								d, err := e.Decode()
								require.NoError(t, err)
								rocEqualFields(t, want, protocolFields(d))
								if df, ok := fields["DroneCAN"].(map[string]any); ok {
									df["Uptime Seconds"] = 999
								}
								rocEqualFields(t, want, e.Session)
							}
							require.Zero(t, ps.BufferedBytes)
							require.Zero(t, asm.UnreassembledBytes)
							require.Zero(t, asm.UnreassembledSegments)
							require.Zero(t, asm.DecodeErrors)
							require.EqualValues(t, 1, asm.CapturedPackets)
							if observe {
								require.Equal(t, 1, seen)
							}
						}
					}
				}
			}
		})
	}
}

func TestDroneCANLayoutAndBounds(t *testing.T) {
	// Independently pack every possible health/mode/sub-mode octet. Receiving
	// reserved Mode and nonzero ignored SubMode is valid and must retain the bits.
	for health := 0; health < 4; health++ {
		for mode := 0; mode < 8; mode++ {
			for sub := 0; sub < 8; sub++ {
				data := []byte{0x78, 0x56, 0x34, 0x12, byte(health*64 + mode*8 + sub), 0xcd, 0xab, 0xdf}
				f, err := decodeDroneCANNodeStatus(0x1f01557f, data)
				require.NoError(t, err)
				require.EqualValues(t, 0x12345678, f["Uptime Seconds"])
				require.EqualValues(t, health, f["Health"])
				require.EqualValues(t, mode, f["Mode"])
				require.EqualValues(t, sub, f["Sub Mode"])
				require.EqualValues(t, 0xabcd, f["Vendor Status Code"])
				require.EqualValues(t, 31, f["Transfer ID"])
				require.EqualValues(t, 31, f["Priority"])
				require.EqualValues(t, 127, f["Source Node ID"])
				clear(data)
				require.EqualValues(t, 0x12345678, f["Uptime Seconds"])
			}
		}
	}
	for cut := 0; cut < 8; cut++ {
		data := []byte{0x78, 0x56, 0x34, 0x12, 0, 0xcd, 0xab, 0xdf}
		f, err := decodeDroneCANNodeStatus(0x1001552a, data[:cut])
		require.Error(t, err)
		require.Nil(t, f)
	}
}

func TestDroneCANResourcesAndSelection(t *testing.T) {
	raw, err := trafficfixture.ReadFile("dronecan-node-status/captures/node-operational.pcap")
	require.NoError(t, err)
	wire := raw[40:]
	for _, limit := range []int{15, 16} {
		for _, deferred := range []bool{false, true} {
			budget := DefaultParserBudget()
			budget.MaxFrameBytes = limit
			budget.MaxMessageBytes = limit
			budget.MaxBufferedBytes = 1
			budget.MaxCollectionElements = 1
			var ev []*ProtocolEvent
			a := &binParser{budget: budget, canDecodeAs: map[int]string{1: "dronecan"}, config: BinParserConfig{Deferred: deferred, MaxBufferedBytes: 1, OnEvent: func(e *ProtocolEvent) { ev = append(ev, e) }}}
			input := bytes.Clone(wire)
			for _, iface := range []int{1, 2} {
				ci := gopacket.CaptureInfo{CaptureLength: 16, Length: 16, InterfaceIndex: iface}
				a.decodeCANRecord(input, captureEvidence{Ref: PacketReference{Number: 9, Domain: CaptureDomain{Section: 4, Interface: iface}}}, ci)
			}
			clear(input)
			require.Len(t, ev, 2)
			require.NotEqual(t, ev[0].Domain, ev[1].Domain)
			for i, e := range ev {
				require.EqualValues(t, 9, e.SourceBytes.PacketRefs[0].Number)
				require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
				require.Zero(t, e.FlowID)
				require.Zero(t, e.TransactionID)
				require.Zero(t, e.ResponseTo)
				if limit < 16 {
					rocTypedError(t, "ResourceExceeded", e.sessionError)
					require.Empty(t, e.Raw)
					require.Nil(t, e.Session)
				} else {
					require.Equal(t, wire, e.Raw)
					f, err := e.GetFields()
					require.NoError(t, err)
					if i == 0 {
						require.Equal(t, "dronecan", e.Protocol)
						df := f["DroneCAN"].(map[string]any)
						require.EqualValues(t, 0x12345678, df["Uptime Seconds"])
						df["Uptime Seconds"] = 0
						require.EqualValues(t, 0x12345678, e.Session["DroneCAN"].(map[string]any)["Uptime Seconds"])
					} else {
						require.Equal(t, "socketcan", e.Protocol)
						require.NotContains(t, f, "DroneCAN")
					}
				}
			}
			require.Zero(t, a.stats().BufferedBytes)
		}
	}
	// FD and controller-error records remain carrier observations even on an
	// explicitly selected classic-only interface. Their complete prior answers apply.
	for _, c := range canControls(t) {
		if c.ID != "fd-sixty-four" && c.ID != "controller-all-errors" {
			continue
		}
		raw, answer := canInput(t, c)
		var ev []*ProtocolEvent
		require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithCANDecodeAs(0, "dronecan"), WithOnProtocolMessage(func(e *ProtocolEvent) { ev = append(ev, e) })))
		require.Len(t, ev, 1)
		require.Equal(t, "socketcan", ev[0].Protocol)
		rocEqualFields(t, answer.Fields, ev[0].Session)
		require.NotContains(t, ev[0].Session, "DroneCAN")
	}
	for _, order := range []bool{false, true} {
		c := NewDefaultConfig()
		opts := []CaptureOption{WithCANDecodeAs(1, "dronecan"), WithBinParserConfig(BinParserConfig{OnEvent: func(*ProtocolEvent) {}})}
		if order {
			opts[0], opts[1] = opts[1], opts[0]
		}
		for _, opt := range opts {
			require.NoError(t, opt(c))
		}
		require.NoError(t, WithCANDecodeAs(1, "dronecan")(c))
		require.Error(t, WithCANDecodeAs(1, "j1939")(c))
		require.Error(t, WithCANDecodeAs(-1, "dronecan")(c))
		require.Error(t, WithCANDecodeAs(1, "cyphal")(c))
		require.NoError(t, c.prepareBinParser())
		a := c.binParser
		require.Equal(t, "dronecan", a.canDecodeAs[1])
		c.canDecodeAs[1] = "j1939"
		require.Equal(t, "dronecan", a.canDecodeAs[1])
		require.NoError(t, c.finishBinParser())
		require.Zero(t, a.stats().BufferedBytes)
	}
}

func TestDroneCANMultiInterface(t *testing.T) {
	raw, err := trafficfixture.ReadFile("dronecan-node-status/captures/node-operational.pcap")
	require.NoError(t, err)
	wire := raw[40:]
	var ng bytes.Buffer
	w, err := pcapgo.NewNgWriter(&ng, layers.LinkType(227))
	require.NoError(t, err)
	id, err := w.AddInterface(pcapgo.NgInterface{LinkType: layers.LinkType(227), SnapLength: 65535})
	require.NoError(t, err)
	require.Equal(t, 1, id)
	for _, iface := range []int{0, 1} {
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, 0), CaptureLength: len(wire), Length: len(wire), InterfaceIndex: iface}, wire))
	}
	require.NoError(t, w.Flush())
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			for _, observe := range []bool{false, true} {
				var ev []*ProtocolEvent
				var stats ProtocolStats
				seen := 0
				opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithCANDecodeAs(1, "dronecan"), WithOnProtocolMessage(func(e *ProtocolEvent) { ev = append(ev, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
				if observe {
					opts = append(opts, WithEveryPacket(func(p gopacket.Packet) {
						require.Equal(t, wire, p.Data())
						require.Equal(t, seen, p.Metadata().CaptureInfo.InterfaceIndex)
						seen++
					}))
				}
				require.NoError(t, ReplayPcap(bytes.NewReader(ng.Bytes()), opts...))
				require.Len(t, ev, 2)
				require.Equal(t, "socketcan", ev[0].Protocol)
				require.Equal(t, "dronecan", ev[1].Protocol)
				require.NotContains(t, ev[0].Session, "DroneCAN")
				f, err := ev[1].GetFields()
				require.NoError(t, err)
				require.EqualValues(t, 0x12345678, f["DroneCAN"].(map[string]any)["Uptime Seconds"])
				for i, e := range ev {
					require.Equal(t, i, e.Domain.Interface)
					require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
					require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
					require.Zero(t, e.FlowID)
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.ResponseTo)
				}
				require.Zero(t, stats.BufferedBytes)
				if observe {
					require.Equal(t, 2, seen)
				}
			}
		}
	}
}
