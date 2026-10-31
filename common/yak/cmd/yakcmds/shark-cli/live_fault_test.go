package sharkcli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestInspectionContinuesAfterSnaplenAndTCPGap(t *testing.T) {
	full := dnsPacket(t)
	path := filepath.Join(t.TempDir(), "snaplen.pcap")
	f, err := os.Create(path)
	require.NoError(t, err)
	w := pcapgo.NewWriter(f)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: 45, Length: len(full)}, full[:45]))
	require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(2, 0), CaptureLength: len(full), Length: len(full)}, full))
	require.NoError(t, f.Close())
	var out bytes.Buffer
	require.NoError(t, invoke(&out, "-r", path, "--json"))
	var rows []packetSummary
	for _, line := range bytes.Split(bytes.TrimSpace(out.Bytes()), []byte{'\n'}) {
		var row packetSummary
		require.NoError(t, json.Unmarshal(line, &row))
		rows = append(rows, row)
	}
	require.Len(t, rows, 2)
	require.Equal(t, "incomplete", rows[0].ProtocolStatus)
	require.Contains(t, rows[0].Info, "truncated capture: 45/")
	require.Equal(t, "DNS", rows[1].Protocol)
	packets := []*capturedPacket{tcpPacket(t, 1, 100, "", false, true, false, false), tcpPacket(t, 2, 101, "GET / HTTP/1.1\r\nHo", false, false, false, false), tcpPacket(t, 3, 160, "st: example.test\r\n\r\n", false, false, false, false)}
	path = fixture(t, "gap.pcap", packets[0].data, packets[1].data, packets[2].data, full)
	out.Reset()
	require.NoError(t, invoke(&out, "-r", path, "--json"))
	physical, publications := 0, 0
	for _, line := range bytes.Split(bytes.TrimSpace(out.Bytes()), []byte{'\n'}) {
		var row map[string]any
		require.NoError(t, json.Unmarshal(line, &row))
		if row["record_type"] == "protocol_publication" {
			publications++
			require.Equal(t, "capture_close", row["publication_phase"])
			require.NotContains(t, row, "number")
		} else {
			physical++
			require.EqualValues(t, physical, row["number"])
		}
	}
	require.Equal(t, 4, physical)
	require.NotZero(t, publications, "unfilled TCP gaps remain available after the final DNS packet")
	require.Contains(t, out.String(), `"protocol":"DNS"`)
}

func TestTLSFailureKeepsIdentityAndExplainsLaterCiphertext(t *testing.T) {
	s := newStreamStore(8, 4096)
	p := tcpPacket(t, 1, 100, "", false, true, false, false)
	s.add(p)
	p = tcpPacket(t, 2, 101, "first", false, false, false, false)
	s.add(p)
	e := trustedStreamEvent(p, "tls")
	e.Session = map[string]any{"Negotiated Version": uint16(0x304)}
	s.observeProtocol(e)
	e.Status, e.Error, e.ExpertCode = "malformed", "TLS record authentication failed", string(pcaputil.ErrAuthenticationFailed)
	s.observeProtocol(e)
	next := tcpPacket(t, 3, 106, "ciphertext", false, false, false, false)
	s.add(next)
	s.annotate(next)
	row := summarize(next)
	require.Equal(t, "TLSv1.3", row.Protocol)
	require.Equal(t, "connection-associated bytes", row.ProtocolScope)
	require.Equal(t, "TLSv1.3", row.ConnectionProtocol)
	require.Equal(t, "context-required", row.ProtocolStatus)
	require.Contains(t, row.Info, "TLS decoding halted after AuthenticationFailed")
	require.NotContains(t, row.Info, "decrypted")
	reused := tcpPacket(t, 4, 900, "", false, true, false, false)
	s.add(reused)
	s.annotate(reused)
	require.Empty(t, reused.connectionDiagnostic)
	require.Empty(t, reused.connectionStack)
	s.finish()
}

func TestResetStreamCannotLabelDataWhenReuseSYNIsMissing(t *testing.T) {
	s := newStreamStore(8, 4096)
	p := tcpPacket(t, 1, 100, "", false, true, false, false)
	s.add(p)
	p = tcpPacket(t, 2, 101, "first", false, false, false, false)
	s.add(p)
	e := trustedStreamEvent(p, "tls")
	e.Session = map[string]any{"Negotiated Version": uint16(0x304)}
	s.observeProtocol(e)
	reset := tcpPacket(t, 3, 106, "", false, false, false, false)
	reset.data[14+20+13] = 0x14 // RST|ACK, same IPv4 tuple
	s.add(reset)
	data := tcpPacket(t, 4, 50000, "*1\r\n$4\r\nPING\r\n", false, false, false, false)
	s.add(data)
	s.annotate(data)
	require.Zero(t, data.streamID)
	require.Empty(t, data.connectionStack)
	require.Empty(t, summarize(data).ConnectionProtocol)
	s.finish()
}

func TestHTTP3SummaryShowsCompleteFrameKindAndPartialReassembly(t *testing.T) {
	e := &pcaputil.ProtocolEvent{Protocol: "http3", Session: map[string]any{"Packet Name": "1-RTT", "HTTP3 Stream ID": uint64(4), "HTTP3 Stream Kind": "request", "HTTP3 Frames": []map[string]any{{"Frame Type": "DATA", "DATA Length": uint64(5776)}}}}
	require.Contains(t, secureTransportSummary(e), "DATA 5776 bytes")
	delete(e.Session, "HTTP3 Frames")
	summary := secureTransportSummary(e)
	require.Contains(t, summary, "HTTP/3 frame reassembly")
	require.NotContains(t, summary, "DATA")
}
