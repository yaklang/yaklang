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

func protocolCorpusNVMeMessages(t *testing.T) [][]byte {
	t.Helper()
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/wireshark-tests/ws-nvme-tcp.pcapng")
	require.Len(t, frames, 3)
	var messages [][]byte
	for _, frame := range frames {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		require.True(t, tcp.SrcPort == 4420 || tcp.DstPort == 4420)
		messages = append(messages, bytes.Clone(tcp.Payload))
	}
	return messages
}

func protocolCorpusNVMeParse(t *testing.T, input []byte, config map[string]any, entry string) (*base.Node, error) {
	t.Helper()
	reader := newProtocolCorpusBoundedReader(input)
	node, err := parser.ParseBinaryWithConfig(reader, "application-layer.nvme_tcp", config, entry)
	if err == nil {
		require.Zero(t, reader.Len())
		count, coverageErr := protocolCorpusTerminalCoverage(node, input)
		require.NoError(t, coverageErr)
		require.Positive(t, count)
	}
	return node, err
}

func TestProtocolCorpusNVMeTCPEveryCapturedField(t *testing.T) {
	messages := protocolCorpusNVMeMessages(t)
	commands := map[int]any{}
	config := map[string]any{"nvmeCommands": commands, "nvmeAdminQueue": true, "nvmeAllowZeroPDO": true}
	for index, data := range messages {
		node, err := protocolCorpusNVMeParse(t, data, config, "NVMeTCP")
		require.NoError(t, err)
		for name, offset := range map[string]int{"PDU Type": 0, "Flags": 1, "Header Length": 2, "PDU Data Offset": 3} {
			protocolCorpusRequireValue(t, node, name, uint64(data[offset]))
		}
		protocolCorpusRequireValue(t, node, "PDU Length", uint64(binary.LittleEndian.Uint32(data[4:])))
		if index < 2 {
			command := protocolCorpusFindNode(node, "Command")
			for name, offset := range map[string]int{"Opcode": 8, "Command Flags": 9} {
				protocolCorpusRequireValue(t, command, name, uint64(data[offset]))
			}
			protocolCorpusRequireValue(t, command, "Command ID", uint64(binary.LittleEndian.Uint16(data[10:])))
			for name, offset := range map[string]int{"Namespace ID": 12, "CDW2": 16, "CDW3": 20, "CDW10": 48, "CDW11": 52, "CDW12": 56, "CDW13": 60, "CDW14": 64, "CDW15": 68} {
				protocolCorpusRequireValue(t, command, name, uint64(binary.LittleEndian.Uint32(data[offset:])))
			}
			protocolCorpusRequireValue(t, command, "Metadata Pointer", binary.LittleEndian.Uint64(data[24:]))
			protocolCorpusRequireValue(t, command, "Data Pointer", data[32:48])
			info := command.Cfg.GetItem("additionInfo").(map[string]any)
			if index == 0 {
				require.Equal(t, "Identify", info["Admin Command"])
				require.EqualValues(t, 1, info["Identify CNS"])
			} else {
				require.Equal(t, "Get Log Page", info["Admin Command"])
				require.EqualValues(t, 112, info["Log Page Identifier"])
				require.EqualValues(t, 3072, info["Requested Byte Length"])
			}
			continue
		}
		message := protocolCorpusFindNode(node, "NVMeTCP")
		require.Equal(t, true, message.Cfg.GetItem("additionInfo").(map[string]any)["Noncanonical Zero PDO"])
		for name, offset := range map[string]int{"Command ID": 8, "Transfer Tag": 10} {
			protocolCorpusRequireValue(t, node, name, uint64(binary.LittleEndian.Uint16(data[offset:])))
		}
		for name, offset := range map[string]int{"Data Offset": 12, "Data Length": 16} {
			protocolCorpusRequireValue(t, node, name, uint64(binary.LittleEndian.Uint32(data[offset:])))
		}
		protocolCorpusRequireValue(t, node, "Reserved", data[20:24])
		log := protocolCorpusFindNode(node, "Discovery Log")
		require.NotNil(t, log)
		protocolCorpusRequireValue(t, log, "Generation Counter", binary.LittleEndian.Uint64(data[24:]))
		protocolCorpusRequireValue(t, log, "Number of Records", binary.LittleEndian.Uint64(data[32:]))
		protocolCorpusRequireValue(t, log, "Record Format", uint64(binary.LittleEndian.Uint16(data[40:])))
		protocolCorpusRequireValue(t, log, "Reserved Header", data[42:1048])
		entries := protocolCorpusNodesNamed(log, "Discovery Entry")
		require.Len(t, entries, 1, "NUMREC, not transfer length, determines the number of entries")
		entry := entries[0]
		wire := data[1048:2072]
		for name, offset := range map[string]int{"Transport Type": 0, "Address Family": 1, "Subsystem Type": 2, "Transport Requirements": 3} {
			protocolCorpusRequireValue(t, entry, name, uint64(wire[offset]))
		}
		for name, offset := range map[string]int{"Port ID": 4, "Controller ID": 6, "Admin Queue Size": 8, "Entry Flags": 10} {
			protocolCorpusRequireValue(t, entry, name, uint64(binary.LittleEndian.Uint16(wire[offset:])))
		}
		for name, region := range map[string][2]int{"Reserved Identity": {12, 32}, "Transport Service ID": {32, 64}, "Reserved Address": {64, 256}, "Subsystem Qualified Name": {256, 512}, "Transport Address": {512, 768}, "Transport Specific Address": {768, 1024}} {
			protocolCorpusRequireValue(t, entry, name, wire[region[0]:region[1]])
		}
		protocolCorpusRequireValue(t, log, "Unused Transfer Bytes", data[2072:])
	}
	require.Len(t, commands, 2)
	_, err := protocolCorpusNVMeParse(t, bytes.Join(messages, nil), map[string]any{"nvmeAdminQueue": true, "nvmeAllowZeroPDO": true}, "NVMeTCPMessages")
	require.NoError(t, err)
}

func TestProtocolCorpusNVMeTCPBoundsAndQueueIsolation(t *testing.T) {
	messages := protocolCorpusNVMeMessages(t)
	_, err := protocolCorpusNVMeParse(t, messages[2], nil, "NVMeTCP")
	require.ErrorContains(t, err, "data present with zero PDO")
	canonicalData := bytes.Clone(messages[2])
	canonicalData[3] = 24
	for _, admin := range []bool{false, true} {
		commands := map[int]any{}
		config := map[string]any{"nvmeCommands": commands, "nvmeAdminQueue": admin}
		_, err := protocolCorpusNVMeParse(t, messages[1], config, "NVMeTCP")
		require.NoError(t, err)
		node, err := protocolCorpusNVMeParse(t, canonicalData, config, "NVMeTCP")
		require.NoError(t, err)
		if admin {
			require.NotNil(t, protocolCorpusFindNode(node, "Discovery Log"))
		} else {
			require.Nil(t, protocolCorpusFindNode(node, "Discovery Log"), "I/O opcode 2 is not admin Get Log Page")
			protocolCorpusRequireValue(t, node, "Data", canonicalData[24:])
		}
		isolated, err := protocolCorpusNVMeParse(t, canonicalData, map[string]any{"nvmeAdminQueue": admin}, "NVMeTCP")
		require.NoError(t, err)
		require.Nil(t, protocolCorpusFindNode(isolated, "Discovery Log"), "commands cannot leak between queue contexts")
	}
	config := map[string]any{"nvmeAdminQueue": true, "nvmeAllowZeroPDO": true}
	for _, data := range messages {
		for cut := 0; cut < len(data); cut++ {
			_, err := protocolCorpusNVMeParse(t, data[:cut], config, "NVMeTCP")
			require.Errorf(t, err, "PDU type %d at cut %d", data[0], cut)
		}
	}
	for name, mutate := range map[string]func([]byte){
		"digest":      func(b []byte) { b[1] = 1 },
		"header":      func(b []byte) { b[2] = 23 },
		"offset":      func(b []byte) { b[3] = 23 },
		"data-length": func(b []byte) { binary.LittleEndian.PutUint32(b[16:], 3073) },
	} {
		t.Run(name, func(t *testing.T) {
			data := bytes.Clone(canonicalData)
			mutate(data)
			_, err := protocolCorpusNVMeParse(t, data, nil, "NVMeTCP")
			require.Error(t, err)
		})
	}
	commands := map[int]any{}
	badCommand := append(bytes.Clone(messages[1]), 0x99)
	binary.LittleEndian.PutUint32(badCommand[4:], uint32(len(badCommand)))
	_, err = protocolCorpusNVMeParse(t, badCommand, map[string]any{"nvmeCommands": commands, "nvmeAdminQueue": true}, "NVMeTCP")
	require.Error(t, err)
	require.Empty(t, commands, "a failed capsule must not publish its command")
}

func TestProtocolCorpusNVMeTCPOtherPDUFieldsAndAlignment(t *testing.T) {
	for _, kind := range []byte{0, 1, 2, 3, 5, 6, 7, 9} {
		t.Run(fmt.Sprintf("type-%d", kind), func(t *testing.T) {
			hlen := 24
			if kind < 2 {
				hlen = 128
			}
			data := make([]byte, hlen)
			data[0], data[2] = kind, byte(hlen)
			for i := 8; i < hlen; i++ {
				data[i] = byte(i*7 + 3)
			}
			if kind == 6 || kind == 7 {
				binary.LittleEndian.PutUint32(data[16:], 3)
				data[3] = 28
				data = append(data, 0, 0, 0, 0, 0xab, 0xcd, 0xef)
			}
			binary.LittleEndian.PutUint32(data[4:], uint32(len(data)))
			node, err := protocolCorpusNVMeParse(t, data, nil, "NVMeTCP")
			require.NoError(t, err)
			if kind == 6 || kind == 7 {
				protocolCorpusRequireValue(t, node, "Data", []byte{0xab, 0xcd, 0xef})
				protocolCorpusRequireValue(t, node, "Alignment Padding", []byte{0, 0, 0, 0})
				bad := bytes.Clone(data)
				bad[24] = 1
				_, err := protocolCorpusNVMeParse(t, bad, nil, "NVMeTCP")
				require.ErrorContains(t, err, "alignment padding must be zero")
			}
			if kind == 5 {
				protocolCorpusRequireValue(t, node, "Result", binary.LittleEndian.Uint64(data[8:]))
				for name, offset := range map[string]int{"Submission Queue Head": 16, "Submission Queue ID": 18, "Command ID": 20, "Status": 22} {
					protocolCorpusRequireValue(t, node, name, uint64(binary.LittleEndian.Uint16(data[offset:])))
				}
			} else if kind < 2 {
				protocolCorpusRequireValue(t, node, "Protocol Format Version", uint64(binary.LittleEndian.Uint16(data[8:])))
				protocolCorpusRequireValue(t, node, "Data Alignment", uint64(data[10]))
				protocolCorpusRequireValue(t, node, "Digest Options", uint64(data[11]))
				protocolCorpusRequireValue(t, node, "Transfer Limit", uint64(binary.LittleEndian.Uint32(data[12:])))
				protocolCorpusRequireValue(t, node, "Reserved", data[16:128])
			} else if kind == 2 || kind == 3 {
				protocolCorpusRequireValue(t, node, "Fatal Status", uint64(binary.LittleEndian.Uint16(data[8:])))
				protocolCorpusRequireValue(t, node, "Fatal Information", uint64(binary.LittleEndian.Uint32(data[10:])))
				protocolCorpusRequireValue(t, node, "Reserved", data[14:24])
			} else {
				protocolCorpusRequireValue(t, node, "Command ID", uint64(binary.LittleEndian.Uint16(data[8:])))
				protocolCorpusRequireValue(t, node, "Transfer Tag", uint64(binary.LittleEndian.Uint16(data[10:])))
				protocolCorpusRequireValue(t, node, "Data Offset", uint64(binary.LittleEndian.Uint32(data[12:])))
				protocolCorpusRequireValue(t, node, "Data Length", uint64(binary.LittleEndian.Uint32(data[16:])))
				protocolCorpusRequireValue(t, node, "Reserved", data[20:24])
			}
		})
	}
}
