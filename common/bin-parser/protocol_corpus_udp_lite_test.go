package bin_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func TestProtocolCorpusUDPLiteChecksumAndCoverage(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-local/gen-udplite.pcap")
	require.Len(t, frames, 1)
	original := frames[0][14:]
	require.Equal(t, byte(136), original[9])
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(original), "internet_protocol", "Internet Protocol")
	require.Error(t, err, "original UDP pseudo-header checksum was accepted as UDP-Lite")
	require.Contains(t, protocolCorpusFailureDiagnostic(err), "udp-lite: checksum mismatch")
	for _, ipv6 := range []bool{false, true} {
		for _, coverage := range []uint16{0, 8, 9, 12} {
			udp := append([]byte(nil), original[20:]...)
			binary.BigEndian.PutUint16(udp[4:6], coverage)
			udp[6], udp[7] = 0, 0
			header := append([]byte(nil), original[:20]...)
			rule, entry := "internet_protocol", "Internet Protocol"
			if ipv6 {
				header = make([]byte, 40)
				header[0], header[6], header[7] = 0x60, 136, 64
				binary.BigEndian.PutUint16(header[4:6], uint16(len(udp)))
				copy(header[8:24], mustHex(t, "20010db8000000000000000000000001"))
				copy(header[24:40], mustHex(t, "20010db8000000000000000000000002"))
				rule, entry = "internet_protocol_version_6", "Internet Protocol Version 6"
			}
			protected := int(coverage)
			if coverage == 0 {
				protected = len(udp)
			}
			pseudo := protocolCorpusUDPLitePseudoHeader(header, len(udp))
			checksum := protocolCorpusOnesComplement(append(pseudo, udp[:protected]...))
			if checksum == 0 {
				checksum = 0xffff
			}
			binary.BigEndian.PutUint16(udp[6:8], checksum)
			data := append(header, udp...)
			node := protocolCorpusRequireBoundedRuleParse(t, data, rule, entry)
			lite := protocolCorpusFindNode(node, "UDP-Lite")
			require.NotNil(t, lite)
			protocolCorpusRequireValue(t, lite, "Source Port", uint64(binary.BigEndian.Uint16(udp[:2])))
			protocolCorpusRequireValue(t, lite, "Destination Port", uint64(binary.BigEndian.Uint16(udp[2:4])))
			protocolCorpusRequireValue(t, lite, "Checksum Coverage", uint64(coverage))
			protocolCorpusRequireValue(t, lite, "Checksum", uint64(checksum))
			if protected > 8 {
				protocolCorpusRequireValue(t, lite, "Covered Payload", udp[8:protected])
			}
			if protected < len(udp) {
				protocolCorpusRequireValue(t, lite, "Uncovered Payload", udp[protected:])
				uncoveredChange := append([]byte(nil), data...)
				uncoveredChange[len(uncoveredChange)-1] ^= 1
				protocolCorpusRequireBoundedRuleParse(t, uncoveredChange, rule, entry)
			}
			corrupt := append([]byte(nil), data...)
			corrupt[len(header)+1] ^= 1 // Covered source-port byte.
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(corrupt), rule, entry)
			require.Error(t, err)
			require.Contains(t, protocolCorpusFailureDiagnostic(err), "udp-lite: checksum mismatch")
			for _, invalid := range []uint16{1, 7, uint16(len(udp) + 1)} {
				bad := append([]byte(nil), data...)
				binary.BigEndian.PutUint16(bad[len(header)+4:], invalid)
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), rule, entry)
				require.Error(t, err)
				require.Contains(t, protocolCorpusFailureDiagnostic(err), "invalid checksum coverage")
			}
		}
	}
}

func protocolCorpusUDPLitePseudoHeader(ip []byte, length int) []byte {
	if ip[0]>>4 == 4 {
		b := make([]byte, 12)
		copy(b, ip[12:20])
		b[9] = 136
		binary.BigEndian.PutUint16(b[10:12], uint16(length))
		return b
	}
	b := make([]byte, 40)
	copy(b, ip[8:40])
	binary.BigEndian.PutUint32(b[32:36], uint32(length))
	b[39] = 136
	return b
}

func protocolCorpusOnesComplement(b []byte) uint16 {
	var sum uint32
	for len(b) > 1 {
		sum += uint32(binary.BigEndian.Uint16(b[:2]))
		b = b[2:]
	}
	if len(b) == 1 {
		sum += uint32(b[0]) << 8
	}
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}
