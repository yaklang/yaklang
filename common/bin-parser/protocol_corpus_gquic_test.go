package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const gquic35TestRule = "application-layer.gquic"

// Independent arbitrary-precision oracle, deliberately unlike the production
// two-limb Mul64 implementation. These inputs are offline field fixtures only.
func gquic35TestHash(header, body []byte) []byte {
	h, _ := new(big.Int).SetString("144066263297769815596495629667062367629", 10)
	prime, _ := new(big.Int).SetString("309485009821345068724781371", 10)
	mask := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	for _, part := range [][]byte{header, body} {
		for _, b := range part {
			h.Xor(h, new(big.Int).SetUint64(uint64(b)))
			h.Mul(h, prime).And(h, mask)
		}
	}
	b := h.FillBytes(make([]byte, 16))
	out := make([]byte, 12)
	for i := range out {
		out[i] = b[15-i]
	}
	return out
}

func gquic35TestHello() []byte {
	// Literal wire table: PAD=953, SNI=11, VER=4, MSPC=4, AEAD=4.
	// CHLO header 8 + five entries 40 + values 976 = 1024 bytes.
	header, _ := hex.DecodeString("0d01020304050607085130333501")
	chlo, _ := hex.DecodeString("43484c4f0500000050414400b9030000534e4900c403000056455200c80300004d535043cc03000041454144d0030000")
	chlo = append(chlo, bytes.Repeat([]byte{'-'}, 953)...)
	chlo = append(chlo, []byte("example.orgQ035\x64\x00\x00\x00CC20")...)
	body := append([]byte{0xa0, 1, 0, 4}, chlo...)
	body = append(body, 0, 0xaa, 0xbb) // Chromium ignores remainder after padding type
	out := append(bytes.Clone(header), gquic35TestHash(header, body)...)
	return append(out, body...)
}

func gquic35TestRehash(wire []byte) {
	copy(wire[14:26], gquic35TestHash(wire[:14], wire[26:]))
}

func gquic35TestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	return protocolCorpusRequireBoundedRuleParse(t, wire, gquic35TestRule, entry)
}

func gquic35TestMessage(t *testing.T, n *base.Node) *base.Node {
	t.Helper()
	leaf := protocolCorpusFindNode(n, "Public Flags")
	require.NotNil(t, leaf)
	return leaf.Cfg.GetItem(base.CfgParent).(*base.Node).Cfg.GetItem(base.CfgParent).(*base.Node)
}

func TestProtocolCorpusGQUIC35IndependentFields(t *testing.T) {
	wire := gquic35TestHello()
	require.Len(t, wire, 1057)
	for _, entry := range []string{"GQUIC35ClientHello", "GQUIC35ClientHelloCarrier", "GQUIC35ClientPacket"} {
		n := gquic35TestParse(t, wire, entry)
		require.Equal(t, wire, NodeToBytes(n))
		message := gquic35TestMessage(t, n)
		megacoTestTree(t, message, 0, uint64(len(wire))*8)
		latTestField(t, n, "Public Flags", "uint8", 0, 8, uint64(13))
		latTestField(t, n, "Connection ID", "uint64", 8, 72, uint64(0x0807060504030201))
		latTestField(t, n, "Version Tag", "string", 72, 104, "Q035")
		latTestField(t, n, "Wire Packet Number", "uint64", 104, 112, uint64(1))
		info := message.Cfg.GetItem("additionInfo").(map[string]any)
		require.Equal(t, false, info["Diversification Nonce Present"])
		for _, key := range []string{"Packet Number Reconstructed", "Connection State Inferred", "Payload Decrypted", "Peer Authenticated", "Handshake Parameters Complete", "Handshake Outcome Inferred"} {
			require.Equal(t, false, info[key], key)
		}
		if entry == "GQUIC35ClientPacket" {
			latTestField(t, n, "Protected or Uninterpreted Body", "raw", 112, uint64(len(wire))*8, wire[14:])
			require.Nil(t, protocolCorpusFindNode(n, "Client Hello"))
			require.Equal(t, false, info["Null Checksum Verified"])
			continue
		}
		require.Equal(t, true, info["Null Checksum Verified"])
		require.Equal(t, 5, info["CHLO Entry Count"])
		latTestField(t, n, "Null Checksum", "raw", 112, 208, wire[14:26])
		latTestField(t, n, "STREAM Type", "uint8", 208, 216, uint64(0xa0))
		latTestField(t, n, "Stream ID", "uint32", 216, 224, uint64(1))
		latTestField(t, n, "Stream Data Length", "uint16", 224, 240, uint64(1024))
		latTestField(t, n, "Handshake Tag", "string", 240, 272, "CHLO")
		latTestField(t, n, "Tag Count", "uint16", 272, 288, uint64(5))
		latTestField(t, n, "CHLO SNI", "string", 1031*8, 1042*8, "example.org")
		latTestField(t, n, "CHLO MSPC", "uint32", 1046*8, 1050*8, uint64(100))
		latTestField(t, n, "AEAD Tag 0", "string", 1050*8, 1054*8, "CC20")
		latTestField(t, n, "Padding Remainder", "raw", 1055*8, 1057*8, []byte{0xaa, 0xbb})
	}
	// Four public packet-number widths, including low zero after wire wrap;
	// a CID-free packet must not acquire a prior packet's connection ID.
	for code, size := range []int{1, 2, 4, 6} {
		packet := append([]byte{byte(code << 4)}, bytes.Repeat([]byte{0}, size)...)
		packet = append(packet, 0x42)
		n := gquic35TestParse(t, packet, "GQUIC35ClientPacket")
		latTestField(t, n, "Wire Packet Number", "uint64", 8, uint64(1+size)*8, uint64(0))
		require.Nil(t, protocolCorpusFindNode(n, "Connection ID"))
		require.Nil(t, protocolCorpusFindNode(n, "Version Tag"))
	}
	// Same bit 0x04 has different layouts by the explicitly supplied direction.
	server := append([]byte{4}, bytes.Repeat([]byte{0x31}, 32)...)
	server = append(server, 7, 0x42)
	n := gquic35TestParse(t, server, "GQUIC35ServerPacket")
	latTestField(t, n, "Diversification Nonce", "raw", 8, 264, server[1:33])
	latTestField(t, n, "Wire Packet Number", "uint64", 264, 272, uint64(7))
	n = gquic35TestParse(t, server, "GQUIC35ClientPacket")
	require.Nil(t, protocolCorpusFindNode(n, "Diversification Nonce"))
	latTestField(t, n, "Wire Packet Number", "uint64", 8, 16, uint64(0x31))
}

func TestProtocolCorpusGQUIC35OriginalRecords(t *testing.T) {
	const path = "testdata/protocol-corpus/captures/ndpi/ndpi-wechat.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "2d82f575a8b9addfc9e66582e34911f79b5388984e54299293a28393c6d7c24e", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 1672)
	expectedFrames := []int{49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 64, 65, 66, 67, 68, 69, 70, 71, 74, 75, 76, 77, 78, 999, 1000, 1001, 1002, 1003, 1004, 1005, 1006, 1007, 1008}
	hashes := map[int]string{49: "006513fcda66f385334ebb26", 51: "022bf354987aae4a3c40f97d", 64: "676c913c15523a5ebf535005", 999: "9b8303652f2c2d09280bb427"}
	var seen []int
	ipv6, tcp, igmp, otherUDP, matches := 0, 0, 0, 0, 0
	for i, frame := range frames {
		require.GreaterOrEqual(t, len(frame), 14)
		if binary.BigEndian.Uint16(frame[12:]) == 0x86dd {
			ipv6++
			require.GreaterOrEqual(t, len(frame), 54)
			require.Equal(t, byte(6), frame[14]>>4)
			require.LessOrEqual(t, 54+int(binary.BigEndian.Uint16(frame[18:])), len(frame))
			continue
		}
		require.Equal(t, uint16(0x800), binary.BigEndian.Uint16(frame[12:]))
		require.GreaterOrEqual(t, len(frame), 34)
		require.Equal(t, byte(4), frame[14]>>4)
		ihl, total := int(frame[14]&15)*4, int(binary.BigEndian.Uint16(frame[16:]))
		require.GreaterOrEqual(t, ihl, 20)
		require.GreaterOrEqual(t, total, ihl)
		require.LessOrEqual(t, 14+total, len(frame))
		require.Zero(t, binary.BigEndian.Uint16(frame[20:])&0x3fff)
		switch frame[23] {
		case 6:
			tcp++
			require.GreaterOrEqual(t, total-ihl, 20)
			tcpHeader := int(frame[14+ihl+12]>>4) * 4
			require.GreaterOrEqual(t, tcpHeader, 20)
			require.LessOrEqual(t, tcpHeader, total-ihl)
			continue
		case 2:
			igmp++
			continue
		case 17:
		default:
			t.Fatalf("unexpected IPv4 protocol at frame %d", i+1)
		}
		udp := 14 + ihl
		require.GreaterOrEqual(t, total-ihl, 8)
		length := int(binary.BigEndian.Uint16(frame[udp+4:]))
		require.GreaterOrEqual(t, length, 8)
		require.Equal(t, total-ihl, length)
		src, dst := binary.BigEndian.Uint16(frame[udp:]), binary.BigEndian.Uint16(frame[udp+2:])
		if src != 443 && dst != 443 {
			otherUDP++
			continue
		}
		// These exact corpus flows are independently documented as Google Q035.
		// Port 443 alone is NOT a runtime classifier or version inference.
		seen = append(seen, i+1)
		wire := frame[udp+8 : udp+length]
		entry, headerSize := "GQUIC35ClientPacket", 10
		if src == 443 {
			entry, headerSize = "GQUIC35ServerPacket", 2
			if wire[0]&4 != 0 {
				headerSize += 32
			}
		} else if wire[0]&1 != 0 {
			headerSize += 4
		}
		n := gquic35TestParse(t, wire, entry)
		require.Equal(t, wire, NodeToBytes(n))
		megacoTestTree(t, gquic35TestMessage(t, n), 0, uint64(len(wire))*8)
		latTestField(t, n, "Protected or Uninterpreted Body", "raw", uint64(headerSize)*8, uint64(len(wire))*8, wire[headerSize:])
		latTestField(t, n, "Wire Packet Number", "uint64", uint64(headerSize-1)*8, uint64(headerSize)*8, uint64(wire[headerSize-1]))
		if src != 443 {
			latTestField(t, n, "Connection ID", "uint64", 8, 72, binary.LittleEndian.Uint64(wire[1:9]))
		} else {
			require.Nil(t, protocolCorpusFindNode(n, "Connection ID"))
		}
		// Independently bounded import retains the complete original Ethernet
		// record and publishes offsets in that physical record, not a fake
		// zero-origin view of decoded or reconstructed bytes.
		importEntry := entry
		if _, ok := hashes[i+1]; ok {
			importEntry = "GQUIC35ClientHello"
		}
		suffix := len(frame) - udp - length
		root := latTestInline(t, fmt.Sprintf("Package:\n  Envelope:\n    operator: |\n      this.ProcessSubNode(\"Prefix\")\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      if %d > 0 { this.ProcessSubNode(\"Suffix\") }\n    Prefix: raw,%d\n    Message: \"import:application-layer/gquic.yaml;node:%s\"\n    Suffix: raw,%d\n", len(wire), suffix, udp+8, importEntry, suffix))
		root.Cfg.SetItem(base.CfgLength, uint64(len(frame))*8)
		require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(frame)), "Envelope"))
		container := base.GetNodeByPath(root, "@Envelope")
		require.Equal(t, frame, NodeToBytes(container))
		if importEntry == "GQUIC35ClientHello" {
			require.Equal(t, 8, gquic35TestOptionsOracle(t, container, wire, uint64(udp+8)*8))
		}
		megacoTestTree(t, gquic35TestMessage(t, container), uint64(udp+8)*8, uint64(udp+length)*8)
		hash := gquic35TestHash(wire[:headerSize], wire[headerSize+12:])
		_, expected := hashes[i+1]
		require.Equal(t, expected, bytes.Equal(hash, wire[headerSize:headerSize+12]))
		if !expected {
			continue // protected, not corrupt; never present a partial CHLO tree
		}
		matches++
		require.Len(t, wire, 1350)
		require.Equal(t, hashes[i+1], hex.EncodeToString(hash))
		n = gquic35TestParse(t, wire, "GQUIC35ClientHello")
		require.Equal(t, 8, gquic35TestOptionsOracle(t, n, wire, 0))
		require.Equal(t, wire, NodeToBytes(n))
		megacoTestTree(t, gquic35TestMessage(t, n), 0, uint64(len(wire))*8)
		protocolCorpusRequireValue(t, n, "Tag Count", uint64(29))
		protocolCorpusRequireValue(t, n, "CHLO MSPC", uint64(100))
		protocolCorpusRequireValue(t, n, "CHLO CFCW", uint64(15728640))
		protocolCorpusRequireValue(t, n, "CHLO SFCW", uint64(6291456))
		protocolCorpusRequireValue(t, n, "CHLO UAID", "Chrome/57.0.2987.133 Linux x86_64")
		protocolCorpusRequireValue(t, n, "VER Tag 0", "Q035")
		protocolCorpusRequireValue(t, n, "AEAD Tag 0", "CC20")
		protocolCorpusRequireValue(t, n, "KEXS Tag 0", "C255")
		sni := "ssl.gstatic.com"
		if i+1 == 64 {
			sni = "docs.google.com"
		}
		protocolCorpusRequireValue(t, n, "CHLO SNI", sni)
		protocolCorpusRequireValue(t, n, "CHLO COPT", []byte{})
		protocolCorpusRequireValue(t, n, "CHLO CSCT", []byte{})
		require.Equal(t, 0, protocolCorpusFindNode(n, "CHLO COPT").Cfg.GetItem("additionInfo").(map[string]any)["Tag Count"])
	}
	require.Equal(t, expectedFrames, seen)
	require.Equal(t, []int{68, 1433, 24, 111, 4}, []int{ipv6, tcp, igmp, otherUDP, matches})
}

func TestProtocolCorpusGQUIC35StreamEncodings(t *testing.T) {
	baseWire := gquic35TestHello()
	for sidSize := 1; sidSize <= 4; sidSize++ {
		for _, offSize := range []int{0, 2, 3, 4, 5, 6, 7, 8} {
			for _, lengthPresent := range []bool{true, false} {
				ft := byte(0x80 | (sidSize - 1))
				if offSize != 0 {
					ft |= byte(offSize-1) << 2
				}
				if lengthPresent {
					ft |= 0x20
				}
				body := append([]byte{ft, 1}, bytes.Repeat([]byte{0}, sidSize-1+offSize)...)
				if lengthPresent {
					body = append(body, 0, 4)
				}
				body = append(body, baseWire[30:1054]...)
				// Table's unused padding is read, not constrained by the source.
				body[len(body)-1024+6] = 0x7f
				wire := append(bytes.Clone(baseWire[:14]), gquic35TestHash(baseWire[:14], body)...)
				wire = append(wire, body...)
				n := gquic35TestParse(t, wire, "GQUIC35ClientHello")
				require.Equal(t, wire, NodeToBytes(n))
				megacoTestTree(t, gquic35TestMessage(t, n), 0, uint64(len(wire))*8)
				latTestField(t, n, "Stream ID", "uint32", 27*8, uint64(27+sidSize)*8, uint64(1))
				if offSize != 0 {
					latTestField(t, n, "Stream Offset", "uint64", uint64(27+sidSize)*8, uint64(27+sidSize+offSize)*8, uint64(0))
					wire[27+sidSize] = 1
					gquic35TestRehash(wire)
					_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), gquic35TestRule, "GQUIC35ClientHello")
					require.ErrorContains(t, err, "reassembly")
				}
			}
		}
	}
}

func TestProtocolCorpusGQUIC35BoundsAndRollback(t *testing.T) {
	valid := gquic35TestHello()
	for cut := 0; cut < len(valid); cut++ {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(valid[:cut]), gquic35TestRule, "GQUIC35ClientHello")
		require.Error(t, err, "prefix %d", cut)
	}
	bad := [][]byte{}
	for _, index := range []int{0, 9, 14, 26, 27, 28, 30, 34, 50, 1054} {
		wire := bytes.Clone(valid)
		wire[index] ^= 0x80
		if index >= 26 {
			gquic35TestRehash(wire)
		}
		bad = append(bad, wire)
	}
	// Duplicate tag, reversed cumulative end, huge declared end, nonzero
	// stream offset, partial known uint32/list widths, and trailing CHLO data.
	for _, change := range []func([]byte){
		func(w []byte) { copy(w[46:50], w[38:42]) },
		func(w []byte) { binary.LittleEndian.PutUint32(w[50:], 1) },
		func(w []byte) { binary.LittleEndian.PutUint32(w[42:], 0xffffffff) },
		func(w []byte) { w[26] |= 0x40 },
		func(w []byte) { binary.LittleEndian.PutUint32(w[66:], 971) },
		func(w []byte) { binary.LittleEndian.PutUint16(w[28:], 1025) },
	} {
		wire := bytes.Clone(valid)
		change(wire)
		gquic35TestRehash(wire)
		bad = append(bad, wire)
	}
	for i, wire := range bad {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), gquic35TestRule, "GQUIC35ClientHello")
		require.Error(t, err, "negative %d", i)
		n := gquic35TestParse(t, wire, "GQUIC35ClientHelloCarrier")
		require.Equal(t, wire, NodeToBytes(n))
		latTestField(t, n, "Unparsed GQUIC35 Client Hello", "raw", 0, uint64(len(wire))*8, wire)
		require.Nil(t, protocolCorpusFindNode(n, "Public Header"))
	}
	for _, flags := range []byte{0x80, 2, 3, 0x40} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader([]byte{flags, 1, 0}), gquic35TestRule, "GQUIC35ClientPacket")
		require.Error(t, err)
	}
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(valid), gquic35TestRule, "GQUIC35ServerPacket")
	require.ErrorContains(t, err, "version negotiation")
	for _, entry := range []string{"GQUIC35ClientPacket", "GQUIC35ServerPacket", "GQUIC35ClientHello", "GQUIC35ClientHelloCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(valid), gquic35TestRule, entry)
		require.ErrorContains(t, err, "boundary")
		for residual := uint64(1); residual < 8; residual++ {
			reader := &giopCarrierTestBitReader{Reader: bytes.NewReader(valid), bits: uint64(len(valid))*8 - residual}
			_, err := parser.ParseBinary(reader, gquic35TestRule, entry)
			require.Error(t, err)
			require.Equal(t, len(valid), reader.Len())
		}
	}
	for _, size := range []int{1452, 1453} {
		wire := append([]byte{0, 1}, bytes.Repeat([]byte{0xab}, size-2)...)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), gquic35TestRule, "GQUIC35ClientPacket")
		if size == 1452 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}

func TestProtocolCorpusGQUIC35ImportedHeldReader(t *testing.T) {
	gquic35TestImportedHeldReader(t, false)
}

func TestProtocolCorpusGQUIC35OptionsImportedHeldReader(t *testing.T) {
	gquic35TestImportedHeldReader(t, true)
}

func gquic35TestImportedHeldReader(t *testing.T, options bool) {
	for _, entry := range []string{"GQUIC35ClientHello", "GQUIC35ClientPacket", "GQUIC35ServerPacket", "GQUIC35ClientHelloCarrier"} {
		for offset := uint64(0); offset < 8; offset++ {
			for _, valid := range []bool{true, false} {
				if !valid && entry != "GQUIC35ClientHelloCarrier" {
					continue
				}
				wire := gquic35TestHello()
				if options {
					wire = gquic35TestOptionsPacket(t, gquic35TestOptionValues())
				}
				if entry == "GQUIC35ServerPacket" {
					wire = append([]byte{4}, bytes.Repeat([]byte{0x31}, 32)...)
					wire = append(wire, 7, 0x42)
				}
				if !valid {
					wire[14] ^= 1
				}
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
    Message: "import:application-layer/gquic.yaml;node:%s"
    Sentinel: uint8
    Padding: uint8,%dbit
`, offset, len(wire), offset, offset, entry, 8-offset))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("gquic-caller", "preserved")
				r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, r.Backup())
				require.NoError(t, root.ParseSubNode(r, "Envelope"))
				n := base.GetNodeByPath(root, "@Envelope")
				if valid {
					megacoTestTree(t, gquic35TestMessage(t, n), offset, offset+uint64(len(wire))*8)
					if options && (entry == "GQUIC35ClientHello" || entry == "GQUIC35ClientHelloCarrier") {
						require.Equal(t, 8, gquic35TestOptionsOracle(t, n, wire, offset))
					}
					if entry == "GQUIC35ServerPacket" {
						latTestField(t, n, "Diversification Nonce", "raw", offset+8, offset+264, wire[1:33])
						require.Nil(t, protocolCorpusFindNode(n, "Connection ID"))
					} else {
						latTestField(t, n, "Connection ID", "uint64", offset+8, offset+72, uint64(0x0807060504030201))
					}
				} else {
					latTestField(t, n, "Unparsed GQUIC35 Client Hello", "raw", offset, offset+uint64(len(wire))*8, wire)
					require.Nil(t, protocolCorpusFindNode(n, "Public Flags"))
				}
				protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
				require.Equal(t, "preserved", root.Ctx.GetItem("gquic-caller"))
				require.Equal(t, packed.Bytes(), NodeToBytes(n))
				require.NoError(t, r.Recovery())
				got, err := r.ReadBits(uint64(packed.Len()) * 8)
				require.NoError(t, err)
				require.Equal(t, packed.Bytes(), got)
				require.ErrorContains(t, r.PopBackup(), "no backup")
			}
		}
	}
}

func TestProtocolCorpusGQUIC35ConcurrentIsolation(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			wire := gquic35TestHello()
			wire[1] = byte(i + 1)
			gquic35TestRehash(wire)
			n := gquic35TestParse(t, wire, "GQUIC35ClientHello")
			require.Equal(t, wire, NodeToBytes(n))
			protocolCorpusRequireValue(t, n, "Connection ID", binary.LittleEndian.Uint64(wire[1:9]))
		}(i)
	}
	wg.Wait()
	// Explicit generated structure cannot silently return a fabricated packet.
	_, err := parser.GenerateBinary(map[string]any{}, gquic35TestRule, "GQUIC35ClientHello")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "unsupported") || strings.Contains(err.Error(), "parse"), err)
}
