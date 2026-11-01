package sharkcli

import (
	"context"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func nativeOpeningPacket(t *testing.T, number uint64, seq, ack uint32, payload string, reverse, syn, rst bool) []byte {
	t.Helper()
	p := tcpPacket(t, number, seq, payload, reverse, syn, false, false)
	decoded := gopacket.NewPacket(p.data, layers.LinkTypeEthernet, gopacket.Default)
	eth := decoded.Layer(layers.LayerTypeEthernet).(*layers.Ethernet)
	ip := decoded.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	tcp := decoded.Layer(layers.LayerTypeTCP).(*layers.TCP)
	tcp.Ack, tcp.RST = ack, rst
	require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload([]byte(payload))))
	return b.Bytes()
}

func TestNativePreviewFollowsConfirmedOpeningAfterDelayedOldReset(t *testing.T) {
	request := "GET /old HTTP/1.1\r\nHost: example.test\r\n\r\n"
	redis := "*1\r\n$4\r\nPING\r\n"
	clientSSH, serverSSH := "SSH-2.0-client\r\n", "SSH-2.0-server\r\n"
	packets := [][]byte{
		nativeOpeningPacket(t, 1, 99, 0, "", false, true, false),
		nativeOpeningPacket(t, 2, 499, 100, "", true, true, false),
		nativeOpeningPacket(t, 3, 100, 500, request, false, false, false),
		nativeOpeningPacket(t, 4, 999, 0, "", false, true, false),
		nativeOpeningPacket(t, 5, 100+uint32(len(request)), 500, "", false, false, true),
		nativeOpeningPacket(t, 6, 1999, 1000, "", true, true, false),
		nativeOpeningPacket(t, 7, 1000, 2000, redis, false, false, false),
		nativeOpeningPacket(t, 8, 2000, 1000+uint32(len(redis)), "+PONG\r\n", true, false, false),
		nativeOpeningPacket(t, 9, 2999, 0, "", false, true, false),
		nativeOpeningPacket(t, 10, 2999, 0, "", false, true, false),
		nativeOpeningPacket(t, 11, 3999, 3000, "", true, true, false),
		nativeOpeningPacket(t, 12, 3000, 4000, clientSSH, false, false, false),
		nativeOpeningPacket(t, 13, 4000, 3000+uint32(len(clientSSH)), serverSSH, true, false, false),
	}
	src, err := openOffline(fixture(t, "preview-delayed-reset.pcap", packets...), "")
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, captureConfig{}, false)
	defer s.Stop()
	var rows []*capturedPacket
	for p := range s.packets {
		rows = append(rows, p)
	}
	require.NoError(t, s.result())
	require.Len(t, rows, len(packets))
	for _, index := range []int{0, 1, 2, 4, 5, 6, 7, 10, 11, 12} {
		require.NotZero(t, rows[index].tcpFlowID, "packet %d has a native generation", index+1)
		require.NotZero(t, rows[index].streamID, "packet %d must retain its native preview", index+1)
	}
	require.Zero(t, rows[3].tcpFlowID, "an unconfirmed SYN stays unassociated in its immutable packet snapshot")
	require.Zero(t, rows[8].tcpFlowID)
	require.Zero(t, rows[9].tcpFlowID)
	require.Equal(t, rows[0].streamID, rows[4].streamID, "the delayed reset belongs to the old stream")
	require.NotEqual(t, rows[0].streamID, rows[5].streamID)
	require.NotEqual(t, rows[5].streamID, rows[10].streamID)
	for _, tc := range []struct {
		index   int
		packets uint64
		bytes   [2]string
		stack   []string
	}{
		{0, 4, [2]string{request, ""}, []string{"HTTP/1.1"}},
		{5, 4, [2]string{redis, "+PONG\r\n"}, []string{"Redis"}},
		{10, 5, [2]string{clientSSH, serverSSH}, []string{"SSH"}},
	} {
		view := s.streams.snapshot(rows[tc.index].streamID)
		require.NotNil(t, view)
		require.Equal(t, tc.packets, view.packets, "held SYN observations count exactly once")
		for direction := range tc.bytes {
			require.Equal(t, tc.bytes[direction], directionData(view, direction), "packet %d direction %d", tc.index+1, direction)
		}
		require.Equal(t, tc.stack, view.protocolStack)
		require.NotEmpty(t, view.events)
		for _, event := range view.events {
			require.Equal(t, rows[tc.index].tcpFlowID, event.TCPFlowID, "message history must use the captured TCP generation")
		}
	}
}

func TestNativePreviewPreservesRepairedBytesAndExposesUnfilledGaps(t *testing.T) {
	request := "GET /repair HTTP/1.1\r\nHost: example.test\r\n\r\n"
	for _, repair := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing-prefix", true: "repaired-with-duplicate"}[repair], func(t *testing.T) {
			packets := [][]byte{
				nativeOpeningPacket(t, 1, 99, 0, "", false, true, false),
				nativeOpeningPacket(t, 2, 100, 0, "GE", false, false, false),
				nativeOpeningPacket(t, 3, 104, 0, request[4:], false, false, false),
			}
			if repair {
				packets = append(packets, nativeOpeningPacket(t, 4, 102, 0, request[2:4], false, false, false), nativeOpeningPacket(t, 5, 100, 0, request, false, false, false))
			}
			src, err := openOffline(fixture(t, "preview-gap.pcap", packets...), "")
			require.NoError(t, err)
			defer src.Close()
			s := startCapture(context.Background(), src, captureConfig{}, false)
			defer s.Stop()
			var first *capturedPacket
			for p := range s.packets {
				if first == nil {
					first = p
				}
				require.Equal(t, first.tcpFlowID, p.tcpFlowID)
				require.Equal(t, first.streamID, p.streamID)
			}
			require.NoError(t, s.result())
			view := s.streams.snapshot(first.streamID)
			require.NotNil(t, view)
			require.EqualValues(t, len(packets), view.packets)
			require.EqualValues(t, 1, view.outOfOrder)
			require.True(t, view.startKnown[0])
			if repair {
				require.Equal(t, request, directionData(view, 0))
				require.EqualValues(t, 1, view.retransmits)
				require.Zero(t, view.gaps)
				require.Empty(t, view.retentionNotice)
				require.Equal(t, []string{"HTTP/1.1"}, view.protocolStack)
			} else {
				require.Equal(t, "GE", directionData(view, 0), "a pending suffix must not be joined at EOF")
				require.Empty(t, view.protocolStack)
				require.True(t, view.prefixGap[0])
				require.NotZero(t, view.gaps)
				require.Contains(t, view.retentionNotice, "incomplete")
				require.Zero(t, view.missing, "no exact missing byte count is claimed from an unavailable tail")
			}
		})
	}
}

func TestNativePreviewEvictionNeverRecreatesAnActiveStream(t *testing.T) {
	request, ssh := "GET /first HTTP/1.1\r\nHost: example.test\r\n\r\n", "SSH-2.0-second\r\n"
	second := func(seq uint32, payload string, syn bool) []byte {
		wire := nativeOpeningPacket(t, 1, seq, 0, payload, false, syn, false)
		packet := gopacket.NewPacket(wire, layers.LinkTypeEthernet, gopacket.Default)
		eth := packet.Layer(layers.LayerTypeEthernet).(*layers.Ethernet)
		ip := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		tcp.SrcPort++
		require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
		b := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload([]byte(payload))))
		return b.Bytes()
	}
	packets := [][]byte{
		nativeOpeningPacket(t, 1, 99, 0, "", false, true, false),
		nativeOpeningPacket(t, 2, 100, 0, request, false, false, false),
		second(199, "", true), second(200, ssh, false),
		nativeOpeningPacket(t, 5, 100+uint32(len(request)), 0, "retained by core only", false, false, false),
	}
	src, err := openOffline(fixture(t, "preview-eviction.pcap", packets...), "")
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, captureConfig{maxStreams: 1, streamBytes: 64}, false)
	defer s.Stop()
	var rows []*capturedPacket
	for p := range s.packets {
		rows = append(rows, p)
	}
	require.NoError(t, s.result())
	require.Equal(t, rows[0].tcpFlowID, rows[4].tcpFlowID)
	require.Equal(t, rows[0].streamID, rows[4].streamID)
	require.Nil(t, s.streams.snapshot(rows[0].streamID), "eviction cannot restart a preview at an arbitrary middle")
	view := s.streams.snapshot(rows[2].streamID)
	require.NotNil(t, view)
	require.Equal(t, ssh, directionData(view, 0))
	require.Equal(t, []string{"SSH"}, view.protocolStack)
	require.Len(t, s.streams.records, 1)
	require.Len(t, s.streams.nativeRecords, 1, "closed evicted metadata is removed")
	require.EqualValues(t, 2, s.streams.next, "no third preview allocated for the still-active first generation")
	require.Empty(t, s.streams.active, "capture-backed previews allocate no independent assembler streams")
}

func TestNativePreviewDoesNotConflateTCPAndProtocolFlowNumbers(t *testing.T) {
	// A control-only TCP connection has no protocol session. Mix in UDP as
	// well; the following HTTP connection uses distinct numeric namespaces.
	request := "GET /mixed-capture HTTP/1.1\r\nHost: example.test\r\n\r\n"
	packets := [][]byte{tcpPacket(t, 1, 99, "", false, true, false, true).data, dnsPacket(t), nativeOpeningPacket(t, 3, 99, 0, "", false, true, false), nativeOpeningPacket(t, 4, 100, 0, request, false, false, false)}
	src, err := openOffline(fixture(t, "preview-mixed-flow-numbers.pcap", packets...), "")
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, captureConfig{}, false)
	defer s.Stop()
	var last *capturedPacket
	for p := range s.packets {
		last = p
	}
	require.NoError(t, s.result())
	view := s.streams.snapshot(last.streamID)
	require.NotNil(t, view)
	var protocolFlow uint64
	for _, event := range last.protocolEvents() {
		if event.Protocol == "http" {
			protocolFlow = event.FlowID
			require.Equal(t, last.tcpFlowID, event.TCPFlowID)
		}
	}
	require.NotZero(t, protocolFlow)
	require.NotEqual(t, last.tcpFlowID, protocolFlow, "the separate ID namespaces must actually differ in this regression")
	require.Equal(t, []string{"HTTP/1.1"}, view.protocolStack, "native identity must not depend on equal numeric IDs")
	require.NotEmpty(t, view.events, "the page must retain messages from the associated protocol namespace")
}

func TestNativePreviewDoesNotApplyClosedConnectionIdentityToNewPayload(t *testing.T) {
	request := "GET /before-reset HTTP/1.1\r\nHost: example.test\r\n\r\n"
	packets := [][]byte{
		nativeOpeningPacket(t, 1, 99, 0, "", false, true, false),
		nativeOpeningPacket(t, 2, 100, 0, request, false, false, false),
		nativeOpeningPacket(t, 3, 100+uint32(len(request)), 0, "", false, false, true),
		nativeOpeningPacket(t, 4, 50000, 0, "SSH-2.0-unconfirmed-generation\r\n", false, false, false),
	}
	src, err := openOffline(fixture(t, "preview-after-reset.pcap", packets...), "")
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, captureConfig{}, false)
	defer s.Stop()
	var beforeClose, last *capturedPacket
	for p := range s.packets {
		if p.number == 2 {
			beforeClose = p
		}
		last = p
	}
	require.NoError(t, s.result())
	require.NotNil(t, beforeClose)
	require.NotZero(t, last.tcpFlowID, "the closed native namespace remains source evidence")
	require.Zero(t, last.streamID, "an unconfirmed generation cannot open the earlier application's page")
	view := s.streams.snapshot(beforeClose.streamID)
	require.NotNil(t, view, "the old packet's closed-flow history remains available for forensics")
	require.Equal(t, request, directionData(view, 0), "post-reset bytes never enter the closed stream")
	require.Equal(t, []string{"HTTP/1.1"}, view.protocolStack, "the old identity stays in its own history")
	u := &tui{streams: s.streams}
	for _, row := range []packetSummary{summarize(last), u.packetRow(&packetEntry{packet: last})} {
		require.Equal(t, "TCP", row.Protocol, "observation of an old flow is not admission of a new application's bytes")
		require.Empty(t, row.ConnectionProtocol)
		require.Zero(t, row.StreamID)
		require.Contains(t, row.Info, "generation unconfirmed")
	}
}
