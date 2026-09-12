package stream_parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEtcdVersionIndependentLayoutsAndSpans(t *testing.T) {
	response := func(body string) []byte {
		return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body))
	}
	for _, body := range []string{
		`{"etcdserver":"3.5.21","etcdcluster":"3.5.0"}`,
		`{"etcdserver":"3.6.0","etcdcluster":"3.6.0","storage":"3.5.2"}`,
		`{"storage":"unknown","etcdcluster":"not_decided","etcdserver":"3.6.0-rc.0"}`,
		" \n{ \"etcdcluster\" : \"not_decided\" , \"etcdserver\" : \"3.5.21\" }\t",
		`{"etcd\u0073erver":"3.5.21","etcdcluster":"3\u002e5.0"}`,
	} {
		wire := response(body)
		fields, info, err := decodeEtcdVersionRecord(wire)
		require.NoError(t, err)
		require.Equal(t, "Version Response", info["Message"])
		var walk func([]etcdField, int) int
		walk = func(fields []etcdField, pos int) int {
			for _, f := range fields {
				require.Equal(t, pos, f.Start, f.Name)
				require.GreaterOrEqual(t, f.End, f.Start)
				require.LessOrEqual(t, f.End, len(wire))
				if f.Type == "" {
					require.Equal(t, f.End, walk(f.Children, f.Start))
				}
				pos = f.End
			}
			return pos
		}
		require.Equal(t, len(wire), walk(fields, 0))
		for cut := 0; cut < len(wire); cut++ {
			_, _, err := decodeEtcdVersionRecord(wire[:cut])
			require.Error(t, err, "prefix %d", cut)
		}
	}
	for _, body := range []string{
		`{}`, `[]`, `{"etcdserver":"3.6.0"}`, `{"etcdserver":"3.6.0","etcdcluster":0}`,
		`{"etcdserver":"3.6.0","etcdcluster":null}`, `{"etcdserver":"3.6.0","etcdcluster":{}}`,
		`{"etcdserver":"3.6.0","etcdcluster":"3.6.0","storage":true}`,
		`{"etcdserver":"3.6.0","etcdcluster":"3.6.0","unknown":"x"}`,
		`{"etcdserver":"3.6.0","etcdserver":"3.5.0","etcdcluster":"3.6.0"}`,
		`{"etcdserver":"3.6.0","etcd\u0073erver":"3.5.0","etcdcluster":"3.6.0"}`,
		`{"etcdserver":"3.6.0","etcdcluster":"3.6.0",}`,
		`{"etcdserver":"3.6.0","etcdcluster":"3.6.0"} null`,
		`{"etcdserver":"3.6.0","etcdcluster":"\ud800"}`,
		`{"etcdserver":"","etcdcluster":"3.6.0"}`,
		`{"etcdserver":"` + strings.Repeat("x", 257) + `","etcdcluster":"3.6.0"}`,
	} {
		_, _, err := decodeEtcdVersionRecord(response(body))
		require.Error(t, err, body)
	}
	for _, size := range []int{4096, 4097} {
		body := `{"etcdserver":"3.6.0","etcdcluster":"3.6.0"}`
		body += strings.Repeat(" ", size-len(body))
		_, _, err := decodeEtcdVersionRecord(response(body))
		if size == 4096 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}
