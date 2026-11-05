package pcaputil

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Independently generated public HMAC/AESGCM/ChaCha20Poly1305 vectors.
// No captured keys or session secrets are included. The GCM explicit nonce
// intentionally differs from the implicit record sequence.
func TestTLS12DHEAEADIndependentVectors(t *testing.T) {
	for _, c := range []struct {
		suite    uint16
		iv, wire string
	}{
		{0x009e, "48092b65", "170303004acafe0011223344556c7d1cbdaa6c24aab88efe99e5ee01155023983479ff01e86eed8639b4c97c53060b4f3af19cbdabf0616496c704463cc29ee2a15350cf70e2a240dd52280335f4c6"},
		{0x009f, "c241a06d", "170303004acafe001122334455619468182aa7745af25888e5795f5b12621416ffc4f51ee02cd150a39c2658d642956b877e77006e1ac7ac68b75a731af47e80536fffcea66afa31dce91784bd21b1"},
		{0xccaa, "f6ba1048b7cd53eb2f258495", "1703030042327cd4cc641f6d1f7174376434c71b9b0062b251e460ab610b3426e5eacf2ab77e4539effd37656cbbff7662c3584a0536a2f8d98cca3475df5428d8801c8799a8d9"},
	} {
		t.Run(fmt.Sprintf("%04x", c.suite), func(t *testing.T) {
			require.True(t, tlsSupportedSuite(0x303, c.suite))
			client := make([]byte, 32)
			server := make([]byte, 32)
			master := make([]byte, 48)
			for i := range client {
				client[i] = byte(i)
				server[i] = byte(i + 32)
			}
			for i := range master {
				master[i] = byte(i)
			}
			provider, err := ParseTLSKeyLog(fmt.Sprintf("CLIENT_RANDOM %x %x\n", client, master))
			require.NoError(t, err)
			state := newBinTLS()
			state.client, state.version, state.suite = 0, 0x303, c.suite
			state.random, state.serverRandom = client, server
			ok, err := state.loadKey(0, provider)
			require.NoError(t, err)
			require.True(t, ok)
			iv, err := hex.DecodeString(c.iv)
			require.NoError(t, err)
			require.Equal(t, iv, state.iv[0])
			wire, err := hex.DecodeString(c.wire)
			require.NoError(t, err)
			state.seq[0] = 5
			plain, kind, err := state.open(0, wire)
			require.NoError(t, err)
			require.Equal(t, byte(23), kind)
			require.Equal(t, []byte("GET /dhe-vector HTTP/1.1\r\nHost: vector.invalid\r\n\r\n"), plain)
			require.EqualValues(t, 6, state.seq[0])
		})
	}
}

func TestTLS12DHEAEADEnvelopeBounds(t *testing.T) {
	for _, c := range []struct {
		suite   uint16
		minimum int
	}{{0x009e, 24}, {0x009f, 24}, {0xccaa, 16}} {
		t.Run(fmt.Sprintf("%04x", c.suite), func(t *testing.T) {
			state := newBinTLS()
			state.version, state.suite = 0x303, c.suite
			for n := 0; n < c.minimum; n++ {
				wire := make([]byte, 5+n)
				wire[0], wire[1], wire[2], wire[4] = 23, 3, 3, byte(n)
				require.Error(t, state.checkCiphertextEnvelope(wire), "impossible negotiated envelope %d", n)
			}
			wire := make([]byte, 5+c.minimum)
			wire[0], wire[1], wire[2], wire[4] = 23, 3, 3, byte(c.minimum)
			require.NoError(t, state.checkCiphertextEnvelope(wire), "possible shape is not authentication")
		})
	}
}
