package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestTCPCallbacksNormalizeCarriersWithoutProtocolSubscription(t *testing.T) {
	request := "GET /worker HTTP/1.1\r\nHost: tunnel.test\r\n\r\n"
	response := "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello"
	for _, profile := range []string{"ipip", "6in4", "gre", "vxlan", "geneve"} {
		original, err := trafficfixture.ReadFile("industrial-link-core/carriers/captures/tunnel-" + profile + "-http-worker-transaction.pcap")
		require.NoError(t, err)
		for _, variant := range []string{"plain", "fragments", "reordered-fragments"} {
			wire := original
			if variant != "plain" {
				wire = fragmentedTunnelCapture(t, original, variant == "reordered-fragments")
			}
			var reference string
			for _, workers := range []int{1, 2, 4} {
				for _, protocols := range []bool{false, true} {
					for _, packets := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/%s/w%d/protocols%t/packets%t", profile, variant, workers, protocols, packets), func(t *testing.T) {
							var mu sync.Mutex
							var endpoints [][2]string
							var domains []CaptureDomain
							var data [2][]byte
							var sources []string
							pools := map[*TrafficPool]bool{}
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithTCPReassemblyStream(4096),
								WithOnTrafficFlowCreated(func(f *TrafficFlow) {
									mu.Lock()
									defer mu.Unlock()
									pools[f.pool] = true
									endpoints = append(endpoints, [2]string{f.ClientConn.LocalAddr().String(), f.ServerConn.LocalAddr().String()})
									domains = append(domains, f.CaptureDomain())
								}), WithOnTrafficFlowPacket(func(f *TrafficFlow, c *TrafficConnection, refs []PacketReference, ts time.Time) {
									mu.Lock()
									defer mu.Unlock()
									sources = append(sources, fmt.Sprintf("%d/%t/%v/%s", f.Index, c == f.ClientConn, refs, ts.UTC().Format(time.RFC3339Nano)))
								}), WithOnTrafficFlowOnDataFrameArrived(func(f *TrafficFlow, c *TrafficConnection, frame *TrafficFrame) {
									mu.Lock()
									defer mu.Unlock()
									direction := 0
									if c != f.ClientConn {
										direction = 1
									}
									data[direction] = append(data[direction], frame.Payload...)
								})}
							if protocols {
								opts = append(opts, WithOnProtocolMessage(func(*ProtocolEvent) {}))
							}
							if packets {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
							}
							require.NoError(t, ReplayPcap(bytes.NewReader(wire), opts...))
							require.Len(t, endpoints, 1, "inner directions form one physical TCP generation")
							want := [2]string{"192.0.2.11:41001", "192.0.2.12:80"}
							if profile == "6in4" {
								want = [2]string{"[2001:db8::11]:41001", "[2001:db8::12]:80"}
							}
							require.Equal(t, want, endpoints[0], "outer carrier addresses cannot be paired with inner TCP ports")
							require.Equal(t, request, string(data[0]))
							require.Equal(t, response, string(data[1]))
							facts := fmt.Sprintf("%v/%v/%v/%q", endpoints, domains, sources, data)
							if reference == "" {
								reference = facts
							} else {
								require.Equal(t, reference, facts, "callbacks cannot change TCP peers, byte order, domains, references or capture times")
							}
							generationPoolsReleased(t, pools)
						})
					}
				}
			}
		}
	}
}

func TestTCPOnlyNormalizationRejectsUnprovedInnerStreamsAndReleasesFragments(t *testing.T) {
	valid := tunnelWorkerWire(t, "geneve", false, 99, true, false, "")
	version := bytes.Clone(valid)
	version[42] = 0x40
	critical := bytes.Clone(valid)
	critical[43] = 0x40
	options := bytes.Clone(valid)
	options[42] = 1
	shortUDP := bytes.Clone(valid)
	binary.BigEndian.PutUint16(shortUDP[38:40], 8)
	missing := icmpFragmentFrames(t, valid, 32, false)
	for _, tc := range []struct {
		name   string
		frames [][]byte
	}{
		{"Geneve-unselected-version", [][]byte{version}},
		{"Geneve-critical-options-unavailable", [][]byte{critical}},
		{"Geneve-option-body-unavailable", [][]byte{options}},
		{"TCP-outside-declared-UDP", [][]byte{shortUDP}},
		{"outer-fragment-missing", missing[:1]},
	} {
		for _, workers := range []int{1, 2, 4} {
			for _, protocols := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/w%d/protocols%t", tc.name, workers, protocols), func(t *testing.T) {
					var conf *CaptureConfig
					var mu sync.Mutex
					created := 0
					opts := []CaptureOption{WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), WithTCPReassemblyWorkers(workers),
						func(c *CaptureConfig) error { conf = c; return nil },
						WithOnTrafficFlowCreated(func(*TrafficFlow) { mu.Lock(); defer mu.Unlock(); created++ })}
					if protocols {
						opts = append(opts, WithOnProtocolMessage(func(*ProtocolEvent) {}))
					}
					require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, tc.frames)), opts...))
					require.Zero(t, created, "a partial/unsupported carrier cannot expose a TCP session")
					a := conf.binParser
					if !protocols {
						require.Nil(t, a, "TCP-only consumers allocate no application parser")
						a = conf.transportNormalizer
						require.NotNil(t, a)
						require.Empty(t, a.specs, "no application plans or probes are prepared")
					}
					require.Zero(t, a.buffered.Load(), "EOF releases incomplete layer-three buffers")
					require.Empty(t, a.fragments.sets)
				})
			}
		}
	}
}

func TestNormalizedTCPDebugOutputPreservesPhysicalCarrier(t *testing.T) {
	frame := tunnelWorkerWire(t, "6in4", false, 99, true, false, "")
	for _, protocols := range []bool{false, true} {
		t.Run(fmt.Sprint(protocols), func(t *testing.T) {
			var output bytes.Buffer
			opts := []CaptureOption{WithTCPReassemblyStream(4096), func(c *CaptureConfig) error {
				c.Output = pcapgo.NewWriter(&output)
				return c.Output.WriteFileHeader(65535, layers.LinkTypeEthernet)
			}}
			if protocols {
				opts = append(opts, WithOnProtocolMessage(func(*ProtocolEvent) {}))
			}
			require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{frame})), opts...))
			reader, err := pcapgo.NewReader(bytes.NewReader(output.Bytes()))
			require.NoError(t, err)
			actual, ci, err := reader.ReadPacketData()
			require.NoError(t, err)
			require.Equal(t, layers.LinkTypeEthernet, reader.LinkType())
			require.Equal(t, frame, actual, "logical normalization cannot replace the captured carrier bytes")
			require.Equal(t, len(frame), ci.CaptureLength)
			require.Equal(t, len(frame), ci.Length)
		})
	}
}
