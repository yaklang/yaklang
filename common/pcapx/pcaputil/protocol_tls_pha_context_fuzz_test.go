package pcaputil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// This fuzz target covers bounded message admission and private context
// ownership. Authenticated record encryption is covered by the AEAD matrix;
// transcript MAC, certificate trust and complete handshake ordering are not
// inferred from this private message-level entry point.
func FuzzTLS13PHAContextAdmission(f *testing.F) {
	for _, seed := range []struct {
		flags         byte
		first, second []byte
	}{
		{1, []byte{1}, []byte{2}},
		{1, []byte{1}, []byte{1}},
		{0, []byte{1}, []byte{2}},
		{2, nil, []byte{2}},
		{5, nil, nil},
		{5, []byte{1}, nil},
		{1, bytes.Repeat([]byte{0xa5}, 255), []byte{0xa5}},
		{6, nil, []byte{1}},
	} {
		f.Add(seed.flags, seed.first, seed.second)
	}
	f.Fuzz(func(t *testing.T, flags byte, first, second []byte) {
		first = bytes.Clone(first[:min(len(first), 255)])
		second = bytes.Clone(second[:min(len(second), 255)])
		initial, unknown, offered := flags&4 != 0, flags&2 != 0, flags&1 != 0
		state := newBinTLS()
		state.version, state.suite, state.client = 0x304, 0x1301, 0
		state.app[1] = !initial
		if !unknown {
			extensions := []tlsResumptionExt{{43, []byte{2, 3, 4}}}
			if offered {
				extensions = append(extensions, tlsResumptionExt{49, nil})
			}
			state.firstHello = tlsResumptionHello(true, nil, state.suite, extensions...)
		}
		wire := phaContextTestRequest(first)
		message, err := state.handshakeMessage(1, wire, true, 16)
		if initial && len(first) != 0 || !initial && !unknown && !offered {
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, ErrMalformedMessage, typed.Kind)
			require.Nil(t, message)
			require.Empty(t, state.requestContexts)
			return
		}
		require.NoError(t, err)
		require.Len(t, state.requestContexts, 1)
		context := string(first)
		clear(wire)
		clear(message["Encoded Certificate Request"].([]byte))
		clear(message["Certificate Request"].(map[string]any)["Context"].([]byte))
		state.app[1] = true
		message, err = state.handshakeMessage(1, phaContextTestRequest(second), true, 16)
		if !unknown && !offered || string(second) == context {
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, ErrMalformedMessage, typed.Kind)
			require.Nil(t, message)
			require.Len(t, state.requestContexts, 1)
		} else {
			require.NoError(t, err)
			require.Len(t, state.requestContexts, 2)
			require.Equal(t, uint64(2), message["Certificate Request Admission"].(map[string]any)["Observed Request Count"])
		}
		_, retained := state.requestContexts[context]
		require.True(t, retained, "public edits cannot retire a previously observed context")
	})
}
