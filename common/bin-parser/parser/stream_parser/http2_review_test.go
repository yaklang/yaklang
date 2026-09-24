package stream_parser

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTP2LayoutValidationParity(t *testing.T) {
	// Exercise every known frame type, flags, stream constraints, payload lengths,
	// and invalid SETTINGS/priority/window values through the common validator.
	for typ := 0; typ <= 10; typ++ {
		for _, flags := range []byte{0, 1, 4, 8, 32, 44, 255} {
			for _, stream := range []uint32{0, 1, 3} {
				for size := 0; size <= 36; size++ {
					wire := http2TestFrame(byte(typ), flags, stream, bytes.Repeat([]byte{1}, size)...)
					full, err := decodeHTTP2WireFrame(wire, 0)
					layout, gotErr := InspectHTTP2Frame(wire)
					require.Equal(t, fmt.Sprint(err), fmt.Sprint(gotErr), "type=%d flags=%d stream=%d size=%d", typ, flags, stream, size)
					if err == nil {
						require.Equal(t, full.typ, layout.Type)
						require.Equal(t, full.flags, layout.Flags)
						require.Equal(t, full.stream, layout.Stream)
						require.Equal(t, full.promised, layout.Promised)
						if full.fragmentStart >= 0 {
							require.Equal(t, wire[full.fragmentStart:full.fragmentEnd], layout.Fragment)
						}
					}
				}
			}
		}
	}
	for _, wire := range [][]byte{http2TestFrame(1, 4, 1, 0x82), http2TestFrame(4, 0, 0, 0, 1, 0, 0, 16, 0)} {
		require.Zero(t, testing.AllocsPerRun(100, func() {
			_, err := InspectHTTP2Frame(wire)
			if err != nil {
				panic(err)
			}
		}))
		for cut := 0; cut < len(wire); cut++ {
			_, err := InspectHTTP2Frame(wire[:cut])
			require.Error(t, err)
		}
		_, err := InspectHTTP2Frame(append(bytes.Clone(wire), 0))
		require.ErrorContains(t, err, "trailing")
	}
}
