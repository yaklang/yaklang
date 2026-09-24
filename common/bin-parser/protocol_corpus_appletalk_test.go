package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math/bits"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// These independent encodings exercise Apple's extended DDP header and
// Linux's optional rotating checksum. The generated records are not a live
// exchange. Types 4 and fe retain their application data without interpreting
// AEP or an unknown protocol as a complete AppleTalk application implementation.
// https://github.com/apple-oss-distributions/xnu/blob/xnu-1228.15.4/bsd/netat/ddp.h
// https://github.com/torvalds/linux/blob/v6.12/net/appletalk/ddp.c
var appleTalkCorpusBodies = []string{
	"0017af9512342345562a04810401a1b2c3007f80fedc55",
	"0c170000234512342a5681040402a1b2c3007f80fedc55",
	"241460e9abcd1234785691a3fe00137f80a5feff",
}

func appleTalkCorpusBody(t *testing.T, index int) []byte {
	t.Helper()
	wire, err := hex.DecodeString(appleTalkCorpusBodies[index])
	require.NoError(t, err)
	return wire
}

func appleTalkCorpusChecksum(body []byte) uint16 {
	var sum uint16
	for _, value := range body[4:] {
		sum = bits.RotateLeft16(sum+uint16(value), 1)
	}
	if sum == 0 {
		return 0xffff
	}
	return sum
}

func appleTalkCorpusEthernet(body []byte, snap bool) []byte {
	header := []byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 0x80, 0x9b}
	if snap {
		binary.BigEndian.PutUint16(header[12:], uint16(8+len(body)))
		header = append(header, 0xaa, 0xaa, 3, 8, 0, 7, 0x80, 0x9b)
	}
	return append(header, body...)
}

func appleTalkCorpusRequireFields(t *testing.T, node *base.Node, body []byte, offset int) {
	t.Helper()
	word := binary.BigEndian.Uint16(body)
	for _, field := range []struct {
		name       string
		start, end int
		value      uint64
	}{
		{"Reserved", 0, 2, uint64(word >> 14)},
		{"Hop Count", 2, 6, uint64(word >> 10 & 15)},
		{"Datagram Length", 6, 16, uint64(word & 1023)},
		{"Checksum", 16, 32, uint64(binary.BigEndian.Uint16(body[2:]))},
		{"Destination Network", 32, 48, uint64(binary.BigEndian.Uint16(body[4:]))},
		{"Source Network", 48, 64, uint64(binary.BigEndian.Uint16(body[6:]))},
		{"Destination Node", 64, 72, uint64(body[8])},
		{"Source Node", 72, 80, uint64(body[9])},
		{"Destination Socket", 80, 88, uint64(body[10])},
		{"Source Socket", 88, 96, uint64(body[11])},
		{"DDP Type", 96, 104, uint64(body[12])},
	} {
		protocolCorpusRequireValue(t, node, field.name, field.value)
		leaf := protocolCorpusFindNode(node, field.name)
		require.Equal(t, [2]uint64{uint64(offset*8 + field.start), uint64(offset*8 + field.end)}, stream_parser.GetNodeResultPos(leaf), field.name)
	}
	if len(body) > 13 {
		protocolCorpusRequireValue(t, node, "DDP Data", body[13:])
		require.Equal(t, [2]uint64{uint64(offset+13) * 8, uint64(offset+len(body)) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "DDP Data")))
	} else {
		require.Nil(t, protocolCorpusFindNode(node, "DDP Data"))
	}
	message := protocolCorpusFindNode(node, "Datagram Length").Cfg.GetItem(base.CfgParent).(*base.Node)
	info, ok := message.Cfg.GetItem("additionInfo").(map[string]any)
	require.True(t, ok)
	present := binary.BigEndian.Uint16(body[2:]) != 0
	require.Equal(t, present, info["Checksum Present"])
	require.Equal(t, present, info["Checksum Validated"])
	require.Equal(t, false, info["Application Data Decoded"])
	if present {
		require.Equal(t, binary.BigEndian.Uint16(body[2:]), appleTalkCorpusChecksum(body))
	}
}

func TestProtocolCorpusAppleTalkCompanionEveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-appletalk-valid.pcap")
	require.Len(t, frames, 3)
	for index, frame := range frames {
		t.Run(fmt.Sprint(index+1), func(t *testing.T) {
			body := appleTalkCorpusBody(t, index)
			expected := appleTalkCorpusEthernet(body, true)
			if index == 1 {
				copy(expected[:6], []byte{2, 0, 0, 0, 0, 1})
				copy(expected[6:12], []byte{2, 0, 0, 0, 0, 2})
			}
			expected = append(expected, make([]byte, 60-len(expected))...)
			require.Equal(t, expected, frame)
			for _, entry := range []struct {
				rule, name string
				wire       []byte
				offset     int
			}{
				{"appletalk", "DDP", body, 0},
				{"llc", "LLC", frame[14 : 22+len(body)], 8},
				{"ethernet", "Ethernet", frame, 22},
			} {
				node := protocolCorpusRequireBoundedRuleParse(t, entry.wire, entry.rule, entry.name)
				appleTalkCorpusRequireFields(t, node, body, entry.offset)
				require.Nil(t, protocolCorpusFindNode(node, "DDP Payload"))
				if entry.rule == "ethernet" {
					protocolCorpusRequireValue(t, node, "Frame Trailer", frame[22+len(body):])
				}
			}
			// EtherType 809b is a separate carrier variant, not the Linux
			// EtherTalk Phase 2 SNAP encoding recorded in the companion.
			variant := appleTalkCorpusEthernet(body, false)
			appleTalkCorpusRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, variant, "ethernet", "Ethernet"), body, 14)
		})
	}
}

func appleTalkCorpusRequireFallback(t *testing.T, body []byte) {
	t.Helper()
	for _, snap := range []bool{false, true} {
		wire := appleTalkCorpusEthernet(body, snap)
		if len(body) == 0 {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "ethernet", "Ethernet")
			require.ErrorContains(t, err, "ddp: empty carrier has no datagram")
			continue
		}
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "ethernet", "Ethernet")
		protocolCorpusRequireValue(t, node, "DDP Payload", body)
		require.Nil(t, protocolCorpusFindNode(node, "Datagram Length"), "candidate fields must not escape rollback")
		offset := 14
		if snap {
			offset = 22
		}
		require.Equal(t, [2]uint64{uint64(offset) * 8, uint64(len(wire)) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "DDP Payload")))
	}
}

func TestProtocolCorpusAppleTalkOriginalNegativeAndLengths(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-appletalk.pcap")
	require.Len(t, frames, 1)
	original, err := hex.DecodeString("020000000002020000000001809b000e010203040102")
	require.NoError(t, err)
	require.Equal(t, original, frames[0])
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(original[14:]), "appletalk", "DDP")
	require.ErrorContains(t, err, "13 through 599")
	appleTalkCorpusRequireFallback(t, original[14:])
	for index := range appleTalkCorpusBodies {
		body := appleTalkCorpusBody(t, index)
		for end := 0; end < len(body); end++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(body[:end]), "appletalk", "DDP")
			require.ErrorContains(t, err, "ddp:", "record %d prefix %d", index+1, end)
			appleTalkCorpusRequireFallback(t, body[:end])
		}
		_, err := parser.ParseBinary(bytes.NewBuffer(body), "appletalk", "DDP")
		require.ErrorContains(t, err, "explicit datagram boundary required")
		_, err = parser.ParseBinary(bytes.NewBuffer(body), "appletalk", "DDPCarrier")
		require.ErrorContains(t, err, "explicit carrier boundary required")
		_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(append(bytes.Clone(body), 0xa5)), "appletalk", "DDP")
		require.ErrorContains(t, err, "declared length")
	}
	body := appleTalkCorpusBody(t, 0)
	for _, length := range []uint16{0, 1, 12, 13, 22, 24, 598, 599, 600, 1023} {
		wire := bytes.Clone(body)
		binary.BigEndian.PutUint16(wire, length)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "appletalk", "DDP")
		require.ErrorContains(t, err, "declared length")
		appleTalkCorpusRequireFallback(t, wire)
	}
	for _, size := range []int{600, 1023, 1024, 65536} {
		wire := make([]byte, size)
		binary.BigEndian.PutUint16(wire, uint16(size&1023))
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "appletalk", "DDP")
		require.ErrorContains(t, err, "13 through 599")
	}
}

func TestProtocolCorpusAppleTalkFieldAndChecksumBoundaries(t *testing.T) {
	// Checksum zero means omitted, while an actual zero sum is encoded ffff.
	for _, size := range []int{13, 14, 598, 599} {
		wire := make([]byte, size)
		binary.BigEndian.PutUint16(wire, uint16(size))
		for i := 4; i < len(wire); i++ {
			wire[i] = byte(i*37 + 11)
		}
		binary.BigEndian.PutUint16(wire[2:], appleTalkCorpusChecksum(wire))
		appleTalkCorpusRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, wire, "appletalk", "DDP"), wire, 0)
	}
	zero := make([]byte, 13)
	zero[1] = 13
	for _, checksum := range []uint16{0, 0xffff} {
		binary.BigEndian.PutUint16(zero[2:], checksum)
		appleTalkCorpusRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, zero, "appletalk", "DDP"), zero, 0)
	}
	for _, reserved := range []uint16{0, 1, 2, 3} {
		for hops := uint16(0); hops < 16; hops++ {
			wire := appleTalkCorpusBody(t, 0)
			// Neither high header bits nor hop count participate in checksum.
			binary.BigEndian.PutUint16(wire, reserved<<14|hops<<10|uint16(len(wire)))
			appleTalkCorpusRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, wire, "appletalk", "DDP"), wire, 0)
		}
	}
	for _, kind := range []byte{0, 1, 2, 3, 4, 5, 6, 7, 0xfe, 0xff} {
		wire := appleTalkCorpusBody(t, 0)
		wire[12] = kind
		binary.BigEndian.PutUint16(wire[2:], appleTalkCorpusChecksum(wire))
		appleTalkCorpusRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, wire, "appletalk", "DDP"), wire, 0)
	}
	for _, index := range []int{0, 2} {
		body := appleTalkCorpusBody(t, index)
		for offset := 2; offset < len(body); offset++ {
			wire := bytes.Clone(body)
			wire[offset] ^= 1
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "appletalk", "DDP")
			require.ErrorContains(t, err, "checksum mismatch", "byte %d", offset)
			appleTalkCorpusRequireFallback(t, wire)
		}
	}
}

func TestProtocolCorpusAppleTalkDispatchAndCarrierTransactions(t *testing.T) {
	body := appleTalkCorpusBody(t, 0)
	for _, mutation := range []struct {
		offset int
		value  byte
	}{
		{14, 0xab}, {15, 0xab}, {16, 0x13}, {17, 0}, {18, 1}, {19, 0}, {21, 0x9a},
	} {
		wire := appleTalkCorpusEthernet(body, true)
		wire[mutation.offset] = mutation.value
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "ethernet", "Ethernet")
		require.Nil(t, protocolCorpusFindNode(node, "Datagram Length"), "not canonical DDP SNAP")
	}
	invalid := bytes.Clone(body)
	invalid[len(invalid)-1] ^= 1 // Failure after all fields have been processed.
	for _, snap := range []bool{false, true} {
		for _, sample := range []struct {
			name    string
			payload []byte
			valid   bool
		}{
			{"success", body, true},
			{"trailer", append(bytes.Clone(body), 0xa5, 0x5a), true},
			{"second-datagram-is-trailer", append(bytes.Clone(body), body...), true},
			{"invalid-checksum", invalid, false},
			{"short-first-byte", body[:1], false},
			{"short-final-byte", body[:len(body)-1], false},
		} {
			t.Run(fmt.Sprintf("snap=%t/%s", snap, sample.name), func(t *testing.T) {
				wire := appleTalkCorpusEthernet(sample.payload, snap)
				root, err := base.ParseRule("ethernet.yaml")
				require.NoError(t, err)
				root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
				reader := bytes.NewReader(wire)
				bitReader := base.NewBitReader(reader)
				require.NoError(t, root.ParseSubNode(bitReader, "Ethernet"))
				node := base.GetNodeByPath(root, "@Ethernet")
				require.Equal(t, wire, NodeToBytes(node))
				if sample.valid {
					offset := 14
					if snap {
						offset = 22
					}
					appleTalkCorpusRequireFields(t, node, body, offset)
					if len(sample.payload) > len(body) {
						protocolCorpusRequireValue(t, node, "DDP Link Trailer", sample.payload[len(body):])
					}
				} else {
					protocolCorpusRequireValue(t, node, "DDP Payload", sample.payload)
					require.Nil(t, protocolCorpusFindNode(node, "Datagram Length"))
				}
				require.Zero(t, reader.Len())
				require.ErrorContains(t, bitReader.Recovery(), "no backup")
				require.ErrorContains(t, bitReader.PopBackup(), "no backup")
				_, err = bitReader.ReadBits(8)
				require.ErrorIs(t, err, io.EOF)
			})
		}
	}
}

func TestProtocolCorpusAppleTalkParallelIsolation(t *testing.T) {
	for worker := 0; worker < 12; worker++ {
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			for index := range appleTalkCorpusBodies {
				body := appleTalkCorpusBody(t, index)
				appleTalkCorpusRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, body, "appletalk", "DDP"), body, 0)
			}
		})
	}
}
