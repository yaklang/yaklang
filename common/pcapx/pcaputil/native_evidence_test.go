package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// The same frozen inputs must retain packet contributors through native
// libpcap and the driver-free reader, regardless of observers or worker count.
func TestNativePacketSourceEvidence(t *testing.T) {
	for _, name := range []string{"ipv4-in-order", "ipv6-tcp-wrap-reorder-duplicate"} {
		t.Run(name, func(t *testing.T) {
			raw, err := trafficfixture.ReadFile("core-adversarial/captures/" + name + ".pcap")
			require.NoError(t, err)
			answer, err := trafficfixture.ReadFile("core-adversarial/answers/" + name + ".json")
			require.NoError(t, err)
			var want struct {
				Messages []struct{ Protocol, Raw string }
				Refs     [][]uint64
			}
			require.NoError(t, json.Unmarshal(answer, &want))
			require.Len(t, want.Messages, 1)
			reader, err := NewCaptureReader(bytes.NewReader(raw))
			require.NoError(t, err)
			var packets [][]byte
			var infos []gopacket.CaptureInfo
			for {
				w, ci, err := reader.ReadPacketData()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				packets = append(packets, bytes.Clone(w))
				infos = append(infos, ci)
			}
			require.NotEmpty(t, packets)
			file := filepath.Join(t.TempDir(), name+".pcap")
			require.NoError(t, os.WriteFile(file, raw, 0600))
			var baselineFields map[string]any
			var baselineRefs []PacketReference
			for _, native := range []bool{false, true} {
				for _, workers := range []int{1, 2, 4} {
					for _, deferred := range []bool{false, true} {
						for _, observe := range []bool{false, true} {
							t.Run(fmt.Sprintf("native%v/w%d/d%v/o%v", native, workers, deferred, observe), func(t *testing.T) {
								var mu sync.Mutex
								var ev []*ProtocolEvent
								var stats ProtocolStats
								var asm TCPReassemblyStats
								seen := 0
								created := 0
								opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); ev = append(ev, e); mu.Unlock() }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { asm = s })}
								if observe {
									opts = append(opts, WithEveryPacket(func(p gopacket.Packet) {
										require.Equal(t, packets[seen], p.Data())
										ci := p.Metadata().CaptureInfo
										require.Equal(t, infos[seen].Timestamp, ci.Timestamp)
										require.Equal(t, infos[seen].CaptureLength, ci.CaptureLength)
										require.Equal(t, infos[seen].Length, ci.Length)
										require.EqualValues(t, seen+1, CapturePacketReference(ci).Number)
										seen++
									}))
								}
								if native {
									opts = append(opts, WithNetInterfaceCreated(func(*PcapHandleWrapper) { created++ }))
									require.NoError(t, OpenPcapFile(file, opts...))
									require.Equal(t, 1, created)
								} else {
									require.NoError(t, ReplayPcap(bytes.NewReader(raw), opts...))
									require.Zero(t, created)
								}
								require.Len(t, ev, 1)
								e := ev[0]
								require.Equal(t, want.Messages[0].Protocol, e.Protocol)
								require.Equal(t, want.Messages[0].Raw, hex.EncodeToString(e.Raw))
								require.Empty(t, e.Error)
								f, err := e.GetFields()
								require.NoError(t, err)
								require.NotEmpty(t, f)
								if baselineFields == nil {
									encoded, err := json.Marshal(f)
									require.NoError(t, err)
									require.NoError(t, json.Unmarshal(encoded, &baselineFields))
									baselineRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
								} else {
									rocEqualFields(t, baselineFields, f)
									require.Equal(t, baselineRefs, e.SourceBytes.PacketRefs)
								}
								require.NotEmpty(t, e.SourceBytes.PacketRefs)
								for _, ref := range e.SourceBytes.PacketRefs {
									require.Greater(t, ref.Number, uint64(0))
									require.LessOrEqual(t, ref.Number, uint64(len(packets)))
									require.Equal(t, e.Domain, ref.Domain)
								}
								if want.Refs != nil {
									var nums []uint64
									for _, ref := range e.SourceBytes.PacketRefs {
										nums = append(nums, ref.Number)
									}
									require.Equal(t, want.Refs[0], nums)
								}
								require.Zero(t, stats.BufferedBytes)
								require.Zero(t, asm.UnreassembledBytes)
								require.Zero(t, asm.UnreassembledSegments)
								require.Zero(t, stats.CallbackPanics)
								require.Zero(t, asm.DecodeErrors)
								require.EqualValues(t, len(packets), asm.CapturedPackets)
								if observe {
									require.Equal(t, len(packets), seen)
								}
							})
						}
					}
				}
			}
		})
	}
}

func TestNativeCANEmptyRecordEvidence(t *testing.T) {
	valid, err := trafficfixture.ReadFile("socketcan-j1939/captures/standard-binary.pcap")
	require.NoError(t, err)
	empty, err := trafficfixture.ReadFile("socketcan-j1939/captures/header-short-0.pcap")
	require.NoError(t, err)
	raw := append(bytes.Clone(valid), empty[24:]...)
	raw = append(raw, valid[24:]...)
	file := filepath.Join(t.TempDir(), "empty-between-can.pcap")
	require.NoError(t, os.WriteFile(file, raw, 0600))
	for _, native := range []bool{false, true} {
		for _, workers := range []int{1, 2, 4} {
			for _, observe := range []bool{false, true} {
				var events []*ProtocolEvent
				var stats ProtocolStats
				var asm TCPReassemblyStats
				seen := 0
				opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { asm = s })}
				if observe {
					opts = append(opts, WithEveryPacket(func(p gopacket.Packet) {
						want := valid[40:]
						if seen == 1 {
							want = nil
						}
						require.True(t, bytes.Equal(want, p.Data()), "exact observed record bytes")
						require.EqualValues(t, seen+1, CapturePacketReference(p.Metadata().CaptureInfo).Number)
						seen++
					}))
				}
				if native {
					opts = append(opts, WithNetInterfaceCreated(func(*PcapHandleWrapper) {}))
					require.NoError(t, OpenPcapFile(file, opts...))
				} else {
					require.NoError(t, ReplayPcap(bytes.NewReader(raw), opts...))
				}
				require.Len(t, events, 3)
				for i, e := range events {
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
					require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
					if i == 1 {
						rocTypedError(t, "MalformedMessage", e.sessionError)
						require.Empty(t, e.Raw)
						require.Nil(t, e.Session)
					} else {
						require.Equal(t, valid[40:], e.Raw)
						require.Empty(t, e.Error)
					}
				}
				require.Zero(t, stats.BufferedBytes)
				require.EqualValues(t, 3, asm.CapturedPackets)
				if observe {
					require.Equal(t, 3, seen)
				}
			}
		}
	}
}

func TestNativeReaderOrdinalFailures(t *testing.T) {
	raw, err := trafficfixture.ReadFile("socketcan-j1939/captures/standard-binary.pcap")
	require.NoError(t, err)
	file := filepath.Join(t.TempDir(), "two-can.pcap")
	require.NoError(t, os.WriteFile(file, append(bytes.Clone(raw), raw[24:]...), 0600))
	handle, err := OpenFile(file)
	require.NoError(t, err)
	w := WrapPcapHandle(handle)
	defer w.close()
	data, ci, err := w.ReadPacketData()
	require.NoError(t, err)
	require.Equal(t, raw[40:], data)
	require.EqualValues(t, 1, CapturePacketReference(ci).Number)
	data, ci, err = w.ZeroCopyReadPacketData()
	require.NoError(t, err)
	require.Equal(t, raw[40:], data)
	require.EqualValues(t, 2, CapturePacketReference(ci).Number)
	for _, read := range []func() ([]byte, gopacket.CaptureInfo, error){w.ReadPacketData, w.ZeroCopyReadPacketData} {
		_, _, err = read()
		require.ErrorIs(t, err, io.EOF)
		require.EqualValues(t, 2, w.packetNumber.Load())
	}
	w.close()
	_, _, err = w.ReadPacketData()
	require.Error(t, err)
	require.EqualValues(t, 2, w.packetNumber.Load())
	original := gopacket.CaptureInfo{InterfaceIndex: 7, AncillaryData: []interface{}{"driver", captureEvidence{Ref: PacketReference{Number: 99, Domain: CaptureDomain{Section: 3, Interface: 7, Encapsulation: "/vlan:9"}}}}}
	numbered := w.numberCapture(original)
	require.Equal(t, CapturePacketReference(original), CapturePacketReference(numbered))
	require.Equal(t, "driver", numbered.AncillaryData[0])
	numbered.AncillaryData[0] = "mutated"
	require.Equal(t, "driver", original.AncillaryData[0])
}
