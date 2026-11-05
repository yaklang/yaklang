package pcaputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzTLSHandshakeVersionAdmission(f *testing.F) {
	for _, kind := range []byte{0, 3, 12, 14, 16, 21, 22, 23, 254, 1, 2} {
		f.Add([]byte{6, kind})
	}
	f.Add([]byte{1, 14})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 2 || len(data) > 258 {
			return
		}
		state := newBinTLS()
		state.version = []uint16{0x304, 0x303, 0}[data[0]%3]
		state.suite, state.client = 0x1301, int(data[0]>>3)%3-1
		dir, authenticated := int(data[0]>>2)&1, data[0]&2 != 0
		body := data[2:]
		wire := append([]byte{data[1], byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}, body...)
		before := *state
		_, err := state.handshakeMessage(dir, wire, authenticated, 16)
		forbidden := false
		if before.version == 0x304 {
			switch data[1] {
			case 0, 3, 12, 14, 16, 21, 22, 23, 254:
				forbidden = true
			case 1, 2:
				forbidden = authenticated
			}
		}
		if forbidden {
			var protocolErr *ProtocolError
			require.ErrorAs(t, err, &protocolErr)
			require.Equal(t, ErrMalformedMessage, protocolErr.Kind)
			require.Equal(t, before, *state, "a refused wire message cannot change connection state")
		}
	})
}
