package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func h225TestHex(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	require.NoError(t, e)
	return b
}

func h225TestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.h225", entry)
	require.Equal(t, wire, NodeToBytes(n))
	h225TestTree(t, n, wire, 0)
	return n
}

func h225TestTree(t *testing.T, n *base.Node, wire []byte, offset uint64) {
	t.Helper()
	var walk func(*base.Node, uint64) uint64
	walk = func(n *base.Node, start uint64) uint64 {
		if stream_parser.NodeHasResult(n) {
			span := stream_parser.GetNodeResultPos(n)
			require.Equal(t, start, span[0], n.Name)
			require.LessOrEqual(t, span[1], offset+uint64(len(wire))*8, n.Name)
			reader := base.NewBitReader(bytes.NewReader(wire))
			if span[0] > offset {
				_, e := reader.ReadBits(span[0] - offset)
				require.NoError(t, e)
			}
			var expected []byte
			if span[1] > span[0] {
				var e error
				expected, e = reader.ReadBits(span[1] - span[0])
				require.NoError(t, e)
			}
			if span[1] > span[0] {
				require.Equal(t, expected, stream_parser.GetBytesByNode(n), n.Name)
			}
			v, e := n.Result()
			require.NoError(t, e, n.Name)
			require.Same(t, n, v.Origin)
			return span[1]
		}
		for _, child := range n.Children {
			start = walk(child, start)
		}
		return start
	}
	require.Equal(t, offset+uint64(len(wire))*8, walk(n, offset))
	require.Equal(t, uint64(len(wire))*8, stream_parser.CalcNodeConsumedLength(n))
}

// This wrapper retains the complete original Ethernet record and passes only
// its independently checked transport payload into the explicit H225 entry.
// It is deliberately not a default IP/TCP/UDP protocol dispatcher.
func h225TestWholeRecord(t *testing.T, record []byte, start, size int, entry string) *base.Node {
	t.Helper()
	suffix := len(record) - start - size
	require.GreaterOrEqual(t, suffix, 0)
	source := fmt.Sprintf("endian: big\nunit: byte\nPackage:\n  Capture:\n    operator: |\n      this.ProcessSubNode(\"Envelope\")\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      if %d > 0 { this.ProcessSubNode(\"Capture Tail\") }\n    Envelope: raw,%d\n    Message: \"import:application-layer/h225.yaml;node:%s\"\n    Capture Tail: raw,%d\n", size, suffix, start, entry, suffix)
	var doc yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
	root, e := base.NewNodeTree(doc)
	require.NoError(t, e)
	root.Cfg.SetItem(base.CfgLength, uint64(len(record))*8)
	reader := base.NewBitReader(bytes.NewReader(record))
	require.NoError(t, root.ParseSubNode(reader, "Capture"))
	n := base.GetNodeByPath(root, "@Capture")
	require.Equal(t, record, NodeToBytes(n))
	h225TestTree(t, n, record, 0)
	require.ErrorContains(t, reader.Recovery(), "no backup")
	return protocolCorpusFindNode(n, "Message")
}

func TestProtocolCorpusH225AllOriginalRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-h323.pcap"
	data, e := os.ReadFile(path)
	require.NoError(t, e)
	require.Equal(t, "cac23940a39b4cd2ef97003f0561dcc8ce20f730ec2e83169dac471fd218edcf", fmt.Sprintf("%x", sha256.Sum256(data)))
	records := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-h323.pcap")
	require.Len(t, records, 75)
	validCall := map[int]uint64{6: 0, 7: 0, 10: 1, 11: 1, 14: 3, 15: 3, 18: 2, 19: 2, 47: 0, 66: 5}
	rasSeq := map[int]uint64{60: 1, 61: 2, 62: 2, 63: 3, 64: 3, 67: 4180, 68: 4180, 69: 4181, 70: 4181, 71: 18067, 72: 18067, 73: 18068, 74: 18068, 75: 18069}
	rasChoice := map[int]uint64{60: 1, 61: 3, 62: 4, 63: 9, 64: 10, 67: 21, 68: 21, 69: 15, 70: 15, 71: 3, 72: 4, 73: 3, 74: 4, 75: 3}
	counts := map[string]int{}
	tcpByFrame := map[int]*layers.TCP{}
	for index, record := range records {
		frame := index + 1
		t.Run(fmt.Sprintf("frame-%d", frame), func(t *testing.T) {
			packet := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
			ip, ok := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
			require.True(t, ok)
			require.Zero(t, ip.FragOffset)
			require.Equal(t, layers.IPv4Flag(0), ip.Flags&layers.IPv4MoreFragments)
			start := 14 + int(ip.IHL)*4
			if udp, ok := packet.Layer(layers.LayerTypeUDP).(*layers.UDP); ok {
				start += 8
				require.Equal(t, len(udp.Payload), int(udp.Length)-8)
				require.Equal(t, udp.Payload, record[start:start+len(udp.Payload)])
				n := h225TestWholeRecord(t, record, start, len(udp.Payload), "H225RASCarrier")
				if frame == 59 {
					counts["malformed"]++
					protocolCorpusRequireValue(t, n, "Unparsed H225 Message", udp.Payload)
					return
				}
				counts["ras"]++
				protocolCorpusRequireValue(t, n, "requestSeqNum", rasSeq[frame])
				protocolCorpusRequireValue(t, n, "RasMessage Choice", rasChoice[frame])
				require.Nil(t, protocolCorpusFindNode(n, "Unparsed H225 Message"))
				switch frame {
				case 60:
					protocolCorpusRequireValue(t, n, "protocolIdentifier Bytes", "0.0.8.2250.0.4")
					protocolCorpusRequireValue(t, n, "gatekeeperIdentifier Bytes", "OpenH323 Gatekeeper on mfottekin")
				case 63:
					protocolCorpusRequireValue(t, n, "bandWidth", uint64(200000))
					protocolCorpusRequireValue(t, n, "calls", uint64(1000))
					protocolCorpusRequireValue(t, n, "group Bytes", "59")
				case 71, 73, 75:
					protocolCorpusRequireValue(t, n, "h323-ID Bytes", "20203@am.sol")
					protocolCorpusRequireValue(t, n, "dialledDigits Bytes", "2098")
					protocolCorpusRequireValue(t, n, "endpointIdentifier Value Bytes", "bd020b80-6d41-11e1-a7fb-0010f30f65a0_17")
					require.NotNil(t, protocolCorpusFindNode(n, "featureSet Value"))
					require.NotNil(t, protocolCorpusFindNode(n, "genericData Value"))
				case 72, 74:
					protocolCorpusRequireValue(t, n, "gatekeeperIdentifier Bytes", "am-vcs-0")
					protocolCorpusRequireValue(t, n, "alternateGatekeeper Value Count", uint64(3))
				}
				return
			}
			tcp, ok := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
			require.True(t, ok)
			tcpByFrame[frame] = tcp
			start += int(tcp.DataOffset) * 4
			if len(tcp.Payload) == 0 {
				counts["empty"]++
				require.Equal(t, 0, int(ip.Length)-int(ip.IHL)*4-int(tcp.DataOffset)*4)
				return
			}
			require.Equal(t, tcp.Payload, record[start:start+len(tcp.Payload)])
			n := h225TestWholeRecord(t, record, start, len(tcp.Payload), "H225CallCarrier")
			if choice, ok := validCall[frame]; ok {
				counts["call"]++
				protocolCorpusRequireValue(t, n, "h323-message-body Choice", choice)
				protocolCorpusRequireValue(t, n, "TPKT Length", uint64(len(tcp.Payload)))
				require.Nil(t, protocolCorpusFindNode(n, "Unparsed H225 Message"))
				if frame == 6 {
					protocolCorpusRequireValue(t, n, "protocolIdentifier Bytes", "0.0.8.2250.0.4")
					protocolCorpusRequireValue(t, n, "h323-ID Bytes", "m.jemec")
					protocolCorpusRequireValue(t, n, "manufacturerCode", uint64(61))
				}
				return
			}
			protocolCorpusRequireValue(t, n, "Unparsed H225 Message", tcp.Payload)
			require.Nil(t, protocolCorpusFindNode(n, "TPKT Version"))
			switch {
			case frame == 48 || frame == 50:
				counts["fragment"]++
			case frame == 65:
				counts["malformed"]++
			case tcp.SrcPort == 1232 || tcp.DstPort == 1232:
				counts["h245"]++
			case frame == 53 || frame == 55 || frame == 57:
				counts["keepalive"]++
				require.Equal(t, []byte{0}, tcp.Payload)
			default:
				t.Fatalf("unaccounted TCP data frame %d", frame)
			}
		})
	}
	require.Equal(t, map[string]int{"empty": 33, "call": 10, "ras": 14, "malformed": 2, "fragment": 2, "h245": 11, "keepalive": 3}, counts)
	// No gap and no duplicate in this two-segment Alerting assembly; all other
	// retransmissions remain individually tested above against the original data.
	a, b := tcpByFrame[48], tcpByFrame[50]
	require.Equal(t, a.Seq+uint32(len(a.Payload)), b.Seq)
	require.Equal(t, a.SrcPort, b.SrcPort)
	require.Equal(t, a.DstPort, b.DstPort)
	assembled := append(bytes.Clone(a.Payload), b.Payload...)
	require.Len(t, assembled, 43)
	require.Equal(t, len(assembled), int(binary.BigEndian.Uint16(assembled[2:4])))
	n := h225TestParse(t, assembled, "H225Call")
	protocolCorpusRequireValue(t, n, "h323-message-body Choice", uint64(3))
	protocolCorpusRequireValue(t, n, "protocolIdentifier Bytes", "0.0.8.2250.0.2")
	for _, pair := range [][2]int{{6, 7}, {10, 11}, {14, 15}, {18, 19}} {
		require.Equal(t, tcpByFrame[pair[0]].Seq, tcpByFrame[pair[1]].Seq)
		require.Equal(t, tcpByFrame[pair[0]].Payload, tcpByFrame[pair[1]].Payload)
	}
	for _, frame := range []int{53, 55, 57} {
		require.Equal(t, tcpByFrame[47].Seq+uint32(len(tcpByFrame[47].Payload))-1, tcpByFrame[frame].Seq)
	}
}

func TestProtocolCorpusH225PR5023NegativeAndIndependentPER(t *testing.T) {
	companion := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-h225-valid.pcap")
	require.Len(t, companion, 1)
	cp := gopacket.NewPacket(companion[0], layers.LayerTypeEthernet, gopacket.Default)
	cu := cp.Layer(layers.LayerTypeUDP).(*layers.UDP)
	require.Equal(t, h225TestHex(t, "00000000060008914a0004007f00000106b70000"), cu.Payload)
	cn := h225TestWholeRecord(t, companion[0], 42, len(cu.Payload), "H225RAS")
	protocolCorpusRequireValue(t, cn, "requestSeqNum", uint64(1))
	protocolCorpusRequireValue(t, cn, "port", uint64(1719))
	records := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-h225.pcap")
	require.Len(t, records, 1)
	record := records[0]
	packet := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
	udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
	require.Equal(t, h225TestHex(t, "001c00080000000000000000"), udp.Payload)
	n := h225TestWholeRecord(t, record, 42, len(udp.Payload), "H225RASCarrier")
	protocolCorpusRequireValue(t, n, "Unparsed H225 Message", udp.Payload)
	_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(udp.Payload), "application-layer.h225", "H225RAS")
	require.Error(t, e)
	// Independently serialized minimal GRQ: choice 0, absent optionals,
	// requestSeqNum 1, H.225 version 4, IPv4 127.0.0.1:1719, empty endpoint.
	valid := h225TestHex(t, "00000000060008914a0004007f00000106b70000")
	n = h225TestParse(t, valid, "H225RAS")
	protocolCorpusRequireValue(t, n, "requestSeqNum", uint64(1))
	protocolCorpusRequireValue(t, n, "port", uint64(1719))
	protocolCorpusRequireValue(t, n, "ip Bytes", []byte{127, 0, 0, 1})
	for cut := 0; cut < len(valid); cut++ {
		wire := valid[:cut]
		_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "application-layer.h225", "H225RAS")
		require.Error(t, e, "prefix %d", cut)
		if cut > 0 {
			n := h225TestParse(t, wire, "H225RASCarrier")
			protocolCorpusRequireValue(t, n, "Unparsed H225 Message", wire)
		}
	}
	for _, wire := range [][]byte{append(bytes.Clone(valid), 0), {0x7c}, {0x80, 0}, {0, 0, 0xff, 0xff, 6, 0, 8, 0x91, 0x4a, 0, 4}} {
		_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "application-layer.h225", "H225RAS")
		require.Error(t, e)
		n := h225TestParse(t, wire, "H225RASCarrier")
		protocolCorpusRequireValue(t, n, "Unparsed H225 Message", wire)
	}
}

func TestProtocolCorpusH225OffsetsBoundsAndIsolation(t *testing.T) {
	valid := h225TestHex(t, "00000000060008914a0004007f00000106b70000")
	for offset := uint64(0); offset < 8; offset++ {
		for _, good := range []bool{true, false} {
			t.Run(fmt.Sprintf("%d/%t", offset, good), func(t *testing.T) {
				wire := bytes.Clone(valid)
				if !good {
					wire = append(wire, 0)
				}
				var packed bytes.Buffer
				w := base.NewBitWriter(&packed)
				if offset > 0 {
					require.NoError(t, w.WriteBits([]byte{0x55}, offset))
				}
				require.NoError(t, w.WriteBits(wire, uint64(len(wire))*8))
				require.NoError(t, w.WriteBits([]byte{0xd3}, 8))
				if offset > 0 {
					require.NoError(t, w.WriteBits([]byte{0}, 8-offset))
				}
				source := fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/h225.yaml;node:H225RASCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(wire), offset, offset, 8-offset)
				var doc yaml.MapSlice
				require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
				root, e := base.NewNodeTree(doc)
				require.NoError(t, e)
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("caller-marker", "held")
				reader := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, root.ParseSubNode(reader, "Wrapped"))
				n := base.GetNodeByPath(root, "@Wrapped")
				message := protocolCorpusFindNode(n, "Message")
				h225TestTree(t, message, wire, offset)
				if good {
					protocolCorpusRequireValue(t, message, "requestSeqNum", uint64(1))
					protocolCorpusRequireValue(t, message, "port", uint64(1719))
				} else {
					protocolCorpusRequireValue(t, message, "Unparsed H225 Message", wire)
					require.Nil(t, protocolCorpusFindNode(message, "RasMessage Choice"))
				}
				protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
				require.Equal(t, packed.Bytes(), NodeToBytes(n))
				require.Equal(t, "held", root.Ctx.GetItem("caller-marker"))
				require.ErrorContains(t, reader.Recovery(), "no backup")
				_, e = reader.ReadBits(8)
				require.ErrorIs(t, e, io.EOF)
			})
		}
	}
	for _, entry := range []string{"H225RAS", "H225Call", "H225RASCarrier", "H225CallCarrier"} {
		_, e := parser.ParseBinary(bytes.NewReader(valid), "application-layer.h225", entry)
		require.ErrorContains(t, e, "explicit")
		_, e = parser.GenerateBinary(map[string]any{}, "application-layer.h225", entry)
		require.Error(t, e)
	}
	for cut := 0; cut < len(valid); cut++ {
		root, e := base.ParseRule("application-layer/h225.yaml")
		require.NoError(t, e)
		root.Cfg.SetItem(base.CfgLength, uint64(len(valid))*8)
		reader := base.NewBitReader(bytes.NewReader(valid[:cut]))
		require.Error(t, root.ParseSubNode(reader, "H225RAS"))
		require.Nil(t, protocolCorpusFindNode(base.GetNodeByPath(root, "@H225RAS"), "RasMessage Choice"))
		require.ErrorContains(t, reader.Recovery(), "no backup")
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprint("isolation-", worker), func(t *testing.T) {
			t.Parallel()
			wire := bytes.Clone(valid)
			wire[3] = byte(worker)
			cfg := map[string]any{"caller-marker": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, "application-layer.h225", "H225RAS", cfg)
			protocolCorpusRequireValue(t, n, "requestSeqNum", uint64(worker+1))
			require.Equal(t, map[string]any{"caller-marker": worker}, cfg)
		})
	}
}
