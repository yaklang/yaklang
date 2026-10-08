package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// MEF 16 section 5.5.3 specifies version one and mandatory information
// elements. IEEE 802.3 clause 57 requires local information except LF_INFO.
// The fixed MRP first-value lengths and event encodings are cross-checked
// against OpenAvnu daemons/mrpd/{mmrp,mvrp,msrp}.c. The retained originals
// violate these rules even when a dissector can identify their EtherTypes.
func TestProtocolCorpusManagementSamplesOriginalRejections(t *testing.T) {
	for _, tc := range []struct{ id, entry, reason string }{
		{"elmi", "ELMI", "elmi: version must be one"},
		{"eoam", "EOAM", "eoam: missing local information TLV"},
		{"mmrp", "MMRP", "mrp: invalid fixed attribute length"},
		{"mvrp", "MVRP", "mrp: invalid fixed attribute length"},
		{"msrp", "MSRP", "mrp: invalid fixed attribute length"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-"+tc.id+".pcap")
			require.Len(t, frames, 1)
			for _, frame := range frames {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(frame[14:]), "management_samples", tc.entry)
				require.ErrorContains(t, err, tc.reason)
				_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(frame), "ethernet", "Ethernet")
				require.ErrorContains(t, err, tc.reason)
			}
		})
	}
}

func managementSampleWire(t *testing.T, id string) []byte {
	t.Helper()
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-"+id+"-valid.pcap")
	require.Len(t, frames, 1)
	return frames[0][14:]
}

func TestProtocolCorpusManagementSamplesEveryRecord(t *testing.T) {
	for _, tc := range []struct {
		id, entry string
		etherType uint16
	}{
		{"elmi", "ELMI", 0x88ee}, {"eoam", "EOAM", 0x8809},
		{"mmrp", "MMRP", 0x88f6}, {"mvrp", "MVRP", 0x88f5}, {"msrp", "MSRP", 0x22ea},
	} {
		t.Run(tc.id, func(t *testing.T) {
			frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-"+tc.id+"-valid.pcap")
			require.Len(t, frames, 1)
			for index, frame := range frames {
				t.Run(fmt.Sprintf("record-%d", index+1), func(t *testing.T) {
					require.Equal(t, tc.etherType, binary.BigEndian.Uint16(frame[12:14]))
					direct := protocolCorpusRequireBoundedRuleParse(t, frame[14:], "management_samples", tc.entry)
					ethernet := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
					for _, node := range []*base.Node{direct, ethernet} {
						switch tc.id {
						case "elmi":
							for field, value := range map[string]uint64{"Version": 1, "Message Type": 117, "Report Type": 1, "Send Sequence": 1, "Receive Sequence": 2, "Reserved": 0, "Data Instance": 1} {
								protocolCorpusRequireValue(t, node, field, value)
							}
							elements := protocolCorpusNodesNamed(node, "Element")
							require.Len(t, elements, 3)
							for i, length := range []uint64{1, 2, 5} {
								protocolCorpusRequireValue(t, elements[i], "Tag", uint64(i+1))
								protocolCorpusRequireValue(t, elements[i], "Length", length)
							}
						case "eoam":
							managementSampleEOAMFields(t, node)
						default:
							managementSampleMRPFields(t, node, tc.id)
						}
					}
				})
			}
		})
	}
}

func managementSampleEOAMFields(t *testing.T, node *base.Node) {
	t.Helper()
	for field, value := range map[string]uint64{"Subtype": 3, "Reserved Flags": 0, "Remote Stable": 1, "Remote Evaluating": 0, "Local Stable": 1, "Local Evaluating": 0, "Critical Event": 0, "Dying Gasp": 0, "Link Fault": 0, "Code": 0, "Information End": 0} {
		protocolCorpusRequireValue(t, node, field, value)
	}
	tlvs := protocolCorpusNodesNamed(node, "TLV")
	require.Len(t, tlvs, 3)
	for i, revision := range []uint64{42, 7} {
		n := tlvs[i]
		for field, value := range map[string]uint64{"Type": uint64(i + 1), "Length": 16, "OAM Version": 1, "Revision": revision, "Reserved State": 0, "Multiplexer Action": 0, "Parser Action": 0, "Reserved Configuration": 0, "Mode": 1, "Maximum PDU Size": 1518, "Reserved PDU Configuration": 0} {
			protocolCorpusRequireValue(t, n, field, value)
		}
		config := byte(0x1f)
		if i == 1 {
			config = 9
		}
		for j, field := range []string{"Unidirectional Support", "Loopback Support", "Link Events", "Variable Response"} {
			protocolCorpusRequireValue(t, n, field, uint64(config>>uint(j+1)&1))
		}
		protocolCorpusRequireValue(t, n, "Organization ID", []byte{2, 0, 0})
		protocolCorpusRequireValue(t, n, "Vendor Information", []byte{byte(1 + i*4), byte(2 + i*4), byte(3 + i*4), byte(4 + i*4)})
	}
	protocolCorpusRequireValue(t, tlvs[2], "Type", uint64(254))
	protocolCorpusRequireValue(t, tlvs[2], "Length", uint64(6))
	protocolCorpusRequireValue(t, tlvs[2], "Information Data", []byte{2, 0, 0, 0xaa})
	protocolCorpusRequireValue(t, node, "Padding", []byte{0, 0, 0})
}

func managementSampleInfoValues(t *testing.T, node *base.Node, key string, expected []uint64) {
	t.Helper()
	info := node.Cfg.GetItem("additionInfo").(map[string]any)
	values := reflect.ValueOf(info[key])
	require.Equal(t, len(expected), values.Len())
	for i, value := range expected {
		require.EqualValues(t, value, values.Index(i).Interface())
	}
}

func managementSampleMRPFields(t *testing.T, node *base.Node, family string) {
	t.Helper()
	protocolCorpusRequireValue(t, node, "Protocol Version", uint64(0))
	protocolCorpusRequireValue(t, node, "Message End", uint64(0))
	messages := protocolCorpusNodesNamed(node, "Message")
	vectors := protocolCorpusNodesNamed(node, "Vector")
	counts := map[string]int{"mmrp": 2, "mvrp": 1, "msrp": 4}
	require.Len(t, messages, counts[family])
	require.Len(t, vectors, counts[family])
	lengths := map[string][]uint64{"mmrp": {1, 6}, "mvrp": {2}, "msrp": {25, 34, 8, 4}}
	for i, message := range messages {
		protocolCorpusRequireValue(t, message, "Attribute Type", uint64(i+1))
		protocolCorpusRequireValue(t, message, "Attribute Length", lengths[family][i])
		protocolCorpusRequireValue(t, message, "Vector End", uint64(0))
		vector := vectors[i]
		count := uint64(3)
		if family == "mmrp" && i == 0 || family == "msrp" && i == 3 {
			count = 2
		}
		if family == "mvrp" {
			count = 4
		}
		protocolCorpusRequireValue(t, vector, "Leave All Event", uint64(0))
		protocolCorpusRequireValue(t, vector, "Number of Values", count)
		events := []uint64{0, 1, 2, 3}[:count]
		managementSampleInfoValues(t, vector, "Events", events)
		if family == "msrp" {
			listLength := uint64(2 + lengths[family][i] + 1 + 2)
			if i == 2 {
				listLength++
			}
			protocolCorpusRequireValue(t, message, "Attribute List Length", listLength)
		}
	}
	switch family {
	case "mmrp":
		protocolCorpusRequireValue(t, vectors[0], "Service Requirement", uint64(0))
		protocolCorpusRequireValue(t, vectors[1], "First MAC Address", mustHex(t, "020000000010"))
	case "mvrp":
		protocolCorpusRequireValue(t, vectors[0], "First VLAN ID", uint64(100))
		protocolCorpusRequireValue(t, vectors[0], "Packed Events", []byte{8, 108})
	case "msrp":
		for i := 0; i < 3; i++ {
			protocolCorpusRequireValue(t, vectors[i], "Stream ID", mustHex(t, "0200000000011234"))
		}
		for i := 0; i < 2; i++ {
			n := vectors[i]
			protocolCorpusRequireValue(t, n, "Destination MAC", mustHex(t, "91e0f0000001"))
			for field, value := range map[string]uint64{"VLAN ID": 100, "Maximum Frame Size": 1500, "Maximum Interval Frames": 1, "Priority": 3, "Rank": 1, "Reserved": 0, "Accumulated Latency": 12345} {
				protocolCorpusRequireValue(t, n, field, value)
			}
		}
		protocolCorpusRequireValue(t, vectors[1], "Bridge ID", mustHex(t, "0200000000025678"))
		protocolCorpusRequireValue(t, vectors[1], "Failure Code", uint64(1))
		protocolCorpusRequireValue(t, vectors[2], "Packed Declarations", []byte{0x6c})
		managementSampleInfoValues(t, vectors[2], "Listener Declarations", []uint64{1, 2, 3})
		for field, value := range map[string]uint64{"Class ID": 6, "Class Priority": 3, "Class VLAN ID": 100} {
			protocolCorpusRequireValue(t, vectors[3], field, value)
		}
	}
}

func TestProtocolCorpusManagementSamplesBoundaries(t *testing.T) {
	for _, tc := range []struct{ id, entry string }{{"elmi", "ELMI"}, {"eoam", "EOAM"}, {"mmrp", "MMRP"}, {"mvrp", "MVRP"}, {"msrp", "MSRP"}} {
		wire := managementSampleWire(t, tc.id)
		_, unboundedErr := parser.ParseBinary(bytes.NewReader(wire), "management_samples", tc.entry)
		require.ErrorContains(t, unboundedErr, "message boundary is required")
		limit := len(wire)
		if tc.id == "eoam" {
			limit -= 3
		} // Ethernet padding is not part of the information PDU.
		for cut := 0; cut < limit; cut++ {
			t.Run(fmt.Sprintf("%s/cut-%d", tc.id, cut), func(t *testing.T) {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "management_samples", tc.entry)
				require.Error(t, err)
			})
		}
		badVersion := append([]byte(nil), wire...)
		badVersion[0] = 0xff
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(badVersion), "management_samples", tc.entry)
		require.Error(t, err)
	}
	for _, tc := range []struct {
		id, entry string
		offset    int
		value     byte
		reason    string
	}{
		{"elmi", "ELMI", 1, 1, "unsupported message type"},
		{"elmi", "ELMI", 3, 2, "report type length"},
		{"elmi", "ELMI", 4, 4, "invalid report type"},
		{"eoam", "EOAM", 5, 15, "length must be sixteen"},
		{"eoam", "EOAM", 6, 2, "version must be one"},
		{"eoam", "EOAM", 4, 2, "first TLV must be local"},
		{"mmrp", "MMRP", 2, 7, "fixed attribute length"},
		{"mvrp", "MVRP", 2, 7, "fixed attribute length"},
		{"msrp", "MSRP", 2, 7, "fixed attribute length"},
		{"msrp", "MSRP", 4, 1, "attribute-list length"},
		{"mvrp", "MVRP", 3, 0x40, "invalid leave-all event"},
		{"mvrp", "MVRP", 7, 216, "invalid packed event"},
	} {
		t.Run(fmt.Sprintf("%s/invalid-%d-%d", tc.id, tc.offset, tc.value), func(t *testing.T) {
			wire := append([]byte(nil), managementSampleWire(t, tc.id)...)
			wire[tc.offset] = tc.value
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "management_samples", tc.entry)
			require.ErrorContains(t, err, tc.reason)
		})
	}
}

func TestProtocolCorpusManagementSamplesVariants(t *testing.T) {
	// An asynchronous E-LMI status needs only Report Type; extension elements
	// retain their declared bytes without claiming to decode an application.
	node := protocolCorpusRequireBoundedRuleParse(t, mustHex(t, "017d010102fd02abcd0000"), "management_samples", "ELMI")
	protocolCorpusRequireValue(t, node, "Report Type", uint64(2))
	protocolCorpusRequireValue(t, node, "Element Data", mustHex(t, "abcd"))
	protocolCorpusRequireValue(t, node, "Padding", []byte{0, 0})
	// LF_INFO explicitly permits an empty information set.
	node = protocolCorpusRequireBoundedRuleParse(t, mustHex(t, "0300010000"), "management_samples", "EOAM")
	protocolCorpusRequireValue(t, node, "Link Fault", uint64(1))
	for _, command := range []byte{1, 2} {
		node = protocolCorpusRequireBoundedRuleParse(t, []byte{3, 0, 0, 4, command}, "management_samples", "EOAM")
		protocolCorpusRequireValue(t, node, "Loopback Command", uint64(command))
	}
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader([]byte{3, 0, 0, 4, 3}), "management_samples", "EOAM")
	require.ErrorContains(t, err, "unsupported loopback command")
	// LeaveAll with no events still contains its first-value field.
	node = protocolCorpusRequireBoundedRuleParse(t, mustHex(t, "00010220000064000000000000"), "management_samples", "MVRP")
	vector := protocolCorpusFindNode(node, "Vector")
	protocolCorpusRequireValue(t, vector, "Leave All Event", uint64(1))
	protocolCorpusRequireValue(t, vector, "Number of Values", uint64(0))
	protocolCorpusRequireValue(t, vector, "First VLAN ID", uint64(100))
	managementSampleInfoValues(t, vector, "Events", nil)
	protocolCorpusRequireValue(t, node, "Padding", []byte{0, 0})
}
