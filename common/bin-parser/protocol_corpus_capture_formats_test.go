package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/internal/corpusutil"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func protocolCorpusUSBPcapFields(t *testing.T, node *base.Node, wire []byte) {
	t.Helper()
	for field, offset := range map[string]int{"Header Length": 0, "Function": 14, "Bus": 17, "Device": 19} {
		protocolCorpusRequireValue(t, node, field, uint64(binary.LittleEndian.Uint16(wire[offset:])))
	}
	for field, offset := range map[string]int{"Status": 10, "Data Length": 23} {
		protocolCorpusRequireValue(t, node, field, uint64(binary.LittleEndian.Uint32(wire[offset:])))
	}
	protocolCorpusRequireValue(t, node, "IRP ID", binary.LittleEndian.Uint64(wire[2:]))
	for field, offset := range map[string]int{"Information": 16, "Endpoint": 21, "Transfer Type": 22} {
		protocolCorpusRequireValue(t, node, field, uint64(wire[offset]))
	}
}

func TestProtocolCorpusUSBPcapEveryRecordAndField(t *testing.T) {
	const path = "testdata/protocol-corpus/captures/google-samples/google-usb-engraver.pcapng"
	data := readProtocolCorpusFile(t, ".", path)
	r, err := pcapgo.NewNgReader(bytes.NewReader(data), pcapgo.NgReaderOptions{})
	require.NoError(t, err)
	require.EqualValues(t, 249, r.LinkType(), "LINKTYPE, not Wireshark's internal encapsulation ID")
	require.Equal(t, "USBPcap", corpusutil.LinkTypeName(r.LinkType()))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 860)
	counts := map[string]int{}
	for index, wire := range frames {
		t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "usb_pcap", "USBPcap")
			protocolCorpusUSBPcapFields(t, node, wire)
			header := int(binary.LittleEndian.Uint16(wire))
			payload := wire[header:]
			require.EqualValues(t, len(payload), binary.LittleEndian.Uint32(wire[23:]))
			info := node.Cfg.GetItem("additionInfo").(map[string]any)
			require.Equal(t, wire[16] == 1, info["Completion"])
			require.Equal(t, wire[16] == 1, info["Status Valid"])
			require.Equal(t, wire[21]&128 != 0, info["Transfer In"])
			require.EqualValues(t, wire[21]&15, info["Endpoint Number"])
			require.Equal(t, false, info["Device Data Decoded"])
			dataOffset := header
			switch wire[22] {
			case 1:
				counts["interrupt"]++
				require.Equal(t, 27, header)
				require.Contains(t, []int{0, 64}, len(payload))
			case 2:
				counts["control"]++
				require.Equal(t, 28, header)
				protocolCorpusRequireValue(t, node, "Control Stage", uint64(wire[27]))
				if wire[27] == 0 {
					counts["setup"]++
					require.Len(t, payload, 8)
					protocolCorpusRequireValue(t, node, "Request Type", uint64(payload[0]))
					protocolCorpusRequireValue(t, node, "Request", uint64(payload[1]))
					for field, offset := range map[string]int{"Value": 2, "Index": 4, "Requested Length": 6} {
						protocolCorpusRequireValue(t, node, field, uint64(binary.LittleEndian.Uint16(payload[offset:])))
					}
					require.Equal(t, payload[0]&128 != 0, info["Request In"])
					require.EqualValues(t, payload[0]>>5&3, info["Request Kind"])
					require.EqualValues(t, payload[0]&31, info["Recipient"])
					dataOffset += 8
				} else {
					counts["complete"]++
					require.EqualValues(t, 3, wire[27])
				}
			default:
				t.Fatalf("unreviewed transfer type %d", wire[22])
			}
			if dataOffset < len(wire) {
				counts["data"]++
				protocolCorpusRequireValue(t, node, "Data", wire[dataOffset:])
			} else {
				require.Nil(t, protocolCorpusFindNode(node, "Data"))
			}
			envelope := protocolCorpusRequireBoundedRuleParse(t, wire, "packet_envelope", "USBPcap Envelope")
			protocolCorpusUSBPcapFields(t, envelope, wire)
			require.NoError(t, protocolCorpusValidateEnvelopeShape(envelope, "USBPcap Envelope"))
			if len(payload) > 0 {
				protocolCorpusRequireValue(t, envelope, "Payload", payload)
			}
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:len(wire)-1]), "usb_pcap", "USBPcap")
			require.Error(t, err, "every captured record rejects a missing final byte")
		})
	}
	require.Equal(t, map[string]int{"interrupt": 840, "control": 20, "setup": 10, "complete": 10, "data": 425}, counts)
}

func protocolCorpusUSBPcapFixture(transfer, information, endpoint byte, extension, data []byte) []byte {
	wire := make([]byte, 27+len(extension)+len(data))
	binary.LittleEndian.PutUint16(wire, uint16(27+len(extension)))
	binary.LittleEndian.PutUint64(wire[2:], 0xfedcba9876543210)
	binary.LittleEndian.PutUint32(wire[10:], 0xc0010001)
	binary.LittleEndian.PutUint16(wire[14:], 9)
	wire[16] = information
	binary.LittleEndian.PutUint16(wire[17:], 513)
	binary.LittleEndian.PutUint16(wire[19:], 1027)
	wire[21], wire[22] = endpoint, transfer
	binary.LittleEndian.PutUint32(wire[23:], uint32(len(data)))
	copy(wire[27:], extension)
	copy(wire[27+len(extension):], data)
	return wire
}

func TestProtocolCorpusUSBPcapVariantsAndBoundaries(t *testing.T) {
	setup := []byte{0x21, 9, 0x34, 0x12, 0x78, 0x56, 3, 0, 0xaa, 0xbb, 0xcc}
	iso := make([]byte, 12+2*12)
	binary.LittleEndian.PutUint32(iso, 0x12345678)
	binary.LittleEndian.PutUint32(iso[4:], 2)
	binary.LittleEndian.PutUint32(iso[8:], 1)
	for i, value := range []uint32{0, 3, 0, 3, 2, 0xc0000001} {
		binary.LittleEndian.PutUint32(iso[12+i*4:], value)
	}
	fixtures := [][]byte{
		protocolCorpusUSBPcapFixture(1, 0, 1, nil, nil),
		protocolCorpusUSBPcapFixture(1, 1, 0x81, nil, []byte{1, 2, 3}),
		protocolCorpusUSBPcapFixture(3, 0, 2, []byte{0, 0}, []byte{0, 1, 2, 3}),
		protocolCorpusUSBPcapFixture(2, 0, 0, []byte{0}, setup),
		protocolCorpusUSBPcapFixture(2, 0, 0, []byte{0}, setup[:8]), // pre-1.5 split setup/data
		protocolCorpusUSBPcapFixture(2, 0, 0, []byte{1}, []byte{0xaa, 0xbb}),
		protocolCorpusUSBPcapFixture(2, 1, 0x80, []byte{1}, []byte{0xaa, 0xbb}),
		protocolCorpusUSBPcapFixture(2, 1, 0x80, []byte{2}, nil),
		protocolCorpusUSBPcapFixture(2, 1, 0x80, []byte{3}, []byte{0xaa, 0xbb}),
		protocolCorpusUSBPcapFixture(0, 1, 0x81, iso, []byte{1, 2, 3, 4, 5}),
		protocolCorpusUSBPcapFixture(0, 0, 1, make([]byte, 12), nil),
	}
	for i, wire := range fixtures {
		t.Run(fmt.Sprintf("valid-%d", i), func(t *testing.T) {
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "usb_pcap", "USBPcap")
			protocolCorpusUSBPcapFields(t, node, wire)
			if wire[22] == 0 && binary.LittleEndian.Uint32(wire[31:]) == 2 {
				protocolCorpusRequireValue(t, node, "Start Frame", uint64(0x12345678))
				protocolCorpusRequireValue(t, node, "Packet Count", uint64(2))
				protocolCorpusRequireValue(t, node, "Error Count", uint64(1))
				list := protocolCorpusFindNode(node, "Isochronous Packets")
				require.NotNil(t, list)
				require.Len(t, list.Children, 2)
				for j, element := range list.Children {
					for field, offset := range map[string]int{"Offset": 0, "Length": 4, "Status": 8} {
						protocolCorpusRequireValue(t, element, field, uint64(binary.LittleEndian.Uint32(iso[12+j*12+offset:])))
					}
				}
			}
			for cut := 0; cut < len(wire); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "usb_pcap", "USBPcap")
				require.Errorf(t, err, "prefix %d/%d", cut, len(wire))
			}
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(bytes.Clone(wire), 0)), "usb_pcap", "USBPcap")
			require.Error(t, err, "unaccounted trailing byte")
		})
	}
	for name, change := range map[string]func([]byte){
		"short-header":      func(b []byte) { binary.LittleEndian.PutUint16(b, 26) },
		"large-header":      func(b []byte) { binary.LittleEndian.PutUint16(b, 65535) },
		"reserved-info":     func(b []byte) { b[16] = 2 },
		"reserved-endpoint": func(b []byte) { b[21] = 0x11 },
		"unknown-transfer":  func(b []byte) { b[22] = 4 },
		"huge-data-length":  func(b []byte) { binary.LittleEndian.PutUint32(b[23:], 0xffffffff) },
		"short-data-length": func(b []byte) { binary.LittleEndian.PutUint32(b[23:], 1) },
	} {
		t.Run(name, func(t *testing.T) {
			wire := bytes.Clone(fixtures[1])
			change(wire)
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "usb_pcap", "USBPcap")
			require.Error(t, err)
		})
	}
	invalid := [][]byte{
		protocolCorpusUSBPcapFixture(2, 0, 0, nil, nil),
		protocolCorpusUSBPcapFixture(2, 0, 0, []byte{4}, nil),
		protocolCorpusUSBPcapFixture(2, 1, 0, []byte{0}, setup),
		protocolCorpusUSBPcapFixture(2, 0, 0, []byte{3}, nil),
		protocolCorpusUSBPcapFixture(2, 1, 0x80, []byte{2}, []byte{1}),
		protocolCorpusUSBPcapFixture(2, 0, 0, []byte{0}, setup[:7]),
		protocolCorpusUSBPcapFixture(0, 0, 1, nil, nil),
		protocolCorpusUSBPcapFixture(0, 1, 0x81, iso[:24], nil),
	}
	for _, wire := range invalid {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "usb_pcap", "USBPcap")
		require.Error(t, err)
	}
	for _, limit := range []int{0, -1, 26} {
		_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(fixtures[0]), "usb_pcap", map[string]any{"usbPcapRecordLimit": limit}, "USBPcap")
		require.Error(t, err)
	}
	_, err := parser.ParseBinary(bytes.NewBuffer(fixtures[0]), "usb_pcap", "USBPcap")
	require.Error(t, err, "unbounded input must not be mistaken for one capture record")
}

func TestProtocolCorpusLinuxSLL2EveryFieldAndBoundaries(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-linux-sll2.pcap")
	require.Len(t, frames, 1)
	wire := frames[0]
	node := protocolCorpusRequireBoundedRuleParse(t, wire, "linux_sll2", "LinuxSLL2")
	for field, value := range map[string]uint64{"Protocol": 0x800, "Reserved": 0, "Interface Index": 1, "ARPHRD Type": 1, "Packet Type": 0, "Address Length": 6, "Identifier": 0, "Sequence Number": 0} {
		protocolCorpusRequireValue(t, node, field, value)
	}
	protocolCorpusRequireValue(t, node, "Address", wire[12:20])
	require.NotNil(t, protocolCorpusFindNode(node, "IP"))
	require.NotNil(t, protocolCorpusFindNode(node, "ICMP"))
	envelope := protocolCorpusRequireBoundedRuleParse(t, wire, "packet_envelope", "Linux SLL2 Envelope")
	require.NoError(t, protocolCorpusValidateEnvelopeShape(envelope, "Linux SLL2 Envelope"))
	protocolCorpusRequireValue(t, envelope, "Payload", wire[20:])
	for cut := 0; cut < len(wire); cut++ {
		_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire[:cut]), "linux_sll2", map[string]any{"linuxSLL2RecordLength": len(wire)}, "LinuxSLL2")
		require.Error(t, err)
	}
	for name, change := range map[string]func([]byte){
		"reserved":         func(b []byte) { b[3] = 1 },
		"zero-index":       func(b []byte) { binary.BigEndian.PutUint32(b[4:], 0) },
		"address-too-long": func(b []byte) { b[11] = 9 },
	} {
		t.Run(name, func(t *testing.T) {
			b := bytes.Clone(wire)
			change(b)
			for _, spec := range []struct{ rule, entry string }{{"linux_sll2", "LinuxSLL2"}, {"packet_envelope", "Linux SLL2 Envelope"}} {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(b), spec.rule, spec.entry)
				require.Error(t, err)
			}
		})
	}
	for _, hardware := range []uint16{1, 772, 778, 824, 0xffff} {
		for _, addressLength := range []byte{0, 4, 6, 8} {
			b := bytes.Clone(wire)
			binary.BigEndian.PutUint16(b[8:], hardware)
			b[10], b[11] = 255, addressLength // retain unknown packet-type values
			node := protocolCorpusRequireBoundedRuleParse(t, b, "linux_sll2", "LinuxSLL2")
			info := node.Cfg.GetItem("additionInfo").(map[string]any)
			require.Equal(t, hardware == 1 && addressLength == 6, info["Address Is MAC"])
			if hardware != 1 && hardware != 772 {
				require.Nil(t, protocolCorpusFindNode(node, "IP"))
				protocolCorpusRequireValue(t, node, "Payload", b[20:])
			}
		}
	}
	unknown := bytes.Clone(wire)
	binary.BigEndian.PutUint16(unknown, 0xffff)
	node = protocolCorpusRequireBoundedRuleParse(t, unknown, "linux_sll2", "LinuxSLL2")
	protocolCorpusRequireValue(t, node, "Payload", unknown[20:])
	protocolCorpusRequireBoundedRuleParse(t, unknown[:20], "linux_sll2", "LinuxSLL2")
	for _, limit := range []int{0, -1, 19} {
		_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), "linux_sll2", map[string]any{"linuxSLL2RecordLimit": limit}, "LinuxSLL2")
		require.Error(t, err)
	}
	_, err := parser.ParseBinary(bytes.NewBuffer(wire), "linux_sll2", "LinuxSLL2")
	require.Error(t, err)
	require.Equal(t, "Linux SLL2", corpusutil.LinkTypeName(layers.LinkType(276)))
}
