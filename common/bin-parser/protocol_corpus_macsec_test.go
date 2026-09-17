package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Literal field vectors are independent of the corpus generator. ICV bytes
// are deliberately just bytes: none of these assertions claim ICV validity.
func macsecTestFixtures(t *testing.T) [][]byte {
	t.Helper()
	var fixtures [][]byte
	for _, literal := range []string{
		"400601020304080001020304000102030405060708090a0b0c0d0e0f",
		"2f08ffffffff0200000000011234a0a1a2a3a4a5a6a7101112131415161718191a1b1c1d1e1f",
		"12000000000086dd000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d202122232425262728292a2b2c2d2e2f",
		"24040000002a020000000001000110203040303132333435363738393a3b3c3d3e3f01020304",
	} {
		wire, err := hex.DecodeString(literal)
		require.NoError(t, err)
		fixtures = append(fixtures, wire)
	}
	return fixtures
}

func macsecTestEthernet(payload []byte) []byte {
	return append([]byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 0x88, 0xe5}, payload...)
}

func macsecTestField(t *testing.T, root *base.Node, name string, value any, start, end uint64) *base.Node {
	t.Helper()
	protocolCorpusRequireValue(t, root, name, value)
	node := protocolCorpusFindNode(root, name)
	require.NotNil(t, node, name)
	require.Equal(t, [2]uint64{start, end}, stream_parser.GetNodeResultPos(node), name)
	result, err := node.Result()
	require.NoError(t, err)
	require.Same(t, node, result.Origin)
	return node
}

func macsecTestRoot(t *testing.T, root *base.Node) *base.Node {
	t.Helper()
	number := protocolCorpusFindNode(root, "Packet Number")
	require.NotNil(t, number)
	tag := number.Cfg.GetItem(base.CfgParent).(*base.Node)
	return tag.Cfg.GetItem(base.CfgParent).(*base.Node)
}

func macsecTestInfo(t *testing.T, node *base.Node, name string, expected any) {
	t.Helper()
	info, ok := node.Cfg.GetItem("additionInfo").(map[string]any)
	require.True(t, ok, node.Name)
	require.Equal(t, expected, info[name], name)
}

func macsecTestTree(t *testing.T, root *base.Node, wire []byte, offset int) {
	t.Helper()
	node := macsecTestRoot(t, root)
	position := uint64(offset * 8)
	var walk func(*base.Node)
	walk = func(current *base.Node) {
		if current.Cfg.Has(base.CfgNodeResult) {
			span := stream_parser.GetNodeResultPos(current)
			require.Equal(t, position, span[0], current.Name)
			require.Greater(t, span[1], span[0], current.Name)
			position = span[1]
			result, err := current.Result()
			require.NoError(t, err)
			require.Same(t, current, result.Origin)
			return
		}
		for _, child := range current.Children {
			require.Same(t, current, child.Cfg.GetItem(base.CfgParent), child.Name)
			require.Same(t, current.Ctx, child.Ctx)
			walk(child)
		}
	}
	walk(node)
	require.Equal(t, uint64((offset+len(wire))*8), position)
	require.Equal(t, wire, NodeToBytes(node)[offset:offset+len(wire)])
	_, err := node.Result()
	require.NoError(t, err)
	for _, name := range []string{"ICV Verified", "Data Decrypted", "Inner Protocol Decoded", "Implicit SCI Resolved", "Extended Packet Number Resolved"} {
		macsecTestInfo(t, node, name, false)
	}
}

func macsecTestFixtureFields(t *testing.T, node *base.Node, wire []byte, offset int) {
	t.Helper()
	start := uint64(offset * 8)
	for index, name := range []string{"Version", "End Station", "SCI Present", "Single Copy Broadcast", "Encrypted", "Changed Text"} {
		macsecTestField(t, node, name, uint64((wire[0]>>uint(7-index))&1), start+uint64(index), start+uint64(index+1))
	}
	macsecTestField(t, node, "Association Number", uint64(wire[0]&3), start+6, start+8)
	macsecTestField(t, node, "Reserved", uint64(0), start+8, start+10)
	macsecTestField(t, node, "Short Length", uint64(wire[1]), start+10, start+16)
	macsecTestField(t, node, "Packet Number", uint64(binary.BigEndian.Uint32(wire[2:6])), start+16, start+48)
	tagLength := 6
	if wire[0]&0x20 != 0 {
		macsecTestField(t, node, "System Identifier", wire[6:12], start+48, start+96)
		macsecTestField(t, node, "Port Identifier", uint64(binary.BigEndian.Uint16(wire[12:14])), start+96, start+112)
		tagLength = 14
	} else {
		require.Nil(t, protocolCorpusFindNode(node, "System Identifier"))
		require.Nil(t, protocolCorpusFindNode(node, "Port Identifier"))
	}
	dataLength := int(wire[1])
	if dataLength == 0 {
		dataLength = len(wire) - tagLength - 16
	}
	icvStart := tagLength + dataLength
	if wire[0]&0x0c == 0 {
		macsecTestField(t, node, "Inner EtherType", uint64(binary.BigEndian.Uint16(wire[tagLength:])), start+uint64(tagLength)*8, start+uint64(tagLength+2)*8)
		if dataLength > 2 {
			macsecTestField(t, node, "Clear Data", wire[tagLength+2:icvStart], start+uint64(tagLength+2)*8, start+uint64(icvStart)*8)
		}
		require.Nil(t, protocolCorpusFindNode(node, "Encrypted Data"))
		require.Nil(t, protocolCorpusFindNode(node, "Changed Data"))
	} else {
		name := "Changed Data"
		if wire[0]&8 != 0 {
			name = "Encrypted Data"
		}
		macsecTestField(t, node, name, wire[tagLength:icvStart], start+uint64(tagLength)*8, start+uint64(icvStart)*8)
		require.Nil(t, protocolCorpusFindNode(node, "Inner EtherType"))
		require.Nil(t, protocolCorpusFindNode(node, "Clear Data"))
	}
	macsecTestField(t, node, "ICV", wire[icvStart:icvStart+16], start+uint64(icvStart)*8, start+uint64(icvStart+16)*8)
	if icvStart+16 < len(wire) {
		macsecTestField(t, node, "MACSec Trailer", wire[icvStart+16:], start+uint64(icvStart+16)*8, start+uint64(len(wire))*8)
	} else {
		require.Nil(t, protocolCorpusFindNode(node, "MACSec Trailer"))
	}
	macsecTestInfo(t, macsecTestRoot(t, node), "Data Length", dataLength)
	macsecTestInfo(t, macsecTestRoot(t, node), "Zero Packet Number Needs XPN Context", binary.BigEndian.Uint32(wire[2:6]) == 0)
	macsecTestTree(t, node, wire, offset)
}

func TestProtocolCorpusMACSecFieldsAndPublicPaths(t *testing.T) {
	for index, wire := range macsecTestFixtures(t) {
		for _, path := range []struct {
			name, rule, entry string
			input             []byte
			offset            int
		}{
			{"direct", "macsec", "MACSec", wire, 0},
			{"carrier", "macsec", "MACSecCarrier", wire, 0},
			{"ethernet", "ethernet", "Ethernet", macsecTestEthernet(wire), 14},
		} {
			t.Run(fmt.Sprintf("%s/%d", path.name, index), func(t *testing.T) {
				node := protocolCorpusRequireBoundedRuleParse(t, path.input, path.rule, path.entry)
				macsecTestFixtureFields(t, node, wire, path.offset)
				require.Nil(t, protocolCorpusFindNode(node, "Unparsed MACSec Payload"))
			})
		}
	}
}

func TestProtocolCorpusMACSecCompanionEveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-macsec-valid.pcap")
	wires := macsecTestFixtures(t)
	require.Len(t, frames, len(wires))
	for index, frame := range frames {
		require.Equal(t, macsecTestEthernet(wires[index]), frame)
		for _, path := range []struct {
			rule, entry string
			input       []byte
			offset      int
		}{
			{"macsec", "MACSec", frame[14:], 0},
			{"macsec", "MACSecCarrier", frame[14:], 0},
			{"ethernet", "Ethernet", frame, 14},
		} {
			node := protocolCorpusRequireBoundedRuleParse(t, path.input, path.rule, path.entry)
			macsecTestFixtureFields(t, node, wires[index], path.offset)
		}
	}
}

func TestProtocolCorpusMACSecOriginalEveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-macsec.pcap")
	require.Len(t, frames, 1)
	expected := append([]byte{0, 0, 0, 0, 0, 1}, make([]byte, 32)...)
	require.Equal(t, macsecTestEthernet(expected), frames[0])
	for _, frame := range frames {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(frame[14:]), "macsec", "MACSec")
		require.ErrorContains(t, err, "macsec: zero short length requires at least 48 data bytes")
		for _, path := range []struct {
			rule, entry string
			input       []byte
			offset      uint64
		}{
			{"macsec", "MACSecCarrier", frame[14:], 0},
			{"ethernet", "Ethernet", frame, 14},
		} {
			node := protocolCorpusRequireBoundedRuleParse(t, path.input, path.rule, path.entry)
			macsecTestField(t, node, "Unparsed MACSec Payload", frame[14:], path.offset*8, (path.offset+38)*8)
			require.Nil(t, protocolCorpusFindNode(node, "Packet Number"))
		}
	}
}

func TestProtocolCorpusMACSecExternalFraming(t *testing.T) {
	fixtures := macsecTestFixtures(t)
	// A short tag identifies its trailer independently of arbitrary trailer bytes.
	short := fixtures[3]
	for _, extra := range [][]byte{nil, {0xff}, {1, 2, 3, 4}, bytes.Repeat([]byte{0xa5}, 128)} {
		wire := append(bytes.Clone(short[:len(short)-4]), extra...)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "macsec", "MACSec")
		macsecTestInfo(t, macsecTestRoot(t, node), "Trailer Length", len(extra))
		macsecTestTree(t, node, wire, 0)
	}
	for _, icvLength := range []int{8, 12, 13, 14, 15, 16} {
		wire := append(bytes.Clone(short[:18]), bytes.Repeat([]byte{byte(icvLength)}, icvLength)...)
		config := map[string]any{"macsecICVLength": icvLength, "macsecTrailerLength": 0}
		node := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, "macsec", "MACSec", config)
		macsecTestField(t, node, "ICV", wire[18:], 144, uint64(len(wire)*8))
		macsecTestInfo(t, macsecTestRoot(t, node), "ICV Length", icvLength)
		macsecTestInfo(t, macsecTestRoot(t, node), "ICV Length Explicit", true)
		macsecTestTree(t, node, wire, 0)
	}
	// Without SL, no algorithm can identify an unmarked capture trailer. An
	// explicit external length relocates the ICV; default means FCS-free input.
	long := append(bytes.Clone(fixtures[2]), 0xde, 0xad, 0xbe, 0xef)
	for _, entry := range []string{"MACSec", "MACSecCarrier"} {
		node := protocolCorpusRequireBoundedRuleParseWithConfig(t, long, "macsec", entry, map[string]any{"macsecTrailerLength": 4})
		macsecTestField(t, node, "MACSec Trailer", long[70:], 560, 592)
		macsecTestField(t, node, "ICV", long[54:70], 432, 560)
		macsecTestTree(t, node, long, 0)
	}
	defaultNode := protocolCorpusRequireBoundedRuleParse(t, long, "macsec", "MACSec")
	macsecTestInfo(t, macsecTestRoot(t, defaultNode), "Data Length", 52)
	macsecTestInfo(t, macsecTestRoot(t, defaultNode), "Trailer Length Explicit", false)
	require.Nil(t, protocolCorpusFindNode(defaultNode, "MACSec Trailer"))
	// One changed/opaque data byte can be bounded; clear data needs EtherType.
	one := append([]byte{4, 1, 0, 0, 0, 1, 0xaa}, make([]byte, 16)...)
	node := protocolCorpusRequireBoundedRuleParse(t, one, "macsec", "MACSec")
	macsecTestField(t, node, "Changed Data", []byte{0xaa}, 48, 56)
	one[0] = 0
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(one), "macsec", "MACSec")
	require.ErrorContains(t, err, "clear data is shorter than its EtherType")
}

func TestProtocolCorpusMACSecNegativeBoundaries(t *testing.T) {
	fixtures := macsecTestFixtures(t)
	for index, wire := range fixtures {
		minimum := len(wire)
		if index == 3 {
			minimum -= 4 // The opaque trailer is optional, not missing message data.
		}
		for cut := 0; cut < minimum; cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "macsec", "MACSec")
			require.Errorf(t, err, "fixture %d prefix %d", index, cut)
			if cut != 0 {
				node := protocolCorpusRequireBoundedRuleParse(t, wire[:cut], "macsec", "MACSecCarrier")
				protocolCorpusRequireValue(t, node, "Unparsed MACSec Payload", wire[:cut])
				require.Nil(t, protocolCorpusFindNode(node, "Packet Number"))
			}
		}
	}
	for _, tc := range []struct {
		index int
		value byte
		want  string
	}{
		{0, 0x80, "unsupported tag version"},
		{0, 0x60, "SCI cannot accompany ES or SCB"},
		{0, 0x30, "SCI cannot accompany ES or SCB"},
		{0, 0x08, "reserved E-without-C encoding"},
		{1, 0x46, "reserved length bits must be zero"},
		{1, 0x86, "reserved length bits must be zero"},
		{1, 48, "short length must be zero or 1 through 47"},
		{1, 63, "short length must be zero or 1 through 47"},
		{1, 7, "short length exceeds available data"},
		{1, 0, "zero short length requires at least 48 data bytes"},
	} {
		wire := bytes.Clone(fixtures[0])
		wire[tc.index] = tc.value
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "macsec", "MACSec")
		require.ErrorContains(t, err, tc.want)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "macsec", "MACSecCarrier")
		protocolCorpusRequireValue(t, node, "Unparsed MACSec Payload", wire)
		require.Nil(t, protocolCorpusFindNode(node, "Packet Number"))
	}
	for _, config := range []map[string]any{
		{"macsecICVLength": 0}, {"macsecICVLength": 4}, {"macsecICVLength": 9}, {"macsecICVLength": 17},
		{"macsecTrailerLength": -1}, {"macsecTrailerLength": 65536}, {"macsecTrailerLength": 0.5},
		{"macsecTrailerLength": 3}, {"macsecTrailerLength": 5},
	} {
		_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(fixtures[3]), "macsec", config, "MACSec")
		require.Error(t, err, config)
	}
	_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(fixtures[0]), "macsec", map[string]any{"macsecICVLength": 8}, "MACSec")
	require.ErrorContains(t, err, "nonstandard ICV length requires changed text")
	for _, entry := range []string{"MACSec", "MACSecCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(fixtures[0]), "macsec", entry)
		require.ErrorContains(t, err, "explicit")
	}
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(nil), "macsec", "MACSecCarrier")
	require.ErrorContains(t, err, "empty carrier has no message")
}

func TestProtocolCorpusMACSecShortPhysicalReader(t *testing.T) {
	valid := macsecTestFixtures(t)[1]
	for _, available := range []int{0, 1, 6, 13, len(valid) - 1} {
		root, err := base.ParseRule("macsec.yaml")
		require.NoError(t, err)
		root.Cfg.SetItem(base.CfgLength, uint64(len(valid))*8)
		bitReader := base.NewBitReader(bytes.NewReader(valid[:available]))
		err = root.ParseSubNode(bitReader, "MACSecCarrier")
		require.Error(t, err, available)
		carrier := base.GetNodeByPath(root, "@MACSecCarrier")
		require.NotNil(t, carrier)
		require.Nil(t, protocolCorpusFindNode(carrier, "Packet Number"), "failed trial must not retain tag fields")
		require.ErrorContains(t, bitReader.Recovery(), "no backup")
		require.ErrorContains(t, bitReader.PopBackup(), "no backup")
	}
}

func TestProtocolCorpusMACSecResourceAndCarrierTransactions(t *testing.T) {
	// A bounded large raw data value uses a constant number of field nodes.
	maximal := append([]byte{0x0c, 0, 0xff, 0xff, 0xff, 0xff}, bytes.Repeat([]byte{0x5a}, 65535-6)...)
	node := protocolCorpusRequireBoundedRuleParse(t, maximal, "macsec", "MACSec")
	macsecTestField(t, node, "Encrypted Data", maximal[6:65519], 48, 65519*8)
	macsecTestTree(t, node, maximal, 0)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(bytes.Clone(maximal), 0)), "macsec", "MACSec")
	require.ErrorContains(t, err, "exceeds the 65535-byte implementation boundary")
	valid := macsecTestFixtures(t)[1]
	bad := bytes.Clone(valid)
	bad[1] = 9 // Fail after parsing the complete optional SCI.
	for _, wire := range [][]byte{valid, bad, {1}} {
		root, err := base.ParseRule("macsec.yaml")
		require.NoError(t, err)
		root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
		reader := bytes.NewReader(wire)
		bitReader := base.NewBitReader(reader)
		require.NoError(t, root.ParseSubNode(bitReader, "MACSecCarrier"))
		carrier := base.GetNodeByPath(root, "@MACSecCarrier")
		require.Equal(t, wire, NodeToBytes(carrier))
		if bytes.Equal(wire, valid) {
			macsecTestFixtureFields(t, carrier, wire, 0)
		} else {
			protocolCorpusRequireValue(t, carrier, "Unparsed MACSec Payload", wire)
			require.Nil(t, protocolCorpusFindNode(carrier, "Packet Number"))
		}
		require.Zero(t, reader.Len())
		require.ErrorContains(t, bitReader.Recovery(), "no backup")
		require.ErrorContains(t, bitReader.PopBackup(), "no backup")
		_, err = bitReader.ReadBits(8)
		require.ErrorIs(t, err, io.EOF)
	}
}

func TestProtocolCorpusMACSecParallelIsolation(t *testing.T) {
	fixtures := macsecTestFixtures(t)
	var group sync.WaitGroup
	errors := make(chan error, 48)
	for i := 0; i < 48; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			wire := fixtures[i%len(fixtures)]
			entry := "MACSec"
			var config map[string]any
			if i%2 == 0 {
				entry = "MACSecCarrier"
				icvLength := []int{8, 12, 13, 14, 15, 16}[(i/2)%6]
				wire = append(bytes.Clone(fixtures[3][:18]), bytes.Repeat([]byte{byte(i)}, icvLength)...)
				config = map[string]any{"macsecICVLength": icvLength, "macsecTrailerLength": 0}
			}
			node, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), "macsec", config, entry)
			if err == nil {
				field := protocolCorpusFindNode(node, "Packet Number")
				if field == nil {
					err = fmt.Errorf("missing packet number in iteration %d", i)
				} else if value, resultErr := field.Result(); resultErr != nil {
					err = resultErr
				} else if fmt.Sprint(value.Value) != fmt.Sprint(binary.BigEndian.Uint32(wire[2:6])) {
					err = fmt.Errorf("packet number crossed records in iteration %d", i)
				}
				if err == nil && !bytes.Equal(wire, NodeToBytes(node)) {
					err = fmt.Errorf("bytes crossed records in iteration %d", i)
				}
			}
			errors <- err
		}(i)
	}
	group.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}
