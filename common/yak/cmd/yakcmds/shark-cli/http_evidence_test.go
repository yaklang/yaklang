package sharkcli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPIdentificationRequiresCompleteValidHeaders(t *testing.T) {
	for _, data := range []string{
		"GET ", "GET binary bytes\x00\x01\r\n\r\n", "HTTP/1.random\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: test\r\n", "GET / HTTP/9.9\r\n\r\n",
		"GET / HTTP/1.1\r\nBad Header: x\r\n\r\n", "GET / HTTP/1.1\r\nX: \x00\r\n\r\n",
		"HTTP/1.1 200\x00 OK\r\n\r\n", "HTTP/1.1 abc OK\r\n\r\n",
		"HTTP/1.1 200 OK\r\nContent-Length: 4\r\nContent-Length: 8\r\n\r\n",
		string(bytes.Repeat([]byte{0xa5}, 4096)),
	} {
		require.Nil(t, inspectHTTP([]byte(data)), "%q", data)
		require.NotEqual(t, "HTTP", sniffPayload([]byte(data)), "%q", data)
		require.NotEqual(t, "HTTP", sniffStreamPayload([]byte(data), 45000, 12011), "%q", data)
	}
	for _, data := range []string{
		"GET / HTTP/1.1\r\n\r\n", "POST /upload HTTP/1.1\r\nHost: test\r\nContent-Length: 100000\r\n\r\n\x00\xff",
		"HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: 900000\r\n\r\n\xff\x00",
		"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n",
	} {
		require.NotNil(t, inspectHTTP([]byte(data)))
		require.Equal(t, "HTTP", sniffPayload([]byte(data)))
	}
}

func TestWorkerCannotPromoteUnverifiedHTTP(t *testing.T) {
	s := newStreamStore(8, 4096)
	p := tcpPacket(t, 1, 100, "", false, true, false, false)
	s.add(p)
	s.add(tcpPacket(t, 2, 101, "GET some bytes\r\n\r\n\xff\x00", false, false, false, false))
	require.False(t, s.identify(p.streamID, "HTTP"))
	protocol, _ := s.label(p.streamID)
	require.Empty(t, protocol)
	u := &tui{streams: s, stream: s.snapshot(p.streamID)}
	u.applyDetail(detailResult{streamID: p.streamID, streamVersion: u.stream.decodeVersion, detail: packetDetail{protocol: "HTTP"}})
	require.Empty(t, u.stream.protocol, "the UI must not trust an unverified worker result either")
	d := decodeStream(streamDecodeRequest{ID: 1, Ports: [2]uint16{45000, 12011}, Prefix: [2][]byte{[]byte("GET some bytes\r\n\r\n\xff\x00")}})
	require.Empty(t, d.protocol)
}

func TestConnectionAttributionDoesNotLabelBinaryPacketsAsHTTP(t *testing.T) {
	s := newStreamStore(8, 4096)
	p := tcpPacket(t, 1, 100, "", false, true, false, false)
	s.add(p)
	head := "POST /upload HTTP/1.1\r\nHost: test\r\nContent-Length: 100000\r\n\r\n"
	h := tcpPacket(t, 2, 101, head, false, false, false, false)
	s.add(h)
	body := tcpPacket(t, 3, 101+uint32(len(head)), string(bytes.Repeat([]byte{0xaa, 0xff}, 128)), false, false, false, false)
	s.add(body)
	u := &tui{streams: s}
	row := u.packetRow(&packetEntry{packet: body})
	require.Equal(t, "TCP/HTTP", row.Protocol)
	require.Equal(t, "connection prefix, not this packet", row.ProtocolScope)
	require.Contains(t, row.Info, "prefix=HTTP")
	row = u.packetRow(&packetEntry{packet: h})
	require.Equal(t, "HTTP", row.Protocol)
	require.Equal(t, "packet", row.ProtocolScope)
	require.Equal(t, "TCP", applyApplication(packetSummary{Protocol: "TCP", packetProtocol: "TCP"}, "HTTP", "port hint", 1).Protocol)
}

func TestBeginningSurvivesRecentWindowTrimmingAndShowsBinaryHTTPHeaders(t *testing.T) {
	s := newStreamStore(8, 256)
	p := tcpPacket(t, 1, 100, "", false, true, false, false)
	s.add(p)
	head := "POST /original-opening HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/octet-stream\r\nContent-Length: 32768\r\n\r\n"
	s.add(tcpPacket(t, 2, 101, head, false, false, false, false))
	for i := 0; i < 32; i++ {
		s.add(tcpPacket(t, uint64(i+3), 101+uint32(len(head)+i*1024), string(bytes.Repeat([]byte{0xa5}, 1024)), false, false, false, false))
	}
	v := s.snapshot(p.streamID)
	require.Positive(t, v.omitted)
	require.NotContains(t, directionData(v, 0), "POST")
	require.Len(t, v.prefix[0], streamPrefixLimit)
	require.True(t, v.startKnown[0])
	require.False(t, v.prefixTime[0].IsZero())
	u := &tui{width: 180, height: 50, pane: 3, stream: v, streamBeginning: true}
	view := streamViewText(u)
	require.Contains(t, view, "POST /original-opening HTTP/1.1")
	require.Contains(t, view, "Content-Type: application/octet-stream")
	require.Contains(t, view, "after HTTP headers")
	require.Contains(t, view, "TCP payload beginning (SYN observed)")
	u.key("l", nil)
	view = streamViewText(u)
	require.NotContains(t, view, "POST /original-opening")
	require.Contains(t, view, "this is not the beginning")
	u.key("h", nil)
	require.Contains(t, streamViewText(u), "POST /original-opening")
	u.key("e", nil)
	view = streamViewText(u)
	require.Contains(t, view, "Verified HTTP at captured offset 0")
	require.Contains(t, view, "50 4f 53 54")
	require.Contains(t, view, "Content-Length: 32768")
	u.key("h", nil)
	u.key("x", nil)
	view = streamViewText(u)
	require.Contains(t, view, "00000000")
	require.Contains(t, view, "50 4f 53 54")
	// The toolbar uses the same keyboard paths and doesn't depend on focus quirks.
	l := layoutDashboard(u.width-1, u.height)
	u.mouse(mouseEvent{action: mousePress, button: 0, x: 87, y: l.tabs})
	require.Equal(t, 3, u.streamMode)
	u.mouse(mouseEvent{action: mousePress, button: 0, x: 73, y: l.tabs})
	require.False(t, u.streamBeginning)
	u.mouse(mouseEvent{action: mousePress, button: 0, x: 56, y: l.tabs})
	require.True(t, u.streamBeginning)
}
func streamViewText(u *tui) string {
	var b strings.Builder
	for _, row := range u.streamLines() {
		b.WriteString(row.text)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestMissingSYNDoesNotClaimActualStreamBeginning(t *testing.T) {
	s := newStreamStore(8, 4096)
	p := tcpPacket(t, 1, 100, "GET /midcapture HTTP/1.1\r\nHost: test\r\n\r\n", false, false, false, false)
	s.add(p)
	s.finish()
	v := s.snapshot(p.streamID)
	require.False(t, v.startKnown[0])
	u := &tui{width: 160, height: 48, pane: 3, stream: v, streamBeginning: true}
	require.Contains(t, streamViewText(u), "actual TCP beginning unknown")
	u.key("e", nil)
	require.Contains(t, streamViewText(u), "Verified HTTP at captured offset 0")
	require.Contains(t, streamViewText(u), "actual TCP beginning unknown")
}

func TestHTTPConnectAndUpgradeSeparateHeadersFromFollowingBytes(t *testing.T) {
	connect := [2][]byte{[]byte("CONNECT test:443 HTTP/1.1\r\nHost: test:443\r\n\r\n"), []byte("HTTP/1.1 200 Connection established\r\n\r\n\x17\x03\x03\x00\x04\xff\xff\xff\xff")}
	require.Contains(t, httpConnectionContext(connect), "CONNECT tunnel established")
	upgrade := [2][]byte{[]byte("GET /ws HTTP/1.1\r\nHost: test\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n"), []byte("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n\x82\x04\x00\xff\x00\xff")}
	require.Contains(t, httpConnectionContext(upgrade), "Upgrade to websocket")
	for _, prefix := range [][2][]byte{connect, upgrade} {
		u := &tui{width: 180, height: 50, pane: 3, streamBeginning: true, stream: &streamSnapshot{protocol: "HTTP", prefix: prefix}}
		view := streamViewText(u)
		require.Contains(t, view, "HTTP/1.1")
		require.Contains(t, view, "body/tunnel bytes, not separately identified")
	}
}

func TestHTTPPortHintDoesNotClaimPrefixEvidence(t *testing.T) {
	u := &tui{width: 180, height: 50, pane: 3, streamBeginning: true, stream: &streamSnapshot{protocol: "HTTP", evidence: "port hint", prefix: [2][]byte{{0xff, 0x00, 0xaa}}}}
	view := streamViewText(u)
	require.Contains(t, view, "unverified port hint: HTTP")
	require.NotContains(t, view, "HTTP headers observed")
	row := applyApplication(packetSummary{Protocol: "TCP", packetProtocol: "TCP"}, "HTTP", "port hint", 1)
	require.NotContains(t, row.Info, "prefix=HTTP")
	require.Equal(t, "TCP", row.Protocol)
}
