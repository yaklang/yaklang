package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func FuzzIPv4OptionsEnvelope(f *testing.F) {
	for _, options := range [][]byte{nil, {1, 1, 1, 0}, {148, 4, 0, 0}, {7, 1, 0, 0}, {1, 1, 1, 7}} {
		f.Add(ipv4OptionsDatagram(253, options, []byte("bounded body")))
	}
	fragment := ipv4OptionsDatagram(253, []byte{1, 1, 0, 0}, bytes.Repeat([]byte{0x5a}, 8))
	fragment[6] = 0x20
	f.Add(fragment)
	f.Fuzz(func(t *testing.T, input []byte) {
		wire := bytes.Clone(input[:min(len(input), 512)])
		// The independent oracle covers only the IPv4 envelope and opaque
		// body. Application grammar and checksum validation are separate.
		if len(wire) >= 20 {
			wire[9] = 253
		}
		valid, header, total := false, 0, 0
		if len(wire) >= 20 {
			header = int(wire[0]&15) * 4
			total = int(binary.BigEndian.Uint16(wire[2:4]))
			valid = wire[0]>>4 == 4 && header >= 20 && header <= total && total <= len(wire)
			if valid {
				for at := 20; at < header; {
					kind := wire[at]
					if kind == 0 {
						break
					}
					if kind == 1 {
						at++
						continue
					}
					if at+1 >= header || wire[at+1] < 2 || int(wire[at+1]) > header-at {
						valid = false
						break
					}
					at += int(wire[at+1])
				}
			}
		}
		reader := bytes.NewReader(wire)
		n, err := parser.ParseBinary(reader, "internet_protocol", "Internet Protocol")
		if !valid {
			require.Error(t, err)
			return
		}
		require.NoError(t, err)
		require.Equal(t, len(wire)-total, reader.Len(), "never consume the next datagram")
		_, err = n.Result()
		require.NoError(t, err)
		if header > 20 {
			protocolCorpusRequireValue(t, n, "IP Header Options", wire[20:header])
			dvmrpRequireSpan(t, n, "IP Header Options", 20*8, uint64(header)*8)
		}
		if total > header {
			if binary.BigEndian.Uint16(wire[6:8])&0x3fff != 0 {
				protocolCorpusRequireValue(t, n, "IP Fragment Data", wire[header:total])
				dvmrpRequireSpan(t, n, "IP Fragment Data", uint64(header)*8, uint64(total)*8)
			} else {
				protocolCorpusRequireValue(t, n, "Next Protocol Data", string(wire[header:total]))
				dvmrpRequireSpan(t, n, "Next Protocol Data", uint64(header)*8, uint64(total)*8)
			}
		}
	})
}
