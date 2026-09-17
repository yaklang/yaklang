package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func protocolCorpusNetBIOSFrameFields(t *testing.T, node *base.Node, wire []byte) {
	t.Helper()
	node = protocolCorpusFindNode(node, "NetBIOSFrame")
	require.NotNil(t, node)
	for field, offset := range map[string]int{"Header Length": 0, "Delimiter": 2, "Data 2": 6, "Transmit Correlator": 8, "Response Correlator": 10} {
		protocolCorpusRequireValue(t, node, field, uint64(binary.LittleEndian.Uint16(wire[offset:])))
	}
	protocolCorpusRequireValue(t, node, "Command", uint64(wire[4]))
	protocolCorpusRequireValue(t, node, "Data 1", uint64(wire[5]))
	header := int(binary.LittleEndian.Uint16(wire))
	if header == 44 {
		protocolCorpusRequireValue(t, node, "Receiver Name Bytes", wire[12:27])
		protocolCorpusRequireValue(t, node, "Receiver Name Suffix", uint64(wire[27]))
		protocolCorpusRequireValue(t, node, "Sender Name Bytes", wire[28:43])
		protocolCorpusRequireValue(t, node, "Sender Name Suffix", uint64(wire[43]))
	} else {
		require.Equal(t, 14, header)
		protocolCorpusRequireValue(t, node, "Remote Session", uint64(wire[12]))
		protocolCorpusRequireValue(t, node, "Local Session", uint64(wire[13]))
	}
	info := node.Cfg.GetItem("additionInfo").(map[string]any)
	command, flags, data := wire[4], wire[5], binary.LittleEndian.Uint16(wire[6:])
	require.Equal(t, command == 0x15 || command == 0x16, info["Session Context Required"])
	switch command {
	case 3:
		require.EqualValues(t, flags, info["Status Request"])
		require.EqualValues(t, data, info["Status Buffer Length"])
	case 0xa, 0xe:
		require.EqualValues(t, data&255, info["Name Local Session"])
		require.EqualValues(t, data>>8, info["Call Name Type"])
	case 0xd:
		require.EqualValues(t, flags, info["Name Status"])
		require.EqualValues(t, data, info["Name Type"])
	case 0xf:
		require.EqualValues(t, flags, info["Status Response"])
		require.Equal(t, data&0x8000 != 0, info["Status Exceeds Frame"])
		require.Equal(t, data&0x4000 != 0, info["Status Exceeds Buffer"])
		require.EqualValues(t, data&0x3fff, info["Status Data Length"])
		require.Equal(t, false, info["Status Data Decoded"])
	case 0x15, 0x16:
		require.Equal(t, flags&8 != 0, info["ACK Included"])
		require.Equal(t, flags&2 != 0, info["ACK Expected"])
		require.EqualValues(t, data, info["Resynchronization Indicator"])
		if command == 0x15 {
			require.Equal(t, flags&1 != 0, info["Receive Continue Requested"])
			require.Equal(t, "first-or-middle", info["Data Fragment Kind"])
		} else {
			require.Equal(t, flags&4 != 0, info["ACK With Data Allowed"])
			require.Equal(t, "only-or-last", info["Data Fragment Kind"])
		}
	case 0x17, 0x19:
		require.Equal(t, flags&0x80 != 0, info["Send Without ACK"])
		require.Equal(t, flags&1 != 0, info["Version Two Or Later"])
		require.EqualValues(t, data, info["Maximum Receive Size"])
		if command == 0x19 {
			require.EqualValues(t, flags>>1&7, info["Largest Frame Code"])
		}
	case 0x18:
		require.EqualValues(t, data, info["Termination Indicator"])
	case 0x1a, 0x1b:
		require.EqualValues(t, data, info["Data Bytes Accepted"])
		if command == 0x1a {
			require.Equal(t, flags&2 != 0, info["Received Send Without ACK"])
		}
	}
	if len(wire) > header {
		protocolCorpusRequireValue(t, node, "User Data", wire[header:])
	} else {
		require.Nil(t, protocolCorpusFindNode(node, "User Data"))
	}
}

func protocolCorpusLLCFields(t *testing.T, root *base.Node, wire []byte) int {
	t.Helper()
	node := protocolCorpusFindNode(root, "LLC")
	require.NotNil(t, node)
	for field, offset := range map[string]int{"DSAP": 0, "SSAP": 1, "Control": 2} {
		protocolCorpusRequireValue(t, node, field, uint64(wire[offset]))
	}
	info := node.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, wire[0]&1 != 0, info["Group Destination"])
	require.Equal(t, wire[1]&1 != 0, info["Response"])
	control := wire[2]
	if control&3 == 3 {
		require.Equal(t, "unnumbered", info["Control Format"])
		require.Equal(t, control&0x10 != 0, info["Poll Final"])
		require.EqualValues(t, control&0xef, info["Unnumbered Function"])
		return 3
	}
	protocolCorpusRequireValue(t, node, "Control Extended", uint64(wire[3]))
	require.Equal(t, wire[3]&1 != 0, info["Poll Final"])
	require.EqualValues(t, wire[3]>>1, info["Receive Sequence"])
	if control&1 == 0 {
		require.Equal(t, "information", info["Control Format"])
		require.EqualValues(t, control>>1, info["Send Sequence"])
	} else {
		require.Equal(t, "supervisory", info["Control Format"])
		require.EqualValues(t, control>>2&3, info["Supervisory Function"])
	}
	return 4
}

func TestProtocolCorpusNetBEUIEveryRecordAndFrameField(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-netbeui.pcap")
	require.Len(t, frames, 220)
	counts, commands := map[string]int{}, map[byte]int{}
	for index, frame := range frames {
		t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
			envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			length := int(binary.BigEndian.Uint16(frame[12:]))
			if length >= 0x600 {
				counts["IPv4"]++
				require.Equal(t, 0x800, length)
				require.Nil(t, protocolCorpusFindNode(envelope, "NetBIOSFrame"))
				return
			}
			require.LessOrEqual(t, 14+length, len(frame))
			llc := frame[14 : 14+length]
			header := protocolCorpusLLCFields(t, envelope, llc)
			wire := llc[header:]
			if llc[0] == 0xe0 {
				counts["IPX"]++
				ipx := protocolCorpusFindNode(envelope, "IPX")
				require.NotNil(t, ipx)
				for field, offset := range map[string]int{"Checksum": 0, "Length": 2, "Destination Socket": 16, "Source Socket": 28} {
					protocolCorpusRequireValue(t, ipx, field, uint64(binary.BigEndian.Uint16(wire[offset:])))
				}
				protocolCorpusRequireValue(t, ipx, "Transport Control", uint64(wire[4]))
				protocolCorpusRequireValue(t, ipx, "Packet Type", uint64(wire[5]))
				protocolCorpusRequireValue(t, ipx, "Destination Network", uint64(binary.BigEndian.Uint32(wire[6:])))
				protocolCorpusRequireValue(t, ipx, "Destination Node", wire[10:16])
				protocolCorpusRequireValue(t, ipx, "Source Network", uint64(binary.BigEndian.Uint32(wire[18:])))
				protocolCorpusRequireValue(t, ipx, "Source Node", wire[22:28])
				if len(wire) > 30 {
					protocolCorpusRequireValue(t, ipx, "Payload", wire[30:])
				}
				return
			}
			require.Equal(t, byte(0xf0), llc[0])
			if llc[2]&1 != 0 && llc[2]&0xef != 3 {
				counts["LLC control"]++
				require.Empty(t, wire, "capture control frames contain only LLC and external padding")
				require.Nil(t, protocolCorpusFindNode(envelope, "NetBIOSFrame"))
				return
			}
			counts["NetBIOS"]++
			commands[wire[4]]++
			protocolCorpusNetBIOSFrameFields(t, envelope, wire)
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "netbios_frame", "NetBIOSFrame")
			protocolCorpusNetBIOSFrameFields(t, node, wire)
			for cut := 0; cut < len(wire); cut++ {
				_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire[:cut]), "netbios_frame", map[string]any{"netBIOSFrameLength": len(wire)}, "NetBIOSFrame")
				require.Error(t, err, "shorter prefix %d", cut)
			}
			for cut := 0; cut < int(binary.LittleEndian.Uint16(wire)); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "netbios_frame", "NetBIOSFrame")
				require.Error(t, err, "incomplete mandatory header %d", cut)
			}
		})
	}
	require.Equal(t, map[string]int{"NetBIOS": 106, "LLC control": 34, "IPX": 18, "IPv4": 62}, counts)
	require.Equal(t, map[byte]int{0: 11, 1: 15, 8: 15, 10: 1, 14: 1, 20: 10, 22: 48, 23: 1, 24: 1, 25: 1, 31: 2}, commands)
}

func netBIOSFrameFixture(command, flags byte, data uint16, body ...byte) []byte {
	header := 14
	if command < 16 {
		header = 44
	}
	wire := make([]byte, header)
	for index := range wire {
		wire[index] = byte(index*17 + 128)
	}
	binary.LittleEndian.PutUint16(wire, uint16(header))
	binary.LittleEndian.PutUint16(wire[2:], 0xefff)
	wire[4], wire[5] = command, flags
	binary.LittleEndian.PutUint16(wire[6:], data)
	return append(wire, body...)
}

func TestProtocolCorpusNetBIOSFrameVariantsAndLimits(t *testing.T) {
	commands := []byte{0, 1, 2, 3, 7, 8, 9, 10, 13, 14, 15, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 31}
	for _, command := range commands {
		for _, flags := range []byte{0, 1, 2, 4, 8, 15, 0x80, 0xff} {
			wire := netBIOSFrameFixture(command, flags, 0xc000)
			if command == 8 || command == 9 || command == 21 || command == 22 || command == 15 {
				wire = append(wire, 0, 0xff, 0x80)
				if command == 15 {
					binary.LittleEndian.PutUint16(wire[6:], 0xc003)
				}
			}
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "netbios_frame", "NetBIOSFrame")
			protocolCorpusNetBIOSFrameFields(t, node, wire)
		}
	}
	valid := netBIOSFrameFixture(1, 0, 0)
	for _, mutation := range []struct{ offset, value int }{{0, 0}, {0, 14}, {0, 43}, {0, 45}, {1, 1}, {2, 0}, {3, 0}, {4, 4}, {4, 0x10}, {4, 0x1d}, {4, 0x20}, {4, 0xff}} {
		changed := append([]byte(nil), valid...)
		changed[mutation.offset] = byte(mutation.value)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(changed), "netbios_frame", "NetBIOSFrame")
		require.Error(t, err)
	}
	for _, wire := range [][]byte{
		append(append([]byte(nil), valid...), 1), append([]byte{0}, valid...),
		netBIOSFrameFixture(0xf, 0, 2, 1), netBIOSFrameFixture(0xf, 0, 1, 1, 2),
	} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "netbios_frame", "NetBIOSFrame")
		require.Error(t, err)
	}
	large := netBIOSFrameFixture(8, 0, 0, make([]byte, 65535-44)...)
	protocolCorpusRequireBoundedRuleParse(t, large, "netbios_frame", "NetBIOSFrame")
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(large, 1)), "netbios_frame", "NetBIOSFrame")
	require.ErrorContains(t, err, "size")
	for _, limit := range []int{0, -1, len(valid) - 1} {
		_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(valid), "netbios_frame", map[string]any{"netBIOSFrameLimit": limit}, "NetBIOSFrame")
		require.Error(t, err)
		llc := append([]byte{0xf0, 0xf0, 3}, valid...)
		_, err = parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(llc), "llc", map[string]any{"netBIOSFrameLimit": limit}, "LLC")
		require.Error(t, err, "import must respect the caller's limit")
	}
	_, err = parser.ParseBinary(bytes.NewReader(valid), "netbios_frame", "NetBIOSFrame")
	require.ErrorContains(t, err, "boundary")
}

func TestProtocolCorpusLLCControlFormatsAndDispatch(t *testing.T) {
	for _, control := range []byte{0, 0xfe, 1, 5, 9, 13, 3, 0x13, 0x43, 0x53, 0x63, 0x73, 0x6f, 0x7f, 0xaf, 0xbf} {
		for _, sap := range []byte{0xf0, 0xf1} {
			wire := []byte{sap, sap, control}
			if control&3 != 3 {
				wire = append(wire, 0xff)
			}
			data := control&1 == 0 || control&0xef == 3
			if data {
				wire = append(wire, netBIOSFrameFixture(1, 0, 0)...)
			} else if control&0xef == 0xaf {
				wire = append(wire, 0x81, 1, 0xfe)
			}
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "llc", "LLC")
			protocolCorpusLLCFields(t, node, wire)
			require.Equal(t, data, protocolCorpusFindNode(node, "NetBIOSFrame") != nil)
		}
	}
	// An unknown SAP or non-data control preserves bytes without interpreting
	// a coincidental EFFF marker. Link padding is outside this bounded payload.
	for _, prefix := range [][]byte{{0xee, 0xee, 3}, {0xf0, 0xee, 3}, {0xf0, 0xf1, 1, 0xff}, {0xf0, 0xf1, 0x73}} {
		body := netBIOSFrameFixture(1, 0, 0)
		wire := append(append([]byte(nil), prefix...), body...)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "llc", "LLC")
		require.Nil(t, protocolCorpusFindNode(node, "NetBIOSFrame"))
		protocolCorpusRequireValue(t, node, "LLC Payload", body)
	}
	for _, wire := range [][]byte{{}, {0xf0}, {0xf0, 0xf0}, {0xf0, 0xf0, 0}, {0xf0, 0xf1, 1}, {0xf0, 0xf0, 3}} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "llc", "LLC")
		require.Error(t, err)
	}
}
