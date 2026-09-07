package bin_parser

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

type protocolCorpusPartialByteReader struct {
	*bytes.Reader
	bits uint64
}

func (r *protocolCorpusPartialByteReader) InputBitLength() uint64 { return r.bits }

// Byte-oriented messages must reject a fractional final octet rather than
// silently floor the declared input boundary. This is independent of a byte
// message's starting bit offset, which the native bridge tests cover separately.
func TestProtocolCorpusNATTMACSecByteBoundaries(t *testing.T) {
	for _, c := range []struct {
		rule, entry string
		wire        []byte
	}{
		{"nat_t", "NATT", nattTestFixtures()[2]},
		{"nat_t", "NATTCarrier", nattTestFixtures()[2]},
		{"macsec", "MACSec", macsecTestFixtures(t)[0]},
		{"macsec", "MACSecCarrier", macsecTestFixtures(t)[0]},
	} {
		for _, legacy := range []bool{false, true} {
			for tail := uint64(1); tail <= 7; tail++ {
				t.Run(fmt.Sprintf("%s/legacy-%t/tail-%d", c.entry, legacy, tail), func(t *testing.T) {
					for _, bits := range []uint64{uint64(len(c.wire))*8 - tail, uint64(len(c.wire))*8 + tail} {
						wire := append(bytes.Clone(c.wire), 0xa5)
						reader := &protocolCorpusPartialByteReader{Reader: bytes.NewReader(wire), bits: bits}
						_, err := parser.ParseBinaryWithConfig(reader, c.rule, map[string]any{"nattPayloadsLegacy": legacy}, c.entry)
						require.ErrorContains(t, err, "boundary must be byte-aligned")
						require.Equal(t, len(wire), reader.Len(), "must reject before reading any bytes")
					}
				})
			}
		}
	}
}
