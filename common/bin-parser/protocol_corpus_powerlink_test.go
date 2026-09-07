package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// EPSG DS 301 4.6.1.1.2 and the publisher's reference implementation:
// https://github.com/OpenAutomationTechnologies/openPOWERLINK_V2/blob/master/stack/include/oplk/frame.h
// SoC has 22 bytes of protocol fields; Ethernet can add 24 padding bytes.
// The retained PR record has invalid zero node IDs. Never overwrite it.
func TestProtocolCorpusPowerlinkEveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-powerlink.pcap")
	require.Len(t, frames, 1)
	for _, frame := range frames {
		require.Len(t, frame, 60)
		require.Equal(t, uint16(0x88ab), binary.BigEndian.Uint16(frame[12:14]))
		require.Equal(t, byte(1), frame[14])
		require.Equal(t, make([]byte, 45), frame[15:])
		for _, entry := range []struct{ rule, name string }{{"powerlink", "Powerlink"}, {"ethernet", "Ethernet"}} {
			wire := frame
			if entry.rule == "powerlink" {
				wire = frame[14:]
			}
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), entry.rule, entry.name)
			require.ErrorContains(t, err, "powerlink: invalid node address")
		}
		valid := append([]byte(nil), frame...)
		valid[15], valid[16] = 255, 240
		for _, node := range []*base.Node{
			protocolCorpusRequireBoundedRuleParse(t, valid[14:], "powerlink", "Powerlink"),
			protocolCorpusRequireBoundedRuleParse(t, valid, "ethernet", "Ethernet"),
		} {
			protocolCorpusRequireValue(t, node, "Message Type", uint64(1))
			protocolCorpusRequireValue(t, node, "Destination Node", uint64(255))
			protocolCorpusRequireValue(t, node, "Source Node", uint64(240))
			for _, field := range []string{"SoC Reserved", "Cycle Flags", "SoC Reserved 2", "Net Time Seconds", "Net Time Nanoseconds", "Relative Time"} {
				protocolCorpusRequireValue(t, node, field, uint64(0))
			}
			protocolCorpusRequireValue(t, node, "Frame Trailer", valid[36:])
		}
	}
}

func TestProtocolCorpusPowerlinkVariantsAndBoundaries(t *testing.T) {
	soc := make([]byte, 22)
	copy(soc, []byte{1, 255, 240, 0, 0xc0, 0})
	binary.LittleEndian.PutUint32(soc[6:], 0x12345678)
	binary.LittleEndian.PutUint32(soc[10:], 123456789)
	binary.LittleEndian.PutUint64(soc[14:], 0x0102030405060708)
	node := protocolCorpusRequireBoundedRuleParse(t, soc, "powerlink", "Powerlink")
	protocolCorpusRequireValue(t, node, "Cycle Flags", uint64(0xc0))
	protocolCorpusRequireValue(t, node, "Net Time Seconds", uint64(0x12345678))
	protocolCorpusRequireValue(t, node, "Net Time Nanoseconds", uint64(123456789))
	protocolCorpusRequireValue(t, node, "Relative Time", uint64(0x0102030405060708))
	require.Nil(t, protocolCorpusFindNode(node, "Frame Trailer"))
	preq := []byte{3, 1, 240, 0, 0x25, 0, 0x12, 0, 3, 0, 0xab, 0xcd, 0xef}
	pres := []byte{4, 255, 42, 0xfd, 0x31, 0x1a, 0x21, 0, 3, 0, 0xde, 0xad, 0xbe}
	// EPSG DS 301 V1.5.1 section 4.5, node-ID assignment table: 253 and
	// 254 can participate in the isochronous phase alongside regular CNs.
	for _, dst := range []byte{1, 239, 253, 254} {
		wire := append([]byte(nil), preq...)
		wire[1] = dst
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "powerlink", "Powerlink")
		protocolCorpusRequireValue(t, node, "Destination Node", uint64(dst))
		protocolCorpusRequireValue(t, node, "Process Data", wire[10:])
	}
	for _, dst := range []byte{0, 240, 241, 250, 251, 252, 255} {
		wire := append([]byte(nil), preq...)
		wire[1] = dst
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "powerlink", "Powerlink")
		require.ErrorContains(t, err, "address", "destination %d", dst)
	}
	for _, wire := range [][]byte{preq, pres} {
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "powerlink", "Powerlink")
		protocolCorpusRequireValue(t, node, "Message Type", uint64(wire[0]))
		protocolCorpusRequireValue(t, node, "Destination Node", uint64(wire[1]))
		protocolCorpusRequireValue(t, node, "Source Node", uint64(wire[2]))
		protocolCorpusRequireValue(t, node, "Poll Flags", uint64(wire[4]))
		protocolCorpusRequireValue(t, node, "Async Request Flags", uint64(wire[5]))
		protocolCorpusRequireValue(t, node, "PDO Version", uint64(wire[6]))
		protocolCorpusRequireValue(t, node, "Payload Size", uint64(3))
		protocolCorpusRequireValue(t, node, "Process Data", wire[10:])
		if wire[0] == 4 {
			protocolCorpusRequireValue(t, node, "NMT Status", uint64(0xfd))
		}
		padded := append(append([]byte(nil), wire...), bytes.Repeat([]byte{0xa5}, 33)...)
		node = protocolCorpusRequireBoundedRuleParse(t, padded, "powerlink", "Powerlink")
		protocolCorpusRequireValue(t, node, "Process Data", wire[10:])
		protocolCorpusRequireValue(t, node, "Frame Trailer", padded[13:])
	}
	soa := []byte{5, 255, 240, 0xfd, 6, 0, 2, 42, 0x20, 1}
	node = protocolCorpusRequireBoundedRuleParse(t, soa, "powerlink", "Powerlink")
	for field, value := range map[string]uint64{"NMT Status": 0xfd, "Async Flags": 6, "Requested Service ID": 2, "Requested Service Target": 42, "Profile Version": 0x20, "Redundancy Flags": 1} {
		protocolCorpusRequireValue(t, node, field, value)
	}
	asnd := []byte{6, 240, 42, 0xfe, 0xde, 0xad}
	node = protocolCorpusRequireBoundedRuleParse(t, asnd, "powerlink", "Powerlink")
	protocolCorpusRequireValue(t, node, "Service ID", uint64(0xfe))
	protocolCorpusRequireValue(t, node, "Service Data", asnd[4:])
	// Only the ASnd service header is covered; no length is invented for an
	// unknown service body. Core fixed headers and declared PDO data are strict.
	for _, tc := range []struct {
		wire     []byte
		required int
	}{{soc, len(soc)}, {preq, len(preq)}, {pres, len(pres)}, {soa, len(soa)}, {asnd, 4}} {
		for end := 0; end < tc.required; end++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(tc.wire[:end]), "powerlink", "Powerlink")
			require.Error(t, err, "message %d prefix %d", tc.wire[0], end)
		}
	}
	for _, wire := range [][]byte{
		{1, 255, 0}, {1, 0, 240}, {1, 255, 255}, {1, 1, 240}, {1, 255, 42},
		{3, 255, 240}, {3, 1, 42}, {4, 240, 42}, {5, 1, 240}, {2, 255, 240},
		append([]byte{3, 1, 240, 0, 0, 0, 0, 0, 0xff, 0xff}, 0),
		make([]byte, 1505),
	} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "powerlink", "Powerlink")
		require.Error(t, err)
	}
	_, err := parser.ParseBinary(bytes.NewBuffer(soc), "powerlink", "Powerlink")
	require.ErrorContains(t, err, "explicit frame boundary required")
}

func TestProtocolCorpusPowerlinkCompanionEveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-powerlink-valid.pcap")
	require.Len(t, frames, 3)
	for index, frame := range frames {
		require.Len(t, frame, 60)
		wire := frame[14:]
		for _, node := range []*base.Node{
			protocolCorpusRequireBoundedRuleParse(t, wire, "powerlink", "Powerlink"),
			protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet"),
		} {
			protocolCorpusRequireValue(t, node, "Message Type", uint64([]byte{1, 3, 4}[index]))
			protocolCorpusRequireValue(t, node, "Destination Node", uint64([]byte{255, 42, 255}[index]))
			protocolCorpusRequireValue(t, node, "Source Node", uint64([]byte{240, 240, 42}[index]))
			if index == 0 {
				protocolCorpusRequireValue(t, node, "Cycle Flags", uint64(0xc0))
				protocolCorpusRequireValue(t, node, "Net Time Seconds", uint64(0x12345678))
				protocolCorpusRequireValue(t, node, "Net Time Nanoseconds", uint64(123456789))
				protocolCorpusRequireValue(t, node, "Relative Time", uint64(0x0102030405060708))
				protocolCorpusRequireValue(t, node, "Frame Trailer", make([]byte, 24))
			} else {
				protocolCorpusRequireValue(t, node, "Poll Flags", uint64([]byte{0x25, 0x31}[index-1]))
				protocolCorpusRequireValue(t, node, "Async Request Flags", uint64([]byte{0, 0x1a}[index-1]))
				protocolCorpusRequireValue(t, node, "PDO Version", uint64([]byte{0x12, 0x21}[index-1]))
				protocolCorpusRequireValue(t, node, "Payload Size", uint64(3))
				protocolCorpusRequireValue(t, node, "Process Data", [][]byte{{0xab, 0xcd, 0xef}, {0xde, 0xad, 0xbe}}[index-1])
				protocolCorpusRequireValue(t, node, "Frame Trailer", make([]byte, 33))
				if index == 2 {
					protocolCorpusRequireValue(t, node, "NMT Status", uint64(0xfd))
				}
			}
		}
	}
}
