package sharkcli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClosedTCPHeaderDoesNotBorrowPreviousApplicationOnRedraw(t *testing.T) {
	row := packetSummary{
		Protocol: "TCP", Transport: "tcp", packetProtocol: "TCP",
		TCPFlowID: 17, TCPFlowState: "closed-observation", Info: "443 → 45000 [ACK] Seq=50000 Ack=60000 Payload=0",
		Layers:          []string{"Loopback", "IPv4", "TCP"},
		ConnectionStack: []string{"TLSv1.3", "HTTP/1.1"},
	}
	first := applyApplication(row, "TLSv1.3", "validated protocol events", 7)
	require.Equal(t, "TCP", first.Protocol)
	require.Empty(t, first.ConnectionProtocol)
	require.Empty(t, first.ConnectionStack)
	require.Equal(t, []string{"Loopback", "IPv4", "TCP"}, first.ProtocolStack)
	require.Equal(t, "context-required", first.ProtocolStatus)
	require.Equal(t, "closed TCP observation", first.ProtocolScope)
	require.Equal(t, "header-only", first.Visibility)
	require.EqualValues(t, 17, first.TCPFlowID, "the native TCP namespace remains source evidence")
	require.Zero(t, first.StreamID, "the closed namespace does not associate an old application page")
	require.Contains(t, first.Info, "TCP header after observed close; generation unconfirmed")
	require.NotContains(t, first.Info, "TLS")
	require.NotContains(t, first.Info, "HTTP")
	require.Equal(t, first, applyApplication(first, "TLSv1.3", "validated protocol events", 7))
	row.TCPFlowState = "closed-direction-observation"
	halfClosed := applyApplication(row, "TLSv1.3", "validated protocol events", 7)
	require.Equal(t, "TLSv1.3", halfClosed.ConnectionProtocol, "one closed direction does not close the active reverse direction")
	require.Contains(t, halfClosed.Info, "connection #7")
	require.NotContains(t, halfClosed.Info, "generation unconfirmed")
}

func TestClosedTupleACKAndResetStayUnconfirmedUntilObservedOpening(t *testing.T) {
	request := "GET /before-reset HTTP/1.1\r\nHost: example.test\r\n\r\n"
	unknown := "SSH-2.0-unconfirmed-generation\r\n"
	redis := "*1\r\n$4\r\nPING\r\n"
	packets := [][]byte{
		nativeOpeningPacket(t, 1, 99, 0, "", false, true, false),
		nativeOpeningPacket(t, 2, 499, 100, "", true, true, false),
		nativeOpeningPacket(t, 3, 100, 500, request, false, false, false),
		nativeOpeningPacket(t, 4, 100+uint32(len(request)), 500, "", false, false, true),
		nativeOpeningPacket(t, 5, 50000, 60000, unknown, false, false, false),
		nativeOpeningPacket(t, 6, 50000+uint32(len(unknown)), 60000, "", false, false, false),
		nativeOpeningPacket(t, 7, 60000, 50000+uint32(len(unknown)), "", true, false, false),
		nativeOpeningPacket(t, 8, 60000, 50000+uint32(len(unknown)), "", true, false, true),
		nativeOpeningPacket(t, 9, 69999, 0, "", false, true, false),
		nativeOpeningPacket(t, 10, 79999, 70000, "", true, true, false),
		nativeOpeningPacket(t, 11, 70000, 80000, redis, false, false, false),
		nativeOpeningPacket(t, 12, 80000, 70000+uint32(len(redis)), "+PONG\r\n", true, false, false),
	}
	src, err := openOffline(fixture(t, "closed-tuple-header-context.pcap", packets...), "")
	require.NoError(t, err)
	defer src.Close()
	session := startCapture(context.Background(), src, captureConfig{}, false)
	defer session.Stop()
	var captured []*capturedPacket
	for packet := range session.packets {
		captured = append(captured, packet)
	}
	require.NoError(t, session.result())
	require.Len(t, captured, len(packets))
	u := &tui{streams: session.streams}
	for index := 4; index <= 7; index++ {
		packet := captured[index]
		require.Equal(t, "closed-observation", packet.tcpFlowState)
		require.Equal(t, captured[0].tcpFlowID, packet.tcpFlowID, "no new generation is invented without its opening")
		require.Zero(t, packet.streamID, "the old application's stream page is not associated")
		for _, row := range []packetSummary{summarize(packet), u.packetRow(&packetEntry{packet: packet})} {
			require.Equal(t, "TCP", row.Protocol)
			require.Empty(t, row.ConnectionProtocol)
			require.Empty(t, row.ConnectionStack)
			require.Zero(t, row.StreamID)
			require.Contains(t, row.Info, "generation unconfirmed")
			require.NotContains(t, row.Info, "HTTP")
			require.NotContains(t, row.Info, "SSH")
		}
	}
	old := session.streams.snapshot(captured[0].streamID)
	require.NotNil(t, old)
	require.Equal(t, request, directionData(old, 0), "unknown post-reset bytes never enter the closed preview")
	require.Equal(t, []string{"HTTP/1.1"}, old.protocolStack)
	require.EqualValues(t, 2, session.streams.next)
	last := captured[len(captured)-1]
	require.NotEqual(t, captured[0].tcpFlowID, last.tcpFlowID)
	for _, row := range []packetSummary{summarize(last), u.packetRow(&packetEntry{packet: last})} {
		require.Equal(t, "Redis", row.Protocol)
		require.Equal(t, "Redis", row.ConnectionProtocol)
		require.NotContains(t, row.Info, "generation unconfirmed")
	}
}
