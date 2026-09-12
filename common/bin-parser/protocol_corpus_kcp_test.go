package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const kcpTestRule = "application-layer.kcp"

// Independent literal layouts, not generated through the decoder. ACK's frg
// and WINS's data intentionally exercise accepted input, not sender defaults.
func kcpTestVectors(t *testing.T) [][]byte {
	t.Helper()
	var vectors [][]byte
	for _, literal := range []string{
		"010203045102341244332211f0ffffffd0c0b0a003000000616263",
		"0102030452ff0000ffffffffffffffff0000000000000000",
		"010203045380ffff0000000000000000ffffffff00000000",
		"01020304540001000403020100000000000000000200000000ff",
		"000000005100000000000000000000000000000000000000",
	} {
		wire, err := hex.DecodeString(literal)
		require.NoError(t, err)
		vectors = append(vectors, wire)
	}
	return vectors
}

func kcpTestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	return protocolCorpusRequireBoundedRuleParse(t, wire, kcpTestRule, entry)
}

func kcpTestMessage(t *testing.T, n *base.Node) *base.Node {
	t.Helper()
	segment := protocolCorpusFindNode(n, "Segment 0")
	require.NotNil(t, segment)
	return segment.Cfg.GetItem(base.CfgParent).(*base.Node)
}

func kcpTestSegment(t *testing.T, segment *base.Node, wire []byte, offset uint64) {
	t.Helper()
	require.NotNil(t, segment)
	fields := []struct {
		name, typ  string
		start, end int
		value      any
	}{
		{"Conversation ID", "uint32", 0, 4, uint64(binary.LittleEndian.Uint32(wire))},
		{"Command", "uint8", 4, 5, uint64(wire[4])},
		{"Fragment", "uint8", 5, 6, uint64(wire[5])},
		{"Window", "uint16", 6, 8, uint64(binary.LittleEndian.Uint16(wire[6:]))},
		{"Timestamp", "uint32", 8, 12, uint64(binary.LittleEndian.Uint32(wire[8:]))},
		{"Sequence Number", "uint32", 12, 16, uint64(binary.LittleEndian.Uint32(wire[12:]))},
		{"Unacknowledged Sequence", "uint32", 16, 20, uint64(binary.LittleEndian.Uint32(wire[16:]))},
		{"Data Length", "uint32", 20, 24, uint64(binary.LittleEndian.Uint32(wire[20:]))},
		{"Segment Data", "raw", 24, len(wire), wire[24:]},
	}
	for _, f := range fields {
		latTestField(t, segment, f.name, f.typ, offset+uint64(f.start)*8, offset+uint64(f.end)*8, f.value)
	}
	megacoTestTree(t, segment, offset, offset+uint64(len(wire))*8)
	info := segment.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, []string{"PUSH", "ACK", "WASK", "WINS"}[wire[4]-81], info["Command Name"])
	require.Equal(t, wire[4] != 81, info["Receiver Ignores Segment Data"])
	require.Equal(t, false, info["Application Data Decoded"])
	require.Equal(t, false, info["Message Reassembled"])
}

func TestProtocolCorpusKCPIndependentFields(t *testing.T) {
	vectors := kcpTestVectors(t)
	for i, wire := range vectors {
		for _, entry := range []string{"KCPSegment", "KCPDatagram", "KCPDatagramCarrier"} {
			n := kcpTestParse(t, wire, entry)
			require.Equal(t, wire, NodeToBytes(n), "vector %d", i)
			message := kcpTestMessage(t, n)
			megacoTestTree(t, message, 0, uint64(len(wire))*8)
			kcpTestSegment(t, message.Children[0], wire, 0)
			info := message.Cfg.GetItem("additionInfo").(map[string]any)
			require.Equal(t, "KCP exact bounded segment layout", info["Profile"])
			require.Equal(t, 1, info["Segment Count"])
			require.Equal(t, entry == "KCPSegment", info["Single Segment"])
			require.Equal(t, true, info["Exact Consumption"])
			for _, key := range []string{"Receiver Trailing Bytes Compatibility", "External Conversation Validated", "Receiver State Validated", "Message Reassembled", "Application Data Decoded", "Outer Prefix Decoded", "Application Identity Inferred"} {
				require.Equal(t, false, info[key], key)
			}
		}
	}
	// Out-of-order, duplicate and wrapped sequence values are not errors in a
	// stateless layout reader. One datagram may aggregate all four commands.
	order := []int{3, 0, 1, 1, 2}
	var aggregate []byte
	for _, index := range order {
		aggregate = append(aggregate, vectors[index]...)
	}
	n := kcpTestParse(t, aggregate, "KCPDatagram")
	require.Equal(t, aggregate, NodeToBytes(n))
	message := kcpTestMessage(t, n)
	require.Len(t, message.Children, len(order))
	megacoTestTree(t, message, 0, uint64(len(aggregate))*8)
	position := uint64(0)
	for i, index := range order {
		kcpTestSegment(t, message.Children[i], vectors[index], position)
		position += uint64(len(vectors[index])) * 8
	}
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(aggregate), kcpTestRule, "KCPSegment")
	require.ErrorContains(t, err, "single segment")
	// A shorter aggregate ending at a complete segment is itself valid. Other
	// short prefixes must fail; no external count or missing packet is invented.
	ends := map[int]bool{}
	end := 0
	for _, index := range order {
		end += len(vectors[index])
		ends[end] = true
	}
	for cut := 0; cut < len(aggregate); cut++ {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(aggregate[:cut]), kcpTestRule, "KCPDatagram")
		if ends[cut] {
			require.NoError(t, err, "complete prefix %d", cut)
		} else {
			require.Error(t, err, "partial prefix %d", cut)
		}
	}
}

func TestProtocolCorpusKCPOriginalNetEaseAllRecords(t *testing.T) {
	path := "captures/ndpi/ndpi-netease-games.pcapng"
	data, err := os.ReadFile("testdata/protocol-corpus/" + path)
	require.NoError(t, err)
	require.Equal(t, "f662aff63082da1da601841bdb3d88f4ca557be2bdb8528ee746f910bd28d38a", fmt.Sprintf("%x", sha256.Sum256(data)))
	packets := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/"+path)
	require.Len(t, packets, 20)
	wantLengths := map[int]int{11: 12, 12: 12, 13: 12, 14: 30, 15: 30, 16: 82, 17: 290, 18: 34, 19: 97, 20: 40}
	var dns, tcp, vendor, kcp int
	for i, frame := range packets {
		number := i + 1
		require.GreaterOrEqual(t, len(frame), 34)
		require.Equal(t, uint16(0x0800), binary.BigEndian.Uint16(frame[12:14]))
		ip := frame[14:]
		header := int(ip[0]&15) * 4
		total := int(binary.BigEndian.Uint16(ip[2:4]))
		require.GreaterOrEqual(t, total, header)
		require.LessOrEqual(t, total, len(ip))
		ip = ip[:total] // short TCP control frames retain Ethernet pad outside IP
		outer := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		require.Equal(t, frame, NodeToBytes(outer), "frame %d", number)
		if ip[9] == 6 {
			tcp++
			continue
		}
		require.Equal(t, byte(17), ip[9])
		udp := ip[header:]
		require.Equal(t, len(udp), int(binary.BigEndian.Uint16(udp[4:6])))
		payload := udp[8:]
		if binary.BigEndian.Uint16(udp[:2]) == 53 || binary.BigEndian.Uint16(udp[2:4]) == 53 {
			dns++
			continue
		}
		vendor++
		require.Len(t, payload, wantLengths[number], "frame %d", number)
		// The whole wrapper is NOT reclassified as a standard KCP datagram.
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(payload), kcpTestRule, "KCPDatagram")
		require.Error(t, err, "wrapped frame %d", number)
		if number != 19 && number != 20 {
			if len(payload) >= 16 {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(payload[16:]), kcpTestRule, "KCPDatagram")
				require.Error(t, err, "other offset-16 candidate frame %d", number)
			}
			continue
		}
		kcp++
		prefix, wire := payload[:16], payload[16:]
		wantPrefix := "0708080cc2827d4900007e0400000001"
		wantCommand, wantUNA, wantData := uint64(81), uint64(0), 57
		if number == 20 {
			wantPrefix = "0708080c53da16090000000100007e04"
			wantCommand, wantUNA, wantData = 82, 1, 0
		}
		require.Equal(t, wantPrefix, hex.EncodeToString(prefix))
		require.Len(t, wire, 24+wantData)
		parsed := kcpTestParse(t, wire, "KCPSegment")
		kcpTestSegment(t, protocolCorpusFindNode(parsed, "Segment 0"), wire, 0)
		protocolCorpusRequireValue(t, parsed, "Conversation ID", uint64(768))
		protocolCorpusRequireValue(t, parsed, "Command", wantCommand)
		protocolCorpusRequireValue(t, parsed, "Window", uint64(128))
		protocolCorpusRequireValue(t, parsed, "Timestamp", uint64(35242))
		protocolCorpusRequireValue(t, parsed, "Sequence Number", uint64(0))
		protocolCorpusRequireValue(t, parsed, "Unacknowledged Sequence", wantUNA)
		start := 14 + header + 8 + 16
		inline := latTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      this.ProcessSubNode("Captured Outer Bytes")
      this.GetSubNode("KCP").SetMaxLength(%d)
      this.ProcessSubNode("KCP")
    Captured Outer Bytes: raw,%d
    KCP: "import:application-layer/kcp.yaml;node:KCPSegment"
`, len(wire), start))
		inline.Cfg.SetItem(base.CfgLength, uint64(len(frame))*8)
		require.NoError(t, inline.ParseSubNode(base.NewBitReader(bytes.NewReader(frame)), "Envelope"))
		imported := base.GetNodeByPath(inline, "@Envelope")
		require.Equal(t, frame, NodeToBytes(imported))
		latTestField(t, imported, "Captured Outer Bytes", "raw", 0, uint64(start)*8, frame[:start])
		kcpTestSegment(t, protocolCorpusFindNode(imported, "Segment 0"), wire, uint64(start)*8)
	}
	require.Equal(t, []int{4, 6, 10, 2}, []int{dns, tcp, vendor, kcp})
}

func TestProtocolCorpusKCPBoundsAndExactConsumption(t *testing.T) {
	vectors := kcpTestVectors(t)
	for _, wire := range vectors {
		for cut := 0; cut < len(wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), kcpTestRule, "KCPDatagram")
			require.Error(t, err, "prefix %d/%d", cut, len(wire))
		}
	}
	var bad [][]byte
	bad = append(bad, []byte{1}, vectors[0][:23])
	for _, cmd := range []byte{0, 80, 85, 255} {
		wire := bytes.Clone(vectors[0])
		wire[4] = cmd
		bad = append(bad, wire)
	}
	for _, length := range []uint32{0, 2, 4, 0x80000000, 0xffffffff} {
		wire := bytes.Clone(vectors[0])
		binary.LittleEndian.PutUint32(wire[20:], length)
		bad = append(bad, wire)
	}
	for count := 1; count < 24; count++ {
		bad = append(bad, append(bytes.Clone(vectors[0]), make([]byte, count)...))
	}
	bad = append(bad, append(bytes.Clone(vectors[0]), vectors[4]...)) // mismatched conv
	bad = append(bad, append(bytes.Clone(vectors[0]), vectors[0][:23]...))
	for i, wire := range bad {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), kcpTestRule, "KCPDatagram")
		require.Error(t, err, "negative %d", i)
		n := kcpTestParse(t, wire, "KCPDatagramCarrier")
		require.Equal(t, wire, NodeToBytes(n))
		latTestField(t, n, "Unparsed KCP Datagram", "raw", 0, uint64(len(wire))*8, wire)
		require.Nil(t, protocolCorpusFindNode(n, "Segment 0"))
	}
	for _, entry := range []string{"KCPSegment", "KCPDatagram", "KCPDatagramCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(vectors[0]), kcpTestRule, entry)
		require.ErrorContains(t, err, "boundary")
		_, err = parser.GenerateBinary(nil, kcpTestRule, entry)
		require.Error(t, err, "generation must be unsupported: %s", entry)
		for _, bits := range []uint64{1, 7, 191, 193, 65536*8 + 1, 65537 * 8} {
			r := &giopCarrierTestBitReader{Reader: bytes.NewReader(vectors[0]), bits: bits}
			_, err := parser.ParseBinary(r, kcpTestRule, entry)
			require.Error(t, err)
			require.Equal(t, len(vectors[0]), r.Len())
		}
	}
	// Full byte limit with one declared payload, plus the next byte rejected
	// before input reads; the limit does not pretend to be an agreed path MTU.
	wire := append(bytes.Clone(vectors[0][:24]), make([]byte, 65536-24)...)
	binary.LittleEndian.PutUint32(wire[20:], 65536-24)
	n := kcpTestParse(t, wire, "KCPSegment")
	require.Equal(t, wire, NodeToBytes(n))
	kcpTestSegment(t, protocolCorpusFindNode(n, "Segment 0"), wire, 0)
}

func TestProtocolCorpusKCPImportedRollbackAndIsolation(t *testing.T) {
	valid := kcpTestVectors(t)[0]
	for offset := uint64(0); offset < 8; offset++ {
		for _, success := range []bool{true, false} {
			wire := bytes.Clone(valid)
			if !success {
				wire = append(wire, valid[:23]...)
			} // fails after a valid segment
			var packed bytes.Buffer
			w := base.NewBitWriter(&packed)
			if offset > 0 {
				require.NoError(t, w.WriteBits([]byte{0x55}, offset))
			}
			require.NoError(t, w.WriteBits(wire, uint64(len(wire))*8))
			require.NoError(t, w.WriteBits([]byte{0xd3}, 8))
			if offset > 0 {
				require.NoError(t, w.WriteBits([]byte{0}, 8-offset))
			}
			root := latTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Message").SetMaxLength(%d)
      this.ProcessSubNode("Message")
      this.ProcessSubNode("Sentinel")
      if %d > 0 { this.ProcessSubNode("Padding") }
    Prefix: uint8,%dbit
    Message: "import:application-layer/kcp.yaml;node:KCPDatagramCarrier"
    Sentinel: uint8
    Padding: uint8,%dbit
`, offset, len(wire), offset, offset, 8-offset))
			root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
			root.Ctx.SetItem("kcp caller", "preserved")
			r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
			require.NoError(t, r.Backup())
			require.NoError(t, root.ParseSubNode(r, "Envelope"))
			n := base.GetNodeByPath(root, "@Envelope")
			require.Equal(t, packed.Bytes(), NodeToBytes(n))
			protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
			require.Equal(t, "preserved", root.Ctx.GetItem("kcp caller"))
			if success {
				kcpTestSegment(t, protocolCorpusFindNode(n, "Segment 0"), wire, offset)
			} else {
				latTestField(t, n, "Unparsed KCP Datagram", "raw", offset, offset+uint64(len(wire))*8, wire)
				require.Nil(t, protocolCorpusFindNode(n, "Conversation ID"))
			}
			require.NoError(t, r.Recovery())
			got, err := r.ReadBits(uint64(packed.Len()) * 8)
			require.NoError(t, err)
			require.Equal(t, packed.Bytes(), got)
			require.ErrorContains(t, r.PopBackup(), "no backup")
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			wire := bytes.Clone(valid)
			binary.LittleEndian.PutUint32(wire, uint32(i))
			n := kcpTestParse(t, wire, "KCPDatagramCarrier")
			protocolCorpusRequireValue(t, n, "Conversation ID", uint64(i))
			require.Equal(t, wire, NodeToBytes(n))
		}(i)
	}
	wg.Wait()
}
