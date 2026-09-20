package sharkcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/pcap"
	cli "github.com/yaklang/yaklang/common/urfavecli"
)

func udpPacket(t *testing.T, port layers.UDPPort, payload []byte) []byte {
	t.Helper()
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IPv4(192, 0, 2, 1), DstIP: net.IPv4(192, 0, 2, 2)}
	udp := &layers.UDP{SrcPort: 45000, DstPort: port}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}, ip, udp, gopacket.Payload(payload)))
	return b.Bytes()
}
func dnsPacket(t *testing.T) []byte {
	dns := &layers.DNS{ID: 0x1234, RD: true, Questions: []layers.DNSQuestion{{Name: []byte("example.com"), Type: layers.DNSTypeA, Class: layers.DNSClassIN}}}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true}, dns))
	return udpPacket(t, 53, b.Bytes())
}
func fixture(t *testing.T, name string, packets ...[]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	f, err := os.Create(path)
	require.NoError(t, err)
	var w packetWriter
	var flush func() error
	if strings.HasSuffix(name, ".pcapng") {
		ng, e := pcapgo.NewNgWriter(f, layers.LinkTypeEthernet)
		require.NoError(t, e)
		w = ng
		flush = ng.Flush
	} else {
		p := pcapgo.NewWriter(f)
		require.NoError(t, p.WriteFileHeader(65535, layers.LinkTypeEthernet))
		w = p
	}
	for i, p := range packets {
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, int64(i)*1000000), CaptureLength: len(p), Length: len(p)}, p))
	}
	if flush != nil {
		require.NoError(t, flush())
	}
	require.NoError(t, f.Close())
	return path
}
func invoke(out io.Writer, args ...string) error {
	app := cli.NewApp()
	app.Writer = out
	app.ErrWriter = out
	app.Commands = []cli.Command{*Command}
	return app.Run(append([]string{"yak", "shark"}, args...))
}

func TestProtocolFiltersCompileAndCompose(t *testing.T) {
	for _, name := range protocolNames() {
		t.Run(name, func(t *testing.T) {
			_, err := pcap.NewBPF(layers.LinkTypeEthernet, 65535, protocolFilters[name])
			require.NoError(t, err)
		})
	}
	filter, err := buildFilter([]string{"dns,tls", "tcp", "DNS"}, "host 192.0.2.2")
	require.NoError(t, err)
	require.Equal(t, "((port 53) or (tcp and (port 443 or port 8443)) or (tcp)) and (host 192.0.2.2)", filter)
	bpf, err := pcap.NewBPF(layers.LinkTypeEthernet, 65535, filter)
	require.NoError(t, err)
	p := dnsPacket(t)
	require.True(t, bpf.Matches(gopacket.CaptureInfo{Length: len(p), CaptureLength: len(p)}, p))
	p = udpPacket(t, 123, make([]byte, 48))
	require.False(t, bpf.Matches(gopacket.CaptureInfo{Length: len(p), CaptureLength: len(p)}, p))
	_, err = buildFilter([]string{"not-a-protocol"}, "")
	require.ErrorContains(t, err, "unknown protocol")
}

func TestOfflineFilterJSONAndExport(t *testing.T) {
	for _, inputExt := range []string{".pcap", ".pcapng"} {
		for _, outputExt := range []string{".pcap", ".pcapng"} {
			t.Run(inputExt+outputExt, func(t *testing.T) {
				dns := dnsPacket(t)
				input := fixture(t, "in"+inputExt, dns, udpPacket(t, 123, make([]byte, 48)), dns)
				output := filepath.Join(t.TempDir(), "out"+outputExt)
				var stdout bytes.Buffer
				require.NoError(t, invoke(&stdout, "--pcap-file", input, "--protocol", "dns", "--output-file", output, "--json", "--count", "1"))
				var row packetSummary
				require.NoError(t, json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &row))
				require.Equal(t, "DNS", row.Protocol)
				require.Equal(t, "192.0.2.1", row.Source)
				require.Contains(t, row.Info, "example.com")
				f, err := openOffline(output, "")
				require.NoError(t, err)
				defer f.Close()
				data, ci, err := f.reader.ReadPacketData()
				require.NoError(t, err)
				require.Equal(t, dns, data)
				require.Equal(t, len(dns), ci.Length)
				_, _, err = f.reader.ReadPacketData()
				require.ErrorIs(t, err, io.EOF)
			})
		}
	}
}

func TestOutputCannotOverwriteInputOrExistingFile(t *testing.T) {
	input := fixture(t, "in.pcap", dnsPacket(t))
	before, err := os.ReadFile(input)
	require.NoError(t, err)
	require.ErrorContains(t, invoke(io.Discard, "-r", input, "-w", input, "--plain"), "create output capture")
	after, err := os.ReadFile(input)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
func TestInvalidOptionsAndMalformedCapture(t *testing.T) {
	for _, args := range [][]string{{"--count", "-1"}, {"--duration", "-1s"}, {"--max-packets", "0"}, {"--snaplen", "0"}, {"--protocol", "bogus"}, {"-r", "x", "-i", "y"}, {"unexpected"}} {
		require.Error(t, invoke(io.Discard, args...))
	}
	input := fixture(t, "in.pcap", dnsPacket(t))
	require.ErrorContains(t, invoke(io.Discard, "-r", input, "--bpf", "invalid filter !", "--plain"), "invalid capture BPF")
	malformed := filepath.Join(t.TempDir(), "bad.pcap")
	require.NoError(t, os.WriteFile(malformed, []byte("not a pcap"), 0600))
	require.Error(t, invoke(io.Discard, "-r", malformed, "--plain"))
}

func TestPacketDetailsDecodeBranchRules(t *testing.T) {
	p := dnsPacket(t)
	raw := &capturedPacket{number: 7, data: p, ci: gopacket.CaptureInfo{Length: len(p), CaptureLength: len(p)}, link: layers.LinkTypeEthernet}
	d := detail(raw)
	var fields []string
	for _, f := range d.fields {
		fields = append(fields, f.text)
	}
	text := strings.Join(fields, "\n")
	require.NotContains(t, text, "failed")
	require.NotContains(t, text, "incomplete")
	require.Contains(t, text, "Ethernet")
	require.Contains(t, text, "DNS")
	require.Contains(t, text, "4660") // 0x1234 DNS transaction ID from YAML dissector.
	require.NotEmpty(t, d.hex)
	// Damaged captures should still expose bytes and available fields.
	raw.data = p[:17]
	d = detail(raw)
	require.NotEmpty(t, d.fields)
	require.NotEmpty(t, d.hex)
}
func TestTerminalTextSanitizesPacketAndFilterContent(t *testing.T) {
	require.Equal(t, "GET /  [31m malicious ", safeText("GET /\r\x1b[31m malicious\u202e"))
}

func TestRingFilterNavigationAndTerminalLayout(t *testing.T) {
	u := &tui{ring: newPacketRing(3), follow: true, width: 120, height: 30}
	for i := 1; i <= 5; i++ {
		p := dnsPacket(t)
		u.ring.add(&capturedPacket{number: uint64(i), data: p, ci: gopacket.CaptureInfo{Length: len(p), CaptureLength: len(p)}, link: layers.LinkTypeEthernet})
	}
	u.refreshVisible()
	require.Equal(t, uint64(2), u.ring.evicted)
	require.Equal(t, uint64(5), u.selected)
	src := &captureSource{link: layers.LinkTypeEthernet, name: "fixture", offline: true}
	require.False(t, u.key("k", src))
	u.refreshVisible()
	require.Equal(t, uint64(4), u.selected)
	u.key("f", src)
	u.input = "udp port 123"
	u.key("\r", src)
	u.refreshVisible()
	require.Empty(t, u.visible)
	u.key("p", src)
	u.input = "dns"
	u.key("\r", src)
	u.refreshVisible()
	require.Len(t, u.visible, 3)
	previous := u.filterText
	u.key("f", src)
	u.input = "bad !"
	u.key("\r", src)
	require.Equal(t, previous, u.filterText)
	require.Contains(t, u.notice, "Filter error")
	u.editor = ""
	u.fields = detail(u.visible[u.selection].packet)
	u.finished = true
	s := &captureSession{}
	view := u.view(s, src, captureConfig{})
	require.Contains(t, view, "EOF")
	require.Contains(t, view, "PROTOCOL FIELDS")
	require.Contains(t, view, "PACKET BYTES")
	require.LessOrEqual(t, len(strings.Split(view, "\r\n")), u.height)
	u.pane = 1
	u.fieldSelection = 1
	before := len(u.fieldIndices())
	u.key("\r", src)
	require.Equal(t, before, len(u.fieldIndices()), "flat sections remain expanded")
}

func TestCancellationUnblocksOfflineCaptureBackpressure(t *testing.T) {
	packets := make([][]byte, 1000)
	for i := range packets {
		packets[i] = dnsPacket(t)
	}
	input := fixture(t, "many.pcap", packets...)
	src, err := openCapture(captureConfig{input: input, snaplen: 65535})
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, captureConfig{}, false)
	done := make(chan struct{})
	go func() { s.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("capture failed to stop")
	}
	require.NoError(t, s.result())
}

func TestReadErrorAndOutputErrorPropagate(t *testing.T) {
	input := fixture(t, "truncated.pcap", dnsPacket(t))
	info, err := os.Stat(input)
	require.NoError(t, err)
	require.NoError(t, os.Truncate(input, info.Size()-10))
	require.ErrorContains(t, invoke(io.Discard, "-r", input, "--plain"), "read capture")
	input = fixture(t, "valid.pcap", dnsPacket(t))
	require.ErrorContains(t, invoke(errorWriter{}, "-r", input, "--plain"), "write failed")
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, fmt.Errorf("write failed") }
