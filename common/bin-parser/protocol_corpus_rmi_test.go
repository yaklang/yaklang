package bin_parser

import (
	"bytes"
	"crypto/sha256"
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

const rmiSingleCallLiteral = "4a524d4900024c50aced0005772200000000000000000000000000000000000000000000ffffffff010203040506070874000673616d706c65"

func rmiTestOperation(t *testing.T, node *base.Node, expected int32) {
	t.Helper()
	field := protocolCorpusFindNode(node, "Operation")
	require.NotNil(t, field)
	value, err := field.Result()
	require.NoError(t, err)
	require.Equal(t, expected, value.Value, "Operation is the signed JRMP int, not an unsigned bit pattern")
}

func rmiTestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	n := protocolCorpusRequireBoundedRuleParse(t, wire, "rmi", entry)
	require.Equal(t, wire, NodeToBytes(n))
	h225TestTree(t, n, wire, 0)
	return n
}
func rmiTestWhole(t *testing.T, record []byte, start, size int, entry string) *base.Node {
	t.Helper()
	tail := len(record) - start - size
	require.GreaterOrEqual(t, tail, 0)
	source := fmt.Sprintf("endian: big\nunit: byte\nPackage:\n  Capture:\n    operator: |\n      this.ProcessSubNode(\"Envelope\")\n      this.GetSubNode(\"Record\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Record\")\n      if %d > 0 { this.ProcessSubNode(\"Capture Tail\") }\n    Envelope: raw,%d\n    Record: \"import:rmi.yaml;node:%s\"\n    Capture Tail: raw,%d\n", size, tail, start, entry, tail)
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
	return protocolCorpusFindNode(n, "Record")
}

func TestProtocolCorpusRMIEveryOriginalAndPhase(t *testing.T) {
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-rmi.pcap"
	data, e := os.ReadFile(path)
	require.NoError(t, e)
	require.Equal(t, "3defef85cdbb4208d21c740f26a2d6ee03c9e94c053aa11f25b0735b84929f4e", fmt.Sprintf("%x", sha256.Sum256(data)))
	records := protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 19)
	counts := map[string]int{}
	nextSeq := map[string]uint32{}
	for index, record := range records {
		frame := index + 1
		t.Run(fmt.Sprintf("frame-%d", frame), func(t *testing.T) {
			p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
			ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
			tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
			require.Zero(t, ip.FragOffset)
			start := 14 + int(ip.IHL)*4 + int(tcp.DataOffset)*4
			if len(tcp.Payload) == 0 {
				counts["empty"]++
				require.Equal(t, 0, int(ip.Length)-int(ip.IHL)*4-int(tcp.DataOffset)*4)
				return
			}
			require.Equal(t, tcp.Payload, record[start:start+len(tcp.Payload)])
			key := fmt.Sprintf("%s:%d/%s:%d", ip.SrcIP, tcp.SrcPort, ip.DstIP, tcp.DstPort)
			if expected, exists := nextSeq[key]; exists {
				require.Equal(t, expected, tcp.Seq, "no gap/retransmission in original directional application bytes")
			}
			nextSeq[key] = tcp.Seq + uint32(len(tcp.Payload))
			entry := "RMIMessage"
			switch frame {
			case 4:
				entry = "RMIHeader"
			case 6:
				entry = "RMIServerHandshake"
			case 8:
				entry = "RMIClientEndpoint"
			}
			counts[entry]++
			n := rmiTestWhole(t, record, start, len(tcp.Payload), entry)
			switch frame {
			case 4:
				protocolCorpusRequireValue(t, n, "Magic", []byte("JRMI"))
				protocolCorpusRequireValue(t, n, "Version", uint64(2))
				protocolCorpusRequireValue(t, n, "Protocol", uint64(0x4b))
			case 6:
				protocolCorpusRequireValue(t, n, "Protocol Ack", uint64(0x4e))
				protocolCorpusRequireValue(t, n, "Host", "127.0.0.1")
				protocolCorpusRequireValue(t, n, "Port", uint64(34450))
			case 8:
				protocolCorpusRequireValue(t, n, "Host", "127.0.1.1")
				protocolCorpusRequireValue(t, n, "Port", uint64(0))
			case 9:
				protocolCorpusRequireValue(t, n, "Type", uint64(0x50))
				protocolCorpusRequireValue(t, n, "Block Length", uint64(34))
				protocolCorpusRequireValue(t, n, "Object Number", uint64(0))
				rmiTestOperation(t, n, 2)
				protocolCorpusRequireValue(t, n, "Method Hash", uint64(0x44154dc9d4e63bdf))
				protocolCorpusRequireValue(t, n, "String", "MessengerService")
			case 11:
				protocolCorpusRequireValue(t, n, "Type", uint64(0x51))
				protocolCorpusRequireValue(t, n, "Return Code", uint64(1))
				protocolCorpusRequireValue(t, n, "UID Unique", uint64(0x65eefacc))
				protocolCorpusRequireValue(t, n, "Interface Count", uint64(2))
				protocolCorpusRequireValue(t, n, "Interface Name", "java.rmi.Remote")
				protocolCorpusRequireValue(t, n, "Class Name", "java.lang.reflect.Proxy")
				require.NotNil(t, protocolCorpusFindNode(n, "Class Data java.rmi.server.RemoteObject"))
				// The invocation-handler class declares no fields, so its empty
				// class-data group has no bytes; its descriptor is still retained.
				var classNames []string
				var classes func(*base.Node)
				classes = func(v *base.Node) {
					if v.Name == "Class Name" {
						value, err := v.Result()
						require.NoError(t, err)
						classNames = append(classNames, strVal(t, value))
					}
					for _, child := range v.Children {
						classes(child)
					}
				}
				classes(n)
				require.Equal(t, []string{"java.lang.reflect.Proxy", "java.rmi.server.RemoteObjectInvocationHandler", "java.rmi.server.RemoteObject"}, classNames)
				require.Nil(t, protocolCorpusFindNode(n, "Unparsed RMI Record"))
			case 13:
				protocolCorpusRequireValue(t, n, "Type", uint64(0x52))
			case 14:
				protocolCorpusRequireValue(t, n, "Type", uint64(0x53))
			case 16:
				protocolCorpusRequireValue(t, n, "Type", uint64(0x54))
				protocolCorpusRequireValue(t, n, "UID Count", uint64(0x8002))
			default:
				t.Fatal("unaccounted original application record")
			}
		})
	}
	require.Equal(t, map[string]int{"empty": 11, "RMIHeader": 1, "RMIServerHandshake": 1, "RMIClientEndpoint": 1, "RMIMessage": 5}, counts)
	negative := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-rmi.pcap")
	require.Len(t, negative, 4)
	for i, record := range negative {
		p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
		if i < 3 {
			require.Empty(t, tcp.Payload)
			continue
		}
		require.Equal(t, h225TestHex(t, "4a524d4900024b0000"), tcp.Payload)
		start := 14 + int(ip.IHL)*4 + int(tcp.DataOffset)*4
		n := rmiTestWhole(t, record, start, len(tcp.Payload), "RMICarrier")
		protocolCorpusRequireValue(t, n, "Unparsed RMI Record", tcp.Payload)
		require.Nil(t, protocolCorpusFindNode(n, "Magic"))
		_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(tcp.Payload), "rmi", "RMIHeader")
		require.ErrorContains(t, e, "transport header has trailing bytes")
	}
	companionPath := "testdata/protocol-corpus/captures/generated-validated/gen-rmi-valid.pcap"
	companionBytes, e := os.ReadFile(companionPath)
	require.NoError(t, e)
	require.Equal(t, "3b7f7d6d71732f85362bbcd40c73475dc8046bd107b2b95cf8dfc9d285632654", fmt.Sprintf("%x", sha256.Sum256(companionBytes)))
	companion := protocolCorpusAuditPackets(t, companionPath)
	require.Len(t, companion, 4)
	for i, record := range companion {
		packet := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		ip := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		if i < 3 {
			require.Empty(t, tcp.Payload)
			continue
		}
		require.Len(t, tcp.Payload, 57)
		require.Equal(t, h225TestHex(t, rmiSingleCallLiteral), tcp.Payload)
		n := rmiTestWhole(t, record, 14+int(ip.IHL)*4+int(tcp.DataOffset)*4, len(tcp.Payload), "RMIHeader")
		protocolCorpusRequireValue(t, n, "Magic", []byte("JRMI"))
		protocolCorpusRequireValue(t, n, "Version", uint64(2))
		protocolCorpusRequireValue(t, n, "Protocol", uint64(0x4c))
		protocolCorpusRequireValue(t, n, "Type", uint64(0x50))
		rmiTestOperation(t, n, -1)
		protocolCorpusRequireValue(t, n, "Method Hash", uint64(0x0102030405060708))
		protocolCorpusRequireValue(t, n, "String", "sample")
	}
}

func TestProtocolCorpusRMIIndependentSerializationAndControls(t *testing.T) {
	// Independently serialized with Oracle RMI 10.2/10.3 and serialization 6.4,
	// not emitted by the production parser or used to modify original captures.
	for _, fixture := range []struct{ name, entry, hex string }{
		{"single-call", "RMIHeader", rmiSingleCallLiteral},
		{"single-ping", "RMIHeader", "4a524d4900024c52"},
		{"multiplex-header", "RMIHeader", "4a524d4900024d"},
		{"nack", "RMIServerHandshake", "4f"},
		{"empty-host", "RMIClientEndpoint", "000000000000"},
		{"void-return", "RMIMessage", "51aced0005770f010000000000000000000000000000"},
		{"null-return", "RMIMessage", "51aced0005770f01000000000000000000000000000070"},
		{"ping-ack", "RMIMessage", "53"},
		{"mux-open", "RMIMultiplex", "e11234"},
		{"mux-close", "RMIMultiplex", "e21234"},
		{"mux-close-ack", "RMIMultiplex", "e31234"},
		{"mux-request", "RMIMultiplex", "e4123400000003"},
		{"mux-transmit", "RMIMultiplex", "e5123400000003aabbcc"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			wire := h225TestHex(t, fixture.hex)
			n := rmiTestParse(t, wire, fixture.entry)
			if fixture.name == "single-call" {
				rmiTestOperation(t, n, -1)
				protocolCorpusRequireValue(t, n, "Method Hash", uint64(0x0102030405060708))
				protocolCorpusRequireValue(t, n, "String", "sample")
			}
		})
	}
	baseCall := append(h225TestHex(t, "50aced00057722"), make([]byte, 34)...)
	mutf := append(bytes.Clone(baseCall), h225TestHex(t, "74000461c08062")...)
	n := rmiTestParse(t, mutf, "RMIMessage")
	protocolCorpusRequireValue(t, n, "String", "a\x00b")
	// ObjID is split between two legal block-data records, not a fake span.
	split := append(h225TestHex(t, "50aced00057710"), make([]byte, 16)...)
	split = append(split, 0x77, 18)
	split = append(split, make([]byte, 18)...)
	n = rmiTestParse(t, split, "RMIMessage")
	require.NotNil(t, protocolCorpusFindNode(n, "UID Time Part"))
	require.Nil(t, protocolCorpusFindNode(n, "UID Time"))
	rmiTestOperation(t, n, 0)
	for _, tail := range []string{"71007e0000", "7d7fffffff", "7a7fffffff", "740002c180", "740004f0908080", "72ffff", "78", "790071007e0000"} {
		wire := append(bytes.Clone(baseCall), h225TestHex(t, tail)...)
		_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "rmi", "RMIMessage")
		require.Error(t, e, tail)
		n := rmiTestParse(t, wire, "RMIMessageCarrier")
		protocolCorpusRequireValue(t, n, "Unparsed RMI Record", wire)
		require.Nil(t, protocolCorpusFindNode(n, "Message"))
	}
	for _, fixture := range []struct{ entry, hex string }{{"RMIHeader", "4a524d4900014b"}, {"RMIHeader", "4a524d4900024c"}, {"RMIHeader", "4a524d4900024b52"}, {"RMIMessage", "50aced0005"}, {"RMIMessage", "51aced0005770f030000000000000000000000000000"}, {"RMIMessage", "51aced0005770f020000000000000000000000000000"}, {"RMIMultiplex", "e4123400000000"}, {"RMIMultiplex", "e512347fffffff"}, {"RMIClientEndpoint", "000000010000"}, {"RMIServerHandshake", "4f00"}} {
		_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(h225TestHex(t, fixture.hex)), "rmi", fixture.entry)
		require.Error(t, e, fixture)
	}
}

func TestProtocolCorpusRMIBoundsOffsetsAndRecovery(t *testing.T) {
	valid := h225TestHex(t, rmiSingleCallLiteral)
	for offset := uint64(0); offset < 8; offset++ {
		for _, good := range []bool{true, false} {
			t.Run(fmt.Sprintf("%d/%t", offset, good), func(t *testing.T) {
				wire := bytes.Clone(valid)
				if !good {
					wire[len(wire)-1] = 0
				}
				var packed bytes.Buffer
				writer := base.NewBitWriter(&packed)
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0x55}, offset))
				}
				require.NoError(t, writer.WriteBits(wire, uint64(len(wire))*8))
				require.NoError(t, writer.WriteBits([]byte{0xd3}, 8))
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0}, 8-offset))
				}
				source := fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Record\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Record\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Record: \"import:rmi.yaml;node:RMICarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(wire), offset, offset, 8-offset)
				var doc yaml.MapSlice
				require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
				root, e := base.NewNodeTree(doc)
				require.NoError(t, e)
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("caller-marker", "held")
				reader := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, root.ParseSubNode(reader, "Wrapped"))
				n := base.GetNodeByPath(root, "@Wrapped")
				record := protocolCorpusFindNode(n, "Record")
				h225TestTree(t, record, wire, offset)
				if good {
					rmiTestOperation(t, record, -1)
					protocolCorpusRequireValue(t, record, "String", "sample")
				} else {
					protocolCorpusRequireValue(t, record, "Unparsed RMI Record", wire)
					require.Nil(t, protocolCorpusFindNode(record, "Magic"))
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
	for _, entry := range []string{"RMIHeader", "RMIServerHandshake", "RMIClientEndpoint", "RMIMessage", "RMIMultiplex", "RMICarrier", "RMIMessageCarrier"} {
		_, e := parser.ParseBinary(bytes.NewReader(valid), "rmi", entry)
		require.ErrorContains(t, e, "explicit")
		_, e = parser.GenerateBinary(map[string]any{}, "rmi", entry)
		require.Error(t, e)
	}
	for cut := 0; cut < len(valid); cut++ {
		root, e := base.ParseRule("rmi.yaml")
		require.NoError(t, e)
		root.Cfg.SetItem(base.CfgLength, uint64(len(valid))*8)
		reader := base.NewBitReader(bytes.NewReader(valid[:cut]))
		require.Error(t, root.ParseSubNode(reader, "RMIHeader"))
		require.Nil(t, protocolCorpusFindNode(base.GetNodeByPath(root, "@RMIHeader"), "Magic"))
		require.ErrorContains(t, reader.Recovery(), "no backup")
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprint("isolated-", worker), func(t *testing.T) {
			t.Parallel()
			wire := bytes.Clone(valid)
			wire[len(wire)-1] = '0' + byte(worker)
			cfg := map[string]any{"caller-marker": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, "rmi", "RMIHeader", cfg)
			protocolCorpusRequireValue(t, n, "String", fmt.Sprint("sampl", worker))
			require.Equal(t, map[string]any{"caller-marker": worker}, cfg)
		})
	}
	// The legacy unbounded header entry still consumes exactly its self-delimited
	// header, while the strict corpus entry rejects any bounded trailing bytes.
	reader := bytes.NewReader(append(h225TestHex(t, "4a524d4900024b"), 0xa5))
	n, e := parser.ParseBinary(reader, "rmi", "RMI")
	require.NoError(t, e)
	protocolCorpusRequireValue(t, n, "Version", uint64(2))
	require.Equal(t, 1, reader.Len())
	require.Equal(t, uint64(7*8), stream_parser.CalcNodeConsumedLength(n))
}
