package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProtocolCorpusXTPAndSwIPeIPv4Dispatch(t *testing.T) {
	for _, sample := range []struct {
		id, first, raw string
		protocol       byte
		count          int
		valid          bool
	}{
		{"generated-validated/gen-xtp-valid", "Key", "Unparsed XTP Payload", 36, 10, true},
		{"generated-validated/gen-swipe-valid", "Packet Type", "Unparsed swIPe Payload", 53, 6, true},
		{"generated-pr5023/pr5023-gen-xtp", "Key", "Unparsed XTP Payload", 36, 1, false},
		{"generated-pr5023/pr5023-gen-swipe", "Packet Type", "Unparsed swIPe Payload", 53, 1, false},
	} {
		frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/"+sample.id+".pcap")
		require.Len(t, frames, sample.count)
		for _, frame := range frames {
			require.Equal(t, byte(0x45), frame[14])
			require.Equal(t, sample.protocol, frame[23])
			require.EqualValues(t, len(frame)-14, binary.BigEndian.Uint16(frame[16:18]))
			for _, options := range []bool{false, true} {
				wire := bytes.Clone(frame)
				start := 34
				if options {
					wire = append(append(bytes.Clone(frame[:34]), 1, 1, 1, 0), frame[34:]...)
					wire[14] = 0x46
					binary.BigEndian.PutUint16(wire[16:18], uint16(len(wire)-14))
					start += 4
				}
				for _, fragment := range []bool{false, true} {
					message := bytes.Clone(wire)
					if fragment {
						binary.BigEndian.PutUint16(message[20:22], 0x2000)
					}
					n := protocolCorpusRequireBoundedRuleParse(t, message, "ethernet", "Ethernet")
					require.Equal(t, message, NodeToBytes(n))
					if fragment {
						protocolCorpusRequireValue(t, n, "IP Fragment Data", message[start:])
						require.Nil(t, protocolCorpusFindNode(n, sample.first))
					} else if sample.valid {
						require.NotNil(t, protocolCorpusFindNode(n, sample.first))
						require.Nil(t, protocolCorpusFindNode(n, sample.raw))
					} else {
						protocolCorpusRequireValue(t, n, sample.raw, message[start:])
						require.Nil(t, protocolCorpusFindNode(n, sample.first))
					}
				}
			}
		}
	}
}
