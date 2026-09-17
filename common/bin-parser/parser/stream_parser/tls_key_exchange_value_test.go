package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func tlsKXTestHandshake(server bool, body []byte) []byte {
	kind := byte(16)
	if server {
		kind = 12
	}
	return append([]byte{kind, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}, body...)
}

func TestTLS12KeyExchangeLayouts(t *testing.T) {
	for _, tc := range []struct {
		profile string
		server  bool
		body    []byte
	}{
		{"ECDHEServer", true, []byte{3, 0, 23, 1, 4, 6, 3, 0, 1, 0xa5}},
		{"DHEServer", true, []byte{0, 1, 2, 0, 1, 3, 0, 1, 4, 6, 1, 0, 1, 0xa5}},
		{"ECDHEClient", false, []byte{1, 4}},
		{"DHEClient", false, []byte{0, 1, 4}},
		{"RSAClient", false, []byte{0, 1, 4}},
		{"RSAClient", false, []byte{0, 0}}, // vector syntax, not usable RSA ciphertext
	} {
		t.Run(fmt.Sprintf("%s-%d", tc.profile, len(tc.body)), func(t *testing.T) {
			wire := tlsKXTestHandshake(tc.server, tc.body)
			fields, info, err := decodeTLS12KeyExchange(wire, tc.profile)
			require.NoError(t, err)
			at := 0
			for _, field := range fields {
				require.Equal(t, at, field.Start)
				require.GreaterOrEqual(t, field.End, field.Start)
				at = field.End
			}
			require.Equal(t, len(wire), at)
			require.Equal(t, "TLS 1.2 "+tc.profile+" layout", info["Profile"])
			for _, key := range []string{"Public Value Validated", "Signature Verified", "Premaster Decrypted", "Cipher Suite Correlation Validated", "Handshake Completion Validated"} {
				require.Equal(t, false, info[key])
			}
			for cut := 0; cut < len(wire); cut++ {
				f, m, e := decodeTLS12KeyExchange(wire[:cut], tc.profile)
				require.Error(t, e)
				require.Nil(t, f)
				require.Nil(t, m)
			}
			_, _, err = decodeTLS12KeyExchange(append(append([]byte(nil), wire...), 0), tc.profile)
			require.Error(t, err)
			bodyExtra := append(append([]byte(nil), tc.body...), 0)
			_, _, err = decodeTLS12KeyExchange(tlsKXTestHandshake(tc.server, bodyExtra), tc.profile)
			require.Error(t, err)
			wire[0] ^= 28 // swap 12 and 16
			_, _, err = decodeTLS12KeyExchange(wire, tc.profile)
			require.Error(t, err)
		})
	}
	// Identical nonempty client vectors are intentionally not distinguishable
	// as DH or RSA without the caller's negotiated cipher context.
	for _, profile := range []string{"DHEClient", "RSAClient"} {
		_, info, err := decodeTLS12KeyExchange(tlsKXTestHandshake(false, []byte{0, 1, 4}), profile)
		require.NoError(t, err)
		require.Equal(t, "TLS 1.2 "+profile+" layout", info["Profile"])
	}
	_, _, err := decodeTLS12KeyExchange(tlsKXTestHandshake(false, []byte{0, 1, 4}), "guess")
	require.Error(t, err)
}

func TestTLS12KeyExchangeVectorBounds(t *testing.T) {
	v16 := func(n int) []byte { return append([]byte{byte(n >> 8), byte(n)}, bytes.Repeat([]byte{0xa5}, n)...) }
	for _, profile := range []string{"DHEClient", "RSAClient"} {
		_, _, err := decodeTLS12KeyExchange(tlsKXTestHandshake(false, v16(65535)), profile)
		require.NoError(t, err)
	}
	_, _, err := decodeTLS12KeyExchange(tlsKXTestHandshake(false, append([]byte{255}, bytes.Repeat([]byte{4}, 255)...)), "ECDHEClient")
	require.NoError(t, err)
	for _, profile := range []string{"ECDHEClient", "DHEClient"} {
		for _, body := range [][]byte{{0}, {0, 0}, {0, 2, 1}, {2, 1}} {
			_, _, err = decodeTLS12KeyExchange(tlsKXTestHandshake(false, body), profile)
			require.Error(t, err)
		}
	}
	// All four uint16 vectors at their maxima: finite field count, exact bound.
	var body []byte
	for i := 0; i < 3; i++ {
		body = append(body, v16(65535)...)
	}
	body = append(body, 255, 255)
	body = append(body, v16(65535)...)
	wire := tlsKXTestHandshake(true, body)
	require.Len(t, wire, tls12KeyExchangeMaxBytes)
	_, _, err = decodeTLS12KeyExchange(wire, "DHEServer")
	require.NoError(t, err)
	_, _, err = decodeTLS12KeyExchange(tlsKXTestHandshake(true, append(body, 0)), "DHEServer")
	require.Error(t, err)
	for _, at := range []int{4, 4 + 65537, 4 + 2*65537} {
		bad := append([]byte(nil), wire...)
		binary.BigEndian.PutUint16(bad[at:at+2], 0)
		_, _, err = decodeTLS12KeyExchange(bad, "DHEServer")
		require.Error(t, err)
	}
	// Unknown numeric group/signature IDs are preserved, not declared usable.
	for _, group := range []uint16{0, 23, 24, 29, 30, 65535} {
		for _, alg := range []byte{0, 1, 3, 4, 8, 255} {
			body := []byte{3, byte(group >> 8), byte(group), 1, 4, 255, alg, 0, 0}
			_, _, err = decodeTLS12KeyExchange(tlsKXTestHandshake(true, body), "ECDHEServer")
			require.NoError(t, err)
			for _, kind := range []byte{0, 1, 2, 4, 255} {
				bad := append([]byte(nil), body...)
				bad[0] = kind
				_, _, err = decodeTLS12KeyExchange(tlsKXTestHandshake(true, bad), "ECDHEServer")
				require.Error(t, err)
			}
		}
	}
}
