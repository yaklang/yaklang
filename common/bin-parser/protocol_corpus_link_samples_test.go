package bin_parser

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// The header field layouts are independently anchored to Wireshark's
// packet-marker.c, packet-homeplug-av.c, packet-vntag.c and packet-ieee8021ah.c:
// https://github.com/wireshark/wireshark/tree/master/epan/dissectors
// Every record in each retained capture is read, then parsed both from its
// declared link boundary and through the public Ethernet dispatcher.
func TestProtocolCorpusLinkSamplesEveryRecord(t *testing.T) {
	for _, tc := range []struct {
		id, entry string
		etherType uint16
		length    int
	}{
		{"lacp-marker", "LACPMarker", 0x8809, 110},
		{"slow-protocols", "SlowProtocols", 0x8809, 110},
		{"homeplug-av", "HomePlugAV", 0x88e1, 22},
		{"vntag", "VNTag", 0x8926, 46},
		{"pbb", "ProviderBackboneBridge", 0x88e7, 46},
	} {
		t.Run(tc.id, func(t *testing.T) {
			frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-"+tc.id+".pcap")
			require.Len(t, frames, 1)
			for index, frame := range frames {
				t.Run(fmt.Sprintf("record-%d", index+1), func(t *testing.T) {
					require.Equal(t, tc.etherType, binary.BigEndian.Uint16(frame[12:14]))
					wire := frame[14:]
					require.Len(t, wire, tc.length)
					direct := protocolCorpusRequireBoundedRuleParse(t, wire, "link_samples", tc.entry)
					ethernet := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
					for _, node := range []*base.Node{direct, ethernet} {
						switch tc.id {
						case "lacp-marker":
							linkSampleMarkerFields(t, node, wire)
						case "slow-protocols":
							protocolCorpusRequireValue(t, node, "Subtype", uint64(1))
							protocolCorpusRequireValue(t, node, "Version", uint64(1))
							for _, part := range []struct {
								name   string
								offset int
							}{{"Actor", 2}, {"Partner", 22}} {
								identity := protocolCorpusFindNode(node, part.name)
								b := wire[part.offset : part.offset+20]
								protocolCorpusRequireValue(t, identity, "Type", uint64(b[0]))
								protocolCorpusRequireValue(t, identity, "Length", uint64(20))
								protocolCorpusRequireValue(t, identity, "System Priority", uint64(binary.BigEndian.Uint16(b[2:])))
								protocolCorpusRequireValue(t, identity, "System ID", b[4:10])
								protocolCorpusRequireValue(t, identity, "Key", uint64(binary.BigEndian.Uint16(b[10:])))
								protocolCorpusRequireValue(t, identity, "Port Priority", uint64(binary.BigEndian.Uint16(b[12:])))
								protocolCorpusRequireValue(t, identity, "Port", uint64(binary.BigEndian.Uint16(b[14:])))
								protocolCorpusRequireValue(t, identity, "State", uint64(b[16]))
								protocolCorpusRequireValue(t, identity, "Reserved", b[17:20])
							}
							protocolCorpusRequireValue(t, node, "Collector Type", uint64(3))
							protocolCorpusRequireValue(t, node, "Collector Length", uint64(16))
							protocolCorpusRequireValue(t, node, "Collector Maximum Delay", uint64(0))
							protocolCorpusRequireValue(t, node, "Collector Reserved", wire[46:58])
							protocolCorpusRequireValue(t, node, "Terminator Type", uint64(0))
							protocolCorpusRequireValue(t, node, "Terminator Length", uint64(0))
							protocolCorpusRequireValue(t, node, "Reserved Padding", wire[60:])
						case "homeplug-av":
							for _, field := range []string{"Version", "Message Type", "Fragment Count", "Fragment Index", "Fragment Sequence"} {
								protocolCorpusRequireValue(t, node, field, uint64(0))
							}
							protocolCorpusRequireValue(t, node, "Message Data", wire[5:])
							require.Nil(t, protocolCorpusFindNode(node, "Organization ID"))
						case "vntag":
							for _, field := range []string{"Direction", "Pointer", "Destination Interface", "Looped", "Reserved", "Version", "Source Interface"} {
								protocolCorpusRequireValue(t, node, field, uint64(0))
							}
							protocolCorpusRequireValue(t, node, "Protocol Type", uint64(0xffff))
							protocolCorpusRequireValue(t, node, "Protocol Data", wire[6:])
							require.Nil(t, protocolCorpusFindNode(node, "IPv4"))
						case "pbb":
							linkSamplePBBFields(t, node, wire)
						}
					}
				})
			}
		})
	}
}

func linkSampleMarkerFields(t *testing.T, node *base.Node, wire []byte) {
	t.Helper()
	for field, offset := range map[string]int{"Subtype": 0, "Version": 1, "TLV Type": 2, "TLV Length": 3, "Terminator Type": 18, "Terminator Length": 19} {
		protocolCorpusRequireValue(t, node, field, uint64(wire[offset]))
	}
	protocolCorpusRequireValue(t, node, "Requester Port", uint64(binary.BigEndian.Uint16(wire[4:])))
	protocolCorpusRequireValue(t, node, "Requester System", wire[6:12])
	protocolCorpusRequireValue(t, node, "Transaction ID", uint64(binary.BigEndian.Uint32(wire[12:])))
	protocolCorpusRequireValue(t, node, "Requester Pad", uint64(binary.BigEndian.Uint16(wire[16:])))
	protocolCorpusRequireValue(t, node, "Reserved Padding", wire[20:])
}

func linkSamplePBBFields(t *testing.T, node *base.Node, wire []byte) {
	t.Helper()
	for field, value := range map[string]uint64{
		"Priority": uint64(wire[0] >> 5), "Drop Eligible": uint64(wire[0]>>4) & 1,
		"No Customer Addresses": uint64(wire[0]>>3) & 1, "Reserved 1": uint64(wire[0]>>2) & 1,
		"Reserved 2": uint64(wire[0]) & 3, "Service ID": uint64(binary.BigEndian.Uint32(wire) & 0xffffff),
	} {
		protocolCorpusRequireValue(t, node, field, value)
	}
	customer := protocolCorpusFindNode(node, "Customer Ethernet")
	require.NotNil(t, customer)
	protocolCorpusRequireValue(t, customer, "Destination", wire[4:10])
	protocolCorpusRequireValue(t, customer, "Source", wire[10:16])
	protocolCorpusRequireValue(t, customer, "Type", uint64(0x0800))
	protocolCorpusRequireValue(t, customer, "Total Length", uint64(28))
	protocolCorpusRequireValue(t, customer, "Protocol", uint64(1))
	protocolCorpusRequireValue(t, customer, "Identifier", uint64(0))
}

func TestProtocolCorpusLinkSamplesVariantsAndBoundaries(t *testing.T) {
	marker := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-lacp-marker.pcap")[0][14:]
	for _, kind := range []byte{1, 2} {
		wire := append([]byte(nil), marker...)
		wire[2], wire[4], wire[5], wire[12], wire[15] = kind, 0x12, 0x34, 0xab, 0xcd
		for _, entry := range []string{"LACPMarker", "SlowProtocols"} {
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "link_samples", entry)
			linkSampleMarkerFields(t, node, wire)
		}
	}
	pbb := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-pbb.pcap")[0][14:]
	changedPBB := append([]byte(nil), pbb...)
	copy(changedPBB, []byte{0xb0, 0x12, 0x34, 0x56})
	linkSamplePBBFields(t, protocolCorpusRequireBoundedRuleParse(t, changedPBB, "link_samples", "ProviderBackboneBridge"), changedPBB)

	// Unlike the original unknown-type fixture this VN-Tag carries an actual
	// IPv4 packet immediately after its inner EtherType, with nonzero bitfields.
	vntag := append([]byte{0xd2, 0x34, 0xa5, 0x67, 0x08, 0x00}, pbb[18:]...)
	node := protocolCorpusRequireBoundedRuleParse(t, vntag, "link_samples", "VNTag")
	for field, value := range map[string]uint64{"Direction": 1, "Pointer": 1, "Destination Interface": 0x1234, "Looped": 1, "Reserved": 0, "Version": 2, "Source Interface": 0x567, "Protocol Type": 0x0800, "Total Length": 28} {
		protocolCorpusRequireValue(t, node, field, value)
	}
	for _, etherType := range []uint16{0x0800, 0x86dd} {
		wire := append([]byte(nil), vntag[:6]...)
		binary.BigEndian.PutUint16(wire[4:], etherType)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "link_samples", "VNTag")
		require.ErrorContains(t, err, "vntag: missing network payload")
	}
	for _, wire := range [][]byte{{0, 0x02, 0xa0, 0, 0xb0, 0x52, 0xab}, {1, 0x02, 0xa0, 0x21, 0x7f, 0, 0xb0, 0x52, 0xab}, {1, 0x01, 0x60, 0x21, 0x7f, 0xab}} {
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "link_samples", "HomePlugAV")
		protocolCorpusRequireValue(t, node, "Message Type", uint64(binary.LittleEndian.Uint16(wire[1:])))
		protocolCorpusRequireValue(t, node, "Message Data", []byte{0xab})
		if wire[0] == 1 {
			protocolCorpusRequireValue(t, node, "Fragment Count", uint64(2))
			protocolCorpusRequireValue(t, node, "Fragment Index", uint64(1))
			protocolCorpusRequireValue(t, node, "Fragment Sequence", uint64(0x7f))
		}
		if wire[2]&0x80 != 0 {
			protocolCorpusRequireValue(t, node, "Organization ID", []byte{0, 0xb0, 0x52})
		}
	}
	// Fixed PDUs require their whole record. Opaque variable bodies have no
	// declared length: only cuts inside their required headers are invalid.
	for _, tc := range []struct {
		entry string
		wire  []byte
		limit int
	}{{"LACPMarker", marker, len(marker)}, {"SlowProtocols", marker, len(marker)}, {"ProviderBackboneBridge", pbb, len(pbb)}, {"VNTag", vntag, 6}, {"HomePlugAV", []byte{0, 0, 0, 0, 0}, 5}} {
		for cut := 0; cut < tc.limit; cut++ {
			t.Run(fmt.Sprintf("%s/cut-%d", tc.entry, cut), func(t *testing.T) {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(tc.wire[:cut]), "link_samples", tc.entry)
				require.Error(t, err)
			})
		}
	}
	for _, offset := range []int{0, 1, 2, 3, 18, 19} {
		wire := append([]byte(nil), marker...)
		wire[offset] = 0xff
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "link_samples", "LACPMarker")
		require.Error(t, err)
	}
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(append([]byte(nil), marker...), 0)), "link_samples", "LACPMarker")
	require.Error(t, err)
	for _, wire := range [][]byte{{2, 0, 0, 0, 0}, {0, 0, 0, 0x12, 0}, {0, 0, 0x80, 0, 0}} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "link_samples", "HomePlugAV")
		require.Error(t, err)
	}
}
