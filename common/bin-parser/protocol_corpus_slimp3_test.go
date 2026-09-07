package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Primary wire-format oracle: the Slim Devices protocol document describes
// SlimServer 5.0.1 and SLIMP3 v2.2 firmware. In particular, every packet has
// an 18-byte header and all integers are unsigned network-order values.
// https://preview.lyrion.org/reference/slimp3-protocol/
//
// The fixture bytes are independent literals, cross-checked against the
// original SlimServer receive/pack formats and the pinned Wireshark decoder.
// SlimServer's discovery `unpack axCC...` is authoritative for the byte-2/3
// device/revision positions; Wireshark currently displays those two fields
// one byte early and accepts truncated headers, neither of which is copied.
// https://github.com/LMS-Community/slimserver/blob/5a1a86444ce511bee79f7c240ea8d72629b64fc8/Slim/Networking/UDP.pm
// https://github.com/LMS-Community/slimserver/blob/5a1a86444ce511bee79f7c240ea8d72629b64fc8/Slim/Networking/SliMP3/Protocol.pm
// https://github.com/LMS-Community/slimserver/blob/5a1a86444ce511bee79f7c240ea8d72629b64fc8/Slim/Player/SLIMP3.pm
// https://gitlab.com/wireshark/wireshark/-/blob/411f78566bed622a2ca2e6480c659486175db129/epan/dissectors/packet-slimp3.c
const slimp3CorpusRule = "application-layer.slimp3"

type slimp3TestFixture struct {
	name, literal, kind, direction string
	directionContext, opaque       bool
	wire                           []byte
}

func slimp3TestHex(t *testing.T, literal string) []byte {
	t.Helper()
	wire, err := hex.DecodeString(literal)
	require.NoError(t, err)
	return wire
}

func slimp3TestFixtures(t *testing.T) []slimp3TestFixture {
	t.Helper()
	tests := []slimp3TestFixture{
		{name: "discovery-request", literal: "640001110000000000000000001122334455", kind: "Discovery Request", direction: "Client to Server"},
		{name: "discovery-response", literal: "4400c00002090d9b00000000000000000000", kind: "Discovery Response", direction: "Server to Client"},
		{name: "client-hello", literal: "680122000000000000000000001122334455", kind: "Hello", direction: "Direction Context Required", directionContext: true},
		{name: "server-hello", literal: "680000000000000000000000000000000000", kind: "Hello", direction: "Direction Context Required", directionContext: true},
		{name: "infrared", literal: "6900000f4240ff100000f732001122334455", kind: "Infrared Code", direction: "Client to Server"},
		{name: "acknowledgement", literal: "610000000000123456789abc001122334455", kind: "MPEG Acknowledgement", direction: "Client to Server"},
		{name: "display", literal: "6c20202020202020202020202020202020200348036902010005", kind: "Display Data", direction: "Server to Client"},
		{name: "mpeg", literal: "6d0300000000123400000102000000000000fffb9064", kind: "MPEG Data", direction: "Server to Client", opaque: true},
		{name: "i2c", literal: "3220202020202020202020202020202020207377347270", kind: "I2C Data", direction: "Direction Context Required", directionContext: true, opaque: true},
		{name: "legacy-control", literal: "730400000000000000000000000000000000", kind: "Legacy Stream Control", direction: "Server to Client"},
		{name: "legacy-request", literal: "720012340000000000000000000000000000", kind: "Legacy Data Request", direction: "Client to Server"},
	}
	for i := range tests {
		tests[i].wire = slimp3TestHex(t, tests[i].literal)
		require.GreaterOrEqual(t, len(tests[i].wire), 18)
	}
	return tests
}

func slimp3TestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	reader := newProtocolCorpusBoundedReader(wire)
	node, err := parser.ParseBinary(reader, slimp3CorpusRule, entry)
	require.NoError(t, err)
	require.NotNil(t, node)
	require.Equal(t, wire, NodeToBytes(node))
	require.Zero(t, reader.Len())
	return node
}

func slimp3TestInfo(t *testing.T, node *base.Node) map[string]any {
	t.Helper()
	opcode := protocolCorpusFindNode(node, "Opcode")
	require.NotNil(t, opcode)
	message, ok := opcode.Cfg.GetItem(base.CfgParent).(*base.Node)
	require.True(t, ok)
	info, ok := message.Cfg.GetItem("additionInfo").(map[string]any)
	require.True(t, ok)
	return info
}

func slimp3RequireSpan(t *testing.T, node *base.Node, name string, offset, start, end int) {
	t.Helper()
	field := protocolCorpusFindNode(node, name)
	require.NotNil(t, field, name)
	require.Equal(t, [2]uint64{uint64(offset+start) * 8, uint64(offset+end) * 8}, stream_parser.GetNodeResultPos(field), name)
}

func slimp3RequireField(t *testing.T, node *base.Node, name string, want any, offset, start, end int) {
	t.Helper()
	protocolCorpusRequireValue(t, node, name, want)
	slimp3RequireSpan(t, node, name, offset, start, end)
}

func slimp3RequireFixtureFields(t *testing.T, node *base.Node, f slimp3TestFixture, offset int) {
	t.Helper()
	wire := f.wire
	slimp3RequireField(t, node, "Opcode", uint64(wire[0]), offset, 0, 1)
	info := slimp3TestInfo(t, node)
	require.Equal(t, f.kind, info["Message Kind"])
	require.Equal(t, f.direction, info["Direction"])
	require.Equal(t, f.directionContext, info["Direction Context Required"])
	require.Equal(t, false, info["Conversation State Validated"])
	require.Equal(t, f.opaque, info["Application Data Opaque"])

	switch f.name {
	case "discovery-request":
		slimp3RequireField(t, node, "Discovery Reserved", uint64(0), offset, 1, 2)
		// The official server uses `axCC`: byte 1 is skipped, then device and
		// firmware are read at bytes 2 and 3.
		slimp3RequireField(t, node, "Device ID", uint64(1), offset, 2, 3)
		slimp3RequireField(t, node, "Firmware Revision", uint64(0x11), offset, 3, 4)
		slimp3RequireField(t, node, "Discovery Request Reserved", wire[4:12], offset, 4, 12)
		slimp3RequireField(t, node, "Client MAC", wire[12:18], offset, 12, 18)
	case "discovery-response":
		slimp3RequireField(t, node, "Discovery Reserved", uint64(0), offset, 1, 2)
		slimp3RequireField(t, node, "Server Address", wire[2:6], offset, 2, 6)
		slimp3RequireField(t, node, "Server Port", uint64(3483), offset, 6, 8)
		slimp3RequireField(t, node, "Discovery Response Reserved", wire[8:18], offset, 8, 18)
	case "client-hello", "server-hello":
		slimp3RequireField(t, node, "Hello Byte 1", uint64(wire[1]), offset, 1, 2)
		slimp3RequireField(t, node, "Hello Byte 2", uint64(wire[2]), offset, 2, 3)
		slimp3RequireField(t, node, "Hello Reserved", wire[3:12], offset, 3, 12)
		slimp3RequireField(t, node, "Hello Endpoint Bytes", wire[12:18], offset, 12, 18)
	case "infrared":
		slimp3RequireField(t, node, "IR Reserved", uint64(0), offset, 1, 2)
		slimp3RequireField(t, node, "Uptime Ticks", uint64(1_000_000), offset, 2, 6)
		slimp3RequireField(t, node, "Code Identifier", uint64(0xff), offset, 6, 7)
		slimp3RequireField(t, node, "Code Bits", uint64(16), offset, 7, 8)
		slimp3RequireField(t, node, "Infrared Code", uint64(0x0000f732), offset, 8, 12)
		slimp3RequireField(t, node, "Client MAC", wire[12:18], offset, 12, 18)
	case "acknowledgement":
		slimp3RequireField(t, node, "Acknowledgement Reserved", wire[1:6], offset, 1, 6)
		slimp3RequireField(t, node, "Write Pointer", uint64(0x1234), offset, 6, 8)
		slimp3RequireField(t, node, "Read Pointer", uint64(0x5678), offset, 8, 10)
		slimp3RequireField(t, node, "Sequence Number", uint64(0x9abc), offset, 10, 12)
		slimp3RequireField(t, node, "Client MAC", wire[12:18], offset, 12, 18)
		require.EqualValues(t, 0x2468, info["Write Pointer Bytes"])
		require.EqualValues(t, 0xacf0, info["Read Pointer Bytes"])
	case "display":
		slimp3RequireField(t, node, "Display Reserved", wire[1:18], offset, 1, 18)
		codes := protocolCorpusNodesNamed(node, "Display Code")
		require.Len(t, codes, 4)
		require.EqualValues(t, 4, info["Display Code Count"])
		wantKinds := []string{"Character", "Character", "Command", "Delay"}
		for i, code := range codes {
			start := 18 + 2*i
			slimp3RequireField(t, code, "Display Code Type", uint64(wire[start]), offset, start, start+1)
			slimp3RequireField(t, code, "Display Code Value", uint64(wire[start+1]), offset, start+1, start+2)
			codeInfo := code.Cfg.GetItem("additionInfo").(map[string]any)
			require.Equal(t, wantKinds[i], codeInfo["Code Class"])
		}
	case "mpeg":
		slimp3RequireField(t, node, "MPEG Control", uint64(3), offset, 1, 2)
		slimp3RequireField(t, node, "MPEG Reserved 2-5", wire[2:6], offset, 2, 6)
		slimp3RequireField(t, node, "Write Pointer", uint64(0x1234), offset, 6, 8)
		slimp3RequireField(t, node, "MPEG Reserved 8-9", wire[8:10], offset, 8, 10)
		slimp3RequireField(t, node, "Sequence Number", uint64(0x0102), offset, 10, 12)
		slimp3RequireField(t, node, "MPEG Reserved 12-17", wire[12:18], offset, 12, 18)
		slimp3RequireField(t, node, "MPEG Data", wire[18:], offset, 18, len(wire))
		require.Equal(t, true, info["MPEG Control Recognized"])
		require.EqualValues(t, 0x2468, info["Write Pointer Bytes"])
	case "i2c":
		slimp3RequireField(t, node, "I2C Header Bytes", wire[1:18], offset, 1, 18)
		slimp3RequireField(t, node, "I2C Application Data", wire[18:], offset, 18, len(wire))
	case "legacy-control":
		slimp3RequireField(t, node, "Legacy Stream Control", uint64(4), offset, 1, 2)
		slimp3RequireField(t, node, "Legacy Control Reserved", wire[2:18], offset, 2, 18)
		require.Equal(t, true, info["Legacy Control Recognized"])
	case "legacy-request":
		slimp3RequireField(t, node, "Legacy Request Reserved", uint64(0), offset, 1, 2)
		slimp3RequireField(t, node, "Requested Offset", uint64(0x1234), offset, 2, 4)
		slimp3RequireField(t, node, "Legacy Request Tail", wire[4:18], offset, 4, 18)
		require.EqualValues(t, 0x2468, info["Requested Offset Bytes"])
	default:
		t.Fatalf("missing field assertions for %s", f.name)
	}
}

func TestProtocolCorpusSliMP3IndependentLiteralMessagesAndFields(t *testing.T) {
	for _, f := range slimp3TestFixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			for _, entry := range []string{"SliMP3", "SliMP3Carrier"} {
				t.Run(entry, func(t *testing.T) {
					node := slimp3TestParse(t, f.wire, entry)
					slimp3RequireFixtureFields(t, node, f, 0)
					require.Nil(t, protocolCorpusFindNode(node, "Unparsed SliMP3 Payload"))
				})
			}
		})
	}
}

func TestProtocolCorpusSliMP3OriginalCaptureIsTruncatedDiscovery(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-slimp3.pcap")
	require.Len(t, frames, 1)
	frame := frames[0]
	want := slimp3TestHex(t, "02000000000202000000000108004500002100010000401166c90a0000010a0000029ca50d9b000ddc906400000001")
	require.Equal(t, want, frame)
	require.Len(t, frame, 47)
	require.Equal(t, []byte{'d', 0, 0, 0, 1}, frame[42:])

	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(frame[42:]), slimp3CorpusRule, "SliMP3")
	require.ErrorContains(t, err, "slimp3: message is shorter than the 18-byte header")
	carrier := slimp3TestParse(t, frame[42:], "SliMP3Carrier")
	protocolCorpusRequireValue(t, carrier, "Unparsed SliMP3 Payload", frame[42:])
	slimp3RequireSpan(t, carrier, "Unparsed SliMP3 Payload", 0, 0, 5)
	require.Nil(t, protocolCorpusFindNode(carrier, "Opcode"))

	// Port dispatch must make the same all-or-nothing decision. TShark labels
	// the five bytes as `d`, but that heuristic label is not a semantic positive.
	envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
	protocolCorpusRequireValue(t, envelope, "Unparsed SliMP3 Payload", frame[42:])
	slimp3RequireSpan(t, envelope, "Unparsed SliMP3 Payload", 0, 42, 47)
	require.Nil(t, protocolCorpusFindNode(envelope, "Opcode"))
}

func TestProtocolCorpusSliMP3ValidatedCompanionEveryRecord(t *testing.T) {
	// These five payloads are literal protocol examples, not output computed by
	// the YAML rule. The checked-in generator is responsible only for wrapping
	// them in Ethernet/IPv4/UDP records.
	all := slimp3TestFixtures(t)
	wantedNames := []string{"discovery-request", "discovery-response", "infrared", "display", "mpeg"}
	byName := make(map[string]slimp3TestFixture, len(all))
	for _, f := range all {
		byName[f.name] = f
	}
	const capturePath = "testdata/protocol-corpus/captures/generated-validated/gen-slimp3-valid.pcap"
	captureData := readProtocolCorpusFile(t, ".", capturePath)
	require.Equal(t, "1089026c253cd41cd5c03b15b8d09e7e5b47496e9026327cba3863b94d444f81", fmt.Sprintf("%x", sha256.Sum256(captureData)))
	frames := protocolCorpusAuditPackets(t, capturePath)
	require.Len(t, frames, len(wantedNames))
	for i, name := range wantedNames {
		t.Run(fmt.Sprintf("frame-%d-%s", i+1, name), func(t *testing.T) {
			f := byName[name]
			packet := gopacket.NewPacket(frames[i], layers.LayerTypeEthernet, gopacket.Default)
			require.Nil(t, packet.ErrorLayer())
			udp, ok := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
			require.True(t, ok)
			require.Equal(t, uint16(len(f.wire)+8), udp.Length)
			require.Equal(t, f.wire, udp.Payload)
			require.True(t, udp.SrcPort == 3483 || udp.DstPort == 3483 || udp.SrcPort == 1069 || udp.DstPort == 1069)

			direct := slimp3TestParse(t, udp.Payload, "SliMP3")
			slimp3RequireFixtureFields(t, direct, f, 0)
			envelope := protocolCorpusRequireBoundedRuleParse(t, frames[i], "ethernet", "Ethernet")
			offset := len(frames[i]) - len(f.wire)
			require.Equal(t, 42, offset)
			slimp3RequireFixtureFields(t, envelope, f, offset)
			require.Nil(t, protocolCorpusFindNode(envelope, "Unparsed SliMP3 Payload"))
		})
	}
}

func TestProtocolCorpusSliMP3AllShortPrefixesAndVariableBoundaries(t *testing.T) {
	fixtures := slimp3TestFixtures(t)
	for _, f := range fixtures {
		t.Run(f.name+"-header-prefixes", func(t *testing.T) {
			for end := 0; end < 18; end++ {
				prefix := f.wire[:end]
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(prefix), slimp3CorpusRule, "SliMP3")
				require.ErrorContainsf(t, err, "slimp3: message is shorter than the 18-byte header", "prefix %d", end)
				if end == 0 {
					_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(prefix), slimp3CorpusRule, "SliMP3Carrier")
					require.ErrorContains(t, err, "slimp3: empty carrier has no message")
					continue
				}
				carrier := slimp3TestParse(t, prefix, "SliMP3Carrier")
				protocolCorpusRequireValue(t, carrier, "Unparsed SliMP3 Payload", prefix)
				require.Nil(t, protocolCorpusFindNode(carrier, "Opcode"))
			}
		})
	}

	byName := make(map[string]slimp3TestFixture, len(fixtures))
	for _, f := range fixtures {
		byName[f.name] = f
	}
	for _, name := range []string{"display", "mpeg"} {
		f := byName[name]
		t.Run(name+"-aligned-tail", func(t *testing.T) {
			for end := 18; end <= len(f.wire); end++ {
				wire := f.wire[:end]
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), slimp3CorpusRule, "SliMP3")
				if (end-18)%2 == 0 {
					require.NoError(t, err, "prefix %d is a complete smaller datagram", end)
				} else {
					if name == "display" {
						require.ErrorContains(t, err, "slimp3: display data is not 16-bit aligned", "odd tail %d", end)
					} else {
						require.ErrorContains(t, err, "slimp3: MPEG data length must be even", "odd tail %d", end)
					}
					carrier := slimp3TestParse(t, wire, "SliMP3Carrier")
					protocolCorpusRequireValue(t, carrier, "Unparsed SliMP3 Payload", wire)
				}
			}
		})
	}
	i2c := byName["i2c"]
	for end := 18; end <= len(i2c.wire); end++ {
		// I2C responses are arbitrary returned bytes, so every datagram boundary
		// after the common header is structurally complete.
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(i2c.wire[:end]), slimp3CorpusRule, "SliMP3")
		require.NoError(t, err, "i2c boundary %d", end)
	}
}

func TestProtocolCorpusSliMP3FixedLengthsOpcodesAndReceiverTolerance(t *testing.T) {
	fixtures := slimp3TestFixtures(t)
	fixedLengthErrors := map[string]string{
		"discovery-request":  "slimp3: discovery request must be exactly 18 bytes",
		"discovery-response": "slimp3: discovery response must be exactly 18 bytes",
		"client-hello":       "slimp3: hello must be exactly 18 bytes",
		"server-hello":       "slimp3: hello must be exactly 18 bytes",
		"infrared":           "slimp3: infrared report must be exactly 18 bytes",
		"acknowledgement":    "slimp3: MPEG acknowledgement must be exactly 18 bytes",
		"legacy-control":     "slimp3: legacy stream control must be exactly 18 bytes",
		"legacy-request":     "slimp3: legacy data request must be exactly 18 bytes",
	}
	for _, f := range fixtures {
		if f.name == "display" || f.name == "mpeg" || f.name == "i2c" {
			continue
		}
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(bytes.Clone(f.wire), 0)), slimp3CorpusRule, "SliMP3")
		require.ErrorContains(t, err, fixedLengthErrors[f.name], f.name)
	}

	unknown := append([]byte{0xff}, make([]byte, 17)...)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(unknown), slimp3CorpusRule, "SliMP3")
	require.ErrorContains(t, err, "slimp3: unsupported opcode")
	carrier := slimp3TestParse(t, unknown, "SliMP3Carrier")
	protocolCorpusRequireValue(t, carrier, "Unparsed SliMP3 Payload", unknown)

	// Reserved/ignored bytes and unknown enumerated values are receiver-visible
	// facts, not grounds for rejection. This prevents sender construction habits
	// from becoming accidental validation requirements.
	discovery := slimp3TestHex(t, "64a50022ffffffffffffffff000000000000")
	node := slimp3TestParse(t, discovery, "SliMP3")
	protocolCorpusRequireValue(t, node, "Discovery Reserved", uint64(0xa5))
	protocolCorpusRequireValue(t, node, "Device ID", uint64(0))
	protocolCorpusRequireValue(t, node, "Firmware Revision", uint64(0x22))

	mpeg := slimp3TestHex(t, "6d7fa1a2a3a41234b1b20102c1c2c3c4c5c6ffee")
	node = slimp3TestParse(t, mpeg, "SliMP3")
	info := slimp3TestInfo(t, node)
	require.Equal(t, false, info["MPEG Control Recognized"])
	protocolCorpusRequireValue(t, node, "MPEG Reserved 2-5", mpeg[2:6])

	display := append(slimp3TestHex(t, "6c0000000000000000000000000000000000"), 0xff, 0x5a)
	node = slimp3TestParse(t, display, "SliMP3")
	codes := protocolCorpusNodesNamed(node, "Display Code")
	require.Len(t, codes, 1)
	require.Equal(t, "Unknown", codes[0].Cfg.GetItem("additionInfo").(map[string]any)["Code Class"])

	// Zero application data is structurally valid for each documented variable
	// form. MPEG explicitly uses such packets operationally; exercise all three
	// documented control values rather than only the companion's reset form.
	for _, opcode := range []byte{'l', '2'} {
		wire := append([]byte{opcode}, make([]byte, 17)...)
		slimp3TestParse(t, wire, "SliMP3")
	}
	for _, control := range []byte{0, 1, 3} {
		wire := append([]byte{'m', control}, make([]byte, 16)...)
		node = slimp3TestParse(t, wire, "SliMP3")
		protocolCorpusRequireValue(t, node, "MPEG Control", uint64(control))
		require.Equal(t, true, slimp3TestInfo(t, node)["MPEG Control Recognized"])
		require.Nil(t, protocolCorpusFindNode(node, "MPEG Data"))
	}
	for _, control := range []byte{1, 2, 4} {
		wire := append([]byte{'s', control}, make([]byte, 16)...)
		node = slimp3TestParse(t, wire, "SliMP3")
		protocolCorpusRequireValue(t, node, "Legacy Stream Control", uint64(control))
		require.Equal(t, true, slimp3TestInfo(t, node)["Legacy Control Recognized"])
	}
}

func slimp3TestUDP(payload []byte, source, destination uint16) []byte {
	wire := make([]byte, 8, 8+len(payload))
	binary.BigEndian.PutUint16(wire[0:2], source)
	binary.BigEndian.PutUint16(wire[2:4], destination)
	binary.BigEndian.PutUint16(wire[4:6], uint16(8+len(payload)))
	return append(wire, payload...)
}

func TestProtocolCorpusSliMP3UDPPortCarrierAndRollback(t *testing.T) {
	discovery := slimp3TestFixtures(t)[0]
	for _, ports := range [][2]uint16{{40000, 3483}, {3483, 40000}, {40000, 1069}, {1069, 40000}, {3483, 3483}} {
		t.Run(fmt.Sprintf("%d-%d", ports[0], ports[1]), func(t *testing.T) {
			udp := slimp3TestUDP(discovery.wire, ports[0], ports[1])
			node := protocolCorpusRequireBoundedRuleParse(t, udp, "user_datagram_protocol", "UDP")
			slimp3RequireFixtureFields(t, node, discovery, 8)
			require.Nil(t, protocolCorpusFindNode(node, "Unparsed SliMP3 Payload"))
			require.Nil(t, protocolCorpusFindNode(node, "Remaining Payload"))
		})
	}

	for _, bad := range [][]byte{
		{'d', 0, 0, 0, 1},
		append(slimp3TestHex(t, "6c0000000000000000000000000000000000"), 3),
		append([]byte{0xff}, make([]byte, 17)...),
	} {
		udp := slimp3TestUDP(bad, 40000, 3483)
		node := protocolCorpusRequireBoundedRuleParse(t, udp, "user_datagram_protocol", "UDP")
		protocolCorpusRequireValue(t, node, "Unparsed SliMP3 Payload", bad)
		slimp3RequireSpan(t, node, "Unparsed SliMP3 Payload", 0, 8, len(udp))
		require.Nil(t, protocolCorpusFindNode(node, "Opcode"), "failed candidate fields must be rolled back")
		require.Nil(t, protocolCorpusFindNode(node, "Remaining Payload"), "port-specific carrier owns its exact fallback")
	}

	// A valid byte sequence on an unrelated port must not be identified as
	// SliMP3. The generic UDP classifier may independently recognize another
	// format, so only the protocol-specific isolation is asserted here.
	udp := slimp3TestUDP(discovery.wire, 40000, 40001)
	node := protocolCorpusRequireBoundedRuleParse(t, udp, "user_datagram_protocol", "UDP")
	require.Nil(t, protocolCorpusFindNode(node, "SliMP3"))
}

func TestProtocolCorpusSliMP3CarrierTransactionsAndHeldReader(t *testing.T) {
	valid := slimp3TestFixtures(t)[7].wire
	lateInvalid := append(bytes.Clone(valid), 0xff)
	unknown := append([]byte{0xff}, make([]byte, 17)...)
	for _, sample := range []struct {
		name    string
		wire    []byte
		decoded bool
	}{
		{name: "valid", wire: valid, decoded: true},
		{name: "late-invalid", wire: lateInvalid},
		{name: "unknown-opcode", wire: unknown},
		{name: "short", wire: []byte{'d', 0, 1}},
	} {
		t.Run(sample.name, func(t *testing.T) {
			root, err := base.ParseRule("application-layer/slimp3.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(sample.wire))*8)
			held := []byte{0xde, 0xad, 0xbe, 0xef}
			reader := bytes.NewReader(append(append([]byte(nil), sample.wire...), held...))
			bitReader := base.NewBitReader(reader)
			require.NoError(t, root.ParseSubNode(bitReader, "SliMP3Carrier"))
			parsed := base.GetNodeByPath(root, "@SliMP3Carrier")
			require.NotNil(t, parsed)
			require.Equal(t, sample.wire, NodeToBytes(parsed))
			if sample.decoded {
				protocolCorpusRequireValue(t, parsed, "Opcode", uint64('m'))
				require.Nil(t, protocolCorpusFindNode(parsed, "Unparsed SliMP3 Payload"))
			} else {
				protocolCorpusRequireValue(t, parsed, "Unparsed SliMP3 Payload", sample.wire)
				require.Nil(t, protocolCorpusFindNode(parsed, "Opcode"))
			}
			require.Equal(t, len(held), reader.Len(), "the next bounded message must remain held")
			require.ErrorContains(t, bitReader.Recovery(), "no backup")
			require.ErrorContains(t, bitReader.PopBackup(), "no backup")
			value, err := bitReader.ReadBits(32)
			require.NoError(t, err)
			require.Equal(t, held, value)
			_, err = bitReader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		})
	}

	for _, entry := range []string{"SliMP3", "SliMP3Carrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(valid), slimp3CorpusRule, entry)
		if entry == "SliMP3" {
			require.ErrorContains(t, err, "slimp3: explicit datagram boundary required")
		} else {
			require.ErrorContains(t, err, "slimp3: explicit carrier boundary required")
		}
	}
}

func TestProtocolCorpusSliMP3ResourceBounds(t *testing.T) {
	// SlimServer reads 1500 bytes per datagram. I2C is deliberately used for
	// the exact limit because response bytes have no smaller protocol grammar.
	header := append([]byte{'2'}, make([]byte, 17)...)
	maximum := append(header, bytes.Repeat([]byte{0xa5}, 1500-len(header))...)
	require.Len(t, maximum, 1500)
	node := slimp3TestParse(t, maximum, "SliMP3")
	protocolCorpusRequireValue(t, node, "I2C Application Data", maximum[18:])

	over := append(bytes.Clone(maximum), 0)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(over), slimp3CorpusRule, "SliMP3")
	require.ErrorContains(t, err, "slimp3: message exceeds the 1500-byte receiver limit")
	carrier := slimp3TestParse(t, over, "SliMP3Carrier")
	protocolCorpusRequireValue(t, carrier, "Unparsed SliMP3 Payload", over)
	require.Nil(t, protocolCorpusFindNode(carrier, "Opcode"))

	// At the same byte limit the most node-dense supported form is finite.
	display := append(append([]byte{'l'}, make([]byte, 17)...), bytes.Repeat([]byte{3, 'A'}, 741)...)
	require.Len(t, display, 1500)
	node = slimp3TestParse(t, display, "SliMP3")
	require.Len(t, protocolCorpusNodesNamed(node, "Display Code"), 741)
	require.EqualValues(t, 741, slimp3TestInfo(t, node)["Display Code Count"])
}

func TestProtocolCorpusSliMP3ConcurrentIsolation(t *testing.T) {
	baseDiscovery := slimp3TestFixtures(t)[0]
	for worker := 0; worker < 16; worker++ {
		worker := worker
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			wire := bytes.Clone(baseDiscovery.wire)
			wire[3] = byte(worker)
			wire[17] = byte(0x80 + worker)
			node := slimp3TestParse(t, wire, "SliMP3")
			protocolCorpusRequireValue(t, node, "Firmware Revision", uint64(worker))
			protocolCorpusRequireValue(t, node, "Client MAC", wire[12:18])

			invalid := append(slimp3TestHex(t, "6c0000000000000000000000000000000000"), byte(worker))
			carrier := slimp3TestParse(t, invalid, "SliMP3Carrier")
			protocolCorpusRequireValue(t, carrier, "Unparsed SliMP3 Payload", invalid)
			require.Nil(t, protocolCorpusFindNode(carrier, "Opcode"))
		})
	}
}
