package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestIndustrialNativeGOOSEOPCUARuleParity(t *testing.T) {
	paths, err := filepath.Glob("testdata/protocol-native/industrial/*.pcap")
	require.NoError(t, err)
	require.Len(t, paths, 10)
	var works []currentCorpusWork
	goose, opcua := 0, 0
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		reader, err := pcapgo.NewReader(bytes.NewReader(raw))
		require.NoError(t, err)
		for frame := 1; ; frame++ {
			wire, _, err := reader.ReadPacketData()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			if reader.LinkType() == layers.LinkTypeEthernet {
				at := 14
				typ := binary.BigEndian.Uint16(wire[12:14])
				for typ == 0x8100 || typ == 0x88a8 {
					typ = binary.BigEndian.Uint16(wire[at+2 : at+4])
					at += 4
				}
				require.EqualValues(t, 0x88b8, typ)
				// Check independent native GOOSE bytes directly, then the original L2 frame.
				iecOutCompare(t, wire[at:], "iec61850", "GOOSE", func(t *testing.T, node *base.Node) {
					protocolCorpusRequireValue(t, node, "APPID", uint64(1000))
					protocolCorpusRequireValue(t, node, "Dataset Entry Count", uint64(3))
					// Independent goose.c publishes this ordered dataset and its native
					// subscriber confirms {1234,true,native-libiec61850} in the manifest.
					dataset := protocolCorpusFindNode(node, "Dataset Values")
					require.NotNil(t, dataset)
					values := protocolCorpusNodesNamed(dataset, "Value")
					require.Len(t, values, 3)
					protocolCorpusRequireValue(t, values[0], "Tag", uint64(0x85))
					protocolCorpusRequireValue(t, values[0], "Length", uint64(2))
					protocolCorpusRequireValue(t, values[0], "Encoded Value", []byte{0x04, 0xd2}) // INTEGER 1234
					protocolCorpusRequireValue(t, values[1], "Tag", uint64(0x83))
					protocolCorpusRequireValue(t, values[1], "Length", uint64(1))
					protocolCorpusRequireValue(t, values[1], "Boolean", uint64(1))
					protocolCorpusRequireValue(t, values[2], "Tag", uint64(0x8a))
					protocolCorpusRequireValue(t, values[2], "Length", uint64(len("native-libiec61850")))
					protocolCorpusRequireValue(t, values[2], "Encoded Value", []byte("native-libiec61850"))
				})
				works = append(works, currentCorpusWork{id: fmt.Sprintf("%s/%d", filepath.Base(path), frame), wire: bytes.Clone(wire)})
				goose++
				continue
			}
			packet := gopacket.NewPacket(wire, reader.LinkType(), gopacket.Default)
			tcp, ok := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
			if !ok {
				continue
			}
			for payload := tcp.Payload; len(payload) >= 8; {
				size := int(binary.LittleEndian.Uint32(payload[4:8]))
				if size < 8 || size > len(payload) {
					break
				}
				message := payload[:size]
				payload = payload[size:]
				switch string(message[:3]) {
				case "HEL", "ACK", "RHE", "OPN":
				default:
					continue
				}
				t.Run(fmt.Sprintf("%s/%d/%s", filepath.Base(path), frame, message[:3]), func(t *testing.T) {
					parsed := parseOPCUATCP(t, message)
					require.Equal(t, string(message[:3]), strVal(t, parsed.Child("Message Type")))
					require.EqualValues(t, len(message), uintVal(t, parsed.Child("Message Size")))
					frame := requireOPCUAFrame(t, tcp.SrcPort, tcp.DstPort, message)
					require.Equal(t, string(message[:3]), strVal(t, frame.Child("Message Type")))
				})
				// The native capture is BSD loopback. This synthetic Ethernet carrier is
				// only a dispatch parity adapter; the OPC UA payload is copied unchanged.
				works = append(works, currentCorpusWork{id: fmt.Sprintf("%s/%d", filepath.Base(path), frame), wire: ipv4TCPFrame(t, tcp.SrcPort, tcp.DstPort, message)})
				opcua++
			}
		}
	}
	require.Equal(t, 9, goose)
	require.Equal(t, 46, opcua)
	assertDispatchEquivalence(t, works)
}
