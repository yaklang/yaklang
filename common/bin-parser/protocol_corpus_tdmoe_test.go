package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func tdmoeTestHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	require.NoError(t, err)
	return decoded
}

func tdmoeTestField(t *testing.T, root *base.Node, name string, value any, start, end uint64) *base.Node {
	t.Helper()
	protocolCorpusRequireValue(t, root, name, value)
	node := protocolCorpusFindNode(root, name)
	require.NotNil(t, node)
	require.Equal(t, [2]uint64{start, end}, stream_parser.GetNodeResultPos(node), "field %q", name)
	return node
}

func tdmoeTestFindAll(root *base.Node, name string) []*base.Node {
	var found []*base.Node
	var walk func(*base.Node)
	walk = func(node *base.Node) {
		if node.Name == name && protocolCorpusNodeHasResult(node) {
			found = append(found, node)
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)
	return found
}

func tdmoeTestFindAny(root *base.Node, name string) *base.Node {
	if root.Name == name {
		return root
	}
	for _, child := range root.Children {
		if found := tdmoeTestFindAny(child, name); found != nil {
			return found
		}
	}
	return nil
}

func tdmoeTestInfo(t *testing.T, node *base.Node, name string, expected any) {
	t.Helper()
	require.NotNil(t, node)
	info, ok := node.Cfg.GetItem("additionInfo").(map[string]any)
	require.True(t, ok, "node %q has no additionInfo", node.Name)
	require.Equal(t, expected, info[name], "node %q info %q", node.Name, name)
}

func tdmoeTestMessage(subaddress uint16, flags byte, counter uint16, signaling []uint16, channels ...[]byte) []byte {
	result := make([]byte, 8)
	binary.BigEndian.PutUint16(result[0:2], subaddress)
	result[2] = 8
	result[3] = flags
	binary.BigEndian.PutUint16(result[4:6], counter)
	binary.BigEndian.PutUint16(result[6:8], uint16(len(channels)))
	for _, word := range signaling {
		result = append(result, byte(word>>8), byte(word))
	}
	for _, samples := range channels {
		if len(samples) != 8 {
			panic("TDMoE test channels require eight samples")
		}
		result = append(result, samples...)
	}
	return result
}

func tdmoeTestEthernet(payload []byte) []byte {
	return append([]byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 0xd0, 0x0d}, payload...)
}

func tdmoeTestFixtures(t *testing.T) [][]byte {
	t.Helper()
	return [][]byte{
		tdmoeTestHex(t, "12340800010200010001020304050607"),
		tdmoeTestHex(t, "00fe0803fffe00054321000510111213141516172021222324252627303132333435363740414243444546475051525354555657"),
		tdmoeTestHex(t, "beef08fd80000002a0a1a2a3a4a5a6a7b0b1b2b3b4b5b6b7"),
	}
}

func TestProtocolCorpusTDMoEFields(t *testing.T) {
	fixtures := tdmoeTestFixtures(t)

	t.Run("without signaling", func(t *testing.T) {
		node := protocolCorpusRequireBoundedRuleParse(t, fixtures[0], "tdmoe", "TDMoE")
		tdmoeTestField(t, node, "Subaddress", uint64(0x1234), 0, 16)
		tdmoeTestField(t, node, "Samples Per Channel", uint64(8), 16, 24)
		tdmoeTestField(t, node, "Reserved", uint64(0), 24, 29)
		tdmoeTestField(t, node, "Loopback", uint64(0), 29, 30)
		tdmoeTestField(t, node, "Signaling Bits Present", uint64(0), 30, 31)
		tdmoeTestField(t, node, "Yellow Alarm", uint64(0), 31, 32)
		tdmoeTestField(t, node, "Packet Counter", uint64(0x0102), 32, 48)
		tdmoeTestField(t, node, "Channel Count", uint64(1), 48, 64)
		tdmoeTestField(t, node, "Channel Samples", []byte{0, 1, 2, 3, 4, 5, 6, 7}, 64, 128)
		require.Nil(t, protocolCorpusFindNode(node, "Signaling Word"))
		tdmoeTestInfo(t, tdmoeTestFindAny(node, "TDMoE"), "Signaling Word Count", 0)
		tdmoeTestInfo(t, tdmoeTestFindAny(node, "TDMoE"), "Channel Data Bytes", 8)
	})

	t.Run("signaling across two words", func(t *testing.T) {
		node := protocolCorpusRequireBoundedRuleParse(t, fixtures[1], "tdmoe", "TDMoE")
		tdmoeTestField(t, node, "Subaddress", uint64(0x00fe), 0, 16)
		tdmoeTestField(t, node, "Samples Per Channel", uint64(8), 16, 24)
		tdmoeTestField(t, node, "Signaling Bits Present", uint64(1), 30, 31)
		tdmoeTestField(t, node, "Yellow Alarm", uint64(1), 31, 32)
		tdmoeTestField(t, node, "Packet Counter", uint64(0xfffe), 32, 48)
		tdmoeTestField(t, node, "Channel Count", uint64(5), 48, 64)

		words := tdmoeTestFindAll(node, "Signaling Word")
		require.Len(t, words, 2)
		for index, expected := range []struct {
			start, represented int
		}{{1, 4}, {5, 1}} {
			tdmoeTestInfo(t, words[index], "First Channel", expected.start)
			tdmoeTestInfo(t, words[index], "Channels Represented", expected.represented)
		}
		for _, expected := range []struct {
			name   string
			values []uint64
			starts []uint64
		}{
			{"High Nibble", []uint64{4, 0}, []uint64{64, 80}},
			{"Upper Middle Nibble", []uint64{3, 0}, []uint64{68, 84}},
			{"Lower Middle Nibble", []uint64{2, 0}, []uint64{72, 88}},
			{"Low Nibble", []uint64{1, 5}, []uint64{76, 92}},
		} {
			fields := tdmoeTestFindAll(node, expected.name)
			require.Len(t, fields, 2)
			for index, field := range fields {
				value, err := field.Result()
				require.NoError(t, err)
				require.Equal(t, expected.values[index], uintVal(t, value))
				require.Equal(t, [2]uint64{expected.starts[index], expected.starts[index] + 4}, stream_parser.GetNodeResultPos(field))
			}
		}
		channels := tdmoeTestFindAll(node, "Channel Samples")
		require.Len(t, channels, 5)
		for index, channel := range channels {
			value, err := channel.Result()
			require.NoError(t, err)
			want := []byte{byte((index + 1) * 0x10), byte((index+1)*0x10 + 1), byte((index+1)*0x10 + 2), byte((index+1)*0x10 + 3), byte((index+1)*0x10 + 4), byte((index+1)*0x10 + 5), byte((index+1)*0x10 + 6), byte((index+1)*0x10 + 7)}
			require.Equal(t, want, value.Value)
			require.Equal(t, [2]uint64{uint64(12+index*8) * 8, uint64(20+index*8) * 8}, stream_parser.GetNodeResultPos(channel))
			tdmoeTestInfo(t, channel, "Channel Number", index+1)
		}
		tdmoeTestInfo(t, tdmoeTestFindAny(node, "TDMoE"), "Signaling Word Count", 2)
		tdmoeTestInfo(t, tdmoeTestFindAny(node, "TDMoE"), "Channel Data Bytes", 40)
	})

	t.Run("reserved and loopback receive flags", func(t *testing.T) {
		node := protocolCorpusRequireBoundedRuleParse(t, fixtures[2], "tdmoe", "TDMoE")
		tdmoeTestField(t, node, "Reserved", uint64(31), 24, 29)
		tdmoeTestField(t, node, "Loopback", uint64(1), 29, 30)
		tdmoeTestField(t, node, "Signaling Bits Present", uint64(0), 30, 31)
		tdmoeTestField(t, node, "Yellow Alarm", uint64(1), 31, 32)
		tdmoeTestField(t, node, "Packet Counter", uint64(0x8000), 32, 48)
		require.Len(t, tdmoeTestFindAll(node, "Channel Samples"), 2)
	})

	t.Run("unused signaling nibbles are preserved", func(t *testing.T) {
		wire := tdmoeTestMessage(7, 0x02, 9, []uint16{0xfff5}, tdmoeTestHex(t, "a0a1a2a3a4a5a6a7"))
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "tdmoe", "TDMoE")
		tdmoeTestField(t, node, "High Nibble", uint64(15), 64, 68)
		tdmoeTestField(t, node, "Upper Middle Nibble", uint64(15), 68, 72)
		tdmoeTestField(t, node, "Lower Middle Nibble", uint64(15), 72, 76)
		tdmoeTestField(t, node, "Low Nibble", uint64(5), 76, 80)
		word := tdmoeTestFindAny(node, "Signaling Word")
		tdmoeTestInfo(t, word, "First Channel", 1)
		tdmoeTestInfo(t, word, "Channels Represented", 1)
	})
}

func TestProtocolCorpusTDMoEPublicPaths(t *testing.T) {
	for index, wire := range tdmoeTestFixtures(t) {
		channels := []uint64{1, 5, 2}[index]
		for _, path := range []struct {
			name, rule, entry string
			input             []byte
			offset            uint64
		}{
			{"direct", "tdmoe", "TDMoE", wire, 0},
			{"carrier", "tdmoe", "TDMoECarrier", wire, 0},
			{"ethernet", "ethernet", "Ethernet", tdmoeTestEthernet(wire), 14},
		} {
			t.Run(path.name+"-record-"+string(rune('1'+index)), func(t *testing.T) {
				node := protocolCorpusRequireBoundedRuleParse(t, path.input, path.rule, path.entry)
				tdmoeTestField(t, node, "Subaddress", uint64(binary.BigEndian.Uint16(wire[:2])), path.offset*8, (path.offset+2)*8)
				tdmoeTestField(t, node, "Samples Per Channel", uint64(8), (path.offset+2)*8, (path.offset+3)*8)
				tdmoeTestField(t, node, "Channel Count", channels, (path.offset+6)*8, (path.offset+8)*8)
				require.Nil(t, protocolCorpusFindNode(node, "Unparsed TDMoE Payload"))
			})
		}
	}
}

func TestProtocolCorpusTDMoECompanionEveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-tdmoe-valid.pcap")
	expected := tdmoeTestFixtures(t)
	require.Len(t, frames, len(expected))
	for index, frame := range frames {
		require.Equalf(t, tdmoeTestEthernet(expected[index]), frame, "frame %d bytes", index+1)
		for _, path := range []struct {
			input       []byte
			rule, entry string
		}{
			{frame[14:], "tdmoe", "TDMoE"},
			{frame[14:], "tdmoe", "TDMoECarrier"},
			{frame, "ethernet", "Ethernet"},
		} {
			node := protocolCorpusRequireBoundedRuleParse(t, path.input, path.rule, path.entry)
			protocolCorpusRequireValue(t, node, "Subaddress", uint64(binary.BigEndian.Uint16(expected[index][:2])))
			protocolCorpusRequireValue(t, node, "Samples Per Channel", uint64(8))
			protocolCorpusRequireValue(t, node, "Packet Counter", []uint64{0x0102, 0xfffe, 0x8000}[index])
			protocolCorpusRequireValue(t, node, "Channel Count", []uint64{1, 5, 2}[index])
			require.Lenf(t, tdmoeTestFindAll(node, "Channel Samples"), []int{1, 5, 2}[index], "frame %d channels", index+1)
			require.Nilf(t, protocolCorpusFindNode(node, "Unparsed TDMoE Payload"), "frame %d", index+1)
			if index == 1 {
				require.Len(t, tdmoeTestFindAll(node, "Signaling Word"), 2)
				protocolCorpusRequireValue(t, node, "High Nibble", uint64(4))
				protocolCorpusRequireValue(t, node, "Low Nibble", uint64(1))
			} else {
				require.Nil(t, protocolCorpusFindNode(node, "Signaling Word"))
			}
		}
	}
}

func TestProtocolCorpusTDMoEOriginalAndBoundaries(t *testing.T) {
	original := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-tdmoe.pcap")[0]
	require.Equal(t, tdmoeTestHex(t, "020000000002020000000001d00d00000000000000000000000000000000000000000000000000000000000000000000"), original)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(original[14:]), "tdmoe", "TDMoE")
	require.ErrorContains(t, err, "tdmoe: samples per channel must be eight")
	imported := protocolCorpusRequireBoundedRuleParse(t, original, "ethernet", "Ethernet")
	protocolCorpusRequireValue(t, imported, "Unparsed TDMoE Payload", original[14:])
	require.Nil(t, protocolCorpusFindNode(imported, "Subaddress"))
	require.Equal(t, [2]uint64{14 * 8, uint64(len(original)) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(imported, "Unparsed TDMoE Payload")))

	for fixtureIndex, wire := range tdmoeTestFixtures(t) {
		for cut := 0; cut < len(wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "tdmoe", "TDMoE")
			require.Errorf(t, err, "fixture %d prefix %d", fixtureIndex, cut)
			if cut > 0 {
				node := protocolCorpusRequireBoundedRuleParse(t, wire[:cut], "tdmoe", "TDMoECarrier")
				protocolCorpusRequireValue(t, node, "Unparsed TDMoE Payload", wire[:cut])
				require.Nil(t, protocolCorpusFindNode(node, "Subaddress"))
			}
		}
	}

	zeroChannels := tdmoeTestHex(t, "0000080000000000")
	tooManyChannels := tdmoeTestHex(t, "0000080000000100")
	bad := []struct {
		wire []byte
		want string
	}{
		{original[14:], "samples per channel must be eight"},
		{tdmoeTestHex(t, "00000800000000"), "shorter than the fixed header"},
		{zeroChannels, "channel count must be nonzero"},
		{tooManyChannels, "channel count exceeds the DAHDI implementation limit"},
		{tdmoeTestHex(t, "000008000000000100010203040506"), "message length does not match"},
		{append(bytes.Clone(tdmoeTestFixtures(t)[0]), 0), "message length does not match"},
		{tdmoeTestHex(t, "00000802000000014321"), "message length does not match"},
		{make([]byte, 2177), "message exceeds the 2176-byte implementation boundary"},
	}
	for index, tc := range bad {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(tc.wire), "tdmoe", "TDMoE")
		require.ErrorContainsf(t, err, tc.want, "invalid %d", index)
		node := protocolCorpusRequireBoundedRuleParse(t, tc.wire, "tdmoe", "TDMoECarrier")
		protocolCorpusRequireValue(t, node, "Unparsed TDMoE Payload", tc.wire)
		require.Nil(t, protocolCorpusFindNode(node, "Subaddress"))
	}

	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(nil), "tdmoe", "TDMoECarrier")
	require.ErrorContains(t, err, "tdmoe: empty carrier has no message")
	for _, entry := range []string{"TDMoE", "TDMoECarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(tdmoeTestFixtures(t)[0]), "tdmoe", entry)
		require.ErrorContains(t, err, "explicit")
	}
}

func TestProtocolCorpusTDMoEResourceBounds(t *testing.T) {
	channels := make([][]byte, 255)
	for index := range channels {
		channels[index] = bytes.Repeat([]byte{byte(index)}, 8)
	}
	signaling := make([]uint16, 64)
	for index := range signaling {
		signaling[index] = uint16(index)*0x101 + 0x000f
	}
	maximal := tdmoeTestMessage(0xffff, 0x02, 0xffff, signaling, channels...)
	require.Len(t, maximal, 2176)
	node := protocolCorpusRequireBoundedRuleParse(t, maximal, "tdmoe", "TDMoE")
	require.Len(t, tdmoeTestFindAll(node, "Signaling Word"), 64)
	require.Len(t, tdmoeTestFindAll(node, "Channel Samples"), 255)
	tdmoeTestInfo(t, tdmoeTestFindAny(node, "TDMoE"), "Channel Data Bytes", 2040)

	noSignaling := tdmoeTestMessage(0, 0, 0, nil, channels...)
	require.Len(t, noSignaling, 2048)
	protocolCorpusRequireBoundedRuleParse(t, noSignaling, "tdmoe", "TDMoE")

	tooMany := make([][]byte, 256)
	for index := range tooMany {
		tooMany[index] = make([]byte, 8)
	}
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(tdmoeTestMessage(0, 0, 0, nil, tooMany...)), "tdmoe", "TDMoE")
	require.ErrorContains(t, err, "channel count exceeds the DAHDI implementation limit")
}

func TestProtocolCorpusTDMoECarrierTransactions(t *testing.T) {
	valid := tdmoeTestFixtures(t)[1]
	invalid := bytes.Clone(valid)
	invalid[2] = 7
	lateInvalid := append(bytes.Clone(valid), 0)
	for _, sample := range []struct {
		wire    []byte
		decoded bool
	}{
		{valid, true},
		{invalid, false},
		{lateInvalid, false},
		{[]byte{1}, false},
	} {
		root, err := base.ParseRule("tdmoe.yaml")
		require.NoError(t, err)
		root.Cfg.SetItem(base.CfgLength, uint64(len(sample.wire))*8)
		reader := bytes.NewReader(sample.wire)
		bitReader := base.NewBitReader(reader)
		require.NoError(t, root.ParseSubNode(bitReader, "TDMoECarrier"))
		carrier := base.GetNodeByPath(root, "@TDMoECarrier")
		require.NotNil(t, carrier)
		require.Equal(t, sample.wire, NodeToBytes(carrier))
		if sample.decoded {
			protocolCorpusRequireValue(t, carrier, "Channel Count", uint64(5))
			require.Nil(t, protocolCorpusFindNode(carrier, "Unparsed TDMoE Payload"))
		} else {
			protocolCorpusRequireValue(t, carrier, "Unparsed TDMoE Payload", sample.wire)
			require.Nil(t, protocolCorpusFindNode(carrier, "Subaddress"))
		}
		require.Zero(t, reader.Len())
		require.ErrorContains(t, bitReader.Recovery(), "no backup")
		require.ErrorContains(t, bitReader.PopBackup(), "no backup")
		_, err = bitReader.ReadBits(8)
		require.ErrorIs(t, err, io.EOF)
	}
}

func TestProtocolCorpusTDMoEParallelIsolation(t *testing.T) {
	fixtures := tdmoeTestFixtures(t)
	var wait sync.WaitGroup
	errors := make(chan error, 48)
	for index := 0; index < 48; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			wire := fixtures[index%len(fixtures)]
			entry := "TDMoE"
			if index%2 != 0 {
				entry = "TDMoECarrier"
			}
			node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "tdmoe", entry)
			if err == nil {
				field := protocolCorpusFindNode(node, "Channel Count")
				if field == nil {
					errors <- io.ErrUnexpectedEOF
					return
				}
				value, resultErr := field.Result()
				var got uint64
				var typeOK bool
				switch number := value.Value.(type) {
				case uint16:
					got, typeOK = uint64(number), true
				case uint64:
					got, typeOK = number, true
				}
				if resultErr != nil || !typeOK || got != uint64([]int{1, 5, 2}[index%3]) {
					err = resultErr
					if err == nil {
						err = io.ErrUnexpectedEOF
					}
				}
			}
			errors <- err
		}(index)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}
