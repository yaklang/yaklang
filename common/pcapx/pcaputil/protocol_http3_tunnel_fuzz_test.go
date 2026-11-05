package pcaputil

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzHTTP3CONNECTTunnelContextAndFragmentation(f *testing.F) {
	f.Add([]byte("x"), uint8(0), uint8(0), uint8(1))
	f.Add([]byte("x"), uint8(1), uint8(0), uint8(2))
	f.Add([]byte("x"), uint8(2), uint8(0), uint8(3))
	f.Add([]byte("ordinary"), uint8(1), uint8(1), uint8(0))
	f.Add([]byte("non2xx"), uint8(2), uint8(2), uint8(4))
	f.Add([]byte("extension"), uint8(0), uint8(12), uint8(1))
	f.Fuzz(func(t *testing.T, data []byte, mode, flags, split uint8) {
		data = data[:min(len(data), 32)]
		mode %= 3
		connect, success, known := flags&1 == 0, flags&2 == 0, flags&4 == 0
		request := h3TunnelRequest()
		if !connect {
			request = h3BodyRequest("GET", "-")
		}
		status := "200"
		if !success {
			status = "407"
		}
		response := h3LiteralFields([2]string{":status", status})
		frame := h3LiteralFields([2]string{"x-trailer", "y"})
		if !known {
			frame = h3Frame(0x21, data)
		} else if flags&8 != 0 {
			frame = append([]byte{0x40, 1}, frame[1:]...)
		}
		q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
		if mode != 1 {
			_, _, err := h3BodyTestFeed(q, 0, 0, 0, request, false)
			require.NoError(t, err)
		}
		if mode != 2 {
			_, _, err := h3BodyTestFeed(q, 1, 0, 0, response, false)
			require.NoError(t, err)
		}
		dir, off := 1, len(response)
		if mode == 2 {
			dir, off = 0, len(request)
		}
		cut := int(split) % (len(frame) + 1)
		_, first, err := h3BodyTestFeed(q, dir, 0, uint64(off), frame[:cut], false)
		if err != nil {
			require.True(t, mode == 0 && connect && success && known)
			require.True(t, isH3ConnectionObservationError(err))
		}
		frozen, err := json.Marshal(first)
		require.NoError(t, err)
		_, _, err = h3BodyTestFeed(q, dir, 0, uint64(off+cut), frame[cut:], false)
		if err != nil {
			require.True(t, mode == 0 && connect && success && known)
			require.True(t, isH3ConnectionObservationError(err))
		}
		if mode == 1 {
			_, _, err = h3BodyTestFeed(q, 0, 0, 0, request, false)
			require.NoError(t, err)
		}
		if mode == 2 {
			_, _, err = h3BodyTestFeed(q, 1, 0, 0, response, false)
			require.NoError(t, err)
		}
		if connect && success && known && mode != 2 {
			require.NotNil(t, q.h3Fault)
			require.Equal(t, uint64(0x105), q.h3Fault.snapshot()["Error Code"])
		} else {
			require.Nil(t, q.h3Fault)
		}
		after, err := json.Marshal(first)
		require.NoError(t, err)
		require.Equal(t, frozen, after)
		require.LessOrEqual(t, q.initialMemory(), int64(q.byteLimit))
	})
}
