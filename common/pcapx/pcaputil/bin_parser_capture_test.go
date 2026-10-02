package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func TestBinParserResetAfterFINReuse(t *testing.T) {
	for _, before := range []bool{false, true} {
		for _, workers := range []int{1, 2, 4} {
			steps := []tcpStep{{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true}, {seq: 1, data: string(binMQTTConnect)}}
			end := uint32(1 + len(binMQTTConnect))
			fin, rst := tcpStep{seq: end, fin: true}, tcpStep{seq: end + 1, rst: true}
			if before {
				steps = append(steps, rst, fin)
			} else {
				steps = append(steps, fin, rst)
			}
			steps = append(steps, tcpStep{seq: 1000, syn: true}, tcpStep{seq: 1001, data: string(binMQTTConnect)})
			events, s, err := binReplay(t, binTestPcap(t, steps, 1883, false, false), workers)
			require.NoError(t, err)
			require.EqualValues(t, 2, s.Decoded)
			require.Len(t, events, 2)
			require.NotEqual(t, events[0].FlowID, events[1].FlowID)
		}
	}
}

// The private borrowed-buffer decoder must match public packet callbacks for
// Windows/macOS loopback, raw IP, UDP truncation and fragmented packets.
func TestBinParserPrivateDatagramParity(t *testing.T) {
	dns := []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 'a', 0, 0, 1, 0, 1}
	for _, ipv6 := range []bool{false, true} {
		for _, link := range []layers.LinkType{layers.LinkTypeEthernet, layers.LinkTypeNull, layers.LinkTypeLoop, layers.LinkTypeRaw} {
			for _, mode := range []string{"valid", "truncated", "bad-udp-length", "fragment", "wrong-version"} {
				t.Run(fmt.Sprintf("ipv6=%v/link=%v/%s", ipv6, link, mode), func(t *testing.T) {
					ip4 := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IP{192, 0, 2, 1}, DstIP: net.IP{192, 0, 2, 2}}
					ip6 := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolUDP, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
					var ip gopacket.SerializableLayer = ip4
					family := layers.ProtocolFamilyIPv4
					etherType := layers.EthernetTypeIPv4
					if ipv6 {
						ip = ip6
						family = layers.ProtocolFamilyIPv6BSD
						etherType = layers.EthernetTypeIPv6
					}
					udp := &layers.UDP{SrcPort: 30000, DstPort: 53}
					require.NoError(t, udp.SetNetworkLayerForChecksum(ip.(gopacket.NetworkLayer)))
					serial := []gopacket.SerializableLayer{ip, udp, gopacket.Payload(dns)}
					offset := 0
					if link == layers.LinkTypeEthernet {
						offset = 14
						serial = append([]gopacket.SerializableLayer{&layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: etherType}}, serial...)
					}
					if link == layers.LinkTypeNull || link == layers.LinkTypeLoop {
						offset = 4
						serial = append([]gopacket.SerializableLayer{&layers.Loopback{Family: family}}, serial...)
					}
					b := gopacket.NewSerializeBuffer()
					require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, serial...))
					raw := bytes.Clone(b.Bytes())
					length := len(raw)
					if link == layers.LinkTypeLoop {
						binary.BigEndian.PutUint32(raw[:4], uint32(family))
					}
					switch mode {
					case "truncated":
						raw = raw[:len(raw)-2]
					case "bad-udp-length":
						header := 20
						if ipv6 {
							header = 40
						}
						binary.BigEndian.PutUint16(raw[offset+header+4:], 7)
					case "wrong-version":
						raw[offset] = (raw[offset] & 15) | 0x70
					case "fragment":
						if ipv6 {
							raw[offset+6] = 44
						} else {
							raw[offset+6] |= 0x20
						}
					}
					var wire bytes.Buffer
					w := pcapgo.NewWriterNanos(&wire)
					require.NoError(t, w.WriteFileHeader(65535, link))
					require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, 1234), CaptureLength: len(raw), Length: length}, raw))
					for _, workers := range []int{1, 2, 4} {
						fast, fs, fe := binReplay(t, wire.Bytes(), workers)
						pub, ps, pe := binReplay(t, wire.Bytes(), workers, WithEveryPacket(func(gopacket.Packet) {}))
						require.Equal(t, pe == nil, fe == nil, "workers=%d private=%v public=%v", workers, fe, pe)
						require.Equal(t, ps, fs)
						require.Len(t, fast, len(pub))
						for i := range fast {
							fast[i].plan, pub[i].plan = nil, nil
							require.Equal(t, pub[i], fast[i])
						}
						if mode == "valid" {
							require.EqualValues(t, 1, fs.Decoded)
							require.Equal(t, "dns", fast[0].Protocol)
						}
					}
				})
			}
		}
	}
}
