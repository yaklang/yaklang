package stream_parser

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBrowserMailslotValueBoundaries(t *testing.T) {
	for _, n := range []int{0, 1, 13, 14, 81, 82, 151, 152, browserMailslotMaxBytes, browserMailslotMaxBytes + 1} {
		for _, strict := range []bool{false, true} {
			fields, info, err := decodeBrowserMailslotDatagram(make([]byte, n), strict)
			require.Error(t, err, "length %d", n)
			require.Nil(t, fields)
			require.Nil(t, info)
		}
	}
	for _, limit := range []int{16, 43} {
		wire := append(bytes.Repeat([]byte{'A'}, limit-1), 0)
		f, err := browserMailslotString("bounded", wire, 0, len(wire), limit)
		require.NoError(t, err)
		require.Equal(t, string(wire[:limit-1]), *f.Value)
		require.Equal(t, 0, f.Info["Ignored Bytes After Terminator"])
		_, err = browserMailslotString("bounded", append(wire, 0), 0, len(wire)+1, limit)
		require.ErrorContains(t, err, "length outside profile")
		_, err = browserMailslotString("bounded", wire[:limit-1], 0, limit-1, limit)
		require.ErrorContains(t, err, "lacks NUL")
		wire[0] = 0xff
		_, err = browserMailslotString("bounded", wire, 0, len(wire), limit)
		require.ErrorContains(t, err, "ASCII")
		wire[0] = 0
		f, err = browserMailslotString("bounded", wire, 0, len(wire), limit)
		require.NoError(t, err) // Fixed-width fields ignore bytes following NUL.
		require.Equal(t, "", *f.Value)
		require.Equal(t, limit-1, f.Info["Ignored Bytes After Terminator"])
	}
}
