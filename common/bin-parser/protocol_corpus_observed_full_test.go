package bin_parser

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestProtocolCorpusEtherCATEveryDatagram(t *testing.T) {
	packets := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/mrhenrike-pcap/mrhenrike-ethercat.pcap")
	require.Len(t, packets, 986)
	datagrams := 0
	commands := make(map[byte]int)
	for frameNumber, frame := range packets {
		t.Run(fmt.Sprintf("frame-%d", frameNumber+1), func(t *testing.T) {
			require.GreaterOrEqual(t, len(frame), 16)
			require.Equal(t, uint16(0x88a4), binary.BigEndian.Uint16(frame[12:14]))
			input := frame[14:]
			node := protocolCorpusRequireBoundedRuleParse(t, input, "ethercat", "EtherCAT")
			header := binary.LittleEndian.Uint16(input[:2])
			protocolCorpusRequireValue(t, node, "Frame Header", uint64(header))
			end := 2 + int(header&0x7ff)
			require.LessOrEqual(t, end, len(input))
			entries := protocolCorpusNodesNamed(node, "Datagram")
			position, index := 2, 0
			for position < end {
				require.Less(t, index, len(entries))
				require.GreaterOrEqual(t, end-position, 12)
				entry := entries[index]
				data := input[position:end]
				command := data[0]
				commands[command]++
				protocolCorpusRequireValue(t, entry, "Command", uint64(command))
				protocolCorpusRequireValue(t, entry, "Index", uint64(data[1]))
				if command >= 10 && command <= 12 {
					protocolCorpusRequireValue(t, entry, "Logical Address", uint64(binary.LittleEndian.Uint32(data[2:6])))
				} else {
					protocolCorpusRequireValue(t, entry, "Station Address", uint64(binary.LittleEndian.Uint16(data[2:4])))
					protocolCorpusRequireValue(t, entry, "Register Offset", uint64(binary.LittleEndian.Uint16(data[4:6])))
				}
				flags := binary.LittleEndian.Uint16(data[6:8])
				length := int(flags & 0x7ff)
				require.GreaterOrEqual(t, len(data), length+12)
				protocolCorpusRequireValue(t, entry, "Length Flags", uint64(flags))
				protocolCorpusRequireValue(t, entry, "Interrupt", uint64(binary.LittleEndian.Uint16(data[8:10])))
				if length > 0 {
					protocolCorpusRequireValue(t, entry, "Data", data[10:10+length])
				}
				protocolCorpusRequireValue(t, entry, "Working Counter", uint64(binary.LittleEndian.Uint16(data[10+length:12+length])))
				position += 12 + length
				require.Equal(t, position < end, flags&0x8000 != 0)
				index++
			}
			require.Equal(t, end, position)
			require.Len(t, entries, index)
			datagrams += index
			if end < len(input) {
				protocolCorpusRequireValue(t, node, "Link Padding", input[end:])
			}
		})
	}
	// All distinct command values in the pinned Wireshark dissection must be
	// represented, including physical and logical addressing variants.
	require.Equal(t, 8140, datagrams)
	require.Equal(t, map[byte]int{1: 4920, 2: 10, 4: 1110, 5: 970, 7: 988, 8: 18, 10: 62, 11: 62}, commands)
	t.Logf("validated %d complete EtherCAT frames and %d datagrams; command distribution %v", len(packets), datagrams, commands)
}

func TestProtocolCorpusDoIPAcknowledgement(t *testing.T) {
	packets := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/scapy/scapy-doip.pcap")
	require.Len(t, packets, 1)
	packet := gopacket.NewPacket(packets[0], layers.LayerTypeEthernet, gopacket.Default)
	require.Nil(t, packet.ErrorLayer())
	input := packet.Layer(layers.LayerTypeTCP).(*layers.TCP).Payload
	require.Len(t, input, 16)
	node := protocolCorpusRequireBoundedRuleParse(t, input, "application-layer.doip", "DoIP")
	for field, want := range map[string]uint64{"Version": 2, "Inverse Version": 253, "Payload Type": 0x8002, "Payload Length": 8, "Source Address": 0x4b, "Target Address": 0xe00, "Acknowledgement Code": 0} {
		protocolCorpusRequireValue(t, node, field, want)
	}
	protocolCorpusRequireValue(t, node, "Previous Message", []byte{0x22, 0xfd, 0x31})
}

// The oracle is deliberately independent of the rule's BER length operator.
// Each returned field owns its complete, bounded value; indefinite encodings
// and long tag numbers are outside these pinned IEC 61850 fixtures.
type protocolCorpusBERField struct {
	tag   byte
	value []byte
}

func protocolCorpusBERFields(t *testing.T, input []byte) []protocolCorpusBERField {
	t.Helper()
	var fields []protocolCorpusBERField
	for offset := 0; offset < len(input); {
		require.GreaterOrEqual(t, len(input)-offset, 2)
		tag, first := input[offset], input[offset+1]
		require.NotEqual(t, byte(0x1f), tag&0x1f)
		offset += 2
		length := uint64(first)
		if first&0x80 != 0 {
			width := int(first & 0x7f)
			require.GreaterOrEqual(t, width, 1)
			require.LessOrEqual(t, width, 4)
			require.GreaterOrEqual(t, len(input)-offset, width)
			length = 0
			for _, octet := range input[offset : offset+width] {
				length = length<<8 | uint64(octet)
			}
			offset += width
		}
		require.LessOrEqual(t, length, uint64(len(input)-offset))
		fields = append(fields, protocolCorpusBERField{tag, input[offset : offset+int(length)]})
		offset += int(length)
	}
	return fields
}

func protocolCorpusBERUnsigned(t *testing.T, input []byte) uint64 {
	t.Helper()
	require.GreaterOrEqual(t, len(input), 1)
	require.LessOrEqual(t, len(input), 5)
	var result uint64
	for _, octet := range input {
		result = result<<8 | uint64(octet)
	}
	return result
}

func protocolCorpusIECFrame(t *testing.T, input []byte, node *base.Node, tag byte) []protocolCorpusBERField {
	t.Helper()
	require.GreaterOrEqual(t, len(input), 10)
	length := int(binary.BigEndian.Uint16(input[2:4]))
	require.GreaterOrEqual(t, length, 10)
	require.LessOrEqual(t, length, len(input))
	for field, offset := range map[string]int{"APPID": 0, "Length": 2, "Reserved 1": 4, "Reserved 2": 6} {
		protocolCorpusRequireValue(t, node, field, uint64(binary.BigEndian.Uint16(input[offset:offset+2])))
	}
	if length < len(input) {
		protocolCorpusRequireValue(t, node, "Link Padding", input[length:])
	}
	outer := protocolCorpusBERFields(t, input[8:length])
	require.Len(t, outer, 1)
	require.Equal(t, tag, outer[0].tag)
	return protocolCorpusBERFields(t, outer[0].value)
}

func TestProtocolCorpusSampledValuesEveryASDU(t *testing.T) {
	packets := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/mgadelha-sv/mgadelha-sv.cap")
	require.Len(t, packets, 10161)
	type workerResult struct {
		asduCount   int
		sampleBytes int
	}
	workerCount := runtime.GOMAXPROCS(0)
	if workerCount > 4 {
		workerCount = 4
	}
	if workerCount > len(packets) {
		workerCount = len(packets)
	}
	results := make([]workerResult, workerCount)
	t.Run("workers", func(t *testing.T) {
		for workerIndex := 0; workerIndex < workerCount; workerIndex++ {
			workerIndex := workerIndex
			t.Run(fmt.Sprintf("worker-%d", workerIndex+1), func(t *testing.T) {
				t.Parallel()
				local := workerResult{}
				defer func() {
					results[workerIndex] = local
				}()
				for frameNumber := workerIndex; frameNumber < len(packets); frameNumber += workerCount {
					frame := packets[frameNumber]
					if !t.Run(fmt.Sprintf("frame-%d", frameNumber+1), func(t *testing.T) {
						require.GreaterOrEqual(t, len(frame), 28)
						require.Equal(t, uint16(0x8100), binary.BigEndian.Uint16(frame[12:14]))
						require.Equal(t, uint16(0x88ba), binary.BigEndian.Uint16(frame[16:18]))
						input := frame[18:]
						node := protocolCorpusRequireBoundedRuleParse(t, input, "iec61850", "SampledValues")
						fields := protocolCorpusIECFrame(t, input, node, 0x60)
						require.Len(t, fields, 2)
						require.Equal(t, byte(0x80), fields[0].tag)
						require.Equal(t, byte(0xa2), fields[1].tag)
						count := protocolCorpusBERUnsigned(t, fields[0].value)
						protocolCorpusRequireValue(t, node, "ASDU Count", count)
						asdus := protocolCorpusBERFields(t, fields[1].value)
						parsed := protocolCorpusNodesNamed(node, "ASDU")
						require.Len(t, asdus, int(count))
						require.Len(t, parsed, len(asdus))
						for index, asdu := range asdus {
							require.Equal(t, byte(0x30), asdu.tag)
							values := protocolCorpusBERFields(t, asdu.value)
							require.Len(t, values, 5)
							for j, expectedTag := range []byte{0x80, 0x82, 0x83, 0x85, 0x87} {
								require.Equal(t, expectedTag, values[j].tag)
							}
							protocolCorpusRequireValue(t, parsed[index], "SV ID", values[0].value)
							protocolCorpusRequireValue(t, parsed[index], "Sample Counter", protocolCorpusBERUnsigned(t, values[1].value))
							protocolCorpusRequireValue(t, parsed[index], "Configuration Revision", protocolCorpusBERUnsigned(t, values[2].value))
							protocolCorpusRequireValue(t, parsed[index], "Sample Synchronization", protocolCorpusBERUnsigned(t, values[3].value))
							protocolCorpusRequireValue(t, parsed[index], "Sample Data", values[4].value)
							require.Len(t, values[4].value, 64)
							local.sampleBytes += len(values[4].value)
						}
						local.asduCount += len(asdus)
					}) {
						break
					}
				}
			})
		}
	})
	asduCount, sampleBytes := 0, 0
	for _, result := range results {
		asduCount += result.asduCount
		sampleBytes += result.sampleBytes
	}
	require.Equal(t, 10161, asduCount)
	require.Equal(t, 650304, sampleBytes)
	t.Logf("validated %d frames, %d ASDUs and %d dataset bytes", len(packets), asduCount, sampleBytes)
}

func TestProtocolCorpusGOOSEEveryDatasetValue(t *testing.T) {
	packets := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/iti-ics/iti-goose.pcap")
	require.Len(t, packets, 8)
	booleans, bitStrings := 0, 0
	for frameNumber, frame := range packets {
		t.Run(fmt.Sprintf("frame-%d", frameNumber+1), func(t *testing.T) {
			require.GreaterOrEqual(t, len(frame), 24)
			require.Equal(t, uint16(0x88b8), binary.BigEndian.Uint16(frame[12:14]))
			input := frame[14:]
			node := protocolCorpusRequireBoundedRuleParse(t, input, "iec61850", "GOOSE")
			fields := protocolCorpusIECFrame(t, input, node, 0x61)
			require.Len(t, fields, 12)
			for index, name := range []string{"Control Block Reference", "Time Allowed To Live", "Dataset", "GOOSE ID", "Timestamp", "State Number", "Sequence Number", "Simulation", "Configuration Revision", "Needs Commissioning", "Dataset Entry Count"} {
				require.Equal(t, byte(0x80+index), fields[index].tag)
				switch index {
				case 0, 2, 3, 4:
					protocolCorpusRequireValue(t, node, name, fields[index].value)
				default:
					protocolCorpusRequireValue(t, node, name, protocolCorpusBERUnsigned(t, fields[index].value))
				}
			}
			require.Equal(t, byte(0xab), fields[11].tag)
			values := protocolCorpusBERFields(t, fields[11].value)
			require.Len(t, values, int(protocolCorpusBERUnsigned(t, fields[10].value)))
			require.Len(t, values, 8)
			dataset := protocolCorpusFindNode(node, "Dataset Values")
			require.NotNil(t, dataset)
			parsed := protocolCorpusNodesNamed(dataset, "Value")
			require.Len(t, parsed, len(values))
			for index, value := range values {
				protocolCorpusRequireValue(t, parsed[index], "Tag", uint64(value.tag))
				protocolCorpusRequireValue(t, parsed[index], "Length", uint64(len(value.value)))
				switch value.tag {
				case 0x83:
					require.Len(t, value.value, 1)
					protocolCorpusRequireValue(t, parsed[index], "Boolean", uint64(value.value[0]))
					booleans++
				case 0x84:
					require.Len(t, value.value, 3)
					protocolCorpusRequireValue(t, parsed[index], "Unused Bits", uint64(value.value[0]))
					protocolCorpusRequireValue(t, parsed[index], "Bits", value.value[1:])
					bitStrings++
				default:
					t.Fatalf("unvalidated GOOSE dataset tag 0x%x", value.tag)
				}
			}
		})
	}
	require.Equal(t, 32, booleans)
	require.Equal(t, 32, bitStrings)
	t.Logf("validated %d frames and %d dataset values", len(packets), booleans+bitStrings)
}

func protocolCorpusNodesNamed(node *base.Node, name string) []*base.Node {
	var result []*base.Node
	var visit func(*base.Node)
	visit = func(current *base.Node) {
		if current.Name == name && protocolCorpusNodeHasResult(current) {
			result = append(result, current)
		}
		for _, child := range current.Children {
			visit(child)
		}
	}
	visit(node)
	return result
}

func TestProtocolCorpusProfinetEveryBlock(t *testing.T) {
	packets := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/iti-ics/iti-profinet-dcp.pcap")
	require.Len(t, packets, 6)
	blocks, deviceOptions, arpFrames := 0, 0, 0
	for frameNumber, frame := range packets {
		t.Run(fmt.Sprintf("frame-%d", frameNumber+1), func(t *testing.T) {
			require.GreaterOrEqual(t, len(frame), 26)
			etherType := binary.BigEndian.Uint16(frame[12:14])
			if etherType == 0x0806 {
				packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
				require.Nil(t, packet.ErrorLayer())
				arp := packet.Layer(layers.LayerTypeARP).(*layers.ARP)
				node := protocolCorpusRequireBoundedRuleParse(t, arp.Contents, "address_resolution_protocol", "Address Resolution Protocol")
				protocolCorpusRequireValue(t, node, "Opcode", uint64(arp.Operation))
				protocolCorpusRequireValue(t, node, "Sender MAC address", arp.SourceHwAddress)
				protocolCorpusRequireValue(t, node, "Sender IP address", arp.SourceProtAddress)
				protocolCorpusRequireValue(t, node, "Target MAC address", arp.DstHwAddress)
				protocolCorpusRequireValue(t, node, "Target IP address", arp.DstProtAddress)
				require.Equal(t, make([]byte, len(arp.Payload)), arp.Payload, "ARP link padding")
				arpFrames++
				return
			}
			require.Equal(t, uint16(0x8892), etherType)
			input := frame[14:]
			node := protocolCorpusRequireBoundedRuleParse(t, input, "profinet_dcp", "ProfinetDCP")
			for field, want := range map[string]uint64{
				"Frame ID": uint64(binary.BigEndian.Uint16(input[:2])), "Service ID": uint64(input[2]), "Service Type": uint64(input[3]),
				"Transaction ID": uint64(binary.BigEndian.Uint32(input[4:8])), "Data Length": uint64(binary.BigEndian.Uint16(input[10:12])),
			} {
				protocolCorpusRequireValue(t, node, field, want)
			}
			if input[2] == 5 && input[3] == 0 {
				protocolCorpusRequireValue(t, node, "Response Delay", uint64(binary.BigEndian.Uint16(input[8:10])))
			} else {
				protocolCorpusRequireValue(t, node, "Reserved", uint64(binary.BigEndian.Uint16(input[8:10])))
			}
			end := 12 + int(binary.BigEndian.Uint16(input[10:12]))
			require.LessOrEqual(t, end, len(input))
			parsed := protocolCorpusNodesNamed(node, "Block")
			index := 0
			for offset := 12; offset < end; {
				require.GreaterOrEqual(t, end-offset, 4)
				require.Less(t, index, len(parsed))
				option, suboption := input[offset], input[offset+1]
				length := int(binary.BigEndian.Uint16(input[offset+2 : offset+4]))
				protocolCorpusRequireValue(t, parsed[index], "Option", uint64(option))
				protocolCorpusRequireValue(t, parsed[index], "Suboption", uint64(suboption))
				protocolCorpusRequireValue(t, parsed[index], "Block Length", uint64(length))
				require.LessOrEqual(t, offset+4+length+(length&1), end)
				body := input[offset+4 : offset+4+length]
				if length == 0 {
					require.Equal(t, []byte{0xff, 0xff}, []byte{option, suboption})
				} else if option == 5 && suboption == 4 {
					require.Len(t, body, 3)
					protocolCorpusRequireValue(t, parsed[index], "Response Option", uint64(body[0]))
					protocolCorpusRequireValue(t, parsed[index], "Response Suboption", uint64(body[1]))
					protocolCorpusRequireValue(t, parsed[index], "Block Error", uint64(body[2]))
				} else {
					require.GreaterOrEqual(t, len(body), 2)
					infoField := "Block Info"
					if input[2] == 4 && input[3] == 0 {
						infoField = "Block Qualifier"
					}
					protocolCorpusRequireValue(t, parsed[index], infoField, uint64(binary.BigEndian.Uint16(body[:2])))
					value := body[2:]
					switch {
					case option == 1 && suboption == 2:
						require.Len(t, value, 12)
						protocolCorpusRequireValue(t, parsed[index], "IP Address", value[:4])
						protocolCorpusRequireValue(t, parsed[index], "Subnet Mask", value[4:8])
						protocolCorpusRequireValue(t, parsed[index], "Gateway", value[8:12])
					case option == 2 && suboption == 1:
						protocolCorpusRequireValue(t, parsed[index], "Device Vendor", value)
					case option == 2 && suboption == 2:
						protocolCorpusRequireValue(t, parsed[index], "Station Name", value)
					case option == 2 && suboption == 3:
						require.Len(t, value, 4)
						protocolCorpusRequireValue(t, parsed[index], "Vendor ID", uint64(binary.BigEndian.Uint16(value[:2])))
						protocolCorpusRequireValue(t, parsed[index], "Device ID", uint64(binary.BigEndian.Uint16(value[2:4])))
					case option == 2 && suboption == 4:
						require.Len(t, value, 2)
						protocolCorpusRequireValue(t, parsed[index], "Device Role", uint64(value[0]))
						protocolCorpusRequireValue(t, parsed[index], "Role Reserved", uint64(value[1]))
					case option == 2 && suboption == 5:
						require.Zero(t, len(value)%2)
						options := protocolCorpusNodesNamed(parsed[index], "Device Option")
						require.Len(t, options, len(value)/2)
						for i, entry := range options {
							protocolCorpusRequireValue(t, entry, "Option", uint64(value[2*i]))
							protocolCorpusRequireValue(t, entry, "Suboption", uint64(value[2*i+1]))
						}
						deviceOptions += len(options)
					default:
						t.Fatalf("unvalidated DCP block %d/%d", option, suboption)
					}
				}
				if length&1 != 0 {
					protocolCorpusRequireValue(t, parsed[index], "Block Padding", uint64(input[offset+4+length]))
				}
				offset += 4 + length + (length & 1)
				index++
			}
			require.Len(t, parsed, index)
			if end < len(input) {
				protocolCorpusRequireValue(t, node, "Link Padding", input[end:])
			}
			blocks += index
		})
	}
	require.Equal(t, 9, blocks)
	require.Equal(t, 13, deviceOptions)
	require.Equal(t, 2, arpFrames)
	t.Logf("validated all %d frames: %d DCP blocks, %d device options, %d ARP messages", len(packets), blocks, deviceOptions, arpFrames)
}

func TestProtocolCorpusTPKTAndCOTPEveryMessage(t *testing.T) {
	messages := 0
	for _, id := range []string{"iti-tpkt", "iti-cotp"} {
		packets := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/iti-ics/"+id+".pcapng")
		require.Len(t, packets, 2)
		for frameNumber, frame := range packets {
			t.Run(fmt.Sprintf("%s/frame-%d", id, frameNumber+1), func(t *testing.T) {
				packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
				require.Nil(t, packet.ErrorLayer())
				tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
				input := tcp.Payload
				require.GreaterOrEqual(t, len(input), 7)
				require.Equal(t, []byte{3, 0}, input[:2])
				require.Equal(t, len(input), int(binary.BigEndian.Uint16(input[2:4])))
				outer := protocolCorpusRequireBoundedRuleParse(t, input, "application-layer.isotp", "TPKT")
				protocolCorpusRequireValue(t, outer, "Version", uint64(3))
				protocolCorpusRequireValue(t, outer, "Reserved", uint64(0))
				protocolCorpusRequireValue(t, outer, "Packet Length", uint64(len(input)))
				inner := protocolCorpusRequireBoundedRuleParse(t, input[4:], "application-layer.isotp", "COTP")
				for _, node := range []*base.Node{outer, inner} {
					protocolCorpusRequireValue(t, node, "Length Indicator", uint64(input[4]))
					protocolCorpusRequireValue(t, node, "PDU Type", uint64(input[5]))
					protocolCorpusRequireValue(t, node, "End Of TSDU", uint64(input[6]>>7))
					protocolCorpusRequireValue(t, node, "TPDU Number", uint64(input[6]&0x7f))
					protocolCorpusRequireValue(t, node, "User Data", input[7:])
				}
				messages++
			})
		}
	}
	require.Equal(t, 4, messages)
}
