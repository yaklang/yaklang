package pcaputil

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzHTTP3LateMethodBodyRevalidation(f *testing.F) {
	f.Add([]byte("x"), uint8(0), uint8(0))    // HEAD with forbidden content
	f.Add([]byte{}, uint8(0), uint8(4))       // legal empty HEAD, nonzero representation length
	f.Add([]byte{}, uint8(1), uint8(4))       // POST response too short at FIN
	f.Add([]byte("data"), uint8(2), uint8(8)) // CONNECT tunnel without Content-Length
	f.Add([]byte("data"), uint8(2), uint8(2)) // early FIN and forbidden CONNECT length
	f.Add([]byte("lost"), uint8(0), uint8(1)) // body gap never becomes complete coverage
	f.Fuzz(func(t *testing.T, value []byte, method, flags uint8) {
		value = value[:min(len(value), 64)]
		method %= 3
		declared := len(value)
		if flags&4 != 0 {
			declared++
		}
		fields := [][2]string{{":status", "200"}}
		if flags&8 == 0 {
			fields = append(fields, [2]string{"content-length", strconv.Itoa(declared)})
		}
		header := h3LiteralFields(fields...)
		data := h3Data(value)
		wire := append(append([]byte(nil), header...), data...)
		q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
		if flags&2 != 0 {
			_, _, err := h3BodyTestFeed(q, 1, 0, uint64(len(wire)), nil, true)
			require.NoError(t, err)
		}
		var response map[string]any
		var err error
		if flags&1 != 0 {
			_, response, err = h3BodyTestFeed(q, 1, 0, 0, header, false)
			require.NoError(t, err)
			_, _, err = h3BodyTestFeed(q, 1, 0, uint64(len(wire)), nil, true)
			require.NoError(t, err)
		} else {
			_, response, err = h3BodyTestFeed(q, 1, 0, 0, wire, flags&2 == 0)
			if err != nil {
				require.True(t, isH3FieldStreamError(err))
			}
		}
		frozen, err := json.Marshal(response)
		require.NoError(t, err)
		if flags&128 != 0 {
			q.retired = true
		}
		if flags&64 != 0 {
			q.streams[0].resetDirection[0] = true
		}
		name := []string{"HEAD", "POST", "CONNECT"}[method]
		request := h3BodyRequest(name, "-")
		if method == 2 {
			request = h3LiteralFields([2]string{":method", "CONNECT"}, [2]string{":authority", "example.com:443"})
		}
		_, info, err := h3BodyTestFeed(q, 0, 0, 0, request, true)
		require.NoError(t, err)
		if flags&192 != 0 {
			require.NotContains(t, info, "HTTP3 Late Associations")
		} else {
			m := h3LateBodyResult(t, info)
			seen := len(value)
			if flags&1 != 0 {
				seen = 0
			}
			bad := method == 0 && seen > 0 || method == 1 && flags&1 == 0 && flags&8 == 0 && seen != declared || method == 2 && flags&8 == 0
			if bad {
				require.Equal(t, "violation-observed", m["Observation Verdict"])
			} else {
				require.Equal(t, "no-violation-observed", m["Observation Verdict"])
			}
			require.Equal(t, uint64(seen), m["Observed DATA Bytes"])
			require.Equal(t, flags&1 == 0, m["Body Coverage Complete"])
		}
		after, err := json.Marshal(response)
		require.NoError(t, err)
		require.Equal(t, frozen, after)
		require.LessOrEqual(t, q.initialMemory(), int64(q.byteLimit))
	})
}
