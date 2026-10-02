package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// These short captures have exactly one protocol-bearing message each. Every
// record is inspected: TCP setup records are explicitly counted, not skipped
// based on the material index's representative-frame selection.
func TestProtocolCorpusPR5023KnownProtocolEveryRecord(t *testing.T) {
	for _, name := range []string{"stp", "pap", "linux-sll", "llmnr-resp", "jdwp-full", "redis", "tacacs-plus", "smb3"} {
		t.Run(name, func(t *testing.T) {
			frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-"+name+".pcap")
			expectedRecords := 4
			if name == "stp" || name == "pap" || name == "linux-sll" || name == "llmnr-resp" {
				expectedRecords = 1
			}
			require.Len(t, frames, expectedRecords)
			controls, messages := 0, 0
			for index, frame := range frames {
				t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
					if expectedRecords == 4 {
						packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
						require.Nil(t, packet.ErrorLayer())
						tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
						protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
						if len(tcp.Payload) == 0 {
							controls++
							require.LessOrEqual(t, index, 2)
							require.Equal(t, index < 2, tcp.SYN)
							require.Equal(t, index > 0, tcp.ACK)
							return
						}
						wire := tcp.Payload
						switch name {
						case "jdwp-full":
							require.Equal(t, []byte("JDWP-Handshake"), wire)
							node := protocolCorpusRequireBoundedRuleParse(t, wire, "jdwp", "JDWP")
							protocolCorpusRequireValue(t, node, "Handshake", wire)
							require.Nil(t, protocolCorpusFindNode(node, "Command"), "the filename does not make this a command exchange")
						case "redis":
							require.Equal(t, []byte("*1\r\n$4\r\nPING\r\n"), wire)
							node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.redis", "Redis")
							protocolCorpusRequireValue(t, node, "Prefix", uint64('*'))
							protocolCorpusRequireValue(t, node, "Count", "1")
							command := protocolCorpusFindNode(node, "RedisCommand")
							protocolCorpusRequireValue(t, command, "Prefix", uint64('$'))
							protocolCorpusRequireValue(t, command, "Len", "4")
							protocolCorpusRequireValue(t, command, "Command", "PING")
							protocolCorpusRequireValue(t, command, "CRLF", []byte("\r\n"))
						case "tacacs-plus":
							require.Equal(t, mustHex(t, "c001010000000001000000080000000000000000"), wire)
							node := protocolCorpusRequireBoundedRuleParse(t, wire, "tacacs", "TACACS")
							protocolCorpusTACACSHeaderFields(t, node, wire)
							protocolCorpusRequireValue(t, node, "Obfuscated Body", wire[12:])
							require.Nil(t, protocolCorpusFindNode(node, "Action"))
						case "smb3":
							require.Zero(t, wire[0])
							require.EqualValues(t, len(wire)-4, binary.BigEndian.Uint32(wire))
							wire = wire[4:]
							node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.smb2", "SMB2")
							protocolCorpusRequireValue(t, node, "ProtocolId", uint64(0x424d53fe))
							protocolCorpusRequireValue(t, node, "Command", uint64(0))
							protocolCorpusRequireValue(t, node, "DialectCount", uint64(3))
							dialects := protocolCorpusNodesNamed(node, "Dialect")
							require.Len(t, dialects, 3)
							for i, value := range []uint64{0x0300, 0x0302, 0x0311} {
								protocolCorpusRequireValue(t, dialects[i], "Dialect", value)
							}
							require.Nil(t, protocolCorpusFindNode(node, "SMB3"))
						}
					} else {
						switch name {
						case "stp":
							require.Equal(t, []byte{0x42, 0x42, 3}, frame[14:17])
							envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
							wire := frame[17:]
							require.Len(t, wire, 35)
							for _, node := range []*base.Node{envelope, protocolCorpusRequireBoundedRuleParse(t, wire, "stp", "STP")} {
								protocolCorpusRequireValue(t, node, "Protocol ID", uint64(0))
								protocolCorpusRequireValue(t, node, "Version", uint64(0))
								protocolCorpusRequireValue(t, node, "BPDU Type", uint64(0))
								protocolCorpusRequireValue(t, node, "Flags", uint64(wire[4]))
								for field, offset := range map[string]int{"Root Priority": 5, "Bridge Priority": 17} {
									protocolCorpusRequireValue(t, node, field, uint64(wire[offset]>>4))
								}
								for field, offset := range map[string]int{"Root Ext High": 5, "Bridge Ext High": 17} {
									protocolCorpusRequireValue(t, node, field, uint64(wire[offset]&15))
								}
								protocolCorpusRequireValue(t, node, "Root Ext Low", uint64(wire[6]))
								protocolCorpusRequireValue(t, node, "Bridge Ext Low", uint64(wire[18]))
								protocolCorpusRequireValue(t, node, "Root MAC", wire[7:13])
								protocolCorpusRequireValue(t, node, "Bridge MAC", wire[19:25])
								protocolCorpusRequireValue(t, node, "Root Path Cost", uint64(binary.BigEndian.Uint32(wire[13:])))
								for field, offset := range map[string]int{"Port ID": 25, "Message Age": 27, "Max Age": 29, "Hello Time": 31, "Forward Delay": 33} {
									protocolCorpusRequireValue(t, node, field, uint64(binary.BigEndian.Uint16(wire[offset:])))
								}
							}
						case "pap":
							protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
							require.Equal(t, mustHex(t, "886411000001000dc023"), frame[12:22])
							wire := frame[22:]
							node := protocolCorpusRequireBoundedRuleParse(t, wire, "password_authentication_protocol", "PAP")
							for field, value := range map[string]uint64{"Code": 1, "Identifier": 0, "Length": 11, "Peer ID Length": 4, "Password Length": 1} {
								protocolCorpusRequireValue(t, node, field, value)
							}
							protocolCorpusRequireValue(t, node, "Peer ID", "user")
							protocolCorpusRequireValue(t, node, "Password", "x")
						case "linux-sll":
							node := protocolCorpusRequireBoundedRuleParse(t, frame, "linux_sll", "LinuxSLL")
							for field, offset := range map[string]int{"Packet Type": 0, "ARPHRD": 2, "Address Length": 4, "Protocol": 14} {
								protocolCorpusRequireValue(t, node, field, uint64(binary.BigEndian.Uint16(frame[offset:])))
							}
							protocolCorpusRequireValue(t, node, "Source MAC", frame[6:12])
							protocolCorpusRequireValue(t, node, "Unused", frame[12:14])
							protocolCorpusRequireValue(t, node, "Source", []byte{10, 0, 0, 1})
							protocolCorpusRequireValue(t, node, "Destination", []byte{10, 0, 0, 2})
						case "llmnr-resp":
							packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
							wire := packet.Layer(layers.LayerTypeUDP).(*layers.UDP).Payload
							node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.nbns", "LLMNR")
							for field, offset := range map[string]int{"ID": 0, "Flags": 2, "Questions": 4, "Answer RRs": 6, "Authority RRs": 8, "Additional RRs": 10} {
								protocolCorpusRequireValue(t, protocolCorpusFindNode(node, "Header"), field, uint64(binary.BigEndian.Uint16(wire[offset:])))
							}
							labels := protocolCorpusNodesNamed(node, "Text")
							require.Len(t, labels, 4)
							for i, label := range []string{"host", "local", "host", "local"} {
								protocolCorpusRequireValue(t, labels[i], "Text", label)
							}
							for _, name := range []string{"Question", "Answer"} {
								child := protocolCorpusFindNode(node, name)
								protocolCorpusRequireValue(t, child, "Type", uint64(1))
								protocolCorpusRequireValue(t, child, "Class", uint64(1))
							}
							protocolCorpusRequireValue(t, node, "TTL", uint64(30))
							protocolCorpusRequireValue(t, node, "RDLength", uint64(4))
							protocolCorpusRequireValue(t, node, "Address", []byte{10, 0, 0, 1})
						}
					}
					messages++
				})
			}
			require.Equal(t, expectedRecords-1, controls)
			require.Equal(t, 1, messages)
		})
	}
}

func protocolCorpusTACACSHeaderFields(t *testing.T, node *base.Node, wire []byte) {
	t.Helper()
	for field, offset := range map[string]int{"Version": 0, "Type": 1, "Sequence": 2, "Flags": 3} {
		protocolCorpusRequireValue(t, node, field, uint64(wire[offset]))
	}
	for field, offset := range map[string]int{"Session ID": 4, "Length": 8} {
		protocolCorpusRequireValue(t, node, field, uint64(binary.BigEndian.Uint32(wire[offset:])))
	}
	info := node.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, wire[3]&1 == 0, info["Body Obfuscated"])
	require.Equal(t, wire[2]&1 != 0, info["Client Packet"])
	require.Equal(t, wire[3]&4 != 0, info["Single Connection"])
}

func TestProtocolCorpusTACACSBodyBoundaries(t *testing.T) {
	makeRecord := func(typ, seq, flags byte, body []byte) []byte {
		header := []byte{0xc0, typ, seq, flags, 0xfe, 0xdc, 0xba, 0x98, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(header[8:], uint32(len(body)))
		return append(header, body...)
	}
	start := append([]byte{1, 1, 1, 1, 1, 2, 3, 4}, []byte("uportxtdat")...)
	for i, wire := range [][]byte{
		makeRecord(1, 1, 0, make([]byte, 8)),
		makeRecord(1, 1, 1, start),
		makeRecord(1, 3, 1, []byte{0, 0, 0, 0, 0}),
		makeRecord(1, 2, 5, []byte{7, 0, 0, 0, 0, 0}),
		makeRecord(2, 2, 0, []byte{0xff, 0x80, 0}),
		makeRecord(3, 1, 4, []byte{0x41}),
		makeRecord(3, 2, 0, nil), // length-zero error record remains inspectable
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "tacacs", "TACACS")
			protocolCorpusTACACSHeaderFields(t, node, wire)
			decoded := i == 1
			require.Equal(t, decoded, node.Cfg.GetItem("additionInfo").(map[string]any)["Body Decoded"])
			if decoded {
				for field, value := range map[string]string{"User": "u", "Port": "po", "RemAddr": "rtx", "Token": "tdat"} {
					protocolCorpusRequireValue(t, node, field, value)
				}
			} else if len(wire) > 12 {
				field := "Obfuscated Body"
				if wire[3]&1 != 0 {
					field = "Cleartext Body"
				}
				protocolCorpusRequireValue(t, node, field, wire[12:])
				require.Nil(t, protocolCorpusFindNode(node, "Action"))
			}
			for cut := 0; cut < len(wire); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "tacacs", "TACACS")
				require.Error(t, err)
			}
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(bytes.Clone(wire), 0)), "tacacs", "TACACS")
			require.ErrorContains(t, err, "tacacs: length does not match record boundary")
		})
	}
	for _, body := range [][]byte{make([]byte, 7), {1, 1, 1, 1, 3, 0, 0, 0, 'a'}} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(makeRecord(1, 1, 1, body)), "tacacs", "TACACS")
		require.Error(t, err)
	}
	wire := makeRecord(1, 1, 0, make([]byte, 8))
	for _, limit := range []int{0, -1, 11, 19} {
		_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), "tacacs", map[string]any{"tacacsRecordLimit": limit}, "TACACS")
		require.Error(t, err)
	}
	protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, "tacacs", "TACACS", map[string]any{"tacacsRecordLimit": 20})
}
