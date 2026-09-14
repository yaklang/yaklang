package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const smb3TestRule = "application-layer/smb3"

type smb3TestField struct {
	name  string
	value any
}

func smb3TestEncode(fields []smb3TestField) []byte {
	var b bytes.Buffer
	for _, f := range fields {
		if err := binary.Write(&b, binary.LittleEndian, f.value); err != nil {
			panic(err)
		}
	}
	return b.Bytes()
}

type smb3TestContext struct {
	kind   uint16
	fields []smb3TestField
}
type smb3TestFixture struct {
	name     string
	response bool
	dialects []uint16
	contexts []smb3TestContext
	token    []byte
}

func smb3TestContexts(reply bool) []smb3TestContext {
	pre := []smb3TestField{{"HashAlgorithmCount", uint16(1)}, {"SaltLength", uint16(3)}, {"HashAlgorithm", uint16(1)}, {"Salt", []byte{0x31, 0x41, 0x59}}}
	enc := []smb3TestField{{"CipherCount", uint16(2)}, {"Cipher", uint16(2)}, {"Cipher", uint16(1)}}
	sign := []smb3TestField{{"SigningAlgorithmCount", uint16(2)}, {"SigningAlgorithm", uint16(2)}, {"SigningAlgorithm", uint16(1)}}
	if reply {
		enc = []smb3TestField{{"CipherCount", uint16(1)}, {"Cipher", uint16(2)}}
		sign = []smb3TestField{{"SigningAlgorithmCount", uint16(1)}, {"SigningAlgorithm", uint16(2)}}
	}
	contexts := []smb3TestContext{
		{2, enc}, // Deliberately precedes PREAUTH: no prescribed context order.
		{1, pre},
		{3, []smb3TestField{{"CompressionAlgorithmCount", uint16(2)}, {"Compression Padding", uint16(0)}, {"Compression Flags", uint32(1)}, {"CompressionAlgorithm", uint16(1)}, {"CompressionAlgorithm", uint16(3)}}},
		{6, []smb3TestField{{"Transport Flags", uint32(0)}}},
		{7, []smb3TestField{{"TransformCount", uint16(2)}, {"Algorithm Reserved1", uint16(0)}, {"Algorithm Reserved2", uint32(0)}, {"RDMATransformId", uint16(1)}, {"RDMATransformId", uint16(2)}}},
		{8, sign},
	}
	if !reply {
		contexts = append(contexts, smb3TestContext{5, []smb3TestField{{"Code Unit", uint16('n')}, {"Code Unit", uint16('s')}}})
	}
	return contexts
}
func smb3TestFixtures() []smb3TestFixture {
	return []smb3TestFixture{
		{name: "request-3.0", dialects: []uint16{0x0202, 0x0300}},
		{name: "request-3.0.2", dialects: []uint16{0x0300, 0x0302}},
		{name: "request-3.1.1-contexts", dialects: []uint16{0x0300, 0x0302, 0x0311}, contexts: smb3TestContexts(false)},
		{name: "response-3.0", response: true, dialects: []uint16{0x0300}},
		{name: "response-3.0.2", response: true, dialects: []uint16{0x0302}},
		{name: "response-3.1.1-contexts", response: true, dialects: []uint16{0x0311}, contexts: smb3TestContexts(true)},
		{name: "request-3.1.1-minimal", dialects: []uint16{0x0311}, contexts: []smb3TestContext{smb3TestContexts(false)[1]}},
	}
}
func (f smb3TestFixture) fields() []smb3TestField {
	fields := []smb3TestField{{"ProtocolId", uint32(0x424d53fe)}, {"Header StructureSize", uint16(64)}, {"CreditCharge", uint16(0)}}
	if f.response {
		fields = append(fields, smb3TestField{"Status", uint32(0)})
	} else {
		fields = append(fields, smb3TestField{"ChannelSequence", uint16(0)}, smb3TestField{"Header Reserved", uint16(0)})
	}
	fields = append(fields, smb3TestField{"Command", uint16(0)})
	flags := uint32(0)
	creditName := "CreditRequest"
	if f.response {
		flags = 1
		creditName = "CreditResponse"
	}
	fields = append(fields, []smb3TestField{{creditName, uint16(1)}, {"Flags", flags}, {"NextCommand", uint32(0)}, {"MessageId", uint64(0)}, {"Sync Reserved", uint32(0)}, {"TreeId", uint32(0)}, {"SessionId", uint64(0)}, {"Signature", make([]byte, 16)}}...)
	guid := []byte{0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15}
	v311 := false
	for _, d := range f.dialects {
		v311 = v311 || d == 0x0311
	}
	if f.response {
		countName, offsetName := "Reserved", "Reserved2"
		if v311 {
			countName = "NegotiateContextCount"
			offsetName = "NegotiateContextOffset"
		}
		offset := uint32(0)
		if v311 {
			offset = uint32((128 + len(f.token) + 7) &^ 7)
		}
		fields = append(fields, []smb3TestField{{"StructureSize", uint16(65)}, {"SecurityMode", uint16(1)}, {"DialectRevision", f.dialects[0]}, {countName, uint16(len(f.contexts))}, {"ServerGuid", guid}, {"Capabilities", uint32(0x3f)}, {"MaxTransactSize", uint32(65536)}, {"MaxReadSize", uint32(131072)}, {"MaxWriteSize", uint32(262144)}, {"SystemTime", uint64(0x0102030405060708)}, {"ServerStartTime", uint64(0)}, {"SecurityBufferOffset", uint16(128)}, {"SecurityBufferLength", uint16(len(f.token))}, {offsetName, offset}}...)
		if len(f.token) > 0 {
			fields = append(fields, smb3TestField{"Security Buffer", f.token})
		}
	} else {
		fields = append(fields, []smb3TestField{{"StructureSize", uint16(36)}, {"DialectCount", uint16(len(f.dialects))}, {"SecurityMode", uint16(1)}, {"Reserved", uint16(0)}, {"Capabilities", uint32(0x7f)}, {"ClientGuid", guid}}...)
		if v311 {
			fields = append(fields, smb3TestField{"NegotiateContextOffset", uint32((100 + len(f.dialects)*2 + 7) &^ 7)}, smb3TestField{"NegotiateContextCount", uint16(len(f.contexts))}, smb3TestField{"Reserved2", uint16(0)})
		} else {
			fields = append(fields, smb3TestField{"ClientStartTime Reserved", make([]byte, 8)})
		}
		for _, d := range f.dialects {
			fields = append(fields, smb3TestField{"Dialect", d})
		}
	}
	if len(f.contexts) > 0 {
		pos := len(smb3TestEncode(fields))
		if pos%8 != 0 {
			fields = append(fields, smb3TestField{"Context Padding", make([]byte, 8-pos%8)})
		}
		for i, c := range f.contexts {
			pos = len(smb3TestEncode(fields))
			if i > 0 && pos%8 != 0 {
				fields = append(fields, smb3TestField{"Context Alignment Padding", make([]byte, 8-pos%8)})
			}
			fields = append(fields, smb3TestField{"ContextType", c.kind}, smb3TestField{"DataLength", uint16(len(smb3TestEncode(c.fields)))}, smb3TestField{"Context Reserved", uint32(0)})
			fields = append(fields, c.fields...)
		}
	}
	return fields
}
func (f smb3TestFixture) wire() []byte { return smb3TestEncode(f.fields()) }

func smb3TestFields(t *testing.T, node *base.Node, fields []smb3TestField, offset uint64) {
	t.Helper()
	_, err := node.Result()
	require.NoError(t, err)
	first := protocolCorpusFindNode(node, "ProtocolId")
	require.NotNil(t, first)
	node = first.Cfg.GetItem(base.CfgParent).(*base.Node)
	var leaves []*base.Node
	var walk func(*base.Node)
	walk = func(parent *base.Node) {
		for _, child := range parent.Children {
			require.Same(t, parent, child.Cfg.GetItem(base.CfgParent))
			require.Same(t, parent.Ctx, child.Ctx)
			if child.Cfg.GetBool(base.CfgIsTerminal) && stream_parser.NodeHasResult(child) {
				leaves = append(leaves, child)
			}
			walk(child)
		}
	}
	walk(node)
	require.Len(t, leaves, len(fields))
	pos := offset
	for i, f := range fields {
		leaf := leaves[i]
		require.Equal(t, f.name, leaf.Name)
		value, err := leaf.Result()
		require.NoError(t, err)
		require.Same(t, leaf, value.Origin)
		require.Equal(t, f.value, value.Value, f.name)
		typ := map[string]string{"uint16": "uint16", "uint32": "uint32", "uint64": "uint64", "[]uint8": "raw"}[fmt.Sprintf("%T", f.value)]
		require.Equal(t, typ, leaf.Cfg.GetItem(base.CfgType))
		n := uint64(len(smb3TestEncode([]smb3TestField{f}))) * 8
		require.Equal(t, [2]uint64{pos, pos + n}, stream_parser.GetNodeResultPos(leaf))
		pos += n
	}
	reader := base.NewBitReader(bytes.NewReader(NodeToBytes(node)))
	if offset > 0 {
		_, err = reader.ReadBits(offset)
		require.NoError(t, err)
	}
	actual, err := reader.ReadBits(pos - offset)
	require.NoError(t, err)
	require.Equal(t, smb3TestEncode(fields), actual)
}
func smb3TestInfo(t *testing.T, node *base.Node, f smb3TestFixture) {
	first := protocolCorpusFindNode(node, "ProtocolId")
	require.NotNil(t, first)
	info := alljoynTestInfo(t, first.Cfg.GetItem(base.CfgParent).(*base.Node))
	require.Equal(t, "SMB 3.x single NEGOTIATE structural fields", info["Profile"])
	require.EqualValues(t, 65536, info["Maximum Record Bytes"])
	require.Equal(t, f.response, info["Response"])
	require.Equal(t, len(f.contexts) > 0, info["SMB 3.1.1 Contexts"])
	for _, key := range []string{"Session State Validated", "Algorithm Negotiation Validated", "Signature Verified", "Security Buffer Decoded", "Transform Payload Decoded", "Transport Reassembled"} {
		require.Equal(t, false, info[key], key)
	}
}
func TestProtocolCorpusSMB3Fields(t *testing.T) {
	for _, f := range smb3TestFixtures() {
		t.Run(f.name, func(t *testing.T) {
			t.Logf("companion %s %d %x", f.name, len(f.wire()), f.wire())
			for _, entry := range []string{"SMB3Negotiate", "SMB3NegotiateCarrier"} {
				node := protocolCorpusRequireBoundedRuleParse(t, f.wire(), smb3TestRule, entry)
				smb3TestFields(t, node, f.fields(), 0)
				smb3TestInfo(t, node, f)
				require.Equal(t, f.wire(), NodeToBytes(node))
			}
		})
	}
}

func smb3TestReject(t *testing.T, wire []byte, cause string) {
	t.Helper()
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), smb3TestRule, "SMB3Negotiate")
	require.Error(t, err)
	if cause != "" {
		require.ErrorContains(t, err, cause)
	}
	if len(wire) == 0 {
		return
	}
	node := protocolCorpusRequireBoundedRuleParse(t, wire, smb3TestRule, "SMB3NegotiateCarrier")
	miopTestField(t, node, "Unparsed SMB3 Negotiate", wire, 0, uint64(len(wire))*8)
	require.Nil(t, protocolCorpusFindNode(node, "ProtocolId"))
	require.Equal(t, wire, NodeToBytes(node))
}
func TestProtocolCorpusSMB3OriginalEveryRecord(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-smb3.pcap"
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "e11c4d69779ca9e3748284165ac2ec98064f3283b0ee460e9f73fde5f446d025", fmt.Sprintf("%x", sha256.Sum256(raw)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 4)
	for i, frame := range frames {
		node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		require.Equal(t, frame, NodeToBytes(node))
		if i < 3 {
			require.Len(t, frame, 54)
			continue
		}
		require.Len(t, frame, 164)
		require.Equal(t, []byte{0, 0, 0, 106}, frame[54:58])
		wire := frame[58:]
		require.Len(t, wire, 106)
		require.Equal(t, []byte{0, 3, 2, 3, 0x11, 3}, wire[100:])
		require.Equal(t, make([]byte, 8), wire[92:100])
		smb3TestReject(t, wire, "requires exactly one PREAUTH")
	}
	// These two existing SMB2-named captures also offer SMB 3.0. Their entire
	// record sets are checked, rather than inferring scope from the filename.
	for _, old := range []struct{ path, sha string }{
		{"generated-local/gen-smb2.pcap", "aff6cb5eada34206758cc3418478d510b6c287c13df1b3e086100fc4a4ac583f"},
		{"generated-pr5023/pr5023-gen-smb2.pcap", "67bd5f1e96cccd4f3a0fdded0b749c5dbbff5599cdd1858e51bbaeb645bf41a1"},
	} {
		path := "testdata/protocol-corpus/captures/" + old.path
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, old.sha, fmt.Sprintf("%x", sha256.Sum256(raw)))
		frames := protocolCorpusAuditPackets(t, path)
		require.Len(t, frames, 4)
		f := smb3TestFixture{dialects: []uint16{0x0202, 0x0210, 0x0300}}
		fields := f.fields()
		for i, field := range fields {
			switch field.name {
			case "CreditRequest", "SecurityMode":
				fields[i].value = uint16(0)
			case "Capabilities":
				fields[i].value = uint32(0)
			case "ClientGuid":
				fields[i].value = make([]byte, 16)
			}
		}
		for i, frame := range frames {
			node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			require.Equal(t, frame, NodeToBytes(node))
			if i < 3 {
				require.Len(t, frame, 54)
				continue
			}
			require.Len(t, frame, 164)
			require.Equal(t, []byte{0, 0, 0, 106}, frame[54:58])
			require.Equal(t, smb3TestEncode(fields), frame[58:])
			for _, entry := range []string{"SMB3Negotiate", "SMB3NegotiateCarrier"} {
				node = protocolCorpusRequireBoundedRuleParse(t, frame[58:], smb3TestRule, entry)
				smb3TestFields(t, node, fields, 0)
			}
		}
	}
}

func TestProtocolCorpusSMB3PrefixesAndBoundaries(t *testing.T) {
	for _, f := range smb3TestFixtures() {
		for cut := 0; cut < len(f.wire()); cut++ {
			smb3TestReject(t, f.wire()[:cut], "")
		}
	}
	f := smb3TestFixtures()[6]
	for _, m := range []struct {
		at    int
		bytes []byte
		cause string
	}{{0, []byte{0xfd}, "ProtocolId"}, {4, []byte{63}, "header StructureSize"}, {12, []byte{1}, "only NEGOTIATE"}, {16, []byte{2}, "asynchronous"}, {16, []byte{4}, "related compound"}, {20, []byte{64}, "compound message"}, {40, []byte{1}, "SessionId"}, {64, []byte{35}, "request StructureSize"}, {66, []byte{0, 0}, "DialectCount"}, {66, []byte{0xff, 0xff}, "DialectCount"}, {92, []byte{105}, "8-byte aligned"}, {92, []byte{96}, "overlapping"}, {92, []byte{0xf8, 0xff, 0xff, 0xff}, "offset"}, {96, []byte{0, 0}, "PREAUTH"}, {96, []byte{2, 0}, "Context Alignment Padding offset"}, {106, []byte{0xff, 0xff}, "DataLength"}, {112, []byte{0, 0}, "HashAlgorithmCount"}, {114, []byte{0xff, 0xff}, "Salt"}} {
		wire := f.wire()
		copy(wire[m.at:], m.bytes)
		smb3TestReject(t, wire, m.cause)
	}
	for _, kind := range []uint16{1, 2, 3, 7, 8} {
		f := smb3TestFixtures()[2]
		var duplicate smb3TestContext
		for _, c := range f.contexts {
			if c.kind == kind {
				duplicate = c
			}
		}
		f.contexts = append(f.contexts, duplicate)
		smb3TestReject(t, f.wire(), "duplicate context")
	}
	for _, reply := range []bool{false, true} {
		for _, kind := range []uint16{1, 2, 3, 7, 8} {
			f := smb3TestFixtures()[2]
			if reply {
				f = smb3TestFixtures()[5]
			}
			for i, c := range f.contexts {
				if c.kind == kind {
					f.contexts[i].fields = append([]smb3TestField(nil), c.fields...)
					f.contexts[i].fields[0].value = uint16(0)
				}
			}
			smb3TestReject(t, f.wire(), "invalid")
		}
	}
	for _, kind := range []uint16{1, 2, 8} {
		f := smb3TestFixtures()[5]
		for i, c := range f.contexts {
			if c.kind == kind {
				f.contexts[i].fields = append([]smb3TestField(nil), c.fields...)
				f.contexts[i].fields[0].value = uint16(2)
			}
		}
		smb3TestReject(t, f.wire(), "invalid")
	}
	for _, dialect := range []uint16{0x0202, 0x0210, 0x02ff, 0xffff} {
		f := smb3TestFixture{dialects: []uint16{dialect}}
		smb3TestReject(t, f.wire(), "no SMB")
	}
}

func TestProtocolCorpusSMB3CompanionEveryRecord(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-validated/gen-smb3-valid.pcap"
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "937bf9022105992ced1a8192fa9f09975880ff2a6afe6cab0fbe9b28d3fbed1f", fmt.Sprintf("%x", sha256.Sum256(raw)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 7)
	for i, frame := range frames {
		f := smb3TestFixtures()[i]
		wire := f.wire()
		require.Len(t, frame, 58+len(wire))
		require.Equal(t, byte(6), frame[23])
		require.EqualValues(t, len(wire), binary.BigEndian.Uint32(frame[54:58]))
		source, dest := uint16(40100), uint16(445)
		if f.response {
			source, dest = dest, source
		}
		require.Equal(t, source, binary.BigEndian.Uint16(frame[34:]))
		require.Equal(t, dest, binary.BigEndian.Uint16(frame[36:]))
		require.Equal(t, wire, frame[58:])
		node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		require.Equal(t, frame, NodeToBytes(node))
		for _, entry := range []string{"SMB3Negotiate", "SMB3NegotiateCarrier"} {
			node = protocolCorpusRequireBoundedRuleParse(t, wire, smb3TestRule, entry)
			smb3TestFields(t, node, f.fields(), 0)
			smb3TestInfo(t, node, f)
			// The production SMB2 dispatcher remains backward compatible. Use
			// an explicit strict import to audit frame-global native spans.
			root := fcoeTestInline(t, fmt.Sprintf("Package:\n  Envelope:\n    Prefix: raw,58\n    Record: \"import:application-layer/smb3.yaml;node:%s\"\n", entry))
			root.Cfg.SetItem(base.CfgLength, uint64(len(frame))*8)
			require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(frame)), "Envelope"))
			node = base.GetNodeByPath(root, "@Envelope")
			smb3TestFields(t, node, f.fields(), 58*8)
			require.Equal(t, frame, NodeToBytes(node))
		}
	}
}

func TestProtocolCorpusSMB3CompatibilityAndResources(t *testing.T) {
	f := smb3TestFixtures()[2]
	f.contexts = append(f.contexts, smb3TestContext{0x100, []smb3TestField{{"Uninterpreted Context Data", []byte{1, 2, 3}}}}, smb3TestContext{0xffee, []smb3TestField{{"Uninterpreted Context Data", []byte{0xff}}}})
	fields := f.fields()
	fields = append(fields, smb3TestField{"Uninterpreted Record Tail", []byte{0xa5, 0x5a}})
	wire := smb3TestEncode(fields)
	node := protocolCorpusRequireBoundedRuleParse(t, wire, smb3TestRule, "SMB3Negotiate")
	smb3TestFields(t, node, fields, 0)
	first := protocolCorpusFindNode(node, "ProtocolId")
	require.EqualValues(t, 6, alljoynTestInfo(t, first.Cfg.GetItem(base.CfgParent).(*base.Node))["Uninterpreted Bytes"])
	// Reserved values are received without normalization. Algorithm support
	// cannot be inferred without both endpoints and is not a parser rejection.
	f = smb3TestFixtures()[6]
	fields = f.fields()
	for i, x := range fields {
		switch x.name {
		case "Header Reserved", "Reserved", "Reserved2":
			fields[i].value = uint16(0xffff)
		case "Sync Reserved", "Context Reserved":
			fields[i].value = uint32(0xffffffff)
		case "HashAlgorithm":
			fields[i].value = uint16(0xffff)
		}
	}
	wire = smb3TestEncode(fields)
	node = protocolCorpusRequireBoundedRuleParse(t, wire, smb3TestRule, "SMB3Negotiate")
	smb3TestFields(t, node, fields, 0)
	// Maximum byte profile with few fields: not a performance benchmark.
	f = smb3TestFixtures()[0]
	fields = f.fields()
	fields = append(fields, smb3TestField{"Uninterpreted Record Tail", bytes.Repeat([]byte{0x5a}, 65536-len(f.wire()))})
	wire = smb3TestEncode(fields)
	node = protocolCorpusRequireBoundedRuleParse(t, wire, smb3TestRule, "SMB3Negotiate")
	smb3TestFields(t, node, fields, 0)
	reader := newProtocolCorpusBoundedReader(append(wire, 0))
	_, err := parser.ParseBinary(reader, smb3TestRule, "SMB3Negotiate")
	require.ErrorContains(t, err, "100..65536")
	require.Equal(t, 65537, reader.Len())
	for _, entry := range []string{"SMB3Negotiate", "SMB3NegotiateCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(f.wire()), smb3TestRule, entry)
		require.Error(t, err)
		_, err = parser.GenerateBinary(map[string]any{}, smb3TestRule, entry)
		require.Error(t, err)
		for bits := uint64(1); bits < 8; bits++ {
			reader := &alljoynTestBitBoundaryReader{bytes.NewReader(f.wire()), uint64(len(f.wire()))*8 - bits}
			_, err := parser.ParseBinary(reader, smb3TestRule, entry)
			require.ErrorContains(t, err, "byte")
			require.Equal(t, len(f.wire()), reader.Len())
		}
	}
}

func TestProtocolCorpusSMB3ContextReceiveRules(t *testing.T) {
	// Salt is explicitly optional, and ignored contexts may have no data.
	zero := smb3TestFixtures()[6]
	zero.contexts[0].fields[1].value = uint16(0)
	zero.contexts[0].fields[3].value = []byte{}
	zero.contexts = append(zero.contexts, smb3TestContext{0x100, []smb3TestField{{"Uninterpreted Context Data", []byte{}}}})
	zeroNode := protocolCorpusRequireBoundedRuleParse(t, zero.wire(), smb3TestRule, "SMB3Negotiate")
	smb3TestFields(t, zeroNode, zero.fields(), 0)
	// Request algorithm lists retain repeats/unknown IDs. The response has
	// additional stateless compression restrictions in MS-SMB2 3.2.5.2.
	f := smb3TestFixtures()[2]
	f.contexts = append(f.contexts, smb3TestContext{6, []smb3TestField{{"Transport Flags", uint32(0)}}})
	node := protocolCorpusRequireBoundedRuleParse(t, f.wire(), smb3TestRule, "SMB3Negotiate")
	smb3TestFields(t, node, f.fields(), 0)
	f = smb3TestFixtures()[5]
	f.contexts = append(f.contexts, smb3TestContext{6, []smb3TestField{{"Transport Flags", uint32(0)}}})
	smb3TestReject(t, f.wire(), "duplicate context")
	for _, second := range []uint16{1, 32} {
		for _, reply := range []bool{false, true} {
			f = smb3TestFixtures()[2]
			if reply {
				f = smb3TestFixtures()[5]
			}
			f.contexts[2].fields[4].value = second
			if reply {
				smb3TestReject(t, f.wire(), "response compression")
			} else {
				node = protocolCorpusRequireBoundedRuleParse(t, f.wire(), smb3TestRule, "SMB3Negotiate")
				smb3TestFields(t, node, f.fields(), 0)
			}
		}
	}
	f = smb3TestFixtures()[6]
	f.contexts[0].fields = append(f.contexts[0].fields, smb3TestField{"Uninterpreted Context Tail", []byte{0xa5}})
	f.contexts = append(f.contexts, smb3TestContext{5, []smb3TestField{{"Uninterpreted NetName", []byte{0xff}}}})
	node = protocolCorpusRequireBoundedRuleParse(t, f.wire(), smb3TestRule, "SMB3Negotiate")
	smb3TestFields(t, node, f.fields(), 0)
	// A supplied GSS token is structurally located by offset, but is opaque.
	f = smb3TestFixtures()[5]
	f.token = []byte{0x60, 1, 0}
	node = protocolCorpusRequireBoundedRuleParse(t, f.wire(), smb3TestRule, "SMB3Negotiate")
	smb3TestFields(t, node, f.fields(), 0)
	for _, m := range []struct {
		at    int
		data  []byte
		cause string
	}{{8, []byte{1}, "non-success"}, {64, []byte{64}, "response StructureSize"}, {120, []byte{127}, "overlapping"}, {120, []byte{0xff, 0xff}, "offset"}, {122, []byte{0xff, 0xff}, "truncated Security Buffer"}, {124, []byte{129}, "8-byte aligned"}, {124, []byte{128}, "overlapping"}} {
		wire := f.wire()
		copy(wire[m.at:], m.data)
		smb3TestReject(t, wire, m.cause)
	}
	// Reserved response context-union bytes are not active before 3.1.1.
	f = smb3TestFixtures()[3]
	fields := f.fields()
	for i, x := range fields {
		if x.name == "Reserved" {
			fields[i].value = uint16(0xffff)
		}
		if x.name == "Reserved2" {
			fields[i].value = uint32(0xffffffff)
		}
	}
	wire := smb3TestEncode(fields)
	node = protocolCorpusRequireBoundedRuleParse(t, wire, smb3TestRule, "SMB3Negotiate")
	smb3TestFields(t, node, fields, 0)
}

func TestProtocolCorpusSMB3CountResourcesAndHeldSuccess(t *testing.T) {
	f := smb3TestFixtures()[6]
	for i := 0; i < 255; i++ {
		f.contexts = append(f.contexts, smb3TestContext{0x100, []smb3TestField{{"Uninterpreted Context Data", []byte{byte(i)}}}})
	}
	node := protocolCorpusRequireBoundedRuleParse(t, f.wire(), smb3TestRule, "SMB3Negotiate")
	smb3TestFields(t, node, f.fields(), 0)
	list := protocolCorpusFindNode(node, "Negotiate Contexts")
	require.NotNil(t, list)
	require.Len(t, list.Children, 256)
	f = smb3TestFixtures()[0]
	f.dialects = make([]uint16, 1024)
	for i := range f.dialects {
		f.dialects[i] = 0x0300
	}
	node = protocolCorpusRequireBoundedRuleParse(t, f.wire(), smb3TestRule, "SMB3Negotiate")
	smb3TestFields(t, node, f.fields(), 0)
	f = smb3TestFixtures()[6]
	for _, valid := range []bool{true, false} {
		for _, rollback := range []bool{true, false} {
			wire := f.wire()
			if !valid {
				wire = wire[:len(wire)-1]
			}
			input := append(append([]byte{0xa5}, wire...), 0x5a)
			reader := base.NewBitReader(bytes.NewReader(input))
			_, err := reader.ReadBits(8)
			require.NoError(t, err)
			require.NoError(t, reader.Backup())
			root, err := base.ParseRule(smb3TestRule + ".yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			require.NoError(t, root.ParseSubNode(reader, "SMB3NegotiateCarrier"))
			node := base.GetNodeByPath(root, "@SMB3NegotiateCarrier")
			require.Equal(t, wire, NodeToBytes(node))
			if valid {
				smb3TestFields(t, node, f.fields(), 0)
			} else {
				require.Nil(t, protocolCorpusFindNode(node, "ProtocolId"))
				protocolCorpusRequireValue(t, node, "Unparsed SMB3 Negotiate", wire)
			}
			if rollback {
				require.NoError(t, reader.Recovery())
				actual, err := reader.ReadBits(uint64(len(wire)) * 8)
				require.NoError(t, err)
				require.Equal(t, wire, actual)
			} else {
				require.NoError(t, reader.PopBackup())
			}
			tail, err := reader.ReadBits(8)
			require.NoError(t, err)
			require.Equal(t, []byte{0x5a}, tail)
			require.ErrorContains(t, reader.Recovery(), "no backup")
		}
	}
}

func TestProtocolCorpusSMB3ImportsAndHeldReader(t *testing.T) {
	f := smb3TestFixtures()[6]
	for offset := 0; offset < 8; offset++ {
		for _, test := range []struct {
			entry string
			valid bool
		}{{"SMB3NegotiateCarrier", true}, {"SMB3NegotiateCarrier", false}, {"SMB3Negotiate", true}} {
			valid := test.valid
			wire := f.wire()
			if !valid {
				wire = wire[:len(wire)-1]
			}
			prefix, padding := "", ""
			if offset > 0 {
				prefix = fmt.Sprintf("    Prefix: uint8,%dbit\n", offset)
				padding = fmt.Sprintf("    Padding: uint8,%dbit\n", 8-offset)
			}
			root := fcoeTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Record").SetMaxLength(%d)
      this.ProcessSubNode("Record")
      this.ProcessSubNode("Suffix")
      if %d > 0 { this.ProcessSubNode("Padding") }
%s    Record: "import:application-layer/smb3.yaml;node:%s"
    Suffix: uint8
%s`, offset, len(wire), offset, prefix, test.entry, padding))
			parts := []snaTestField{}
			if offset > 0 {
				parts = append(parts, snaBit("Prefix", uint8((1<<offset)-1), uint64(offset)))
			}
			parts = append(parts, snaRaw("Record", wire), snaU8("Suffix", 0x5a))
			if offset > 0 {
				parts = append(parts, snaBit("Padding", 0, uint64(8-offset)))
			}
			input := snaEncode(parts)
			root.Cfg.SetItem(base.CfgLength, uint64(len(input))*8)
			reader := base.NewBitReader(bytes.NewReader(input))
			require.NoError(t, reader.Backup())
			require.NoError(t, root.ParseSubNode(reader, "Envelope"))
			node := base.GetNodeByPath(root, "@Envelope")
			record := protocolCorpusFindNode(node, "Record")
			if valid {
				smb3TestFields(t, record, f.fields(), uint64(offset))
			} else {
				miopTestField(t, record, "Unparsed SMB3 Negotiate", wire, uint64(offset), uint64(offset+len(wire)*8))
				require.Nil(t, protocolCorpusFindNode(record, "ProtocolId"))
			}
			require.Equal(t, input, NodeToBytes(node))
			miopTestField(t, node, "Suffix", uint8(0x5a), uint64(offset+len(wire)*8), uint64(offset+(len(wire)+1)*8))
			require.NoError(t, reader.Recovery())
			replay, err := reader.ReadBits(uint64(len(input)) * 8)
			require.NoError(t, err)
			require.Equal(t, input, replay)
		}
	}
	for cut := 0; cut < len(f.wire()); cut++ {
		for _, entry := range []string{"SMB3Negotiate", "SMB3NegotiateCarrier"} {
			root, err := base.ParseRule(smb3TestRule + ".yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(f.wire()))*8)
			reader := base.NewBitReader(bytes.NewReader(f.wire()[:cut]))
			require.NoError(t, reader.Backup())
			require.Error(t, root.ParseSubNode(reader, entry))
			require.NoError(t, reader.Recovery())
			if cut > 0 {
				replay, err := reader.ReadBits(uint64(cut) * 8)
				require.NoError(t, err)
				require.Equal(t, f.wire()[:cut], replay)
			}
			_, err = reader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		}
	}
}

func TestProtocolCorpusSMB3ContextIsolation(t *testing.T) {
	var wait sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			f := smb3TestFixtures()[i%7]
			node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(f.wire()), smb3TestRule, "SMB3NegotiateCarrier")
			if err == nil && !bytes.Equal(NodeToBytes(node), f.wire()) {
				err = fmt.Errorf("context bytes differ")
			}
			if err == nil {
				_, err = node.Result()
			}
			errs <- err
		}(i)
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}
