package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"net"
	"testing"
	"time"
)

func TestUDPForeignClockExistingAPI(t *testing.T) {
	for _, proto := range []struct {
		name, q, r string
		port       uint16
	}{{"SLMP", "500000ffff03000d0010001906000005004142434445", "d00000ffff03000900000005004142434445", 5000}, {"Wrapper", "000100100001000dc001c1000f0000280000ff0200", "0001000100100006c401c100112a", 4059}, {"HDLC", "7ea0190321107fdae6e600c001c1000f0000280000ff020091537e", "7ea012210330689de6e700c401c100112a43c37e", 4059}} {
		t.Run(proto.name, func(t *testing.T) {
			q, _ := hex.DecodeString(proto.q)
			r, _ := hex.DecodeString(proto.r)
			for _, foreign := range []string{"none", "TFTP", "SLMP-other-endpoint", "SLMP-other-interface"} {
				t.Run(foreign, func(t *testing.T) {
					var es []*ProtocolEvent
					var stats ProtocolStats
					a, err := NewPacketAnalyzer(WithProtocolDecodeAs("udp", 5000, "slmp"), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }))
					require.NoError(t, err)
					feed := func(dir int, sp, dp uint16, sec int64, iface int, w []byte) {
						src, dst := net.IP{192, 0, 2, 1}, net.IP{192, 0, 2, 2}
						if dir == 1 {
							src, dst = dst, src
						}
						eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
						ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: src, DstIP: dst}
						u := &layers.UDP{SrcPort: layers.UDPPort(sp), DstPort: layers.UDPPort(dp)}
						require.NoError(t, u.SetNetworkLayerForChecksum(ip))
						b := gopacket.NewSerializeBuffer()
						require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, u, gopacket.Payload(w)))
						raw := bytes.Clone(b.Bytes())
						require.NoError(t, a.Feed(raw, gopacket.CaptureInfo{Timestamp: time.Unix(sec, 0), CaptureLength: len(raw), Length: len(raw), InterfaceIndex: iface}, layers.LinkTypeEthernet))
						clear(raw)
					}
					feed(0, 40000, proto.port, 100, 0, q)
					switch foreign {
					case "TFTP":
						feed(0, 45000, 69, 3600, 0, []byte("\x00\x01other\x00octet\x00"))
					case "SLMP-other-endpoint":
						feed(0, 45000, proto.port, 3600, 0, q)
					case "SLMP-other-interface":
						feed(0, 40000, proto.port, 3600, 1, q)
					}
					feed(1, proto.port, 40000, 101, 0, r)
					require.NoError(t, a.Close())
					require.NoError(t, a.Close())
					n := 2
					if foreign != "none" {
						n = 3
					}
					if (proto.name == "Wrapper" || proto.name == "HDLC") && (foreign == "SLMP-other-endpoint" || foreign == "SLMP-other-interface") {
						require.Len(t, es, n+1)
						require.NotEqual(t, es[0].FlowID, es[1].FlowID)
						require.Equal(t, es[1].FlowID, es[n].FlowID)
						require.EqualValues(t, 1, stats.Incomplete)
						name := proto.name + "-foreign-endpoint"
						if foreign == "SLMP-other-interface" {
							name = proto.name + "-foreign-interface"
						}
						if proto.name == "HDLC" {
							for _, old := range es[:n] {
								require.NotEqual(t, old.ID, es[n].ID)
							}
							hdlcGetCloseHistorical(t, es[n:], name, "common/pcapx/pcaputil/udp-context/"+name+".pcapng")
						} else {
							wrapperCloseHistorical(t, es[n:], name, "common/pcapx/pcaputil/udp-context/"+name+".pcapng", "udp", false, "ctx-canceled")
						}
						es = es[:n]
					}
					require.Len(t, es, n)
					first, last := es[0], es[n-1]
					require.Equal(t, q, first.Raw)
					require.Equal(t, r, last.Raw)
					require.Empty(t, first.Error)
					require.Empty(t, last.Error, "foreign endpoint/domain capture time must not expire this timely request")
					require.Equal(t, first.ID, last.ResponseTo)
					require.Equal(t, first.FlowID, last.FlowID)
					require.Equal(t, 1, last.Direction)
					require.Zero(t, stats.BufferedBytes)
				})
			}
		})
	}
}
func TestObservedServiceQueryOwnership(t *testing.T) {
	for _, p := range []struct {
		name, q, r string
		port       uint16
	}{{"SLMP", "500000ffff03000d0010001906000005004142434445", "d00000ffff03000900000005004142434445", 5000}, {"Wrapper", "000100100001000dc001c1000f0000280000ff0200", "0001000100100006c401c100112a", 4059}, {"HDLC", "7ea0190321107fdae6e600c001c1000f0000280000ff020091537e", "7ea012210330689de6e700c401c100112a43c37e", 4059}} {
		t.Run(p.name, func(t *testing.T) {
			for _, transport := range []string{"udp", "tcp"} {
				t.Run(transport, func(t *testing.T) {
					for _, edit := range []bool{false, true} {
						t.Run(fmt.Sprint(edit), func(t *testing.T) {
							s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, p.port))
							require.NoError(t, err)
							q, _ := hex.DecodeString(p.q)
							r, _ := hex.DecodeString(p.r)
							a := s.Feed(0, time.Unix(100, 0), q)
							require.Len(t, a.Events, 1)
							b := s.Feed(1, time.Unix(101, 0), r)
							require.Len(t, b.Events, 1)
							e := b.Events[0]
							require.Empty(t, e.Error)
							before, err := e.Decode()
							require.NoError(t, err)
							gold, err := json.Marshal(before)
							require.NoError(t, err)
							if edit {
								tlsDTLSPoison(e, "redis")
							}
							after, err := e.Decode()
							require.NoError(t, err)
							got, err := json.Marshal(after)
							require.NoError(t, err)
							require.JSONEq(t, string(gold), string(got), "caller metadata cannot change an accepted observation")
							require.Empty(t, s.Close("done"))
							require.Empty(t, s.Close("again"))
							require.Zero(t, s.Stats().BufferedBytes)
						})
					}
				})
			}
		})
	}
}
