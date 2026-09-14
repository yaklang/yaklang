package bin_parser

import (
	"bytes"
	"crypto/rsa"
	"encoding/binary"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"golang.org/x/crypto/ssh"
)

const sshPlaintextTestRule = "application-layer.ssh_plaintext"

type sshPlaintextTestMessage struct {
	wire []byte
	at   int
}

type sshPlaintextTestFlow struct {
	tlsCertificateSourceFlow
	client           bool
	stream           []byte
	identEnd, keyEnd int
	packets          []sshPlaintextTestMessage
	names            []string
}

// Independent fixture observer: all physical records, observed SYN sequence
// bases, checked overlap/gaps, identification line then packet lengths until
// each direction's first NEWKEYS. Encrypted suffixes are counted, never fed to
// a plaintext grammar. Sorting sequence positions recovers the delayed client
// identification in frame 265 without trusting a dissector's display state.
func sshPlaintextTestObserve(t *testing.T, records [][]byte) []*sshPlaintextTestFlow {
	t.Helper()
	flows := map[string]*sshPlaintextTestFlow{}
	for i, record := range records {
		p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, p.ErrorLayer(), "frame %d", i+1)
		tcp, ok := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
		require.True(t, ok, "all original records are TCP")
		net := p.NetworkLayer().NetworkFlow()
		src, dst := fmt.Sprint(net.Src(), ":", tcp.SrcPort), fmt.Sprint(net.Dst(), ":", tcp.DstPort)
		key, pair := src+" -> "+dst, src+" -> "+dst
		if src > dst {
			pair = dst + " -> " + src
		}
		f := flows[key]
		if f == nil {
			f = &sshPlaintextTestFlow{tlsCertificateSourceFlow: tlsCertificateSourceFlow{Pair: pair}}
			flows[key] = f
		}
		if tcp.SYN {
			if f.SYN {
				require.Equal(t, f.Initial, tcp.Seq+1)
			}
			f.SYN, f.Initial, f.client = true, tcp.Seq+1, !tcp.ACK
		}
		if len(tcp.Payload) == 0 {
			continue // Acknowledgment/control record; remains in the 322 count.
		}
		offset := 0
		for _, layer := range p.Layers() {
			offset += len(layer.LayerContents())
			if layer.LayerType() == layers.LayerTypeTCP {
				break
			}
		}
		require.Equal(t, tcp.Payload, record[offset:offset+len(tcp.Payload)])
		f.Segments = append(f.Segments, tlsCertificateSourceSegment{Frame: i + 1, FrameOffset: offset, Seq: tcp.Seq, Payload: tcp.Payload})
	}
	var ordered []*sshPlaintextTestFlow
	for _, f := range flows {
		require.True(t, f.SYN)
		sort.SliceStable(f.Segments, func(i, j int) bool { return uint32(f.Segments[i].Seq-f.Initial) < uint32(f.Segments[j].Seq-f.Initial) })
		for _, s := range f.Segments {
			at := int(uint32(s.Seq - f.Initial))
			require.LessOrEqual(t, at, len(f.stream), "captured gap must not be bridged")
			for j, b := range s.Payload {
				if at+j < len(f.stream) {
					require.Equal(t, f.stream[at+j], b, "conflicting overlap at frame %d", s.Frame)
				} else {
					f.stream = append(f.stream, b)
				}
			}
		}
		require.True(t, bytes.HasPrefix(f.stream, []byte("SSH-2.0-")))
		f.identEnd = bytes.Index(f.stream, []byte("\r\n")) + 2
		require.Greater(t, f.identEnd, 2)
		require.LessOrEqual(t, f.identEnd, 255)
		for at := f.identEnd; at < len(f.stream); {
			require.GreaterOrEqual(t, len(f.stream)-at, 6)
			size := int(binary.BigEndian.Uint32(f.stream[at:])) + 4
			require.GreaterOrEqual(t, size, 16)
			require.LessOrEqual(t, size, len(f.stream)-at)
			w := f.stream[at : at+size]
			require.Zero(t, size%8)
			require.GreaterOrEqual(t, int(w[4]), 4)
			f.packets = append(f.packets, sshPlaintextTestMessage{wire: w, at: at})
			if w[5] == 20 {
				require.Nil(t, f.names, "one initial KEXINIT per direction")
				pos := 22 // length + pad + message + cookie
				for i := 0; i < 10; i++ {
					n := int(binary.BigEndian.Uint32(w[pos:]))
					pos += 4
					require.LessOrEqual(t, pos+n, size-int(w[4])-5)
					f.names = append(f.names, string(w[pos:pos+n]))
					pos += n
				}
				require.Zero(t, w[pos], "fixture has no optimistic KEX packet to discard")
				require.Equal(t, size-int(w[4]), pos+5)
			}
			at += size
			if w[5] == 21 {
				require.Equal(t, 6, size-int(w[4]))
				f.keyEnd = at
				break
			}
		}
		require.Positive(t, f.keyEnd)
		require.Greater(t, len(f.stream), f.keyEnd)
		ordered = append(ordered, f)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Segments[0].Frame < ordered[j].Segments[0].Frame })
	return ordered
}

func sshPlaintextTestFirstCommon(t *testing.T, client, server string) string {
	t.Helper()
	for _, c := range strings.Split(client, ",") {
		for _, s := range strings.Split(server, ",") {
			if c == s {
				return c
			}
		}
	}
	t.Fatal("no common fixture algorithm")
	return ""
}

// Walk an independently decoded SSH string schema, including nested RSA blobs.
// It checks values as well as exact leaf positions; the full-tree oracle also
// proves contiguous coverage, raw hashes, consumed bytes and Result origins.
func sshPlaintextTestFields(t *testing.T, n *base.Node, w []byte, offset uint64, profile string) {
	t.Helper()
	at := 0
	field := func(name, typ string, size int) []byte {
		t.Helper()
		b := w[at : at+size]
		var value any = b
		switch typ {
		case "uint8":
			value = uint64(b[0])
		case "uint32":
			value = uint64(binary.BigEndian.Uint32(b))
		case "string":
			value = string(b)
		}
		latTestField(t, n, name, typ, offset+uint64(at)*8, offset+uint64(at+size)*8, value)
		at += size
		return b
	}
	str := func(name, typ string) []byte {
		size := binary.BigEndian.Uint32(field(name+" Length", "uint32", 4))
		return field(name, typ, int(size))
	}
	point := func(name string) {
		size := int(binary.BigEndian.Uint32(field(name+" Length", "uint32", 4)))
		end := at + size
		format := field(name+" Format", "uint8", 1)[0]
		field(name+" X", "raw", 32)
		if format == 4 {
			field(name+" Y", "raw", 32)
		}
		require.Equal(t, end, at)
	}
	reply := func(ecdh bool) {
		size := int(binary.BigEndian.Uint32(field("Host Key Length", "uint32", 4)))
		start := at
		require.Equal(t, "ssh-rsa", string(str("Host Key Format", "string")))
		exponent, modulus := str("RSA Exponent", "raw"), str("RSA Modulus", "raw")
		require.Equal(t, start+size, at)
		key, err := ssh.ParsePublicKey(w[start:at])
		require.NoError(t, err)
		rsaKey := key.(ssh.CryptoPublicKey).CryptoPublicKey().(*rsa.PublicKey)
		require.Equal(t, int64(rsaKey.E), new(big.Int).SetBytes(exponent).Int64())
		require.Equal(t, 0, rsaKey.N.Cmp(new(big.Int).SetBytes(modulus)))
		if ecdh {
			point("Server Point")
		} else {
			str("Server Exchange Value", "raw")
		}
		size = int(binary.BigEndian.Uint32(field("Signature Length", "uint32", 4)))
		start = at
		format := string(str("Signature Format", "string"))
		require.Contains(t, []string{"ssh-rsa", "rsa-sha2-512"}, format)
		str("RSA Signature Bytes", "raw")
		require.Equal(t, start+size, at)
	}
	field("Packet Length", "uint32", 4)
	pad := int(field("Padding Length", "uint8", 1)[0])
	msg := field("Message Number", "uint8", 1)[0]
	switch {
	case msg == 20:
		field("Cookie", "raw", 16)
		for _, name := range []string{"Kex Algorithms", "Host Key Algorithms", "Encryption C2S", "Encryption S2C", "MAC C2S", "MAC S2C", "Compression C2S", "Compression S2C", "Languages C2S", "Languages S2C"} {
			str(name, "string")
		}
		field("First Kex Packet Follows", "uint8", 1)
		field("Reserved", "uint32", 4)
	case msg == 21:
	case profile == "dh-gex" && msg == 34:
		field("Minimum Group Bits", "uint32", 4)
		field("Preferred Group Bits", "uint32", 4)
		field("Maximum Group Bits", "uint32", 4)
	case profile == "dh-gex" && msg == 31:
		str("Group Prime", "raw")
		str("Group Generator", "raw")
	case profile == "dh-gex" && msg == 32 || profile == "dh" && msg == 30:
		str("Client Exchange Value", "raw")
	case profile == "dh-gex" && msg == 33 || profile == "dh" && msg == 31:
		reply(false)
	case profile == "ecdh-nistp256" && msg == 30:
		point("Client Point")
	case profile == "ecdh-nistp256" && msg == 31:
		reply(true)
	default:
		t.Fatalf("unexpected fixture message %d/%s", msg, profile)
	}
	require.Equal(t, len(w)-pad, at)
	field("Padding", "raw", pad)
	tlsCertificateTestTree(t, n, w, offset)
	info := n.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, true, info["Payload Layout Decoded"])
	for _, key := range []string{"Negotiation Validated", "Key Mathematics Validated", "Signature Validated", "Peer Identity Validated", "Handshake Completion Validated", "TCP Reassembly Performed", "Structured Generation Supported"} {
		require.Equal(t, false, info[key], key)
	}
}

func sshPlaintextTestProfile(t *testing.T, f *sshPlaintextTestFlow, flows []*sshPlaintextTestFlow) (algorithm, profile, entry string) {
	t.Helper()
	var client, server *sshPlaintextTestFlow
	for _, peer := range flows {
		if peer.Pair == f.Pair {
			if peer.client {
				client = peer
			} else {
				server = peer
			}
		}
	}
	require.NotNil(t, client)
	require.NotNil(t, server)
	algorithm = sshPlaintextTestFirstCommon(t, client.names[0], server.names[0])
	hostAlgorithm := sshPlaintextTestFirstCommon(t, client.names[1], server.names[1])
	require.Contains(t, []string{"ssh-rsa", "rsa-sha2-512"}, hostAlgorithm)
	switch algorithm {
	case "diffie-hellman-group-exchange-sha256":
		profile, entry = "dh-gex", "SSHPlaintextDHGEX"
	case "diffie-hellman-group14-sha1":
		profile, entry = "dh", "SSHPlaintextDH"
	case "ecdh-sha2-nistp256":
		profile, entry = "ecdh-nistp256", "SSHPlaintextECDHP256"
	default:
		t.Fatalf("unhandled original exchange %s", algorithm)
	}
	return
}

func TestProtocolCorpusSSHPlaintextAllOriginalRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-ssh.pcap"
	file, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "581856f8ab7248ba785d6a98734296054a2290002ecfa298b0ae89a62c12c030", tlsCertificateTestSHA(file))
	records := protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 322)
	flows := sshPlaintextTestObserve(t, records)
	require.Len(t, flows, 6)
	var starts []int
	counts, algorithms := map[string]int{}, map[string]int{}
	identBytes, clearBytes, protectedBytes, physicalPayloadBytes, noPayloadFrames := 0, 0, 0, 0, 322
	for _, f := range flows {
		algorithm, profile, entry := sshPlaintextTestProfile(t, f, flows)
		if f.client {
			algorithms[algorithm]++
		}
		ident := protocolCorpusRequireBoundedRuleParse(t, f.stream[:f.identEnd], "application-layer.ssh", "SSH")
		require.Equal(t, f.stream[:f.identEnd], NodeToBytes(ident))
		for _, message := range f.packets {
			counts[profile]++
			n := protocolCorpusRequireBoundedRuleParse(t, message.wire, sshPlaintextTestRule, entry)
			sshPlaintextTestFields(t, n, message.wire, 0, profile)
			// Every contributing physical segment, including retransmissions,
			// must agree. These originals happen to hold each PDU in one frame,
			// including two PDUs within frames 16 and 270.
			covered, first := make([]bool, len(message.wire)), 0
			for _, segment := range f.Segments {
				s := int(uint32(segment.Seq - f.Initial))
				e := s + len(segment.Payload)
				a, b := max(s, message.at), min(e, message.at+len(message.wire))
				if a >= b {
					continue
				}
				if first == 0 {
					first = segment.Frame
				}
				source := records[segment.Frame-1][segment.FrameOffset+a-s : segment.FrameOffset+b-s]
				require.Equal(t, tlsCertificateTestSHA(source), tlsCertificateTestSHA(message.wire[a-message.at:b-message.at]))
				for j := a - message.at; j < b-message.at; j++ {
					covered[j] = true
				}
				if a == message.at && b == message.at+len(message.wire) {
					physical := segment.FrameOffset + a - s
					tail := len(records[segment.Frame-1]) - physical - len(message.wire)
					root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Envelope:\n    operator: |\n      this.ProcessSubNode(\"Prefix\")\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      if %d > 0 { this.ProcessSubNode(\"Tail\") }\n    Prefix: raw,%d\n    Message: \"import:application-layer/ssh_plaintext.yaml;node:%s\"\n    Tail: raw,%d\n", len(message.wire), tail, physical, entry, tail))
					root.Cfg.SetItem(base.CfgLength, uint64(len(records[segment.Frame-1]))*8)
					require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(records[segment.Frame-1])), "Envelope"))
					sshPlaintextTestFields(t, protocolCorpusFindNode(root, "Message"), message.wire, uint64(physical)*8, profile)
				}
			}
			require.NotContains(t, covered, false)
			require.Positive(t, first)
			starts = append(starts, first)
			if message.wire[5] == 20 || message.wire[5] == 21 {
				protocolCorpusRequireBoundedRuleParse(t, message.wire, sshPlaintextTestRule, "SSHPlaintextPacket")
			} else {
				opaque := protocolCorpusRequireBoundedRuleParse(t, message.wire, sshPlaintextTestRule, "SSHPlaintextPacket")
				require.Equal(t, false, opaque.Cfg.GetItem("additionInfo").(map[string]any)["Payload Layout Decoded"])
			}
		}
		// Attribute every captured payload byte to version, clear packet, or
		// protected suffix. No sample/record is silently skipped by port.
		for _, segment := range f.Segments {
			noPayloadFrames--
			physicalPayloadBytes += len(segment.Payload)
			at := int(uint32(segment.Seq - f.Initial))
			for j := range segment.Payload {
				switch {
				case at+j < f.identEnd:
					identBytes++
				case at+j < f.keyEnd:
					clearBytes++
				default:
					protectedBytes++
				}
			}
		}
	}
	sort.Ints(starts)
	require.Equal(t, []int{8, 10, 12, 13, 15, 16, 16, 18, 265, 267, 269, 270, 270, 272, 293, 294, 295, 297, 299, 301}, starts)
	require.Equal(t, map[string]int{"dh-gex": 8, "dh": 6, "ecdh-nistp256": 6}, counts)
	require.Equal(t, map[string]int{"diffie-hellman-group-exchange-sha256": 1, "diffie-hellman-group14-sha1": 1, "ecdh-sha2-nistp256": 1}, algorithms)
	require.Equal(t, physicalPayloadBytes, identBytes+clearBytes+protectedBytes)
	require.Equal(t, 135, noPayloadFrames)
	require.Equal(t, 28527, physicalPayloadBytes)
	require.Equal(t, 171, identBytes)
	require.Equal(t, 8760, clearBytes)
	require.Equal(t, 19596, protectedBytes)
	t.Logf("records=%d no-payload=%d payload-bytes=%d identification=%d clear=%d protected=%d packets=%v", len(records), noPayloadFrames, physicalPayloadBytes, identBytes, clearBytes, protectedBytes, counts)
}

func TestProtocolCorpusSSHPlaintextOriginalBoundaries(t *testing.T) {
	records := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-ssh.pcap")
	flows := sshPlaintextTestObserve(t, records)
	for _, f := range flows {
		_, _, entry := sshPlaintextTestProfile(t, f, flows)
		for _, message := range f.packets {
			w := message.wire
			for cut := 0; cut < len(w); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w[:cut]), sshPlaintextTestRule, entry)
				require.Error(t, err, "%s prefix %d", entry, cut)
			}
			var invalid [][]byte
			invalid = append(invalid, append(bytes.Clone(w), 0), append(bytes.Clone(w), w...))
			for i := 0; i < 5; i++ {
				bad := bytes.Clone(w)
				bad[i] = 255
				invalid = append(invalid, bad)
			}
			if w[5] == 20 {
				bad := bytes.Clone(w)
				bad[26] = ','
				invalid = append(invalid, bad)
			}
			for _, bad := range invalid {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), sshPlaintextTestRule, entry)
				require.Error(t, err)
				n := protocolCorpusRequireBoundedRuleParse(t, bad, sshPlaintextTestRule, entry+"Carrier")
				require.Nil(t, protocolCorpusFindNode(n, "Message Number"))
				latTestField(t, n, "Unparsed SSH Plaintext Packet", "raw", 0, uint64(len(bad))*8, bad)
				require.Equal(t, bad, NodeToBytes(n))
			}
		}
	}
}

func TestProtocolCorpusSSHPlaintextIsolation(t *testing.T) {
	for _, entry := range []string{"SSHPlaintextPacket", "SSHPlaintextDH", "SSHPlaintextDHGEX", "SSHPlaintextECDHP256"} {
		entry := entry
		t.Run(entry, func(t *testing.T) {
			t.Parallel()
			w := sshPlaintextTestPacket([]byte{21})
			cfg := map[string]any{"caller-ssh": entry}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, w, sshPlaintextTestRule, entry, cfg)
			n.Cfg.GetItem("additionInfo").(map[string]any)["Signature Validated"] = true
			again := protocolCorpusRequireBoundedRuleParse(t, w, sshPlaintextTestRule, entry)
			require.Equal(t, false, again.Cfg.GetItem("additionInfo").(map[string]any)["Signature Validated"])
			require.Equal(t, map[string]any{"caller-ssh": entry}, cfg)
		})
	}
}

func sshPlaintextTestPacket(payload []byte) []byte {
	pad := 8 - (5+len(payload))%8
	if pad < 4 {
		pad += 8
	}
	w := make([]byte, 5+len(payload)+pad)
	binary.BigEndian.PutUint32(w, uint32(len(w)-4))
	w[4] = byte(pad)
	copy(w[5:], payload)
	return w
}

func TestProtocolCorpusSSHPlaintextOffsetsAndRollback(t *testing.T) {
	for _, entry := range []string{"SSHPlaintextPacket", "SSHPlaintextDH", "SSHPlaintextDHGEX", "SSHPlaintextECDHP256"} {
		for offset := uint64(0); offset < 8; offset++ {
			for _, valid := range []bool{true, false} {
				w := sshPlaintextTestPacket([]byte{21})
				if !valid {
					w[5] = 20
				} // valid outer length, incomplete KEXINIT
				var packed bytes.Buffer
				writer := base.NewBitWriter(&packed)
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0x55}, offset))
				}
				require.NoError(t, writer.WriteBits(w, uint64(len(w))*8))
				require.NoError(t, writer.WriteBits([]byte{0xd3}, 8))
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0}, 8-offset))
				}
				root := latTestInline(t, fmt.Sprintf(`endian: little
Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Message").SetMaxLength(%d)
      this.ProcessSubNode("Message")
      this.ProcessSubNode("Sentinel")
      if %d > 0 { this.ProcessSubNode("Tail") }
    Prefix: uint8,%dbit
    Message: "import:application-layer/ssh_plaintext.yaml;node:%sCarrier"
    Sentinel: uint8
    Tail: uint8,%dbit
`, offset, len(w), offset, offset, entry, 8-offset))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("caller-ssh", "preserved")
				r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, r.Backup())
				require.NoError(t, root.ParseSubNode(r, "Envelope"))
				if valid {
					n := protocolCorpusFindNode(root, "Message Number").Cfg.GetItem(base.CfgParent).(*base.Node).Cfg.GetItem(base.CfgParent).(*base.Node)
					sshPlaintextTestFields(t, n, w, offset, "transport")
				} else {
					require.Nil(t, protocolCorpusFindNode(root, "Message Number"))
					latTestField(t, root, "Unparsed SSH Plaintext Packet", "raw", offset, offset+uint64(len(w))*8, w)
				}
				protocolCorpusRequireValue(t, root, "Sentinel", uint64(0xd3))
				require.Equal(t, "preserved", root.Ctx.GetItem("caller-ssh"))
				require.NoError(t, r.Recovery())
				recovered, err := r.ReadBits(uint64(packed.Len()) * 8)
				require.NoError(t, err)
				require.Equal(t, packed.Bytes(), recovered)
				require.ErrorContains(t, r.PopBackup(), "no backup")
			}
		}
		for _, bits := range []uint64{0, 1, 7, 9, 35001 * 8} {
			for _, suffix := range []string{"", "Carrier"} {
				r := &tlsSHTestHeldReader{bits: bits}
				_, err := parser.ParseBinary(r, sshPlaintextTestRule, entry+suffix)
				require.Error(t, err)
				require.Zero(t, r.reads)
			}
		}
		for _, bits := range []uint64{8, 15 * 8, 17 * 8} {
			r := &tlsSHTestHeldReader{bits: bits}
			_, err := parser.ParseBinary(r, sshPlaintextTestRule, entry)
			require.Error(t, err)
			require.Zero(t, r.reads)
		}
		_, err := parser.ParseBinary(bytes.NewReader(sshPlaintextTestPacket([]byte{21})), sshPlaintextTestRule, entry)
		require.Error(t, err)
		_, err = parser.GenerateBinary(map[string]any{}, sshPlaintextTestRule, entry)
		require.Error(t, err)
	}
}
