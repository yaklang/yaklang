package pcaputil

import (
	"bytes"
	"context"
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
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tcpCapturePrefixTape(t *testing.T, clip int) []byte {
	t.Helper()
	steps := []tcpStep{{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true}}
	for off := 0; off < len(binMQTTConnect); off += 7 {
		steps = append(steps, tcpStep{seq: uint32(1 + off), data: string(binMQTTConnect[off:])})
	}
	full := binTestPcap(t, steps, 1883, false, false)
	if clip == 0 {
		return full
	}
	rd, err := pcapgo.NewReader(bytes.NewReader(full))
	require.NoError(t, err)
	var b bytes.Buffer
	wr := pcapgo.NewWriterNanos(&b)
	require.NoError(t, wr.WriteFileHeader(65535, rd.LinkType()))
	for {
		raw, ci, e := rd.ReadPacketData()
		if e == io.EOF {
			break
		}
		require.NoError(t, e)
		if len(raw) > 54+clip {
			raw = raw[:54+clip]
			ci.CaptureLength = len(raw)
		}
		require.NoError(t, wr.WritePacket(ci, raw))
	}
	return b.Bytes()
}
func TestTCPCapturePrefixPublicRegression(t *testing.T) {
	for _, clip := range []int{0, 7} {
		for _, workers := range []int{1, 2, 4} {
			for _, observe := range []bool{false, true} {
				name := "complete"
				if clip > 0 {
					name = "captured-prefix"
				}
				t.Run(name+"/"+string(rune('0'+workers))+"/"+map[bool]string{false: "private", true: "observer"}[observe], func(t *testing.T) {
					var opts []CaptureOption
					if observe {
						opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
					}
					events, stats, err := binReplay(t, tcpCapturePrefixTape(t, clip), workers, opts...)
					if clip > 0 {
						require.ErrorContains(t, err, "capture contains truncated packets")
					} else {
						require.NoError(t, err)
					}
					var n int
					for _, e := range events {
						if e.Protocol == "mqtt" && bytes.Equal(e.Raw, binMQTTConnect) {
							n++
							require.Empty(t, e.Error)
						}
					}
					require.Equal(t, 1, n, "captured contiguous prefix bytes should recover the complete MQTT PDU")
					require.GreaterOrEqual(t, stats.Decoded, uint64(1))
					require.Zero(t, stats.BufferedBytes)
				})
			}
		}
	}
}

// Only captured bytes may advance the cursor; FIN belongs after the declared
// wire payload. A later retransmission can fill that suffix, including at wrap.
func TestTCPCapturePrefixFINAndGap(t *testing.T) {
	for _, base := range []uint32{1, 0xfffffff8} {
		for _, ipv6 := range []bool{false, true} {
			for _, fill := range []bool{false, true} {
				for _, worker := range []int{1, 2, 4} {
					for _, observer := range []bool{false, true} {
						t.Run(fmt.Sprintf("base=%d/v6=%v/fill=%v/w%d/observer=%v", base, ipv6, fill, worker, observer), func(t *testing.T) {
							steps := []tcpStep{{seq: base - 1, syn: true}, {seq: base - 1, syn: true, reverse: true}, {seq: base, data: string(binMQTTConnect), fin: true}}
							if fill {
								steps = append(steps, tcpStep{seq: base + 7, data: string(binMQTTConnect[7:])}, tcpStep{seq: base + uint32(len(binMQTTConnect)), fin: true})
							}
							data := tcpCapturePrefixClip(t, binTestPcap(t, steps, 1883, ipv6, false), func(i int, raw []byte, ci *gopacket.CaptureInfo) []byte {
								if i == 2 {
									h := 54
									if ipv6 {
										h = 74
									}
									raw = raw[:h+7]
									ci.CaptureLength = len(raw)
								}
								return raw
							})
							var assembly TCPReassemblyStats
							var delivered []byte
							opts := []CaptureOption{WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s }), WithOnTrafficFlowOnDataFrameArrived(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) {
								delivered = append(delivered, f.Payload...)
							})}
							if observer {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
							}
							events, stats, err := binReplay(t, data, worker, opts...)
							require.ErrorContains(t, err, "capture contains truncated packets")
							require.EqualValues(t, 1, assembly.TruncatedCaptures)
							require.Zero(t, assembly.DecodeErrors)
							require.Zero(t, assembly.InvalidSegments)
							require.Zero(t, stats.BufferedBytes)
							if fill {
								require.Equal(t, binMQTTConnect, delivered)
								require.Len(t, events, 1)
								require.Equal(t, binMQTTConnect, events[0].Raw)
								require.Empty(t, events[0].Error)
								require.Zero(t, assembly.UnreassembledSegments)
							} else {
								require.Equal(t, binMQTTConnect[:7], delivered)
								require.Zero(t, stats.Decoded)
								require.NotEmpty(t, events)
								for _, e := range events {
									require.Equal(t, "incomplete", e.Status)
								}
								require.EqualValues(t, 1, assembly.UnreassembledSegments)
							}
							for i := range data {
								data[i] = 0
							}
							if fill {
								require.Equal(t, binMQTTConnect, events[0].Raw)
							}
						})
					}
				}
			}
		}
	}
}

func tcpCapturePrefixClip(t *testing.T, full []byte, mutate func(int, []byte, *gopacket.CaptureInfo) []byte) []byte {
	t.Helper()
	rd, err := pcapgo.NewReader(bytes.NewReader(full))
	require.NoError(t, err)
	var b bytes.Buffer
	wr := pcapgo.NewWriterNanos(&b)
	require.NoError(t, wr.WriteFileHeader(65535, rd.LinkType()))
	for i := 0; ; i++ {
		raw, ci, e := rd.ReadPacketData()
		if e == io.EOF {
			break
		}
		require.NoError(t, e)
		raw = mutate(i, raw, &ci)
		require.NoError(t, wr.WritePacket(ci, raw))
	}
	return b.Bytes()
}

func TestTCPCapturePrefixRejectHeadersAndWireLength(t *testing.T) {
	for _, mode := range []string{"tcp-header", "tcp-options", "false-wire-length", "ip-length-beyond-wire"} {
		for _, worker := range []int{1, 2, 4} {
			for _, observer := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/w%d/observer=%v", mode, worker, observer), func(t *testing.T) {
					steps := []tcpStep{{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true}, {seq: 1, data: string(binMQTTConnect)}, {seq: 1, data: string(binMQTTConnect)}}
					pcap := tcpCapturePrefixClip(t, binTestPcap(t, steps, 1883, false, false), func(i int, raw []byte, ci *gopacket.CaptureInfo) []byte {
						if i != 2 {
							return raw
						}
						switch mode {
						case "tcp-header":
							raw = raw[:50]
						case "tcp-options":
							raw[46] = 0xf0
							raw = raw[:61]
						case "false-wire-length":
							raw = raw[:61]
							ci.Length = len(raw)
						case "ip-length-beyond-wire":
							binary.BigEndian.PutUint16(raw[16:18], 100)
							raw = raw[:61]
						}
						ci.CaptureLength = len(raw)
						return raw
					})
					var assembly TCPReassemblyStats
					opts := []CaptureOption{WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s })}
					if observer {
						opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
					}
					events, stats, err := binReplay(t, pcap, worker, opts...)
					require.Error(t, err)
					require.Positive(t, assembly.DecodeErrors)
					require.Len(t, events, 1)
					require.Equal(t, binMQTTConnect, events[0].Raw)
					require.Empty(t, events[0].Error)
					require.EqualValues(t, 1, stats.Decoded)
					require.Zero(t, stats.BufferedBytes)
				})
			}
		}
	}
}

func TestTCPCapturePrefixFINBudgetRelease(t *testing.T) {
	p := newTrafficPool(context.Background(), TCPReassemblyOptions{MaxPendingSegments: 1})
	defer p.Close()
	ip := &layers.IPv4{SrcIP: net.IPv4(192, 0, 2, 1), DstIP: net.IPv4(192, 0, 2, 2)}
	var flow *TrafficFlow
	var reason TrafficFlowCloseReason
	p.onFlowCreated = func(f *TrafficFlow) { flow = f }
	p.onFlowClosed = func(r TrafficFlowCloseReason, _ *TrafficFlow) { reason = r }
	p.Feed(nil, ip, &layers.TCP{SrcPort: 12345, DstPort: 1883, SYN: true, Seq: 0})
	future := &layers.TCP{SrcPort: 12345, DstPort: 1883, Seq: 10}
	future.Payload = []byte("x")
	p.Feed(nil, ip, future)
	part := &layers.TCP{SrcPort: 12345, DstPort: 1883, Seq: 1, FIN: true}
	part.Payload = []byte("1234567")
	p.feedEvidence(nil, ip, part, captureEvidence{tcpMissingTail: 9}, time.Unix(100, 0))
	require.True(t, flow.IsClosed())
	require.Equal(t, TrafficFlowCloseReason_RESOURCE_LIMIT, reason)
	require.Zero(t, p.pendingBytes)
	require.Zero(t, p.pendingSegments)
	p.Close()
	require.Zero(t, p.pendingBytes)
	require.Zero(t, p.pendingSegments)
}

func tcpCapturePrefixFile(t *testing.T, name string) []byte {
	t.Helper()
	if root := os.Getenv("TCP_CAPTURE_PREFIX_TEST_ROOT"); root != "" {
		b, e := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, e)
		return b
	}
	b, e := trafficfixture.ReadFile("tcp-capture-prefix/" + name)
	require.NoError(t, e)
	return b
}
func TestTCPCapturePrefixSealedMatrix(t *testing.T) {
	var manifest struct {
		Cases []struct {
			ID, Capture, Answer, SHA256 string
			AnswerSHA                   string `json:"answer_sha256"`
			Packets                     int
		}
	}
	require.NoError(t, json.Unmarshal(tcpCapturePrefixFile(t, "manifest.json"), &manifest))
	require.Len(t, manifest.Cases, 24)
	for _, c := range manifest.Cases {
		t.Run(c.ID, func(t *testing.T) {
			cap := tcpCapturePrefixFile(t, c.Capture)
			answer := tcpCapturePrefixFile(t, c.Answer)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(cap)))
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
			var a struct {
				Stream          string `json:"expected_stream_hex"`
				Decoded         int
				Truncated       int  `json:"capture_truncated"`
				Bad             bool `json:"decode_error"`
				Gap             bool `json:"unfilled_fin"`
				Fields, Session map[string]any
			}
			require.NoError(t, json.Unmarshal(answer, &a))
			want, err := hex.DecodeString(a.Stream)
			require.NoError(t, err)
			for _, deferred := range []bool{false, true} {
				for _, worker := range []int{1, 2, 4} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("deferred=%v/w%d/observer=%v", deferred, worker, observe), func(t *testing.T) {
							var es []*ProtocolEvent
							var assembly TCPReassemblyStats
							var stats ProtocolStats
							var delivered []byte
							opts := []CaptureOption{WithTCPReassemblyWorkers(worker), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s }), WithOnTrafficFlowOnDataFrameArrived(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) {
								delivered = append(delivered, f.Payload...)
							})}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
							}
							err := ReplayPcap(bytes.NewReader(cap), opts...)
							require.ErrorContains(t, err, "capture contains truncated packets")
							require.Equal(t, want, delivered)
							require.EqualValues(t, c.Packets, assembly.CapturedPackets)
							require.EqualValues(t, a.Truncated, assembly.TruncatedCaptures)
							require.Equal(t, a.Bad, assembly.DecodeErrors > 0)
							require.Zero(t, assembly.InvalidSegments)
							require.Zero(t, stats.BufferedBytes)
							require.EqualValues(t, len(want), stats.InputBytes)
							if a.Gap {
								require.EqualValues(t, 1, assembly.UnreassembledSegments)
								require.Zero(t, stats.Decoded)
								for _, e := range es {
									require.Equal(t, "incomplete", e.Status)
								}
								require.NotEmpty(t, es)
								return
							}
							require.Zero(t, assembly.UnreassembledSegments)
							require.Len(t, es, 1)
							e := es[0]
							require.Equal(t, "mqtt", e.Protocol)
							require.Equal(t, "mqtt", e.Profile)
							require.Equal(t, 0, e.Direction)
							require.EqualValues(t, 1, e.ID)
							require.Zero(t, e.ResponseTo)
							require.Empty(t, e.Error)
							require.Equal(t, binMQTTConnect, e.Raw)
							if deferred {
								require.EqualValues(t, a.Decoded, stats.Deferred)
								require.Zero(t, stats.Decoded)
							} else {
								require.EqualValues(t, a.Decoded, stats.Decoded)
								require.Zero(t, stats.Deferred)
							}
							require.NotEmpty(t, e.FlowID)
							rocEqualFields(t, a.Session, e.Session)
							for _, fn := range []func() (map[string]any, error){e.GetFields, e.Decode} {
								f, err := fn()
								require.NoError(t, err)
								rocEqualFields(t, a.Fields, protocolFields(f))
								clear(f)
							}
							saved := bytes.Clone(e.Raw)
							clear(e.Raw)
							clear(e.Session)
							f, err := e.GetFields()
							require.NoError(t, err)
							rocEqualFields(t, a.Fields, protocolFields(f))
							require.Equal(t, binMQTTConnect, saved)
						})
					}
				}
			}
		})
	}
}
