package bin_parser

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func parsePR5023MobilitySample(t *testing.T, wire []byte, entry string) (*base.Node, error) {
	t.Helper()
	reader := newProtocolCorpusBoundedReader(wire)
	node, err := parser.ParseBinary(reader, "mobility_samples", entry)
	if err == nil {
		require.Zero(t, reader.Len())
	}
	return node, err
}

func TestProtocolCorpusPR5023MobilityFieldsAndBoundaries(t *testing.T) {
	const corpus = "testdata/protocol-corpus/captures"
	for _, spec := range []struct {
		path   string
		entry  string
		offset int
		frames int
		bad    string
	}{
		{"generated-pr5023/pr5023-gen-mip.pcap", "MobileIP", 42, 1, "missing required registration extension"},
		{"generated-validated/gen-mip-valid.pcap", "MobileIP", 42, 1, ""},
		{"generated-pr5023/pr5023-gen-mipv6.pcap", "MIPv6Packet", 14, 1, "Mobility Header length differs"},
		{"generated-validated/gen-mipv6-valid.pcap", "MIPv6Packet", 14, 2, ""},
	} {
		t.Run(spec.path, func(t *testing.T) {
			frames := protocolCorpusAuditPackets(t, filepath.Join(corpus, spec.path))
			require.Len(t, frames, spec.frames)
			for index, frame := range frames {
				t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
					require.Greater(t, len(frame), spec.offset)
					wire := frame[spec.offset:]
					node, err := parsePR5023MobilitySample(t, wire, spec.entry)
					if spec.bad != "" {
						require.ErrorContains(t, err, spec.bad)
						return
					}
					require.NoError(t, err)
					info := node.Cfg.GetItem("additionInfo").(map[string]any)
					if spec.entry == "MobileIP" {
						protocolCorpusRequireValue(t, node, "Message Type", uint64(1))
						protocolCorpusRequireValue(t, node, "Lifetime", uint64(60))
						protocolCorpusRequireValue(t, node, "Identification", uint64(1))
						protocolCorpusRequireValue(t, node, "SPI", uint64(256))
						require.Equal(t, false, info["Association Validated"])
						mac := hmac.New(md5.New, []byte("protocol-sample-key"))
						_, err = mac.Write(wire[:30])
						require.NoError(t, err)
						require.Equal(t, mac.Sum(nil), wire[30:], "fixture extension value must match the documented fixture key")
					} else {
						require.Equal(t, true, info["Checksum Valid"])
						require.Equal(t, false, info["Binding Context Validated"])
						if index == 0 {
							protocolCorpusRequireValue(t, node, "MH Type", uint64(1))
							cookie := protocolCorpusFindNode(node, "Init Cookie")
							value, err := cookie.Result()
							require.NoError(t, err)
							require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, value.Value)
						} else {
							protocolCorpusRequireValue(t, node, "MH Type", uint64(5))
							protocolCorpusRequireValue(t, node, "Sequence Number", uint64(1))
							protocolCorpusRequireValue(t, node, "Lifetime", uint64(60))
							protocolCorpusRequireValue(t, node, "Home Nonce Index", uint64(1))
							protocolCorpusRequireValue(t, node, "Care-of Nonce Index", uint64(2))
							require.Len(t, protocolCorpusFindNode(node, "Options").Children, 2)
							bindingKey := sha1.Sum([]byte("homekey1carekey1"))
							mac := hmac.New(sha1.New, bindingKey[:])
							_, err := mac.Write(wire[8:40])
							require.NoError(t, err)
							mh := bytes.Clone(wire[40:60])
							mh[4], mh[5] = 0, 0
							_, err = mac.Write(mh)
							require.NoError(t, err)
							require.Equal(t, mac.Sum(nil)[:12], wire[60:])
						}
					}
					for cut := 0; cut < len(wire); cut++ {
						_, err := parsePR5023MobilitySample(t, wire[:cut], spec.entry)
						require.Errorf(t, err, "%s accepted truncated input at %d/%d", spec.entry, cut, len(wire))
					}
				})
			}
		})
	}
}

func mobilityTestPacket(mh []byte) []byte {
	packet := make([]byte, 40)
	packet[0], packet[6], packet[7] = 0x60, 135, 64
	packet[8], packet[9], packet[10], packet[11], packet[23] = 0x20, 1, 0xd, 0xb8, 1
	copy(packet[24:], packet[8:24])
	packet[39] = 2
	binary.BigEndian.PutUint16(packet[4:], uint16(len(mh)))
	packet = append(packet, mh...)
	packet[44], packet[45] = 0, 0
	sum := uint32(135 + len(mh))
	for _, data := range [][]byte{packet[8:40], packet[40:]} {
		for i := 0; i < len(data); i += 2 {
			sum += uint32(binary.BigEndian.Uint16(data[i:]))
		}
	}
	for sum > 0xffff {
		sum = sum&0xffff + sum>>16
	}
	binary.BigEndian.PutUint16(packet[44:], ^uint16(sum))
	return packet
}

func TestPR5023MobilityProtocolVariants(t *testing.T) {
	const corpus = "testdata/protocol-corpus/captures"
	mip := protocolCorpusAuditPackets(t, filepath.Join(corpus, "generated-validated/gen-mip-valid.pcap"))[0][42:]
	// A foreign-agent denial is allowed without a home-agent extension.
	reply := append([]byte{3, 64}, mip[2:12]...)
	reply = append(reply, mip[16:24]...)
	node, err := parsePR5023MobilitySample(t, reply, "MobileIP")
	require.NoError(t, err)
	protocolCorpusRequireValue(t, node, "Code", uint64(64))
	protocolCorpusRequireValue(t, node, "Identification", uint64(1))
	invalidHomeReply := bytes.Clone(reply)
	invalidHomeReply[1] = 194
	_, err = parsePR5023MobilitySample(t, invalidHomeReply, "MobileIP")
	require.NoError(t, err, "code 194 is a foreign-agent denial in RFC 5944")
	unknown := append(bytes.Clone(mip), 128, 2, 0x11, 0x22)
	node, err = parsePR5023MobilitySample(t, unknown, "MobileIP")
	require.NoError(t, err, "unknown skippable short extensions remain opaque")
	require.Len(t, protocolCorpusFindNode(node, "Extensions").Children, 2)
	generalized := append(bytes.Clone(mip), 36, 1, 0, 20)
	generalized = append(generalized, mip[26:]...)
	_, err = parsePR5023MobilitySample(t, generalized, "MobileIP")
	require.NoError(t, err)
	missingHome := append(bytes.Clone(mip[:24]), generalized[len(mip):]...)
	_, err = parsePR5023MobilitySample(t, missingHome, "MobileIP")
	require.ErrorContains(t, err, "generalized extension must follow")
	shortGeneralized := bytes.Clone(generalized)
	shortGeneralized[len(mip)+3] = 19
	_, err = parsePR5023MobilitySample(t, shortGeneralized, "MobileIP")
	require.ErrorContains(t, err, "at least 20")
	_, err = parsePR5023MobilitySample(t, append(bytes.Clone(mip), mip[24:]...), "MobileIP")
	require.ErrorContains(t, err, "duplicate Mobile-Home")
	acceptedReply := bytes.Clone(reply)
	acceptedReply[1] = 0
	_, err = parsePR5023MobilitySample(t, acceptedReply, "MobileIP")
	require.ErrorContains(t, err, "missing required registration extension")
	_, err = parsePR5023MobilitySample(t, append(acceptedReply, mip[24:]...), "MobileIP")
	require.NoError(t, err)
	ignoredFlags := bytes.Clone(mip)
	ignoredFlags[1] |= 5
	_, err = parsePR5023MobilitySample(t, ignoredFlags, "MobileIP")
	require.NoError(t, err, "RFC 5944 r/x bits are ignored on receipt")
	_, err = parsePR5023MobilitySample(t, append(bytes.Clone(mip), bytes.Repeat([]byte{128, 0}, 127)...), "MobileIP")
	require.NoError(t, err)
	_, err = parsePR5023MobilitySample(t, append(bytes.Clone(mip), bytes.Repeat([]byte{128, 0}, 128)...), "MobileIP")
	require.ErrorContains(t, err, "too many extensions")
	for _, mutation := range []struct {
		offset int
		value  byte
		bad    string
	}{{0, 2, "unsupported registration message type"}, {24, 31, "unsupported critical extension"}, {25, 21, "extension length exceeds"}, {25, 4, "extension has no SPI or value"}} {
		invalid := bytes.Clone(mip)
		invalid[mutation.offset] = mutation.value
		_, err = parsePR5023MobilitySample(t, invalid, "MobileIP")
		require.ErrorContains(t, err, mutation.bad)
	}

	for kind, length := range []int{8, 16, 16, 24, 24, 16, 16, 24} {
		mh := make([]byte, length)
		mh[0], mh[1], mh[2], mh[3] = 59, byte(length/8-1), byte(kind), 0x80
		packet := mobilityTestPacket(mh)
		node, err := parsePR5023MobilitySample(t, packet, "MIPv6Packet")
		require.NoError(t, err, "base MH type %d", kind)
		protocolCorpusRequireValue(t, node, "MH Type", uint64(kind))
		protocolCorpusRequireValue(t, node, "Reserved", uint64(0x80))
		packet[len(packet)-1] ^= 1
		_, err = parsePR5023MobilitySample(t, packet, "MIPv6Packet")
		require.Error(t, err, "changed payload must not pass checksum")
	}
	for _, spec := range []struct {
		mh  []byte
		bad string
	}{
		{[]byte{59, 0, 5, 0, 0, 0, 0, 0}, "truncated message fields"},
		{[]byte{59, 0, 8, 0, 0, 0, 0, 0}, "unsupported Mobility Header type"},
		{[]byte{17, 0, 0, 0, 0, 0, 0, 0}, "unsupported payload"},
		{[]byte{59, 1, 5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128, 3, 0, 0}, "option length exceeds"},
		{[]byte{59, 1, 5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 1, 0, 0}, "invalid known option length"},
	} {
		_, err = parsePR5023MobilitySample(t, mobilityTestPacket(spec.mh), "MIPv6Packet")
		require.ErrorContains(t, err, spec.bad)
	}
	unknownOption := []byte{59, 1, 5, 0, 0, 0, 0, 1, 0, 0, 0, 60, 128, 2, 0xaa, 0xbb}
	node, err = parsePR5023MobilitySample(t, mobilityTestPacket(unknownOption), "MIPv6Packet")
	require.NoError(t, err)
	protocolCorpusRequireValue(t, node, "Option Type", uint64(128))
	refresh := bytes.Clone(unknownOption)
	refresh[2], refresh[12], refresh[14], refresh[15] = 6, 2, 0, 15
	node, err = parsePR5023MobilitySample(t, mobilityTestPacket(refresh), "MIPv6Packet")
	require.NoError(t, err)
	protocolCorpusRequireValue(t, node, "Refresh Interval", uint64(15))
	for _, mutation := range []struct {
		offset int
		value  byte
		bad    string
	}{{0, 0x40, "expected IPv6"}, {6, 60, "only directly carried"}, {5, 15, "payload length differs"}, {23, 3, "checksum mismatch"}} {
		invalid := mobilityTestPacket(unknownOption)
		invalid[mutation.offset] = mutation.value
		_, err = parsePR5023MobilitySample(t, invalid, "MIPv6Packet")
		require.ErrorContains(t, err, mutation.bad)
	}
	padOptions := make([]byte, 8+256)
	padOptions[0], padOptions[1] = 59, 32
	_, err = parsePR5023MobilitySample(t, mobilityTestPacket(padOptions), "MIPv6Packet")
	require.NoError(t, err)
	padOptions = append(padOptions, make([]byte, 8)...)
	padOptions[1]++
	_, err = parsePR5023MobilitySample(t, mobilityTestPacket(padOptions), "MIPv6Packet")
	require.ErrorContains(t, err, "too many mobility options")
	update := protocolCorpusAuditPackets(t, filepath.Join(corpus, "generated-validated/gen-mipv6-valid.pcap"))[1][54:]
	trailingPad := append(bytes.Clone(update), make([]byte, 8)...)
	trailingPad[1]++
	_, err = parsePR5023MobilitySample(t, mobilityTestPacket(trailingPad), "MIPv6Packet")
	require.ErrorContains(t, err, "binding data must be the final option")
}
