package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

const browserMailslotDomainHex = "110efefdc0a80164008a00c60000204548454a455046474542454f454f454a434e464145444341434143414341414100204142414346504650454e4644454346434550464846444546465046504143414200ff534d42250000000000000000000000000000000000000000000000000000001100002c000000000000000000e80300000000000000002c00560003000100010002003d005c4d41494c534c4f545c42524f575345000c00a0bb0d00574f524b47524f555000000000000000030a00100080fe07000047494f56414e4e492d504300"
const browserMailslotLocalHex = "110eff03c0a80164008a00bb0000204548454a455046474542454f454f454a434e46414544434143414341434143410020464845504643454c45484643455046464641434143414341434143414341424f00ff534d422500000000000000000000000000000000000000000000000000000011000021000000000000000000e803000000000000000021005600030001000000020032005c4d41494c534c4f545c42524f575345000f0080fc0a0047494f56414e4e492d504300000000000601031205000f0155aa00"

func browserMailslotTestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.browser_mailslot", entry)
	require.Equal(t, wire, NodeToBytes(n))
	h225TestTree(t, n, wire, 0)
	return n
}

func browserMailslotTestWhole(t *testing.T, record []byte, entry string) *base.Node {
	t.Helper()
	require.Greater(t, len(record), 42)
	source := fmt.Sprintf("endian: big\nunit: byte\nPackage:\n  Capture:\n    operator: |\n      this.ProcessSubNode(\"Envelope\")\n      this.GetSubNode(\"Datagram\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Datagram\")\n    Envelope: raw,42\n    Datagram: \"import:application-layer/browser_mailslot.yaml;node:%s\"\n", len(record)-42, entry)
	var doc yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
	root, err := base.NewNodeTree(doc)
	require.NoError(t, err)
	root.Cfg.SetItem(base.CfgLength, uint64(len(record))*8)
	reader := base.NewBitReader(bytes.NewReader(record))
	require.NoError(t, root.ParseSubNode(reader, "Capture"))
	n := base.GetNodeByPath(root, "@Capture")
	require.Equal(t, record, NodeToBytes(n))
	h225TestTree(t, n, record, 0)
	require.ErrorContains(t, reader.Recovery(), "no backup")
	return protocolCorpusFindNode(n, "Datagram")
}

func TestProtocolCorpusBrowserMailslotAllOriginalRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-wechat.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "2d82f575a8b9addfc9e66582e34911f79b5388984e54299293a28393c6d7c24e", fmt.Sprintf("%x", sha256.Sum256(data)))
	records := protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 1672)
	var found []int
	for i, record := range records {
		p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		layer := p.Layer(layers.LayerTypeUDP)
		if layer == nil {
			continue
		}
		udp := layer.(*layers.UDP)
		if udp.SrcPort != 138 && udp.DstPort != 138 {
			continue
		}
		frame := i + 1
		found = append(found, frame)
		require.Nil(t, p.ErrorLayer(), "frame %d", frame)
		ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		require.Equal(t, uint8(5), ip.IHL)
		require.Equal(t, len(record)-14, int(ip.Length))
		require.Equal(t, layers.UDPPort(138), udp.SrcPort)
		require.Equal(t, layers.UDPPort(138), udp.DstPort)
		require.Equal(t, record[42:], udp.Payload)
		require.Equal(t, len(udp.Payload)+8, int(udp.Length))
		hex := browserMailslotDomainHex
		if frame == 1416 {
			hex = browserMailslotLocalHex
		}
		want := h225TestHex(t, hex)
		if frame == 1659 {
			want[2], want[3] = 0xff, 0x28
		}
		require.Equal(t, want, udp.Payload)
		n := browserMailslotTestWhole(t, record, "BrowserMailslotDatagram")
		for name, value := range map[string]uint64{
			"Datagram Message Type": 17, "Datagram Flags": 14, "Datagram Source Port": 138, "Packet Offset": 0,
			"SMB Command": 0x25, "Word Count": 17, "Total Parameter Count": 0, "Parameter Count": 0, "Setup Count": 3,
			"Data Offset": 86, "Timeout": 1000, "Class": 2, "Mailslot Opcode": 1,
		} {
			protocolCorpusRequireValue(t, n, name, value)
		}
		protocolCorpusRequireValue(t, n, "Datagram Source IP", []byte{192, 168, 1, 100})
		protocolCorpusRequireValue(t, n, "Mailslot Name", `\MAILSLOT\BROWSE`)
		protocolCorpusRequireValue(t, n, "Mailslot Padding", []byte{})
		info := n.Cfg.GetItem("additionInfo").(map[string]any)
		require.Equal(t, false, info["Data Alignment Conformant"])
		require.Equal(t, false, info["Sender Conformance Validated"])
		require.Equal(t, false, info["Declared Names Verified"])
		require.Equal(t, true, info["Byte Count Validated"])
		require.Equal(t, frame != 1416, info["Browser Version Advisory Applicable"])
		require.Equal(t, 168, info["Browser Body Offset"])
		source := protocolCorpusFindNode(protocolCorpusFindNode(n, "Source NetBIOS Name"), "Encoded Name")
		protocolCorpusRequireValue(t, source, "Encoded Name", "GIOVANNI-PC")
		if frame == 1416 {
			for name, value := range map[string]uint64{"Datagram ID": 0xff03, "Datagram Length": 187, "Browser Opcode": 15, "Periodicity": 720000, "Server Type": 0x51203, "OS Version Major": 6, "OS Version Minor": 1, "Browser Signature": 0xaa55, "Data Count": 33, "Byte Count": 50, "Priority": 0} {
				protocolCorpusRequireValue(t, n, name, value)
			}
			protocolCorpusRequireValue(t, n, "Server Name", "GIOVANNI-PC")
			protocolCorpusRequireValue(t, n, "Comment", "")
			require.Equal(t, byte(32), source.Cfg.GetItem("additionInfo").(map[string]any)["Suffix"])
			destination := protocolCorpusFindNode(protocolCorpusFindNode(n, "Destination NetBIOS Name"), "Encoded Name")
			protocolCorpusRequireValue(t, destination, "Encoded Name", "WORKGROUP")
			require.Equal(t, byte(30), destination.Cfg.GetItem("additionInfo").(map[string]any)["Suffix"])
		} else {
			for name, value := range map[string]uint64{"Datagram Length": 198, "Browser Opcode": 12, "Periodicity": 900000, "Server Type": 0x80001000, "Browser Version Major": 254, "Browser Version Minor": 7, "Browser Signature": 0, "Data Count": 44, "Byte Count": 61, "Priority": 1} {
				protocolCorpusRequireValue(t, n, name, value)
			}
			protocolCorpusRequireValue(t, n, "Machine Group", "WORKGROUP")
			protocolCorpusRequireValue(t, n, "Local Master Browser Name", "GIOVANNI-PC")
			require.Equal(t, false, info["Browser Version Advisory Conformant"])
			require.Equal(t, byte(0), source.Cfg.GetItem("additionInfo").(map[string]any)["Suffix"])
		}
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(udp.Payload), "application-layer.browser_mailslot", "BrowserMailslotStrictDatagram")
		require.ErrorContains(t, err, "32-bit data alignment")
		fallback := browserMailslotTestWhole(t, record, "BrowserMailslotStrictCarrier")
		protocolCorpusRequireValue(t, fallback, "Unparsed Browser Mailslot Datagram", udp.Payload)
		require.Nil(t, protocolCorpusFindNode(fallback, "Browser Announcement"))
	}
	require.Equal(t, []int{1022, 1416, 1659}, found)
}

// Independent serializer from the RFC1002/MS-MAIL/MS-BRWS layouts, not a
// correction of any original record. No new capture file is manufactured.
func browserMailslotTestIndependent(domain bool) []byte {
	body := make([]byte, 32)
	body[0], body[22], body[23] = 15, 6, 1
	binary.LittleEndian.PutUint32(body[2:], 720000)
	copy(body[6:22], "NODE")
	binary.LittleEndian.PutUint32(body[24:], 0x51203)
	body[28], body[29] = 15, 1
	binary.LittleEndian.PutUint16(body[30:], 0xaa55)
	body = append(body, []byte("demo\x00")...)
	if domain {
		body[0], body[22], body[23] = 12, 3, 10
		binary.LittleEndian.PutUint32(body[2:], 900000)
		copy(body[6:22], "SAMPLE")
		binary.LittleEndian.PutUint32(body[24:], 0x80001000)
		copy(body[32:], "NODE\x00")
	}
	smb := make([]byte, 88)
	copy(smb[:5], []byte{255, 'S', 'M', 'B', 0x25})
	smb[9], smb[10] = 0x18, 4
	binary.LittleEndian.PutUint16(smb[26:], 0xfeff)
	smb[32], smb[59] = 17, 3
	for at, value := range map[int]uint16{35: uint16(len(body)), 55: uint16(len(body)), 57: 88, 61: 1, 63: 1, 65: 2, 67: uint16(19 + len(body))} {
		binary.LittleEndian.PutUint16(smb[at:], value)
	}
	copy(smb[69:], []byte("\\MAILSLOT\\BROWSE\x00"))
	smb = append(smb, body...)
	wire := make([]byte, 82)
	wire[0], wire[1], wire[2], wire[3] = 0x11, 2, 0x12, 0x34
	copy(wire[4:8], []byte{192, 0, 2, 1})
	binary.BigEndian.PutUint16(wire[8:], 138)
	binary.BigEndian.PutUint16(wire[10:], uint16(68+len(smb)))
	for i := 0; i < 2; i++ {
		name := bytes.Repeat([]byte{' '}, 16)
		copy(name, "NODE")
		name[15] = 32
		if i == 1 {
			copy(name, "SAMPLE")
			name[15] = 30
			if domain {
				copy(name, []byte("\x01\x02__MSBROWSE__\x02\x01"))
			}
		}
		at := 14 + i*34
		wire[at] = 32
		for j, b := range name {
			wire[at+1+j*2], wire[at+2+j*2] = 'A'+b>>4, 'A'+b&15
		}
	}
	return append(wire, smb...)
}

func TestProtocolCorpusBrowserMailslotIndependentAndBoundaries(t *testing.T) {
	for _, domain := range []bool{true, false} {
		wire := browserMailslotTestIndependent(domain)
		n := browserMailslotTestParse(t, wire, "BrowserMailslotStrictDatagram")
		protocolCorpusRequireValue(t, n, "Data Offset", uint64(88))
		protocolCorpusRequireValue(t, n, "Datagram ID", uint64(0x1234))
		protocolCorpusRequireValue(t, n, "Mailslot Padding", []byte{0, 0})
		require.Equal(t, true, n.Cfg.GetItem("additionInfo").(map[string]any)["Data Alignment Conformant"])
		for cut := 0; cut < len(wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.browser_mailslot", "BrowserMailslotDatagram")
			require.Error(t, err, "short prefix %d", cut)
		}
	}
	valid := browserMailslotTestIndependent(false)
	for _, edit := range []struct {
		at    int
		value byte
	}{
		{0, 16}, {1, 3}, {1, 0}, {1, 0x12}, {12, 1}, {10, 1}, {14, 31}, {15, 'Q'}, {47, 1},
		{82, 0}, {82 + 4, 0x26}, {82 + 32, 16}, {82 + 35, 0}, {82 + 55, 0}, {82 + 57, 0}, {82 + 58, 255}, {82 + 59, 2},
		{82 + 61, 2}, {82 + 63, 10}, {82 + 65, 1}, {82 + 67, 0}, {82 + 69, '/'}, {82 + 85, 'x'}, {170, 8}, {170 + 30, 0}, {len(valid) - 1, 'x'},
	} {
		wire := bytes.Clone(valid)
		wire[edit.at] = edit.value
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "application-layer.browser_mailslot", "BrowserMailslotDatagram")
		require.Error(t, err, "edit byte %d", edit.at)
		n := browserMailslotTestParse(t, wire, "BrowserMailslotCarrier")
		protocolCorpusRequireValue(t, n, "Unparsed Browser Mailslot Datagram", wire)
		require.Nil(t, protocolCorpusFindNode(n, "SMB Mailslot"))
	}
	// Ignored receiver/advisory bytes are preserved, not silently zeroed and
	// not reinterpreted as parameter arrays, extra data or source identity.
	ignored := bytes.Clone(valid)
	for _, at := range []int{82 + 5, 82 + 9, 82 + 10, 82 + 33, 82 + 37, 82 + 39, 82 + 41, 82 + 42, 82 + 49, 82 + 51, 82 + 53, 82 + 60, 82 + 86, 170 + 1, 170 + 21} {
		ignored[at] = 0x5a
	}
	browserMailslotTestParse(t, ignored, "BrowserMailslotStrictDatagram")
	for _, entry := range []string{"BrowserMailslotDatagram", "BrowserMailslotStrictDatagram", "BrowserMailslotCarrier", "BrowserMailslotStrictCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(valid), "application-layer.browser_mailslot", entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, "application-layer.browser_mailslot", entry)
		require.Error(t, err)
	}
}

func TestProtocolCorpusBrowserMailslotOffsetsRollbackAndIsolation(t *testing.T) {
	valid := browserMailslotTestIndependent(false)
	for offset := uint64(0); offset < 8; offset++ {
		for _, good := range []bool{true, false} {
			wire := bytes.Clone(valid)
			if !good {
				wire[170] = 0x08
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
			source := fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Record\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Record\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Record: \"import:application-layer/browser_mailslot.yaml;node:BrowserMailslotCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(wire), offset, offset, 8-offset)
			var doc yaml.MapSlice
			require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
			root, err := base.NewNodeTree(doc)
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
			root.Ctx.SetItem("body_length", 123)
			reader := base.NewBitReader(bytes.NewReader(packed.Bytes()))
			require.NoError(t, root.ParseSubNode(reader, "Wrapped"))
			n := base.GetNodeByPath(root, "@Wrapped")
			record := protocolCorpusFindNode(n, "Record")
			h225TestTree(t, record, wire, offset)
			if good {
				protocolCorpusRequireValue(t, record, "Datagram ID", uint64(0x1234))
				protocolCorpusRequireValue(t, record, "Server Type", uint64(0x51203))
			} else {
				protocolCorpusRequireValue(t, record, "Unparsed Browser Mailslot Datagram", wire)
				require.Nil(t, protocolCorpusFindNode(record, "Browser Announcement"))
			}
			protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
			require.Equal(t, packed.Bytes(), NodeToBytes(n))
			require.Equal(t, 123, root.Ctx.GetItem("body_length"))
			require.ErrorContains(t, reader.Recovery(), "no backup")
			_, err = reader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		}
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprint("isolated-", worker), func(t *testing.T) {
			t.Parallel()
			wire := bytes.Clone(valid)
			wire[3] = byte(worker)
			cfg := map[string]any{"marker": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, "application-layer.browser_mailslot", "BrowserMailslotDatagram", cfg)
			protocolCorpusRequireValue(t, n, "Datagram ID", uint64(0x1200+worker))
			require.Equal(t, map[string]any{"marker": worker}, cfg)
		})
	}
}
