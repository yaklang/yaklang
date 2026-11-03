package pcaputil

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Exercise header fragmentation, loss, invalid values and late lifecycle
// changes through the native STREAM consumer. Protected packets and capture
// domains have separate integration tests; this is not a whole QUIC fuzzer.
func FuzzHTTP3LateHeaderAssociation(f *testing.F) {
	f.Add([]byte("value"), uint8(0), uint8(0))
	f.Add([]byte("fragment"), uint8(3), uint8(1))
	f.Add([]byte{0}, uint8(2), uint8(2))
	f.Add([]byte("lost"), uint8(7), uint8(3))
	f.Add([]byte("retired"), uint8(5), uint8(4))
	f.Add([]byte("reset"), uint8(9), uint8(5))
	f.Fuzz(func(t *testing.T, value []byte, split, mode uint8) {
		value = value[:min(len(value), 64)]
		mode %= 6
		request := h3LiteralFields([2]string{":method", "GET"}, [2]string{":scheme", "https"}, [2]string{":path", "/"}, [2]string{":authority", "example.com"}, [2]string{"x-audit", string(value)})
		at := int(split) % len(request)
		q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
		if mode == 1 {
			_, err := qpackRecoveryStream(t, q, 10, 0, 0, 0, request[:at])
			require.NoError(t, err)
		}
		response, err := qpackRecoveryStream(t, q, 11, 1, 0, 0, h3ResponseHeaders())
		require.NoError(t, err)
		frozen, err := json.Marshal(response)
		require.NoError(t, err)
		if mode != 1 {
			_, err = qpackRecoveryStream(t, q, 10, 0, 0, 0, request[:at])
			require.NoError(t, err)
		}
		anchor := q.streams[0].requestPDU
		if anchor == 0 {
			anchor = 12
		}
		switch mode {
		case 4:
			q.retired = true
		case 5:
			q.streams[0].resetDirection[0] = true
		}
		tail, offset := request[at:], uint64(at)
		if mode == 3 {
			tail, offset = nil, uint64(len(request)+1) // a missing suffix is never a valid request
		}
		info, requestError := qpackRecoveryStream(t, q, 12, 0, 0, offset, tail)
		links, _ := info["HTTP3 Late Associations"].([]map[string]any)
		if mode >= 3 || requestError != nil {
			require.Empty(t, links)
		} else {
			require.Len(t, links, 1)
			require.Equal(t, uint64(11), links[0]["Response PDU ID"])
			require.Equal(t, anchor, links[0]["Request PDU ID"])
			require.Equal(t, uint64(12), links[0]["Request Headers PDU ID"])
			require.Equal(t, uint64(0), links[0]["Stream ID"])
			require.Equal(t, []uint64{11}, links[0]["Response Header Origin PDUs"])
			duplicate, _ := qpackRecoveryStream(t, q, 13, 0, 0, 0, request)
			require.NotContains(t, duplicate, "HTTP3 Late Associations")
		}
		after, err := json.Marshal(response)
		require.NoError(t, err)
		require.Equal(t, frozen, after, "later context must not rewrite a published response")
		require.LessOrEqual(t, q.initialMemory(), int64(q.byteLimit))
	})
}
