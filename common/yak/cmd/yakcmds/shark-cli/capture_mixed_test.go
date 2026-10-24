package sharkcli

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"io"
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

func mixedCapture(t *testing.T) string {
	t.Helper()
	var b bytes.Buffer
	w, err := pcapgo.NewNgWriter(&b, layers.LinkTypeEthernet)
	require.NoError(t, err)
	i, err := w.AddInterface(pcapgo.NgInterface{LinkType: layers.LinkTypeRaw})
	require.NoError(t, err)
	frame := dnsPacket(t)
	for j, raw := range [][]byte{frame, frame[14:], frame} {
		ci := gopacket.CaptureInfo{Timestamp: time.Unix(1, int64(j)), CaptureLength: len(raw), Length: len(raw)}
		if j == 1 {
			ci.InterfaceIndex = i
		}
		require.NoError(t, w.WritePacket(ci, raw))
	}
	require.NoError(t, w.Flush())
	path := filepath.Join(t.TempDir(), "mixed.pcapng")
	require.NoError(t, os.WriteFile(path, b.Bytes(), 0600))
	return path
}

func TestMixedLinkReplayAndExport(t *testing.T) {
	path := mixedCapture(t)
	for _, filter := range []string{"", "udp port 53"} {
		t.Run(filter, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "export.pcapng")
			cfg := captureConfig{input: path, output: out, filter: filter}
			src, err := openCapture(cfg)
			require.NoError(t, err)
			defer src.Close()
			s := startCapture(context.Background(), src, cfg, false)
			var rows []packetSummary
			for p := range s.packets {
				rows = append(rows, summarize(p))
			}
			require.NoError(t, s.result())
			require.Len(t, rows, 3)
			for i, row := range rows {
				require.Equal(t, uint64(i+1), row.Number)
				require.Equal(t, "DNS", row.Protocol)
			}
			f, err := os.Open(out)
			require.NoError(t, err)
			defer f.Close()
			r, err := pcaputil.NewCaptureReader(f)
			require.NoError(t, err)
			var links []layers.LinkType
			var domains []pcaputil.CaptureDomain
			for {
				_, ci, err := r.ReadPacketData()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				links = append(links, r.LinkType())
				domains = append(domains, pcaputil.CapturePacketReference(ci).Domain)
			}
			require.Equal(t, []layers.LinkType{layers.LinkTypeEthernet, layers.LinkTypeRaw, layers.LinkTypeEthernet}, links)
			require.Equal(t, domains[0], domains[2])
			require.NotEqual(t, domains[0], domains[1])
		})
	}
}

func TestMixedClassicExportRefusesWrongHeader(t *testing.T) {
	cfg := captureConfig{input: mixedCapture(t), output: filepath.Join(t.TempDir(), "export.pcap")}
	src, err := openCapture(cfg)
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, cfg, false)
	for range s.packets {
	}
	require.ErrorContains(t, s.result(), "use .pcapng")
}

func TestExplicitEmbeddedSecretDiscard(t *testing.T) {
	base, err := os.ReadFile(mixedCapture(t))
	require.NoError(t, err)
	// A synthetic DSB, never actual key material. Insert consecutive blocks to
	// check bounded skipping without recursion or exposing them to pcapgo.
	block := make([]byte, 24)
	binary.LittleEndian.PutUint32(block, 10)
	binary.LittleEndian.PutUint32(block[4:], 24)
	binary.LittleEndian.PutUint32(block[8:], 0x544c534b)
	binary.LittleEndian.PutUint32(block[12:], 4)
	copy(block[16:], "test")
	binary.LittleEndian.PutUint32(block[20:], 24)
	shb := int(binary.LittleEndian.Uint32(base[4:]))
	wrap := func(dsb []byte) []byte {
		b := append([]byte(nil), base[:shb]...)
		b = append(b, dsb...)
		return append(b, base[shb:]...)
	}
	raw := wrap(append(append([]byte(nil), block...), block...))
	_, err = pcaputil.NewCaptureReader(bytes.NewReader(raw))
	require.ErrorContains(t, err, "embedded secrets")
	r, err := pcaputil.NewCaptureReaderWithOptions(bytes.NewReader(raw), pcaputil.PcapReaderOptions{IgnoreEmbeddedSecrets: true})
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		_, _, err = r.ReadPacketData()
		require.NoError(t, err)
	}
	_, _, err = r.ReadPacketData()
	require.ErrorIs(t, err, io.EOF)
	binary.LittleEndian.PutUint32(block[12:], 8) // extends beyond the trailer
	_, err = pcaputil.NewCaptureReaderWithOptions(bytes.NewReader(wrap(block)), pcaputil.PcapReaderOptions{IgnoreEmbeddedSecrets: true})
	require.ErrorContains(t, err, "secret length")
}

func TestMPacketNativeAndListAgree(t *testing.T) {
	frame := dnsPacket(t)
	wire := append([]byte{0x55, 0x55, 0xd5}, frame...)
	wire = binary.LittleEndian.AppendUint32(wire, crc32.ChecksumIEEE(frame))
	var b bytes.Buffer
	w, err := pcapgo.NewNgWriter(&b, 274)
	require.NoError(t, err)
	require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(wire), Length: len(wire)}, wire))
	require.NoError(t, w.Flush())
	var events []*pcaputil.ProtocolEvent
	require.NoError(t, pcaputil.ReplayPcap(bytes.NewReader(b.Bytes()), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	require.Equal(t, "dns", events[0].Protocol)
	require.Equal(t, "decoded", events[0].Status)
	path := filepath.Join(t.TempDir(), "express.pcapng")
	require.NoError(t, os.WriteFile(path, b.Bytes(), 0600))
	cfg := captureConfig{input: path}
	src, err := openCapture(cfg)
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, cfg, false)
	var rows []packetSummary
	for packet := range s.packets {
		rows = append(rows, summarize(packet))
	}
	require.NoError(t, s.result())
	require.Len(t, rows, 1)
	require.Equal(t, "DNS", rows[0].Protocol)
	require.Contains(t, rows[0].Info, "example")
}

func TestMalformedTCPDoesNotEnterUIAssembler(t *testing.T) {
	// Complete Ethernet/IPv4, but only one TCP byte. A partially populated TCP
	// layer must never format empty port endpoints or acquire a stream ID.
	frame := dnsPacket(t)
	frame = append([]byte(nil), frame[:35]...)
	frame[23] = 6
	binary.BigEndian.PutUint16(frame[16:], 21)
	path := fixture(t, "bad-tcp.pcap", frame, dnsPacket(t))
	src, err := openOffline(path, "")
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, captureConfig{}, false)
	var packets []*capturedPacket
	for p := range s.packets {
		packets = append(packets, p)
	}
	require.NoError(t, s.result())
	require.Len(t, packets, 2)
	require.Zero(t, packets[0].streamID)
	require.Equal(t, "DNS", summarize(packets[1]).Protocol)
}
