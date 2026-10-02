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

func protocolCorpusEtherSIOFields(t *testing.T, node *base.Node, wire []byte) int {
	t.Helper()
	if nested := protocolCorpusFindNode(node, "EtherSIO"); nested != nil {
		node = nested
	}
	protocolCorpusRequireValue(t, node, "Magic", "ESIO")
	for field, offset := range map[string]int{"Telegram Type": 4, "Version": 6, "Length": 8, "Transaction ID": 10} {
		protocolCorpusRequireValue(t, node, field, uint64(binary.BigEndian.Uint16(wire[offset:])))
	}
	protocolCorpusRequireValue(t, node, "Source Station ID", uint64(binary.BigEndian.Uint32(wire[16:])))
	if binary.BigEndian.Uint16(wire[4:]) == 2 {
		protocolCorpusRequireValue(t, node, "Status Type", uint64(binary.BigEndian.Uint16(wire[12:])))
		protocolCorpusRequireValue(t, node, "Status Length", uint64(binary.BigEndian.Uint16(wire[14:])))
		for field, offset := range map[string]int{"RIO Status": 20, "Lost Telegrams": 21, "RIO Diagnostics": 22, "RIO Flags": 23} {
			protocolCorpusRequireValue(t, node, field, uint64(wire[offset]))
		}
		require.Len(t, wire, 24)
		return 0
	}
	protocolCorpusRequireValue(t, node, "Telegram ID", uint64(binary.BigEndian.Uint32(wire[12:])))
	protocolCorpusRequireValue(t, node, "Transfer Count", uint64(wire[20]))
	protocolCorpusRequireValue(t, node, "Transfer Flags", uint64(wire[21]))
	var transfers []*base.Node
	if list := protocolCorpusFindNode(node, "Transfers"); list != nil {
		transfers = list.Children
	}
	require.Len(t, transfers, int(wire[20]))
	offset := 22
	for _, transfer := range transfers {
		require.LessOrEqual(t, offset+10, len(wire))
		protocolCorpusRequireValue(t, transfer, "Transfer ID", uint64(binary.BigEndian.Uint32(wire[offset:])))
		protocolCorpusRequireValue(t, transfer, "Destination Station ID", uint64(binary.BigEndian.Uint32(wire[offset+4:])))
		length := int(binary.BigEndian.Uint16(wire[offset+8:]))
		protocolCorpusRequireValue(t, transfer, "Data Length", uint64(length))
		require.LessOrEqual(t, offset+10+length, len(wire))
		if length > 0 {
			protocolCorpusRequireValue(t, transfer, "Data", wire[offset+10:offset+10+length])
		}
		offset += 10 + length
	}
	require.Equal(t, len(wire), offset, "every declared transfer and every body byte must be consumed")
	return len(transfers)
}

func TestProtocolCorpusEtherSIOEveryRecordAndBoundary(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-ethersio.pcap")
	require.Len(t, frames, 36)
	counts, total := map[uint16]int{}, 0
	for index, frame := range frames {
		t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
			packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
			require.Nil(t, packet.ErrorLayer())
			udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
			wire := udp.Payload
			require.Equal(t, int(udp.Length)-8, len(wire))
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.ether_sio", "EtherSIO")
			total += protocolCorpusEtherSIOFields(t, node, wire)
			counts[binary.BigEndian.Uint16(wire[4:])]++
			envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			require.NotNil(t, protocolCorpusFindNode(envelope, "EtherSIO"))
			protocolCorpusEtherSIOFields(t, envelope, wire)
			for cut := 0; cut < len(wire); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.ether_sio", "EtherSIO")
				require.Error(t, err, "shorter prefix %d", cut)
			}
		})
	}
	require.Equal(t, map[uint16]int{1: 34, 2: 2}, counts)
	require.Equal(t, 34, total)
}

func etherSIOTransferFixture(values ...[]byte) []byte {
	wire := []byte{'E', 'S', 'I', 'O', 0, 1, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0, 0, 1, 0x80, 0, 0, 2, byte(len(values)), 0xa5}
	for index, value := range values {
		header := make([]byte, 10)
		binary.BigEndian.PutUint32(header, 0x80000000+uint32(index))
		binary.BigEndian.PutUint32(header[4:], uint32(index)+1)
		binary.BigEndian.PutUint16(header[8:], uint16(len(value)))
		wire = append(wire, header...)
		wire = append(wire, value...)
	}
	binary.BigEndian.PutUint16(wire[8:], uint16(len(wire)))
	return wire
}

func TestProtocolCorpusEtherSIOMultipleTransfersAndLimits(t *testing.T) {
	for _, values := range [][][]byte{nil, {{}}, {{1, 2, 3}, {}, {0xff, 0x80}}, make([][]byte, 255)} {
		wire := etherSIOTransferFixture(values...)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.ether_sio", "EtherSIO")
		require.Equal(t, len(values), protocolCorpusEtherSIOFields(t, node, wire))
	}
	valid := etherSIOTransferFixture([]byte{1, 2, 3}, []byte{0xff, 0x80})
	for cut := 0; cut < len(valid); cut++ {
		changed := append([]byte(nil), valid[:cut]...)
		if len(changed) >= 10 {
			// Even after adjusting the outer length, an incomplete list must
			// fail on its own count, header or inner data length.
			binary.BigEndian.PutUint16(changed[8:], uint16(len(changed)))
		}
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(changed), "application-layer.ether_sio", "EtherSIO")
		require.Error(t, err)
	}
	for _, mutation := range []struct {
		index int
		value byte
	}{
		{0, 'e'}, {4, 1}, {5, 0}, {5, 3}, {6, 1}, {7, 1}, {8, 0xff},
		{20, 0}, {20, 1}, {20, 3}, {30, 0xff}, {31, 2},
	} {
		changed := append([]byte(nil), valid...)
		changed[mutation.index] = mutation.value
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(changed), "application-layer.ether_sio", "EtherSIO")
		require.Error(t, err)
	}
	trailer := append(append([]byte(nil), valid...), 0)
	binary.BigEndian.PutUint16(trailer[8:], uint16(len(trailer)))
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(trailer), "application-layer.ether_sio", "EtherSIO")
	require.ErrorContains(t, err, "trailing")
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-ethersio.pcap")
	packet := gopacket.NewPacket(frames[9], layers.LayerTypeEthernet, gopacket.Default)
	status := packet.Layer(layers.LayerTypeUDP).(*layers.UDP).Payload
	for _, mutation := range []struct {
		index int
		value byte
	}{{12, 1}, {13, 0}, {13, 2}, {14, 1}, {15, 11}, {15, 13}} {
		changed := append([]byte(nil), status...)
		changed[mutation.index] = mutation.value
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(changed), "application-layer.ether_sio", "EtherSIO")
		require.Error(t, err)
	}
	for _, limit := range []int{0, -1, len(status) - 1} {
		_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(status), "application-layer.ether_sio", map[string]any{"etherSIODatagramLimit": limit}, "EtherSIO")
		require.Error(t, err)
		node := protocolCorpusRequireBoundedRuleParseWithConfig(t, frames[9], "ethernet", "Ethernet", map[string]any{"etherSIODatagramLimit": limit})
		require.Nil(t, protocolCorpusFindNode(node, "EtherSIO"))
		protocolCorpusRequireValue(t, node, "Remaining Payload", status)
	}
	_, err = parser.ParseBinary(bytes.NewReader(valid), "application-layer.ether_sio", "EtherSIO")
	require.ErrorContains(t, err, "boundary")
}
