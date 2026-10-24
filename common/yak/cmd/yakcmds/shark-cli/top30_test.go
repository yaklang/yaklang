package sharkcli

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

const top30Sessions = "../../../../pcapx/pcaputil/testdata/protocol-sessions/"
const top30Corpus = "../../../../bin-parser/testdata/protocol-corpus/captures/"

// Family coverage is not a claim that every version or encrypted profile is
// supported. Each case names its actual carrier fixture and asserted output.
type top30Case struct {
	name, file, key string
	labels, layers  []string
}

func top30Cases() []top30Case {
	tls := top30Sessions + "first-batch-m1/tls-h2-bidi.pcap"
	tlsKey := top30Sessions + "first-batch-m1/tls-h2-bidi.keys"
	return []top30Case{
		{"01 TCP", tls, "", []string{"TCP"}, []string{"TCP"}},
		{"02 IPv4 IPv6", top30Sessions + "first-batch-m1/dhcpv6-native.pcap", "", nil, []string{"IPv6"}},
		{"03 TLS 1.2 1.3", tls, "", []string{"TLSv1.2", "TLSv1.3"}, nil},
		{"04 DNS", top30Corpus + "ndpi/ndpi-dns.pcap", "", []string{"DNS"}, nil},
		{"05 UDP", top30Corpus + "ndpi/ndpi-dns.pcap", "", nil, []string{"UDP"}},
		{"06 Ethernet VLAN", "../../../../bin-parser/testdata/protocol-recognition-regressions/goose-opcua/goose-vlan.pcap", "", nil, []string{"Ethernet", "Dot1Q"}},
		{"07 HTTP1", top30Sessions + "first-batch-m1/http-ws-native.pcap", "", []string{"HTTP/1.1"}, nil},
		{"08 QUIC HTTP3", top30Sessions + "m2b-quic-native.pcap", top30Sessions + "m2b-quic.keys", []string{"QUIC", "HTTP/3"}, []string{"UDP"}},
		{"09 HTTP2 gRPC", tls, tlsKey, []string{"HTTP/2", "gRPC"}, nil},
		{"10 ARP", "synthetic:ethernet-arp", "", []string{"ARP"}, nil},
		{"11 ICMP NDP", top30Sessions + "first-batch-m1/dhcpv6-native.pcap", "", []string{"ICMPV6"}, nil},
		{"12 DHCP", top30Sessions + "first-batch-m1/dhcpv6-native.pcap", "", []string{"DHCPv6"}, nil},
		{"13 mDNS LLMNR NBNS", top30Corpus + "wireshark-tests/wireshark-mdns.pcap", "", []string{"mDNS"}, nil},
		{"14 WebSocket", top30Sessions + "first-batch-m1/http-ws-native.pcap", "", []string{"WebSocket"}, nil},
		{"15 SMB2 SMB3", top30Sessions + "first-batch-oracles/smb2-samba-live.pcap", "", []string{"SMB2"}, nil},
		{"16 Kerberos", top30Corpus + "ndpi/ndpi-kerberos-login.pcap", "", []string{"Kerberos"}, nil},
		{"17 SPNEGO NTLM", top30Sessions + "first-batch-oracles/smb2-samba-live.pcap", "", []string{"SPNEGO", "NTLMSSP"}, nil},
		{"18 LDAP", top30Sessions + "ldap-bind-search.pcap", "", []string{"LDAP"}, nil},
		{"19 RDP", top30Sessions + "rdp-tpkt-negotiate-mcs.pcap", "", []string{"RDP"}, nil},
		{"20 SSH", top30Sessions + "ssh-kex-newkeys.pcap", "", []string{"SSH"}, nil},
		{"21 PostgreSQL", top30Sessions + "postgres-extended-query.pcap", "", []string{"PostgreSQL"}, nil},
		{"22 MySQL MariaDB", top30Sessions + "m2b-database-native.pcap", "", []string{"MySQL"}, nil},
		{"23 Redis", top30Sessions + "first-batch-m2a/m2-native.pcap", "", []string{"Redis"}, nil},
		{"24 MQTT", top30Sessions + "mqtt5-connect-qos.pcap", "", []string{"MQTT"}, nil},
		{"25 SMTP IMAP POP3", top30Sessions + "smtp-ehlo-mail.pcap", "", []string{"SMTP"}, nil},
		{"26 FTP FTPS", top30Sessions + "ftp-user-pass-pasv.pcap", "", []string{"FTP"}, nil},
		{"27 NTP", top30Corpus + "ndpi/ndpi-ntpv3.pcap", "", []string{"NTP"}, nil},
		{"28 SNMP", top30Sessions + "first-batch-oracles/net-snmp-v1-live.pcap", "", []string{"SNMP"}, nil},
		{"29 STUN TURN ICE", top30Sessions + "upstream/stun_tcp_multiple_msgs_same_pkt.pcap", "", []string{"TURN"}, nil},
		{"30 SIP RTP RTCP", top30Sessions + "first-batch-oracles/sip-sipp-rtp-duplex-loopback.pcap", "", []string{"SIP", "RTP"}, nil},
	}
}

func top30Replay(t testing.TB, tc top30Case) ([]*capturedPacket, *captureSession) {
	t.Helper()
	var path string
	var err error
	if tc.file == "synthetic:ethernet-arp" {
		path = top30ARP(t)
	} else {
		path, err = trafficfixture.Materialize(tc.file, t.TempDir())
	}
	require.NoError(t, err)
	cfg := captureConfig{input: path, snaplen: 65535, count: 2000}
	if tc.key != "" {
		data, err := trafficfixture.ReadFile(tc.key)
		require.NoError(t, err)
		cfg.tlsSecrets, err = pcaputil.ParseTLSKeyLog(string(data))
		require.NoError(t, err)
	}
	src, err := openCapture(cfg)
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, cfg, false)
	var packets []*capturedPacket
	for p := range s.packets {
		packets = append(packets, p)
	}
	require.NoError(t, s.result())
	require.NotEmpty(t, packets)
	return packets, s
}

func top30ARP(t testing.TB) string {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{255, 255, 255, 255, 255, 255}, EthernetType: layers.EthernetTypeARP}
	arp := &layers.ARP{AddrType: layers.LinkTypeEthernet, Protocol: layers.EthernetTypeIPv4, HwAddressSize: 6, ProtAddressSize: 4, Operation: 1, SourceHwAddress: eth.SrcMAC, SourceProtAddress: []byte{192, 0, 2, 1}, DstHwAddress: make([]byte, 6), DstProtAddress: []byte{192, 0, 2, 2}}
	frame := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(frame, gopacket.SerializeOptions{FixLengths: true}, eth, arp))
	var b bytes.Buffer
	w := pcapgo.NewWriter(&b)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(frame.Bytes()), Length: len(frame.Bytes())}, frame.Bytes()))
	path := filepath.Join(t.TempDir(), "synthetic-arp.pcap")
	require.NoError(t, os.WriteFile(path, b.Bytes(), 0600))
	return path
}

func TestTop30CapturePresentation(t *testing.T) {
	cases := top30Cases()
	require.Len(t, cases, 30)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			packets, s := top30Replay(t, tc)
			seen, layers := map[string]int{}, map[string]bool{}
			for _, p := range packets {
				row := summarize(p)
				seen[row.Protocol]++
				for _, name := range row.Layers {
					layers[name] = true
				}
				if row.ProtocolEvidence == "validated protocol events" {
					require.Contains(t, []string{"decoded", "deferred"}, row.ProtocolStatus)
					require.NotEmpty(t, row.ProtocolStack)
				}
				if p.streamID != 0 && len(p.connectionStack) > 0 {
					v := s.streams.snapshot(p.streamID)
					if v != nil {
						require.NotEmpty(t, v.protocolStack, "connection identity survives after the packet")
					}
				}
			}
			for _, want := range tc.labels {
				require.Positive(t, seen[want], "missing %s; observed %v", want, seen)
			}
			for _, want := range tc.layers {
				require.True(t, layers[want], "missing %s; observed %v", want, layers)
			}
			t.Logf("profile=%s fixture=%s rows=%d labels=%v", tc.name, filepath.Base(tc.file), len(packets), seen)
		})
	}
}

func TestTop30DiagnosticNeverPromotesPartialIdentity(t *testing.T) {
	for _, name := range []string{"tls", "dns", "http", "quic", "http3", "http2", "grpc", "arp", "icmpv6", "dhcp", "dhcpv6", "mdns", "llmnr", "nbns", "websocket", "smb2", "kerberos", "spnego", "ntlmssp", "ldap", "rdp", "ssh", "postgresql", "mysql", "redis", "mqtt", "smtp", "imap", "pop3", "ftp", "ntp", "snmp", "stun", "turn", "sip", "rtp", "rtcp"} {
		for _, status := range []string{"incomplete", "unrecognized", "malformed", "context-required", "limited"} {
			t.Run(name+"/"+status, func(t *testing.T) {
				e := &pcaputil.ProtocolEvent{Protocol: name, Status: status, ExpertCode: "NeedMore", Error: "only the first fields matched", Fields: map[string]any{"first": 1, "second": 2}}
				row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
				require.Equal(t, "TCP", row.Protocol)
				require.Empty(t, row.ProtocolStack)
				require.Contains(t, row.Info, "first fields")
			})
		}
	}
}

func TestTop30NoBinaryMagicPromotion(t *testing.T) {
	for _, payload := range []string{"SSH-", "SSH-2.0-\r\n", "SSH-7.9-fake\r\n", "\xfeSMB", "\xffSMB", "\x16\x03\x03\x00\x04aaaa", "\x10\x10\x00\x04MQTT\x04", "AMQP\x00\x00\x09\x01", "\x00\x00\x00\x08\x00\x03\x00\x00", "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"} {
		t.Run(fmt.Sprintf("%x", payload), func(t *testing.T) {
			require.Empty(t, sniffPayload([]byte(payload)))
			require.Empty(t, sniffStreamPayload([]byte(payload), 443, 445))
		})
	}
}

func TestTop30RelatedProfiles(t *testing.T) {
	cases := []top30Case{
		{"DNS TCP is not proof of TLS", top30Sessions + "dot-dns-tcp-length.pcap", "", []string{"DNS"}, nil},
		{"DoH HTTP envelope", top30Sessions + "doh-http-get-post.pcap", "", []string{"DoH"}, nil},
		{"DoQ native QUIC", top30Sessions + "m2b-quic-native.pcap", top30Sessions + "m2b-quic.keys", []string{"DoQ"}, nil},
		{"DHCPv4", top30Corpus + "wireshark-tests/wireshark-dhcp.pcap", "", []string{"DHCP"}, nil},
		{"LLMNR", top30Corpus + "generated-local/gen-llmnr.pcap", "", []string{"LLMNR"}, nil},
		{"NBNS", top30Corpus + "generated-local/gen-nbns.pcap", "", []string{"NBNS"}, nil},
		{"IMAP", top30Sessions + "imap-capability-login.pcap", "", []string{"IMAP"}, nil},
		{"POP3", top30Sessions + "pop3-capa-stat.pcap", "", []string{"POP3"}, nil},
		{"NTPv4", top30Corpus + "ndpi/ndpi-ntpv4.pcap", "", []string{"NTP"}, nil},
		{"SNMPv2c", top30Sessions + "first-batch-oracles/net-snmp-rsyslog-interop-v2c.pcap", "", []string{"SNMP"}, nil},
		{"SNMPv3 security boundaries", top30Sessions + "first-batch-oracles/net-snmp-v3-security-boundaries.pcap", "", []string{"SNMP"}, nil},
		{"STUN", top30Corpus + "ndpi/ndpi-stun.pcap", "", []string{"STUN"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			packets, _ := top30Replay(t, tc)
			seen := map[string]bool{}
			for _, p := range packets {
				seen[summarize(p).Protocol] = true
			}
			for _, name := range tc.labels {
				require.True(t, seen[name], "%v", seen)
			}
			if tc.name == "DNS TCP is not proof of TLS" {
				require.False(t, seen["DoT"])
			}
		})
	}
}
